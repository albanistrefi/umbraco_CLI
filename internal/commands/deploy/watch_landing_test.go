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
	if !strings.Contains(monitorStatuses(lines), "refused") {
		t.Fatalf("expected a refused log-monitor line, got %s", monitorStatuses(lines))
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
