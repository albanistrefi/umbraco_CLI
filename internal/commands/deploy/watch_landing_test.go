package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdtest"
)

func sizeRefusal() error {
	var payload map[string]any
	_ = json.Unmarshal([]byte(logSizeRefusalBody), &payload)
	return &api.APIError{StatusCode: http.StatusBadRequest, Method: http.MethodGet, Path: "/umbraco/management/api/v1/log-viewer/log", Payload: payload}
}

func phaseLine(lines []map[string]any, phase string) map[string]any {
	for _, line := range lines {
		if line["type"] == "phase" && line["phase"] == phase {
			return line
		}
	}
	return nil
}

func detailOf(line map[string]any) map[string]any {
	detail, _ := line["detail"].(map[string]any)
	return detail
}

func monitorStatuses(lines []map[string]any) string {
	statuses := make([]string, 0)
	for _, event := range ofType(lines, "log-monitor") {
		statuses = append(statuses, fmt.Sprint(event["status"]))
	}
	return strings.Join(statuses, ",")
}

func assertNotLanded(t *testing.T, lines []map[string]any) {
	t.Helper()
	for _, phase := range phasesOf(lines) {
		if phase == "landed" || phase == "verified" {
			t.Fatalf("must not claim %s: %v", phase, phasesOf(lines))
		}
	}
}

// The 15-10 live deploy: the log viewer refused every read for size, the app
// restarted and recovered, and the watch must still land and verify.
func TestWatchLandsOnARestartCycleWhenTheLogViewerRefusesFromBaseline(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 1 << 30, refuseLogs: true}
	lines, err := runWatch(t, env, "--logs", "--restart-min-down", "1ms")
	if err != nil {
		t.Fatalf("watch failed: %v\n%v", err, lines)
	}
	if got := strings.Join(phasesOf(lines), ","); got != "baseline,restarting,app-alive,serving,landed,verified" {
		t.Fatalf("unexpected phases %s", got)
	}
	baseline := detailOf(phaseLine(lines, "baseline"))
	if baseline["landingBasis"] != "restart-cycle" || baseline["processId"] != "unavailable" || !strings.Contains(fmt.Sprint(baseline["processIdReason"]), "100 MB") {
		t.Fatalf("baseline must say landing falls back to the restart cycle and why: %v", baseline)
	}
	landed := detailOf(phaseLine(lines, "landed"))
	if landed["basis"] != "restart-cycle" || landed["processId"] != "unavailable" || landed["downFor"] == nil || landed["downSince"] == nil ||
		!strings.Contains(fmt.Sprint(landed["processIdReason"]), "CancelledByLogsSizeValidation") {
		t.Fatalf("landed must name the fallback basis and the outage: %v", landed)
	}
	// The refusal is known from the first read and stays refused through
	// the restart's 503s: no "unavailable" in between.
	if got := monitorStatuses(lines); got != "refused,stopped" {
		t.Fatalf("unexpected monitor statuses %s", got)
	}
	refused := detailOf(ofType(lines, "log-monitor")[0])
	if refused["operationStatus"] != "CancelledByLogsSizeValidation" || strings.Contains(fmt.Sprint(refused["note"]), "expected while the app restarts") {
		t.Fatalf("the refused line must say what happened: %v", refused)
	}
	if lines[1]["type"] != "log-monitor" || lines[1]["status"] != "refused" {
		t.Fatalf("the refusal is reported right after the baseline: %v", lines[1])
	}
}

// The 15-10 sequence exactly: the baseline read the process id, then the log
// viewer started refusing before the restart.
func TestWatchLandsOnARestartCycleWhenTheLogViewerStartsRefusingAfterBaseline(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{3: true, 4: true}, landTick: 1 << 30, refuseLogs: true, refuseLogsFrom: 1}
	lines, err := runWatch(t, env, "--logs", "--restart-min-down", "1ms")
	if err != nil {
		t.Fatalf("watch failed: %v\n%v", err, lines)
	}
	if got := strings.Join(phasesOf(lines), ","); got != "baseline,restarting,app-alive,serving,landed,verified" {
		t.Fatalf("unexpected phases %s", got)
	}
	if baseline := detailOf(phaseLine(lines, "baseline")); baseline["processId"] != "111" || baseline["landingBasis"] != "process-id" {
		t.Fatalf("the baseline read the process id: %v", baseline)
	}
	if landed := detailOf(phaseLine(lines, "landed")); landed["basis"] != "restart-cycle" || landed["processId"] != "unavailable" {
		t.Fatalf("landed must fall back to the restart cycle: %v", landed)
	}
}

