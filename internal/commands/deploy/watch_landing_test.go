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
