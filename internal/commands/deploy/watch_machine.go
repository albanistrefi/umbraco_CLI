package deploy

import (
	"fmt"
	"sort"
	"time"
)

// watchObservation is one poll of the target environment. Unknown values
// (unreadable during a restart window) are represented explicitly rather
// than defaulted, so the machine never mistakes "could not read" for a
// state.
type watchObservation struct {
	At          time.Time
	MgmtAlive   bool
	MgmtStatus  int // HTTP status of the unauthenticated token probe; 0 = unreachable
	ProcessID   string
	MachineName string
	NewestLogAt time.Time // timestamp of the newest log entry, by the server's clock
	// SchemaPending holds verified back while --uda-dir artifacts await
	// confirmation; false when no schema is tracked.
	SchemaPending bool
	LogErr        error           // the typed error from the log probe, surfaced at baseline
	Health        map[string]bool // per health path; nil when the probe errored entirely
	BadIndexes    []string        // rebuilding/unhealthy index names; nil = unknown this tick
}

type watchOutcome int

const (
	watchOutcomeNone watchOutcome = iota
	watchOutcomeVerified
	watchOutcomeFailed
)

// watchMachine turns observations into phase-transition events. It is fed
// one observation per poll and emits each phase at most once; fast recycles
// may legitimately skip phases (e.g. landed without an observed restart).
type watchMachine struct {
	baselineProcessID   string
	baselineMachineName string
	baselineHealthy     []string
	baselineBadIndexes  map[string]struct{}
	escalation          time.Duration
	settle              time.Duration
	skipIndexVerify     bool

	phase          string
	prevAlive      bool
	downSince      time.Time
	healthBadSince time.Time
	sawRestarting  bool
	sawAppAlive    bool
	sawServing     bool
	sawLanded      bool
	settleStart    time.Time
	settleAttempt  int
	failureReason  string
}

func newWatchMachine(baseline watchObservation, escalation time.Duration, settle time.Duration, skipIndexVerify bool) (*watchMachine, error) {
	if !baseline.MgmtAlive {
		return nil, fmt.Errorf("cannot baseline the target: the management endpoint is not answering (status %d) — refusing to arm, a watch started mid-outage cannot tell a deploy from the outage", baseline.MgmtStatus)
	}
	if baseline.ProcessID == "" {
		// Surface the real probe error (wrapped, so auth failures keep exit
		// code 3 and API errors keep 4) instead of a generic local failure.
		if baseline.LogErr != nil {
			return nil, fmt.Errorf("cannot baseline the target: reading the newest log entry failed: %w", baseline.LogErr)
		}
		return nil, fmt.Errorf("cannot baseline the target: no ProcessId readable from the newest log entry — the landing signal would never fire")
	}
	healthy := make([]string, 0, len(baseline.Health))
	for path, ok := range baseline.Health {
		if ok {
			healthy = append(healthy, path)
		}
	}
	sort.Strings(healthy)
	bad := map[string]struct{}{}
	for _, name := range baseline.BadIndexes {
		bad[name] = struct{}{}
	}
	return &watchMachine{
		baselineProcessID:   baseline.ProcessID,
		baselineMachineName: baseline.MachineName,
		baselineHealthy:     healthy,
		baselineBadIndexes:  bad,
		escalation:          escalation,
		settle:              settle,
		skipIndexVerify:     skipIndexVerify,
		phase:               "baseline",
		prevAlive:           true,
	}, nil
}

