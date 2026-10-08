package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/commands/cmdtest"
)

// The 08-10 live deploy: app-alive and landed were stamped 08:41:24 but
// written at 08:43:12, and no heartbeat came in between, because one poll
// waited on every request in turn (three health paths, the newest-log
// probe re-authenticating, the indexer, the log tail). These tests slow or
// hang one request and assert that phase lines and heartbeats still arrive
// on time.

// timedLine is one output line and when it was written.
type timedLine struct {
	at   time.Time
	text string
}

// timedWriter records the arrival time of every line written to it.
type timedWriter struct {
	mu      sync.Mutex
	pending bytes.Buffer
	lines   []timedLine
}

func (w *timedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	w.pending.Write(p)
	for {
		line, err := w.pending.ReadString('\n')
		if err != nil {
			// Keep the unterminated remainder for the next write.
			w.pending.Reset()
			w.pending.WriteString(line)
			return len(p), nil
		}
		w.lines = append(w.lines, timedLine{at: now, text: strings.TrimSuffix(line, "\n")})
	}
}

func (w *timedWriter) snapshot() []timedLine {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]timedLine(nil), w.lines...)
}

// slowed delays (or, with a delay < 0, hangs until the request is
// cancelled) the requests delay picks, before the fake answers them. The
// delay is served outside the fake's lock, so other requests keep flowing.
func slowed(inner cmdtest.RoundTripper, delay func(req *http.Request) time.Duration) cmdtest.RoundTripper {
	return func(req *http.Request) (*http.Response, error) {
		if wait := delay(req); wait != 0 {
			var timer <-chan time.Time
			if wait > 0 {
				timer = time.After(wait)
			}
			select {
			case <-timer:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return inner(req)
	}
}

func (e *fakeDeployEnv) currentTick() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick - 1
}

type timedRun struct {
	stdout   []timedLine
	stderr   []timedLine
	err      error
	finished bool
}

// runWatchTimed runs deploy watch against handler and gives up after
// watchdog, so a regression that blocks shows up as a failure, not a hang.
func runWatchTimed(t *testing.T, handler cmdtest.RoundTripper, watchdog time.Duration, args ...string) timedRun {
	t.Helper()
	root := cmdtest.BuildRoot(t, cmdtest.Deps(handler), Register)
	stdout, stderr := &timedWriter{}, &timedWriter{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append([]string{"deploy", "watch"}, args...))
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	run := timedRun{}
	select {
	case run.err = <-done:
		run.finished = true
	case <-time.After(watchdog):
	}
	run.stdout, run.stderr = stdout.snapshot(), stderr.snapshot()
	return run
}

type timedEvent struct {
	at    time.Time
	value map[string]any
}

func (r timedRun) events(t *testing.T) []timedEvent {
	t.Helper()
	events := make([]timedEvent, 0, len(r.stdout))
	for _, line := range r.stdout {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line.text), &decoded); err != nil {
			t.Fatalf("not NDJSON: %q (%v)", line.text, err)
		}
		events = append(events, timedEvent{at: line.at, value: decoded})
	}
	return events
}

func (r timedRun) describe(t *testing.T) string {
	var out strings.Builder
	for _, line := range r.stdout {
		fmt.Fprintf(&out, "  stdout %s %s\n", line.at.Format("15:04:05.000"), line.text)
	}
	for _, line := range r.stderr {
		fmt.Fprintf(&out, "  stderr %s %s\n", line.at.Format("15:04:05.000"), line.text)
	}
	return out.String()
}

// phaseLateness is how long after its observation a phase line was
// written. Phase timestamps are RFC3339 (whole seconds), so up to a second
// of the difference is truncation; callers allow for it.
func phaseLateness(t *testing.T, event timedEvent) time.Duration {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339, fmt.Sprint(event.value["timestamp"]))
	if err != nil {
		t.Fatalf("phase timestamp %v: %v", event.value["timestamp"], err)
	}
	return event.at.Sub(stamp)
}

// assertPhasesOnTime checks each named phase was written within budget of
// its observation (plus the second of timestamp truncation).
func assertPhasesOnTime(t *testing.T, run timedRun, budget time.Duration, names ...string) {
	t.Helper()
	seen := map[string]timedEvent{}
	for _, event := range run.events(t) {
		if event.value["type"] == "phase" {
			seen[fmt.Sprint(event.value["phase"])] = event
		}
	}
	for _, name := range names {
		event, ok := seen[name]
		if !ok {
			t.Fatalf("phase %s never written (finished=%v err=%v):\n%s", name, run.finished, run.err, run.describe(t))
		}
		if late := phaseLateness(t, event); late > budget+time.Second {
			t.Fatalf("phase %s written %s after its observation (budget %s):\n%s", name, late, budget, run.describe(t))
		}
	}
}

