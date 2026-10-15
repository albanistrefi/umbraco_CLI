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

// The 15-10 live deploy: the log viewer refused every read for size, the app
// restarted and recovered, and the watch must still land and verify.
func TestWatchLandsOnARestartCycleWhenTheLogViewerRefusesFromBaseline(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 1 << 30, refuseLogs: true}
	lines, err := runWatch(t, env, "--logs")
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
	if landed["basis"] != "restart-cycle" || landed["processId"] != "unavailable" || !strings.Contains(fmt.Sprint(landed["processIdReason"]), "CancelledByLogsSizeValidation") {
		t.Fatalf("landed must name the fallback basis: %v", landed)
	}
	var refused []map[string]any
	for _, event := range ofType(lines, "log-monitor") {
		switch event["status"] {
		case "refused":
			refused = append(refused, event)
		case "unavailable":
			if reason := fmt.Sprint(detailOf(event)["reason"]); !strings.Contains(reason, "API 503") {
				t.Fatalf("a size refusal must not be reported as an expected restart outage: %v", event)
			}
		}
	}
	if len(refused) == 0 {
		t.Fatalf("expected a refused log-monitor line, got %v", ofType(lines, "log-monitor"))
	}
	detail := detailOf(refused[0])
	if detail["operationStatus"] != "CancelledByLogsSizeValidation" || !strings.Contains(fmt.Sprint(detail["note"]), "unavailable for the rest of this run") ||
		strings.Contains(fmt.Sprint(detail["note"]), "expected while the app restarts") {
		t.Fatalf("the refused line must say what happened: %v", refused[0])
	}
	// Downtime ticks can interleave a 503 between refusals; past that the
	// follower backs off instead of reading every 1ms interval.
	if env.tailReads > 8 {
		t.Fatalf("the log follower kept reading at full rate after the refusal: %d reads", env.tailReads)
	}
}

// The 15-10 sequence exactly: the baseline read the process id, then the log
// viewer started refusing before the restart.
func TestWatchLandsOnARestartCycleWhenTheLogViewerStartsRefusingAfterBaseline(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{3: true, 4: true}, landTick: 1 << 30, refuseLogs: true, refuseLogsFrom: 1}
	lines, err := runWatch(t, env, "--logs")
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

func TestWatchWithoutARestartOrAProcessIDDoesNotLand(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true}
	lines, err := runWatch(t, env, "--logs", "--timeout", "150ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v", err)
	}
	for _, phase := range phasesOf(lines) {
		if phase == "landed" || phase == "verified" || phase == "serving" {
			t.Fatalf("a watch that never saw a restart must not claim %s: %v", phase, phasesOf(lines))
		}
	}
}

func TestWatchStillRefusesToArmWhenTheLogViewerFailsOtherwise(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, routes: nil}
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

func TestWatchHeartbeatsSayTheLogStreamIsRefused(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true}
	deps := cmdtest.Deps(env.handler())
	out, errOut, err := cmdtest.ExecuteWithErr(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--json", "--logs",
		"--interval", "5ms", "--request-timeout", "4ms", "--heartbeat", "20ms", "--settle", "0", "--skip-index-verify", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if !strings.Contains(errOut, "logs: unavailable (size), 0 read, 0 emitted") {
		t.Fatalf("stderr heartbeats must show the refused log stream:\n%s", errOut)
	}
	heartbeats := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var decoded map[string]any
		if json.Unmarshal([]byte(line), &decoded) != nil || decoded["type"] != "heartbeat" {
			continue
		}
		heartbeats++
		if logs, _ := decoded["logs"].(map[string]any); logs["status"] != "refused" {
			t.Fatalf("JSON heartbeat must carry the log status: %v", decoded)
		}
	}
	if heartbeats == 0 {
		t.Fatalf("no JSON heartbeats:\n%s", out)
	}
}

func TestWatchMachineLandsOnRestartCycleOnlyWithoutAProcessID(t *testing.T) {
	base := watchBaseline()
	at := base.At
	tick := func(s int) time.Time { return at.Add(time.Duration(s) * time.Second) }
	healthy := map[string]bool{"/": true}

	// Process id unreadable after baseline, a full restart cycle: lands.
	m, err := newWatchMachine(base, time.Hour, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var all []watchEvent
	for _, obs := range []watchObservation{
		{At: tick(1), MgmtAlive: true, LogErr: sizeRefusal(), Health: healthy},
		{At: tick(2), MgmtAlive: false, MgmtStatus: 503},
		{At: tick(3), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: map[string]bool{"/": false}},
		{At: tick(4), MgmtAlive: true, MgmtStatus: 401, LogErr: sizeRefusal(), Health: healthy},
	} {
		events, _ := m.observe(obs)
		all = append(all, events...)
	}
	if got := strings.Join(phases(all), ","); got != "restarting,app-alive,serving,landed,verified" {
		t.Fatalf("unexpected phases %s", got)
	}

	// The process id is readable: the restart cycle alone does not land
	// while the newest entry still shows the old process.
	m, _ = newWatchMachine(base, time.Hour, 0, true)
	all = nil
	for _, obs := range []watchObservation{
		{At: tick(1), MgmtAlive: false, MgmtStatus: 503},
		{At: tick(2), MgmtAlive: true, MgmtStatus: 401, ProcessID: base.ProcessID, MachineName: base.MachineName, Health: healthy},
	} {
		events, _ := m.observe(obs)
		all = append(all, events...)
	}
	for _, event := range all {
		if event.Phase == "landed" {
			t.Fatalf("a readable, unchanged process id must not land: %v", phases(all))
		}
	}

	// No process id and no restart: never lands.
	m, _ = newWatchMachine(base, time.Hour, 0, true)
	for s := 1; s < 20; s++ {
		events, _ := m.observe(watchObservation{At: tick(s), MgmtAlive: true, LogErr: sizeRefusal(), Health: healthy})
		if len(events) > 0 {
			t.Fatalf("no restart observed, yet %v", phases(events))
		}
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
	feed := &watchLogFeed{}
	_, events := feed.record(t.Context(), "now", nil, sizeRefusal())
	if len(events) != 1 || events[0].(watchLogMonitorEvent).Status != "refused" || feed.state() != "refused" {
		t.Fatalf("a size refusal is refused: %v", events)
	}
	if _, events = feed.record(t.Context(), "now", nil, sizeRefusal()); len(events) != 0 {
		t.Fatalf("a persisting refusal is reported once, got %v", events)
	}
	_, events = feed.record(t.Context(), "now", nil, &api.APIError{StatusCode: http.StatusServiceUnavailable})
	if len(events) != 1 || !strings.Contains(fmt.Sprint(events[0].(watchLogMonitorEvent).Detail["note"]), "expected while the app restarts") {
		t.Fatalf("a 503 is the expected restart outage: %v", events)
	}
	feed = &watchLogFeed{}
	_, events = feed.record(t.Context(), "now", nil, &api.APIError{StatusCode: http.StatusForbidden})
	if len(events) != 1 || strings.Contains(fmt.Sprint(events[0].(watchLogMonitorEvent).Detail["note"]), "restart") {
		t.Fatalf("a 4xx is not described as a restart: %v", events)
	}
}