func (m *watchMachine) observe(obs watchObservation) ([]watchEvent, watchOutcome) {
	var events []watchEvent
	stamp := obs.At.UTC().Format(time.RFC3339)
	transition := func(phase string, detail map[string]any) {
		m.phase = phase
		events = append(events, watchEvent{Timestamp: stamp, Phase: phase, Detail: detail})
	}

	if !obs.MgmtAlive {
		if m.downSince.IsZero() {
			m.downSince = obs.At
		}
		if !m.sawRestarting {
			m.sawRestarting = true
			transition("restarting", map[string]any{"managementStatus": obs.MgmtStatus})
		}
		// Downtime breaks an in-progress settle: without this, an outage
		// longer than the settle window (but under escalation) would count
		// as healthy time and verify on the first recovery tick.
		if !m.settleStart.IsZero() {
			m.settleStart = time.Time{}
			transition("settle-interrupted", map[string]any{"reason": "management endpoint down"})
		}
		if obs.At.Sub(m.downSince) >= m.escalation {
			m.failureReason = fmt.Sprintf("management endpoint down for %s (threshold %s)", obs.At.Sub(m.downSince).Round(time.Second), m.escalation)
			transition("failed", map[string]any{"reason": m.failureReason})
			return events, watchOutcomeFailed
		}
		m.prevAlive = false
		return events, watchOutcomeNone
	}

	// Management endpoint is answering.
	if !m.prevAlive && m.sawRestarting && !m.sawAppAlive {
		m.sawAppAlive = true
		downFor := obs.At.Sub(m.downSince).Round(time.Second)
		transition("app-alive", map[string]any{"managementStatus": obs.MgmtStatus, "downFor": downFor.String()})
	}
	m.downSince = time.Time{}
	m.prevAlive = true

	if !m.sawLanded && obs.ProcessID != "" &&
		(obs.ProcessID != m.baselineProcessID || obs.MachineName != m.baselineMachineName) {
		m.sawLanded = true
		transition("landed", map[string]any{
			"processId":   fmt.Sprintf("%s → %s", m.baselineProcessID, obs.ProcessID),
			"machineName": fmt.Sprintf("%s → %s", m.baselineMachineName, obs.MachineName),
		})
	}

	healthKnown := obs.Health != nil
	healthOK := healthKnown && m.baselineHealthyOK(obs.Health)
	if (m.sawRestarting || m.sawLanded) && !m.sawServing && healthOK {
		m.sawServing = true
		transition("serving", map[string]any{"paths": m.baselineHealthy})
	}

	// Post-landing health escalation: the deploy landed but the site never
	// came back. Only paths healthy at baseline count — a path already
	// failing before the deploy is not a deploy failure.
	if m.sawLanded && healthKnown && !healthOK {
		if m.healthBadSince.IsZero() {
			m.healthBadSince = obs.At
		}
		if obs.At.Sub(m.healthBadSince) >= m.escalation {
			m.failureReason = fmt.Sprintf("health paths failing for %s after the deploy landed (threshold %s)", obs.At.Sub(m.healthBadSince).Round(time.Second), m.escalation)
			transition("failed", map[string]any{"reason": m.failureReason, "failingPaths": unhealthyPathNames(obs.Health)})
			return events, watchOutcomeFailed
		}
	} else if healthOK {
		m.healthBadSince = time.Time{}
	}

	// Verification: everything must look good, and stay good for a full
	// settle window. Deployment pipelines can disturb the environment after
	// the app is already serving (index rebuilds discard replicated-clean
	// indexes at deployment completion), so a single passing sample is not
	// verification — it can land exactly in the healthy gap.
	allClear := m.sawLanded && m.sawServing && healthOK && m.indexesClean(obs) && !obs.SchemaPending
	if allClear {
		if m.settle <= 0 {
			transition("verified", map[string]any{"paths": m.baselineHealthy, "indexVerify": !m.skipIndexVerify, "settledFor": "disabled"})
			return events, watchOutcomeVerified
		}
		if m.settleStart.IsZero() {
			m.settleStart = obs.At
			m.settleAttempt++
			transition("settling", map[string]any{"for": m.settle.String(), "attempt": m.settleAttempt})
		} else if obs.At.Sub(m.settleStart) >= m.settle {
			transition("verified", map[string]any{"paths": m.baselineHealthy, "indexVerify": !m.skipIndexVerify, "settledFor": obs.At.Sub(m.settleStart).Round(time.Second).String()})
			return events, watchOutcomeVerified
		}
	} else if !m.settleStart.IsZero() {
		// The settle window broke: make the disturbance visible and restart
		// the window once the environment recovers.
		detail := map[string]any{}
		if !m.indexesClean(obs) {
			if obs.BadIndexes == nil {
				detail["reason"] = "index state unreadable"
			} else {
				detail["reason"] = "index rebuild observed"
				detail["indexes"] = obs.BadIndexes
			}
		} else if healthKnown && !healthOK {
			detail["reason"] = "health paths failing"
			detail["paths"] = unhealthyPathNames(obs.Health)
		} else {
			detail["reason"] = "health state unreadable"
		}
		m.settleStart = time.Time{}
		transition("settle-interrupted", detail)
	}

	return events, watchOutcomeNone
}

// baselineHealthyOK reports whether every path that was healthy at baseline
// is healthy again. Paths already failing at baseline are excluded — a
// signal already true on the target is not a signal.
func (m *watchMachine) baselineHealthyOK(health map[string]bool) bool {
	if len(m.baselineHealthy) == 0 {
		return false
	}
	for _, path := range m.baselineHealthy {
		if !health[path] {
			return false
		}
	}
	return true
}

// indexesClean reports whether no index is rebuilding or unhealthy beyond
// the set that was already bad at baseline. A nil BadIndexes means the
// state could not be read this tick, which is never treated as clean.
func (m *watchMachine) indexesClean(obs watchObservation) bool {
	if m.skipIndexVerify {
		return true
	}
	if obs.BadIndexes == nil {
		return false
	}
	for _, name := range obs.BadIndexes {
		if _, preexisting := m.baselineBadIndexes[name]; !preexisting {
			return false
		}
	}
	return true
}

func unhealthyPathNames(health map[string]bool) []string {
	names := make([]string, 0)
	for path, ok := range health {
		if !ok {
			names = append(names, path)
		}
	}
	sort.Strings(names)
	return names
}