// assertHeartbeatsOnSchedule checks NDJSON heartbeats kept coming at
// every, from the first one to the end of the window, with no gap over
// maxGap.
func assertHeartbeatsOnSchedule(t *testing.T, run timedRun, from time.Time, until time.Time, maxGap time.Duration) {
	t.Helper()
	times := []time.Time{from}
	for _, event := range run.events(t) {
		if event.value["type"] == "heartbeat" && event.at.After(from) && event.at.Before(until) {
			times = append(times, event.at)
		}
	}
	times = append(times, until)
	if len(times) < 4 {
		t.Fatalf("expected heartbeats between %s and %s, got %d:\n%s", from.Format("15:04:05.000"), until.Format("15:04:05.000"), len(times)-2, run.describe(t))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap > maxGap {
			t.Fatalf("heartbeat gap of %s (max %s) ending %s:\n%s", gap, maxGap, times[i].Format("15:04:05.000"), run.describe(t))
		}
	}
}

func firstPhaseAt(t *testing.T, run timedRun, name string) time.Time {
	t.Helper()
	for _, event := range run.events(t) {
		if event.value["type"] == "phase" && event.value["phase"] == name {
			return event.at
		}
	}
	t.Fatalf("phase %s never written:\n%s", name, run.describe(t))
	return time.Time{}
}

const (
	timingInterval  = "500ms"
	timingHeartbeat = "200ms"
	// The request timeout these tests run with: half of timingInterval.
	timingRequestTimeout = 250 * time.Millisecond
)

func TestWatchHangingLogViewerDoesNotHoldBackPhasesOrHeartbeats(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: 2, entries: deployLogs(2)}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	handler := slowed(env.handler(), func(req *http.Request) time.Duration {
		// After the restart the log viewer accepts tail reads and never
		// answers; the single-entry landing probe keeps working.
		if req.URL.Path == "/umbraco/management/api/v1/log-viewer/log" && req.URL.Query().Get("take") != "1" && env.currentTick() >= 2 {
			select {
			case <-release:
			case <-req.Context().Done():
			}
			return time.Nanosecond
		}
		return 0
	})
	run := runWatchTimed(t, handler, 8*time.Second, "--json", "--logs", "--interval", timingInterval, "--heartbeat", timingHeartbeat,
		"--settle", "1500ms", "--skip-index-verify", "--escalation", "1h", "--timeout", "20s")

	assertPhasesOnTime(t, run, timingRequestTimeout+500*time.Millisecond, "restarting", "app-alive", "landed", "serving", "settling", "verified")
	if !run.finished || run.err != nil {
		t.Fatalf("expected the watch to verify and end (finished=%v err=%v):\n%s", run.finished, run.err, run.describe(t))
	}
	// The settle window ran while every tail read hung: heartbeats had to
	// keep coming through it.
	assertHeartbeatsOnSchedule(t, run, firstPhaseAt(t, run, "settling"), firstPhaseAt(t, run, "verified"), 2*200*time.Millisecond)

	events := run.events(t)
	last := events[len(events)-1].value
	if last["type"] != "log-monitor" || last["status"] != "stopped" {
		t.Fatalf("the monitor must still stop with the watch, last line %v", last)
	}
}

func TestWatchSlowProbeDoesNotHoldBackPhasesOrHeartbeats(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		// 08-10: public pages were slow while the new process warmed up.
		{name: "health path", path: "/"},
		// 08-10: the indexes were opened and replicated on first access.
		{name: "indexer", path: "/umbraco/management/api/v1/indexer"},
		// The newest-log probe that carries the landing signal.
		{name: "newest log entry", path: "/umbraco/management/api/v1/log-viewer/log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: 2}
			env.routes = func(tick int, req *http.Request) *http.Response {
				if req.URL.Path == "/umbraco/management/api/v1/indexer" {
					return cmdtest.JSONResponse(http.StatusOK, `{"total":1,"items":[{"name":"ExternalIndex","healthStatus":{"status":"Healthy"}}]}`)
				}
				return nil
			}
			// From the restart until three seconds later, the request
			// takes three seconds — slower than the poll interval, well
			// under the old 10s/15s/60s timeouts.
			var slowUntil time.Time
			var once sync.Once
			handler := slowed(env.handler(), func(req *http.Request) time.Duration {
				if req.URL.Path != tc.path || env.currentTick() < 2 {
					return 0
				}
				if tc.path == "/umbraco/management/api/v1/log-viewer/log" && req.URL.Query().Get("take") != "1" {
					return 0
				}
				once.Do(func() { slowUntil = time.Now().Add(3 * time.Second) })
				if time.Now().Before(slowUntil) {
					return 3 * time.Second
				}
				return 0
			})
			run := runWatchTimed(t, handler, 12*time.Second, "--json", "--interval", timingInterval, "--heartbeat", timingHeartbeat,
				"--settle", "0", "--escalation", "1h", "--timeout", "20s")

			assertPhasesOnTime(t, run, timingRequestTimeout+500*time.Millisecond, "restarting", "app-alive", "verified")
			if !run.finished || run.err != nil {
				t.Fatalf("expected the watch to verify once the request is fast again (finished=%v err=%v):\n%s", run.finished, run.err, run.describe(t))
			}
			// The window in which the request was slow, from app-alive (or
			// restarting, when the slow request is the landing probe) to
			// verified.
			assertHeartbeatsOnSchedule(t, run, firstPhaseAt(t, run, "restarting"), firstPhaseAt(t, run, "verified"), 2*200*time.Millisecond)
		})
	}
}