// P1-a, the reviewer's sequence: refusal from tick 1, one 502 on the
// liveness probe only, the process never changes. One failed probe is not a
// restart.
func TestWatchOneFailedProbeIsNotARestart(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, blipTicks: map[int]bool{2: true}, landTick: 1 << 30, refuseLogs: true, refuseLogsFrom: 1}
	lines, err := runWatch(t, env, "--logs", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	assertNotLanded(t, lines)
	if !strings.Contains(strings.Join(phasesOf(lines), ","), "restarting") {
		t.Fatalf("the blip itself is still reported: %v", phasesOf(lines))
	}
}

// P1-b, scenario 2: a real cycle, but the first alive poll's newest-entry
// read fails with a 500 (not a size refusal), and the next one reads the
// baseline process.
func TestWatchATransientLogErrorAfterACycleDoesNotLand(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, newestFails: map[int]bool{4: true}, landTick: 1 << 30}
	lines, err := runWatch(t, env, "--restart-min-down", "1ms", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	assertNotLanded(t, lines)
}

// P1-b, scenario 1: the process id is known, an outage is followed by the
// baseline process (no landing), and the log viewer starts refusing much
// later. The disproved outage must not land the watch.
func TestWatchAnOutageDisprovedByTheBaselineProcessDoesNotLandLater(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 1 << 30, refuseLogs: true, refuseLogsFrom: 20}
	lines, err := runWatch(t, env, "--restart-min-down", "1ms", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	assertNotLanded(t, lines)
	if env.tick < 25 {
		t.Fatalf("the refusal never started (tick %d)", env.tick)
	}
}

// Baseline refused, then the newest entry reads again mid-run with no
// restart: there is no baseline process to compare, so it must not land.
func TestWatchARecoveredLogViewerWithoutARestartDoesNotLand(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true, refuseLogsUntil: 5}
	lines, err := runWatch(t, env, "--restart-min-down", "1ms", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	assertNotLanded(t, lines)
}

// Baseline refused, a real cycle, then the log viewer reads again and shows
// the new process: without a baseline process to compare, the cycle lands.
func TestWatchARecoveredLogViewerAfterACycleLandsOnTheCycle(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 4, refuseLogs: true, refuseLogsUntil: 4}
	lines, err := runWatch(t, env, "--restart-min-down", "1ms")
	if err != nil {
		t.Fatalf("watch failed: %v\n%v", err, phasesOf(lines))
	}
	if landed := detailOf(phaseLine(lines, "landed")); landed["basis"] != "restart-cycle" {
		t.Fatalf("expected a restart-cycle landing: %v", lines)
	}
}

func TestWatchWithoutARestartOrAProcessIDDoesNotLand(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true}
	lines, err := runWatch(t, env, "--logs", "--restart-min-down", "0", "--timeout", "150ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v", err)
	}
	assertNotLanded(t, lines)
}

