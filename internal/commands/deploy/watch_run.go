package deploy

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// watchRun is one deploy watch after baseline. The probe loop, the log
// follower and the heartbeat run side by side and share this state under
// mu; network requests are made without it. So a slow or hanging request
// in one never holds back another's output: on 08-10 a single poll waited
// on every request in turn, and app-alive, landed and the heartbeat came
// out 1m48s late.
type watchRun struct {
	mu      sync.Mutex
	write   func(any) // the stream emitter; call with mu held
	errOut  io.Writer
	jsonOut bool
	started time.Time

	baseline watchObservation
	machine  *watchMachine
	feed     *watchLogFeed // nil without --logs and --uda-dir
	logs     *logMonitor   // nil without --logs
	schema   *schemaTracker
	// landed tells the schema recheck the new process has been seen.
	landed   bool
	lastPoll time.Time
	// ended is set with the terminal phase; no heartbeat follows it.
	ended bool
}

// emit writes values to the stream as one uninterrupted sequence.
func (r *watchRun) emit(values ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, value := range values {
		r.write(value)
	}
}

// watchHeartbeatEvent is the periodic still-alive line in the --json
// stream, type "heartbeat". The phase the watch is in is currentPhase, not
// phase: a line with a "phase" key is a transition.
type watchHeartbeatEvent struct {
	Timestamp    string         `json:"timestamp"`
	Type         string         `json:"type"`
	CurrentPhase string         `json:"currentPhase"`
	Elapsed      string         `json:"elapsed"`
	LastPoll     string         `json:"lastPoll"`
	Logs         map[string]any `json:"logs,omitempty"`
}

// heartbeats writes a still-alive line every interval until ctx ends, on
// its own clock: no request the watch is waiting on can delay it.
func (r *watchRun) heartbeats(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.heartbeat(now)
		}
	}
}

// heartbeat writes the text line to stderr, as always, and with --json the
// same facts as an NDJSON line on stdout, where a reader relaying the
// stream sees them. lastPoll is the start of the newest completed poll, so
// a reader can tell a stuck poll from a quiet deploy.
func (r *watchRun) heartbeat(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return
	}
	elapsed := now.Sub(r.started).Round(time.Second)
	event := watchHeartbeatEvent{
		Timestamp:    now.UTC().Format(time.RFC3339),
		Type:         "heartbeat",
		CurrentPhase: r.machine.phase,
		Elapsed:      elapsed.String(),
		LastPoll:     r.lastPoll.UTC().Format(time.RFC3339),
	}
	logStatus := ""
	if r.logs != nil {
		logStatus = fmt.Sprintf(", logs: %d read, %d emitted", r.feed.read, r.logs.total())
		event.Logs = map[string]any{"read": r.feed.read, "emitted": r.logs.total()}
	}
	fmt.Fprintf(r.errOut, "%s still watching — phase %s, elapsed %s%s\n", event.Timestamp, event.CurrentPhase, elapsed, logStatus)
	if r.jsonOut {
		r.write(event)
	}
}

// followLogs reads the log feed every interval until ctx ends. Its reads
// run next to the probe loop, never in it: a burst paged through 20 times,
// or a log viewer that does not answer, delays only the log lines.
func (r *watchRun) followLogs(ctx context.Context, every time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		r.readLogs(ctx)
	}
}

// readLogs reads the feed once and writes what it found: monitor events and
// log lines with --logs, schema-pass lines with --uda-dir.
func (r *watchRun) readLogs(ctx context.Context) {
	stamp := time.Now().UTC().Format(time.RFC3339)
	entries, err := r.feed.fetch(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, status := r.feed.record(ctx, stamp, entries, err)
	if r.logs != nil {
		for _, event := range status {
			r.write(event)
		}
		for _, event := range r.logs.consume(entries) {
			r.write(event)
		}
	}
	if r.schema != nil {
		for _, event := range r.schema.observeLogs(entries) {
			r.write(event)
		}
	}
}

// recheckSchema re-compares the tracked artifacts once the deploy has
// landed and writes the status changes. It runs alongside a poll's probes,
// so its lines precede that poll's phase lines.
func (r *watchRun) recheckSchema(ctx context.Context) {
	r.mu.Lock()
	pending, endedBefore := r.schema.recheckPlan(r.landed)
	r.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	results := r.schema.recheckCompare(ctx, pending)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.schema.recheckApply(results, endedBefore) {
		r.write(event)
	}
}

// poll observes the environment once and writes the phase transitions as
// soon as the observation is complete.
func (r *watchRun) poll(ctx context.Context, probes *watchProbes) watchOutcome {
	var alongside func()
	if r.schema != nil {
		alongside = func() { r.recheckSchema(ctx) }
	}
	observation := probes.observe(ctx, alongside)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastPoll = observation.At
	if r.schema != nil {
		observation.SchemaPending = r.schema.pending()
	}
	events, terminal := r.machine.observe(observation)
	for _, event := range events {
		r.write(event)
	}
	if r.machine.sawLanded || (observation.ProcessID != "" &&
		(observation.ProcessID != r.baseline.ProcessID || observation.MachineName != r.baseline.MachineName)) {
		r.landed = true
	}
	if r.schema != nil && terminal == watchOutcomeNone && r.machine.sawLanded && r.machine.sawServing {
		for _, event := range r.schema.waiting(observation.At.UTC().Format(time.RFC3339)) {
			r.write(event)
		}
	}
	if terminal != watchOutcomeNone {
		r.ended = true
	}
	return terminal
}

// timedOut writes the timeout phase and returns the reason.
func (r *watchRun) timedOut(timeout time.Duration) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	reason := fmt.Sprintf("no verification within %s (last phase: %s) — deployment status unknown", timeout, r.machine.phase)
	if r.schema != nil && r.schema.pending() {
		reason += fmt.Sprintf("; schema: %d tracked artifacts unconfirmed, Umbraco Deploy's schema pass end not observed", len(r.schema.pendingFiles()))
	}
	r.ended = true
	r.write(watchEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Phase: "timeout", Detail: map[string]any{"reason": reason}})
	return reason
}

// finish closes the stream after the terminal phase, once the heartbeat
// and the log follower have stopped: the feed is read one last time within
// drainBudget, then the schema summary and the log monitor's stopped line.
func (r *watchRun) finish(ctx context.Context, drainBudget time.Duration) {
	if r.feed != nil {
		drainCtx, cancel := context.WithTimeout(ctx, drainBudget)
		r.readLogs(drainCtx)
		cancel()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.schema != nil {
		r.write(r.schema.summary(r.machine.phase))
	}
	if r.logs != nil {
		r.write(r.logs.stopped(r.machine.phase, r.feed))
	}
}