func TestWatchHeartbeatIsInTheJSONStream(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: 1 << 30, entries: deployLogs(2)}
	run := runWatchTimed(t, env.handler(), 8*time.Second, "--json", "--logs", "--interval", timingInterval, "--heartbeat", timingHeartbeat,
		"--settle", "0", "--skip-index-verify", "--escalation", "1h", "--timeout", "1200ms")
	if !run.finished || exitCode(run.err) != 6 {
		t.Fatalf("expected the timeout exit, got finished=%v err=%v", run.finished, run.err)
	}
	beats := 0
	for _, event := range run.events(t) {
		if event.value["type"] != "heartbeat" {
			continue
		}
		beats++
		value := event.value
		if _, has := value["phase"]; has {
			t.Fatalf("a heartbeat carries a phase key, so a consumer reading phases could take it for a transition: %v", value)
		}
		if phase := fmt.Sprint(value["currentPhase"]); phase != "baseline" && phase != "restarting" && phase != "app-alive" && phase != "serving" {
			t.Fatalf("unexpected currentPhase: %v", value)
		}
		if _, err := time.Parse(time.RFC3339, fmt.Sprint(value["timestamp"])); err != nil {
			t.Fatalf("heartbeat timestamp: %v", value)
		}
		if _, err := time.ParseDuration(fmt.Sprint(value["elapsed"])); err != nil {
			t.Fatalf("heartbeat elapsed should be a duration: %v", value)
		}
		if _, err := time.Parse(time.RFC3339, fmt.Sprint(value["lastPoll"])); err != nil {
			t.Fatalf("heartbeat lastPoll should say when the environment was last polled: %v", value)
		}
		logs, ok := value["logs"].(map[string]any)
		if !ok || logs["read"] == nil || logs["emitted"] == nil {
			t.Fatalf("with --logs the heartbeat carries the log counts: %v", value)
		}
	}
	if beats < 3 {
		t.Fatalf("expected heartbeats every %s on stdout, got %d:\n%s", timingHeartbeat, beats, run.describe(t))
	}
	// stderr keeps its text line for existing readers.
	textBeats := 0
	for _, line := range run.stderr {
		if strings.Contains(line.text, " still watching — phase ") {
			textBeats++
		}
	}
	if textBeats != beats {
		t.Fatalf("expected one stderr line per NDJSON heartbeat, got %d and %d", textBeats, beats)
	}
}

func TestWatchTextModeKeepsHeartbeatsOffStdout(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: 1 << 30}
	run := runWatchTimed(t, env.handler(), 8*time.Second, "--interval", timingInterval, "--heartbeat", timingHeartbeat,
		"--settle", "0", "--skip-index-verify", "--escalation", "1h", "--timeout", "800ms")
	if !run.finished || exitCode(run.err) != 6 {
		t.Fatalf("expected the timeout exit, got finished=%v err=%v", run.finished, run.err)
	}
	for _, line := range run.stdout {
		if strings.Contains(line.text, "heartbeat") || strings.Contains(line.text, "still watching") {
			t.Fatalf("text mode stdout changed: %q", line.text)
		}
	}
	if len(run.stderr) == 0 || !strings.Contains(run.stderr[0].text, " still watching — phase ") {
		t.Fatalf("text mode heartbeats stay on stderr: %v", run.stderr)
	}
}

func TestWatchRequestTimeoutValidation(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request may be sent on a usage error, got %s", req.URL)
		return nil, nil
	})
	for _, args := range [][]string{
		{"--request-timeout", "5s"},
		{"--request-timeout", "6s"},
		{"--interval", "10s", "--heartbeat", "2s", "--request-timeout", "3s"},
		{"--request-timeout", "-1s"},
	} {
		_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), append([]string{"deploy", "watch"}, args...)...)
		if err == nil || exitCode(err) != 1 || !strings.Contains(err.Error(), "--request-timeout") {
			t.Fatalf("%v: expected a usage error naming --request-timeout, got %v", args, err)
		}
	}
}

