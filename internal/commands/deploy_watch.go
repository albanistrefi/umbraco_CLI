package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/version"
)

// deployWatchFailedError maps the watch's failed terminal phase to exit
// code 5 so CI can gate on it.
type deployWatchFailedError struct{ reason string }

func (e deployWatchFailedError) Error() string { return "deploy watch failed: " + e.reason }
func (deployWatchFailedError) ExitCode() int   { return 5 }

// deployWatchTimeoutError maps the watch's timeout terminal phase to exit
// code 6. Timeout is an honest "status unknown", never an inferred success
// or failure.
type deployWatchTimeoutError struct{ reason string }

func (e deployWatchTimeoutError) Error() string { return "deploy watch timeout: " + e.reason }
func (deployWatchTimeoutError) ExitCode() int   { return 6 }

func deployWatch(deps Dependencies) *cobra.Command {
	var healthPaths []string
	var publicURL string
	var interval time.Duration
	var timeout time.Duration
	var escalation time.Duration
	var settle time.Duration
	var heartbeat time.Duration
	var jsonOut bool
	var skipIndexVerify bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch an environment for the effects of a deployment and report phase transitions",
		Long: `Observes the target environment for state deltas only a deployment can cause — no pipeline or portal API involved, so it works identically on Umbraco Cloud and on-prem, and it is strictly read-only.

Signals: the newest log entry's ProcessId/MachineName (an app recycle means the deploy landed), the management token endpoint probed unauthenticated (503/unreachable = down; 401 = app alive and rejecting the probe — the earliest all-clear, typically ~15s before public pages return), configured health paths on the public host, and Examine index health (deploys can trigger full index rebuilds, during which search is empty — "deploy succeeded" and "the site works" are different questions).

Phases: baseline → restarting → app-alive → serving → landed → settling → verified | failed | timeout. Everything is baselined before arming — a signal already true on the target is not a signal. Verified requires the environment to stay healthy for a full --settle window after everything first looks good: deployment pipelines can disturb the environment AFTER the app is already serving (observed in production: Umbraco Deploy wiped every Examine index 27 seconds after a single-sample check had passed, leaving search empty for 17 minutes), so a single passing sample is not verification. An interrupted settle (index rebuild, health flap) is emitted as settle-interrupted and the window restarts once the environment recovers. Transitions are emitted with timestamps as they are observed (fast recycles may skip phases); silence between transitions means "still in the current phase", and --heartbeat writes a periodic still-alive line to stderr so silence is never ambiguous. Success is never inferred from silence: reaching --timeout without verification exits 6 (status unknown), and sustained downtime or post-landing health failure beyond --escalation exits 5.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return fmt.Errorf("--interval must be greater than zero")
			}
			base := strings.TrimRight(deps.currentConfig().BaseURL, "/")
			public := base
			if strings.TrimSpace(publicURL) != "" {
				public = strings.TrimRight(publicURL, "/")
			}
			if len(healthPaths) == 0 {
				healthPaths = []string{"/"}
			}

			if settle < 0 {
				return fmt.Errorf("--settle cannot be negative")
			}
			probes := &watchProbes{
				deps:        deps,
				httpClient:  watchHTTPClient(deps),
				tokenURL:    base + "/umbraco/management/api/v1/security/back-office/token",
				publicURL:   public,
				healthPaths: healthPaths,
				skipIndexes: skipIndexVerify,
			}

			ctx := cmd.Context()
			baseline := probes.observe(ctx)
			machine, err := newWatchMachine(baseline, escalation, settle, skipIndexVerify)
			if err != nil {
				return err
			}
			emit := watchEmitter(cmd.OutOrStdout(), jsonOut)
			emit(watchEvent{
				Timestamp: baseline.At.UTC().Format(time.RFC3339),
				Phase:     "baseline",
				Detail: map[string]any{
					"processId":      baseline.ProcessID,
					"machineName":    baseline.MachineName,
					"healthyPaths":   machine.baselineHealthy,
					"unhealthyPaths": unhealthyPathNames(baseline.Health),
					"ignoredIndexes": sortedKeys(machine.baselineBadIndexes),
				},
			})

			started := time.Now()
			deadline := started.Add(timeout)
			lastHeartbeat := started
			timeoutErr := func() error {
				reason := fmt.Sprintf("no verification within %s (last phase: %s) — deployment status unknown", timeout, machine.phase)
				emit(watchEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Phase: "timeout", Detail: map[string]any{"reason": reason}})
				return deployWatchTimeoutError{reason: reason}
			}
			for {
				// The sleep never overshoots the deadline, so --timeout is
				// honored even when --interval is longer or probes are slow.
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return timeoutErr()
				}
				sleep := interval
				if remaining < sleep {
					sleep = remaining
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(sleep):
				}

				observation := probes.observe(ctx)
				events, terminal := machine.observe(observation)
				for _, event := range events {
					emit(event)
				}
				switch terminal {
				case watchOutcomeVerified:
					return nil
				case watchOutcomeFailed:
					return deployWatchFailedError{reason: machine.failureReason}
				}

				if time.Now().After(deadline) {
					return timeoutErr()
				}
				if heartbeat > 0 && time.Since(lastHeartbeat) >= heartbeat {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s still watching — phase %s, elapsed %s\n", time.Now().UTC().Format(time.RFC3339), machine.phase, time.Since(started).Round(time.Second))
					lastHeartbeat = time.Now()
				}
			}
		},
	}

	cmd.Flags().StringArrayVar(&healthPaths, "health-path", nil, "Public path that must return 2xx for the serving/verified phases (repeatable; default /)")
	cmd.Flags().StringVar(&publicURL, "public-url", "", "Public host for health paths when it differs from the management base URL")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "Poll interval")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Give up after this long without verification (exit 6, status unknown)")
	cmd.Flags().DurationVar(&escalation, "escalation", 10*time.Minute, "Treat sustained downtime or post-landing health failure longer than this as failed (exit 5)")
	cmd.Flags().DurationVar(&settle, "settle", 90*time.Second, "How long the environment must stay healthy after everything first looks good before verified is emitted; 0 disables (single-sample verification)")
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", time.Minute, "Interval for still-alive lines on stderr; 0 disables")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit phase transitions as NDJSON")
	cmd.Flags().BoolVar(&skipIndexVerify, "skip-index-verify", false, "Do not require Examine indexes to be healthy for the verified phase")
	return cmd
}

// watchEvent is one emitted phase transition.
type watchEvent struct {
	Timestamp string         `json:"timestamp"`
	Phase     string         `json:"phase"`
	Detail    map[string]any `json:"detail,omitempty"`
}

func watchEmitter(out io.Writer, jsonOut bool) func(watchEvent) {
	encoder := json.NewEncoder(out)
	return func(event watchEvent) {
		if jsonOut {
			_ = encoder.Encode(event)
			return
		}
		detail := ""
		if len(event.Detail) > 0 {
			parts := make([]string, 0, len(event.Detail))
			keys := make([]string, 0, len(event.Detail))
			for key := range event.Detail {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				parts = append(parts, fmt.Sprintf("%s=%v", key, event.Detail[key]))
			}
			detail = " — " + strings.Join(parts, " ")
		}
		fmt.Fprintf(out, "%s %s%s\n", event.Timestamp, event.Phase, detail)
	}
}

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
	LogErr      error           // the typed error from the log probe, surfaced at baseline
	Health      map[string]bool // per health path; nil when the probe errored entirely
	BadIndexes  []string        // rebuilding/unhealthy index names; nil = unknown this tick
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
	allClear := m.sawLanded && m.sawServing && healthOK && m.indexesClean(obs)
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

// watchProbes gathers one observation per poll. Probe failures during a
// restart window are expected signals, not command errors.
type watchProbes struct {
	deps        Dependencies
	httpClient  *http.Client
	tokenURL    string
	publicURL   string
	healthPaths []string
	skipIndexes bool
}

func watchHTTPClient(deps Dependencies) *http.Client {
	if deps.HTTPClient != nil {
		return deps.HTTPClient
	}
	return http.DefaultClient
}

func (p *watchProbes) observe(ctx context.Context) watchObservation {
	obs := watchObservation{At: time.Now()}
	obs.MgmtAlive, obs.MgmtStatus = p.probeManagement(ctx)
	obs.Health = p.probeHealth(ctx)
	if obs.MgmtAlive {
		obs.ProcessID, obs.MachineName, obs.LogErr = p.newestProcess(ctx)
		if !p.skipIndexes {
			obs.BadIndexes = p.badIndexes(ctx)
		}
	}
	return obs
}

// probeManagement POSTs an empty unauthenticated request to the token
// endpoint. 5xx or unreachable means the app is down; any 4xx means the
// app is alive and rejecting the probe — the earliest all-clear during a
// restart window.
func (p *watchProbes) probeManagement(ctx context.Context) (bool, int) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, p.tokenURL, strings.NewReader(""))
	if err != nil {
		return false, 0
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", version.UserAgent())
	response, err := p.httpClient.Do(request)
	if err != nil {
		return false, 0
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode < 500, response.StatusCode
}

func (p *watchProbes) probeHealth(ctx context.Context) map[string]bool {
	health := make(map[string]bool, len(p.healthPaths))
	for _, path := range p.healthPaths {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.publicURL+path, nil)
		if err != nil {
			cancel()
			health[path] = false
			continue
		}
		request.Header.Set("User-Agent", version.UserAgent())
		response, err := p.httpClient.Do(request)
		if err != nil {
			cancel()
			health[path] = false
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		cancel()
		health[path] = response.StatusCode >= 200 && response.StatusCode < 300
	}
	return health
}

func (p *watchProbes) newestProcess(ctx context.Context) (string, string, error) {
	result, err := p.deps.Client.Get(ctx, logViewerLogPath, api.RequestOptions{Params: map[string]any{
		"take": 1, "skip": 0, "orderDirection": "Descending",
	}})
	if err != nil {
		return "", "", err
	}
	envelope, ok := result.(map[string]any)
	if !ok {
		return "", "", nil
	}
	items, _ := envelope["items"].([]any)
	if len(items) == 0 {
		return "", "", nil
	}
	entry, ok := items[0].(map[string]any)
	if !ok {
		return "", "", nil
	}
	var processID, machineName string
	if properties, ok := entry["properties"].([]any); ok {
		for _, item := range properties {
			property, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name, _ := property["name"].(string)
			value, _ := property["value"].(string)
			switch name {
			case "ProcessId":
				processID = value
			case "MachineName":
				machineName = value
			}
		}
	}
	return processID, machineName, nil
}

func (p *watchProbes) badIndexes(ctx context.Context) []string {
	result, err := p.deps.Client.Get(ctx, "/indexer", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 100}})
	if err != nil {
		return nil
	}
	envelope, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	items, _ := envelope["items"].([]any)
	bad := make([]string, 0)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status := indexerHealthStatus(entry)
		if status != "" && !strings.EqualFold(status, "Healthy") {
			if name, _ := entry["name"].(string); name != "" {
				bad = append(bad, name)
			}
		}
	}
	sort.Strings(bad)
	return bad
}