func TestWatchStillRefusesToArmWhenTheLogViewerFailsOtherwise(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30}
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/log-viewer/log" {
			return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"log viewer broke"}`), nil
		}
		return env.handler()(req)
	})
	_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--json", "--interval", "1ms", "--heartbeat", "0", "--timeout", "1s")
	var apiErr *api.APIError
	if err == nil || !strings.Contains(err.Error(), "cannot baseline the target: reading the newest log entry failed") || !errors.As(err, &apiErr) || apiErr.ExitCode() != 4 {
		t.Fatalf("a broken log viewer at baseline must still refuse to arm with exit 4, got %v", err)
	}
}

func TestWatchRestartMinDownMustNotBeNegative(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request may be sent on a usage error, got %s", req.URL)
		return nil, nil
	})
	_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--restart-min-down", "-1s")
	if err == nil || exitCode(err) != 1 || !strings.Contains(err.Error(), "--restart-min-down cannot be negative") {
		t.Fatalf("expected a usage error, got %v", err)
	}
}

func TestWatchHeartbeatsSayTheLogStreamIsRefused(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true}
	deps := cmdtest.Deps(env.handler())
	out, errOut, err := cmdtest.ExecuteWithErr(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--json", "--logs",
		"--interval", "5ms", "--request-timeout", "4ms", "--heartbeat", "20ms", "--settle", "0", "--skip-index-verify", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if strings.Contains(errOut, "logs: 0 read") || !strings.Contains(errOut, "logs: unavailable (size), 0 read, 0 emitted") {
		t.Fatalf("every stderr heartbeat must show the refused log stream:\n%s", errOut)
	}
	heartbeats := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var decoded map[string]any
		if json.Unmarshal([]byte(line), &decoded) != nil || decoded["type"] != "heartbeat" {
			continue
		}
		heartbeats++
		if logs, _ := decoded["logs"].(map[string]any); logs["status"] != "refused" {
			t.Fatalf("every JSON heartbeat must carry the refused status: %v", decoded)
		}
	}
	if heartbeats == 0 {
		t.Fatalf("no JSON heartbeats:\n%s", out)
	}
}

// Per-poll reads over 300ms at a 5ms interval would be dozens; after the
// refusal the follower waits 5 minutes, so only the first read and the final
// drain reach the log viewer.
func TestWatchLogFollowerBacksOffWhileRefused(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true}
	deps := cmdtest.Deps(env.handler())
	_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--json", "--logs",
		"--interval", "5ms", "--request-timeout", "4ms", "--heartbeat", "0", "--settle", "0", "--skip-index-verify", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if env.tick < 20 {
		t.Fatalf("too few polls (%d) to tell per-poll reads from backoff", env.tick)
	}
	if env.tailReads > 3 {
		t.Fatalf("the follower read %d times over %d polls while refused", env.tailReads, env.tick)
	}
}

// When a poll reads the newest entry again, the follower reads at once
// instead of waiting out the 5-minute backoff.
func TestWatchLogFollowerWakesWhenTheNewestEntryReadsAgain(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true, refuseLogsUntil: 5}
	lines, err := runWatch(t, env, "--logs", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if got := monitorStatuses(lines); got != "refused,resumed,stopped" {
		t.Fatalf("unexpected monitor statuses %s", got)
	}
	if env.firstTailOK == 0 || env.firstTailOK > 15 || env.tick < 40 {
		t.Fatalf("the follower should read soon after tick 5, first good read at tick %d of %d", env.firstTailOK, env.tick)
	}
}

func TestWatchMachineRestartCycleNeedsAnOutageOfRestartMinDown(t *testing.T) {
	base := watchBaseline()
	base.ProcessID, base.MachineName, base.LogErr = "", "", sizeRefusal()
	tick := func(s int) time.Time { return base.At.Add(time.Duration(s) * time.Second) }
	healthy := map[string]bool{"/": true}
	m, err := newWatchMachine(base, time.Hour, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	m.restartMinDown = 20 * time.Second
	var all []watchEvent
	feed := func(obs watchObservation) {
		events, _ := m.observe(obs)
		all = append(all, events...)
	}
	// A 5s blip, then serving: not a restart.
	feed(watchObservation{At: tick(5), MgmtAlive: false, MgmtStatus: 502})
	feed(watchObservation{At: tick(10), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy})
	feed(watchObservation{At: tick(15), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy})
	if got := strings.Join(phases(all), ","); got != "restarting,app-alive,serving" {
		t.Fatalf("a 5s blip must not land, got %s", got)
	}
	// A 30s outage: lands once the paths serve.
	feed(watchObservation{At: tick(20), MgmtAlive: false, MgmtStatus: 503})
	feed(watchObservation{At: tick(50), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: map[string]bool{"/": false}})
	feed(watchObservation{At: tick(55), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy})
	if got := strings.Join(phases(all), ","); got != "restarting,app-alive,serving,landed,verified" {
		t.Fatalf("a 30s outage should land, got %s", got)
	}
	landed := all[3]
	if landed.Detail["downFor"] != "30s" || landed.Detail["basis"] != "restart-cycle" {
		t.Fatalf("landed must carry the qualifying outage: %v", landed.Detail)
	}
	if landed.Timestamp != tick(55).UTC().Format(time.RFC3339) {
		t.Fatalf("landing waits for the paths to serve after the outage, landed at %s", landed.Timestamp)
	}
}

func TestWatchMachineRestartCycleOnlyForASizeRefusal(t *testing.T) {
	base := watchBaseline()
	tick := func(s int) time.Time { return base.At.Add(time.Duration(s) * time.Second) }
	healthy := map[string]bool{"/": true}
	cycle := []watchObservation{
		{At: tick(1), MgmtAlive: false, MgmtStatus: 503},
		{At: tick(40), MgmtAlive: false, MgmtStatus: 503},
	}
	for _, tc := range []struct {
		name string
		obs  watchObservation
		land bool
	}{
		{"a size refusal lands", watchObservation{MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy}, true},
		{"a timed-out read waits", watchObservation{MgmtAlive: true, MgmtStatus: 401, LogErr: errors.New("context deadline exceeded"), Health: healthy}, false},
		{"a 500 waits", watchObservation{MgmtAlive: true, MgmtStatus: 401, LogErr: &api.APIError{StatusCode: 500}, Health: healthy}, false},
		{"an entry without ProcessId waits", watchObservation{MgmtAlive: true, MgmtStatus: 401, Health: healthy}, false},
		{"the old process still newest waits", watchObservation{MgmtAlive: true, MgmtStatus: 401, ProcessID: base.ProcessID, MachineName: base.MachineName, Health: healthy}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newWatchMachine(base, time.Hour, 0, true)
			m.restartMinDown = 20 * time.Second
			var all []watchEvent
			for _, obs := range cycle {
				events, _ := m.observe(obs)
				all = append(all, events...)
			}
			obs := tc.obs
			obs.At = tick(45)
			events, _ := m.observe(obs)
			all = append(all, events...)
			landed := strings.Contains(strings.Join(phases(all), ","), "landed")
			if landed != tc.land {
				t.Fatalf("landed %v, want %v: %v", landed, tc.land, phases(all))
			}
		})
	}
}

func TestWatchMachineTheBaselineProcessDisprovesTheCycle(t *testing.T) {
	base := watchBaseline()
	tick := func(s int) time.Time { return base.At.Add(time.Duration(s) * time.Second) }
	healthy := map[string]bool{"/": true}
	m, _ := newWatchMachine(base, time.Hour, 0, true)
	m.restartMinDown = 20 * time.Second
	var all []watchEvent
	for _, obs := range []watchObservation{
		{At: tick(1), MgmtAlive: false, MgmtStatus: 503},
		{At: tick(40), MgmtAlive: true, MgmtStatus: 401, ProcessID: base.ProcessID, MachineName: base.MachineName, Health: healthy},
		{At: tick(300), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy},
	} {
		events, _ := m.observe(obs)
		all = append(all, events...)
	}
	if strings.Contains(strings.Join(phases(all), ","), "landed") {
		t.Fatalf("an outage the baseline process survived must not land later: %v", phases(all))
	}
	// A later qualifying outage arms a new cycle.
	for _, obs := range []watchObservation{
		{At: tick(310), MgmtAlive: false, MgmtStatus: 503},
		{At: tick(350), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy},
	} {
		events, _ := m.observe(obs)
		all = append(all, events...)
	}
	if !strings.Contains(strings.Join(phases(all), ","), "landed") {
		t.Fatalf("a new outage should land: %v", phases(all))
	}
}

func TestWatchMachineCompareProcessesOnlyAgainstABaselineProcess(t *testing.T) {
	base := watchBaseline()
	base.ProcessID, base.MachineName, base.LogErr = "", "", sizeRefusal()
	m, _ := newWatchMachine(base, time.Hour, 0, true)
	if m.processChanged(watchObservation{ProcessID: "222", MachineName: "m1"}) {
		t.Fatalf("a process id read without a baseline one is not a change")
	}
}

func TestWatchMachineArmsWithoutAProcessIDOnlyForASizeRefusal(t *testing.T) {
	base := watchBaseline()
	base.ProcessID, base.MachineName = "", ""
	base.LogErr = sizeRefusal()
	m, err := newWatchMachine(base, time.Hour, 0, true)
	if err != nil || m.processIDUnavailable == "" {
		t.Fatalf("a size refusal at baseline must arm on the restart-cycle basis, got %v", err)
	}
	base.LogErr = &api.APIError{StatusCode: http.StatusInternalServerError, Payload: map[string]any{"title": "broken"}}
	if _, err := newWatchMachine(base, time.Hour, 0, true); err == nil {
		t.Fatalf("another log error at baseline must still refuse to arm")
	}
	base.LogErr = nil
	if _, err := newWatchMachine(base, time.Hour, 0, true); err == nil {
		t.Fatalf("a newest entry without ProcessId must still refuse to arm")
	}
}

func TestWatchLogFeedReportsEachFailureTruthfully(t *testing.T) {
	ctx := t.Context()
	feed := &watchLogFeed{}
	_, events := feed.record(ctx, "now", nil, sizeRefusal())
	if len(events) != 1 || events[0].(watchLogMonitorEvent).Status != "refused" || feed.state() != "refused" {
		t.Fatalf("a size refusal is refused: %v", events)
	}
	if note := fmt.Sprint(events[0].(watchLogMonitorEvent).Detail["note"]); strings.Contains(note, "for the rest of this run") {
		t.Fatalf("the note must not promise a refusal for the whole run, resumed can follow: %s", note)
	}
	for _, err := range []error{sizeRefusal(), &api.APIError{StatusCode: http.StatusServiceUnavailable}, errors.New("connection refused"), sizeRefusal()} {
		if _, events = feed.record(ctx, "now", nil, err); len(events) != 0 || feed.state() != "refused" {
			t.Fatalf("refused stays refused through %v, got %v (%s)", err, events, feed.state())
		}
	}
	if _, events = feed.record(ctx, "now", []map[string]any{}, nil); len(events) != 1 || events[0].(watchLogMonitorEvent).Status != "resumed" || feed.state() != "reading" {
		t.Fatalf("a good read resumes: %v", events)
	}

	feed = &watchLogFeed{}
	_, events = feed.record(ctx, "now", nil, &api.APIError{StatusCode: http.StatusServiceUnavailable})
	if len(events) != 1 || !strings.Contains(fmt.Sprint(events[0].(watchLogMonitorEvent).Detail["note"]), "expected while the app restarts") {
		t.Fatalf("a 503 is the expected restart outage: %v", events)
	}
	feed = &watchLogFeed{}
	_, events = feed.record(ctx, "now", nil, &api.APIError{StatusCode: http.StatusForbidden})
	if len(events) != 1 || strings.Contains(fmt.Sprint(events[0].(watchLogMonitorEvent).Detail["note"]), "restart") {
		t.Fatalf("a 4xx is not described as a restart: %v", events)
	}
}

// With no baseline process id, entries written before the deploy cannot be
// told from the schema pass's, so pass detection is off: an older "Work
// Status" entry must not book the pass, and a deploy leaving the artifact
// drifted times out (exit 6) instead of exiting 7.
func TestWatchSchemaPassDetectionIsOffWithoutABaselineProcess(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	oldPassEnd := `{"timestamp":"` + future + `","level":"Information","renderedMessage":"Work Status \"Completed\".","properties":[` +
		`{"name":"SourceContext","value":"Umbraco.Deploy.Infrastructure.Disk.DiskService"},{"name":"ProcessId","value":"111"},{"name":"MachineName","value":"m1"}]}`
	env := schemaScenario{
		dataType: func(int) (int, string) { return http.StatusOK, driftedRemoteDataType },
		logs:     []fakeLogEntry{{5, oldPassEnd}},
	}.env()
	env.downTicks, env.landTick = map[int]bool{8: true, 9: true}, 1<<30
	env.refuseLogs, env.refuseLogsUntil = true, 5
	lines, err := runWatch(t, env, "--uda-dir", schemaDir(t, false), "--restart-min-down", "1ms", "--timeout", "400ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	for _, line := range ofType(lines, "schema-pass") {
		if line["status"] != "waiting" || !strings.Contains(fmt.Sprint(detailOf(line)["reason"]), "detection is off") {
			t.Fatalf("an entry from before the deploy booked the schema pass, or the wait names a pass: %v", line)
		}
	}
	if timeout := detailOf(phaseLine(lines, "timeout")); !strings.Contains(fmt.Sprint(timeout["reason"]), "schema pass detection off") {
		t.Fatalf("the timeout must not say a pass end was awaited: %v", timeout)
	}
	if pass, _ := detailOf(ofType(lines, "schema-summary")[0])["deployPass"].(map[string]any); pass["detection"] != "off" || pass["ended"] != false {
		t.Fatalf("the summary must say detection was off: %v", pass)
	}
	schema, _ := detailOf(phaseLine(lines, "baseline"))["schema"].(map[string]any)
	if !strings.HasPrefix(fmt.Sprint(schema["passDetection"]), "off:") {
		t.Fatalf("the baseline must say pass detection is off: %v", schema)
	}
	if env.firstTailOK == 0 {
		t.Fatalf("the log viewer never recovered, so the old entry was never offered")
	}
}