func TestWatchRequestTimeoutDefaultsBelowTheIntervals(t *testing.T) {
	for _, tc := range []struct {
		interval, heartbeat, want time.Duration
	}{
		{5 * time.Second, time.Minute, 2500 * time.Millisecond},
		{500 * time.Millisecond, 200 * time.Millisecond, 250 * time.Millisecond},
		{10 * time.Second, 2 * time.Second, time.Second},
		{10 * time.Second, 0, 5 * time.Second},
		{2 * time.Minute, 0, 15 * time.Second},
		{time.Millisecond, 0, 250 * time.Millisecond},
	} {
		got, err := watchRequestTimeout(0, tc.interval, tc.heartbeat)
		if err != nil || got != tc.want {
			t.Fatalf("interval %s heartbeat %s: got %s %v, want %s", tc.interval, tc.heartbeat, got, err, tc.want)
		}
	}
}

func TestWatchRateLimitedSchemaRecheckDoesNotHoldBackPhasesOrTimeout(t *testing.T) {
	// --uda-dir: the tracked data type is drifted at baseline; after the
	// deploy lands every read of it answers 429 with Retry-After: 60. The
	// re-check runs alongside a poll's probes, so a client that honoured
	// the full Retry-After would hold the poll for a minute.
	sc := schemaScenario{dataType: func(tick int) (int, string) { return http.StatusOK, driftedRemoteDataType }}
	env := sc.env()
	env.landTick = 2
	inner := env.handler()
	handler := func(req *http.Request) (*http.Response, error) {
		tick := env.currentTick()
		if req.URL.Path == dataTypePath && tick >= 2 {
			resp := cmdtest.JSONResponse(http.StatusTooManyRequests, `{"title":"slow down"}`)
			resp.Header.Set("Retry-After", "60")
			return resp, nil
		}
		// The public page comes back two polls after landing, while the
		// re-check is rate limited.
		if req.URL.Path == "/" && (tick == 2 || tick == 3) {
			return cmdtest.JSONResponse(http.StatusServiceUnavailable, `{}`), nil
		}
		return inner(req)
	}
	started := time.Now()
	run := runWatchTimed(t, handler, 12*time.Second, "--json", "--uda-dir", schemaDir(t, false), "--interval", timingInterval, "--heartbeat", timingHeartbeat,
		"--settle", "0", "--skip-index-verify", "--escalation", "1h", "--timeout", "3s")

	assertPhasesOnTime(t, run, timingRequestTimeout+500*time.Millisecond, "restarting", "app-alive", "landed", "serving", "timeout")
	if !run.finished || exitCode(run.err) != 6 {
		t.Fatalf("expected the timeout exit (the schema never confirms), got finished=%v err=%v:\n%s", run.finished, run.err, run.describe(t))
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("--timeout 3s took %s to end the watch:\n%s", elapsed, run.describe(t))
	}
	assertHeartbeatsOnSchedule(t, run, firstPhaseAt(t, run, "landed"), firstPhaseAt(t, run, "timeout"), 2*200*time.Millisecond)
}

// blockingWriter never returns from Write until released, like a stderr
// pipe nobody drains.
type blockingWriter struct {
	release chan struct{}
	inner   timedWriter
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return w.inner.Write(p)
}

func TestWatchBlockedStderrDoesNotHoldBackStdout(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: 2}
	root := cmdtest.BuildRoot(t, cmdtest.Deps(env.handler()), Register)
	stdout := &timedWriter{}
	stderr := &blockingWriter{release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(stderr.release) }) }
	t.Cleanup(release)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"deploy", "watch", "--json", "--interval", timingInterval, "--heartbeat", timingHeartbeat,
		"--settle", "1500ms", "--skip-index-verify", "--escalation", "1h", "--timeout", "20s"})
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()

	// While stderr is blocked, the whole run must still reach stdout.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(fmt.Sprint(stdout.snapshot()), `"phase":"verified"`) {
		time.Sleep(20 * time.Millisecond)
	}
	run := timedRun{stdout: stdout.snapshot()}
	release()
	select {
	case run.err = <-done:
		run.finished = true
	case <-time.After(5 * time.Second):
	}
	assertPhasesOnTime(t, run, timingRequestTimeout+500*time.Millisecond, "restarting", "app-alive", "landed", "serving", "settling", "verified")
	assertHeartbeatsOnSchedule(t, run, firstPhaseAt(t, run, "settling"), firstPhaseAt(t, run, "verified"), 2*200*time.Millisecond)
	if !run.finished || run.err != nil {
		t.Fatalf("expected the watch to end once stderr drains (finished=%v err=%v)", run.finished, run.err)
	}
	if len(stderr.inner.snapshot()) == 0 {
		t.Fatalf("the stderr heartbeat lines should be written once stderr drains")
	}
}
