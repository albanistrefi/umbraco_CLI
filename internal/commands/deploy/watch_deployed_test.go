package deploy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"umbraco-cli/internal/commands/cmdtest"
)

// deployCompletedLogs is Deploy's pass with the deployment's own completion
// line after the work status, as logged on a completed deploy.
func deployCompletedLogs(startTick, endTick int, workStatus string) []fakeLogEntry {
	return append(deployPassLogs(startTick, endTick, workStatus),
		fakeLogEntry{endTick, logJSON(32, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkEnvironment", `Deployment "9bfb818b" completed.`, "")})
}

func decodeEntry(t *testing.T, raw string) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func countPhase(lines []map[string]any, phase string) int {
	count := 0
	for _, name := range phasesOf(lines) {
		if name == phase {
			count++
		}
	}
	return count
}

func assertNoDeployed(t *testing.T, lines []map[string]any) {
	t.Helper()
	if countPhase(lines, "deployed") != 0 {
		t.Fatalf("deployed must not be emitted: %v", phasesOf(lines))
	}
}

// The settle window is already running when Deploy logs the end of its
// pass (no --uda-dir, so nothing holds the settle back): deployed is a
// phase line of its own and the window carries on to verified.
func TestWatchDeployedFiresOnceDuringARunningSettle(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true, 2: true}, landTick: 3, entries: deployCompletedLogs(5, 8, "Completed")}
	lines, err := runWatch(t, env, "--logs", "--settle", "300ms")
	if err != nil {
		t.Fatalf("expected verified, got %v\n%v", err, phasesOf(lines))
	}
	if got := countPhase(lines, "deployed"); got != 1 {
		t.Fatalf("expected one deployed line, got %d: %v", got, phasesOf(lines))
	}
	landed, serving, settling := indexOf(lines, isPhase("landed")), indexOf(lines, isPhase("serving")), indexOf(lines, isPhase("settling"))
	deployed, verified := indexOf(lines, isPhase("deployed")), indexOf(lines, isPhase("verified"))
	if landed < 0 || serving < 0 || deployed < landed || deployed < serving || deployed < settling || verified < deployed {
		t.Fatalf("expected landed, serving, settling < deployed < verified: %v", phasesOf(lines))
	}
	if countPhase(lines, "settling") != 1 || countPhase(lines, "settle-interrupted") != 0 {
		t.Fatalf("deployed must neither restart nor end the settle: %v", phasesOf(lines))
	}
	detail := detailOf(phaseLine(lines, "deployed"))
	if detail["deploymentId"] != "9bfb818b" || detail["workStatus"] != "Completed" || detail["loggedAt"] != "2026-10-01T10:00:31Z" || detail["schema"] != nil {
		t.Fatalf("unexpected deployed detail %v", detail)
	}
	if baseline := detailOf(phaseLine(lines, "baseline"))["deployed"].(map[string]any); baseline["enabled"] != true {
		t.Fatalf("the baseline must say deployed is on: %v", baseline)
	}
	if len(ofType(lines, "schema-pass")) != 0 {
		t.Fatalf("schema-pass lines are --uda-dir's: %v", ofType(lines, "schema-pass"))
	}
}

// With --uda-dir, deployed waits for a re-check after the pass ended: the
// data type cannot be read right after the pass, then is in sync.
func TestWatchDeployedWithUdaDirWaitsForTheRecheck(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			switch {
			case tick == 4 || tick == 5:
				return http.StatusInternalServerError, `{"title":"busy"}`
			case tick >= 6:
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		logs: deployCompletedLogs(4, 4, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false), "--settle", "200ms")
	if err != nil {
		t.Fatalf("expected verified, got %v\n%v", err, phasesOf(lines))
	}
	if got := countPhase(lines, "deployed"); got != 1 {
		t.Fatalf("expected one deployed line, got %d: %v", got, phasesOf(lines))
	}
	ended := indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema-pass" && l["status"] == "ended" })
	confirmed := indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema" && l["status"] == "in-sync" })
	// The re-check and the log follower run independently, so the data
	// type can read in sync before the pass end is read: then the settle
	// starts first and deployed follows inside it.
	deployed, verified := indexOf(lines, isPhase("deployed")), indexOf(lines, isPhase("verified"))
	if ended < 0 || confirmed < 0 || deployed < ended || deployed < confirmed || verified < deployed || countPhase(lines, "settle-interrupted") != 0 {
		t.Fatalf("expected ended, confirmed < deployed < verified, got %d %d %d %d:\n%v", ended, confirmed, deployed, verified, lines)
	}
	schema := detailOf(lines[deployed])["schema"].(map[string]any)
	if schema["confirmed"] != float64(1) || schema["remaining"] != float64(0) || schema["unknown"] != float64(0) {
		t.Fatalf("unexpected schema counts %v", schema)
	}
}

// Deploy completed its pass but skipped an artifact: deployed reports it as
// remaining, and the watch still exits 7.
func TestWatchDeployedReportsArtifactsStillMissing(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick >= 5 {
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		automateAvailable: true,
		logs:              deployCompletedLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, true))
	if exitCode(err) != 7 {
		t.Fatalf("expected exit 7, got %d %v", exitCode(err), err)
	}
	deployed := phaseLine(lines, "deployed")
	if deployed == nil || countPhase(lines, "deployed") != 1 || indexOf(lines, isPhase("deployed")) > indexOf(lines, isPhase("verified")) {
		t.Fatalf("expected one deployed line before verified: %v", phasesOf(lines))
	}
	if schema := detailOf(deployed)["schema"].(map[string]any); schema["confirmed"] != float64(1) || schema["remaining"] != float64(1) {
		t.Fatalf("unexpected schema counts %v", schema)
	}
}

func TestWatchDeployedNeverFiresOnAFailedPass(t *testing.T) {
	t.Run("uda-dir", func(t *testing.T) {
		sc := schemaScenario{
			dataType: func(int) (int, string) { return http.StatusOK, driftedRemoteDataType },
			logs:     deployCompletedLogs(4, 5, "Failed"),
		}
		lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false), "--logs")
		if exitCode(err) != 7 || !strings.Contains(fmt.Sprint(err), `work status "Failed"`) {
			t.Fatalf("expected today's exit 7 naming the work status, got %d %v", exitCode(err), err)
		}
		assertNoDeployed(t, lines)
	})
	t.Run("logs", func(t *testing.T) {
		env := &fakeDeployEnv{downTicks: map[int]bool{1: true, 2: true}, landTick: 3, entries: deployPassLogs(4, 5, "Failed")}
		lines, err := runWatch(t, env, "--logs", "--settle", "200ms")
		if err != nil {
			t.Fatalf("expected verified, got %v", err)
		}
		read := indexOf(lines, func(l map[string]any) bool { return l["type"] == "log" && l["message"] == `Work Status "Failed".` })
		if read < 0 || read > indexOf(lines, isPhase("verified")) {
			t.Fatalf("the failed work status must be read before the watch ends: %v", lines)
		}
		assertNoDeployed(t, lines)
	})
}

func TestWatchDeployedIsOffWithoutDeploysLog(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true, 2: true}, landTick: 3, entries: deployCompletedLogs(4, 5, "Completed")}
	lines, err := runWatch(t, env, "--settle", "100ms")
	if err != nil {
		t.Fatalf("expected verified, got %v", err)
	}
	assertNoDeployed(t, lines)
	baseline := detailOf(phaseLine(lines, "baseline"))["deployed"].(map[string]any)
	if baseline["enabled"] != false || !strings.Contains(fmt.Sprint(baseline["reason"]), "--logs or --uda-dir") {
		t.Fatalf("the baseline must say deployed is off and why: %v", baseline)
	}
	if env.tailReads != 0 {
		t.Fatalf("no log tail may be read without --logs or --uda-dir, read %d times", env.tailReads)
	}
}

func TestWatchDeployedNotInferredWhileTheLogViewerRefuses(t *testing.T) {
	t.Run("from baseline", func(t *testing.T) {
		env := &fakeDeployEnv{downTicks: fakeOutage(2, 6), landTick: 1 << 30, refuseLogs: true, entries: deployCompletedLogs(8, 9, "Completed")}
		lines, err := runWatch(t, env, "--logs", "--restart-min-down", "3ms", "--settle", "100ms")
		if err != nil {
			t.Fatalf("watch failed: %v\n%v", err, phasesOf(lines))
		}
		assertNoDeployed(t, lines)
		baseline := detailOf(phaseLine(lines, "baseline"))["deployed"].(map[string]any)
		if baseline["enabled"] != false || !strings.Contains(fmt.Sprint(baseline["reason"]), "no baseline process id") {
			t.Fatalf("the baseline must say deployed is off and why: %v", baseline)
		}
	})
	t.Run("after baseline", func(t *testing.T) {
		env := &fakeDeployEnv{downTicks: fakeOutage(3, 7), landTick: 1 << 30, refuseLogs: true, refuseLogsFrom: 1, entries: deployCompletedLogs(9, 10, "Completed")}
		lines, err := runWatch(t, env, "--logs", "--restart-min-down", "3ms", "--settle", "100ms")
		if err != nil {
			t.Fatalf("watch failed: %v\n%v", err, phasesOf(lines))
		}
		assertNoDeployed(t, lines)
		if baseline := detailOf(phaseLine(lines, "baseline"))["deployed"].(map[string]any); baseline["enabled"] != true {
			t.Fatalf("the baseline read the process id, so deployed is on: %v", baseline)
		}
	})
}

func TestWatchDeployedTextLine(t *testing.T) {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true, 2: true}, landTick: 3, entries: deployCompletedLogs(4, 5, "Completed")}
	deps := cmdtest.Deps(env.handler())
	out, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--logs", "--interval", "1ms", "--settle", "200ms",
		"--skip-index-verify", "--escalation", "1h", "--heartbeat", "0", "--timeout", "10s")
	if err != nil {
		t.Fatalf("expected verified, got %v", err)
	}
	found := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, " deployed — deployment complete — ") {
			found++
			if !strings.Contains(line, "deploymentId=9bfb818b") {
				t.Fatalf("the text line should carry the deployment id: %q", line)
			}
		}
	}
	if found != 1 {
		t.Fatalf("expected one deployed text line, got %d:\n%s", found, out)
	}
}

func TestWatchMachineDeployedWaitsForLandedAndServingAndKeepsTheSettle(t *testing.T) {
	machine, _ := newWatchMachine(watchBaseline(), 10*time.Minute, time.Minute, true)
	at := watchBaseline().At
	done := map[string]any{"workStatus": "Completed"}
	// Deploy's completion read before the poll that sees the new process:
	// held back until landed and serving.
	events, _ := machine.observe(watchObservation{At: at.Add(10 * time.Second), MgmtAlive: true, MgmtStatus: 401, ProcessID: "1000", MachineName: "web-a", Health: map[string]bool{"/": true}, Deployed: done})
	if len(events) != 0 {
		t.Fatalf("nothing has landed yet: %v", phases(events))
	}
	events, _ = machine.observe(watchObservation{At: at.Add(20 * time.Second), MgmtAlive: true, MgmtStatus: 401, ProcessID: "2000", MachineName: "web-a", Health: map[string]bool{"/": true}, Deployed: done})
	if got := strings.Join(phases(events), ","); got != "landed,serving,deployed,settling" {
		t.Fatalf("expected deployed between serving and settling, got %s", got)
	}
	if machine.phase != "settling" {
		t.Fatalf("expected phase settling, got %s", machine.phase)
	}
	// Emitted once.
	events, outcome := machine.observe(watchObservation{At: at.Add(81 * time.Second), MgmtAlive: true, MgmtStatus: 401, ProcessID: "2000", MachineName: "web-a", Health: map[string]bool{"/": true}, Deployed: done})
	if outcome != watchOutcomeVerified || strings.Join(phases(events), ",") != "verified" {
		t.Fatalf("expected only verified, got %v %v", phases(events), outcome)
	}

	// Completion read while a settle window is running: the window keeps
	// its start and the current phase stays settling.
	machine, _ = newWatchMachine(watchBaseline(), 10*time.Minute, time.Minute, true)
	good := func(offset time.Duration, deployed map[string]any) watchObservation {
		return watchObservation{At: at.Add(offset), MgmtAlive: true, MgmtStatus: 401, ProcessID: "2000", MachineName: "web-a", Health: map[string]bool{"/": true}, Deployed: deployed}
	}
	machine.observe(good(10*time.Second, nil))
	start := machine.settleStart
	events, _ = machine.observe(good(40*time.Second, done))
	if strings.Join(phases(events), ",") != "deployed" || machine.settleStart != start || machine.phase != "settling" {
		t.Fatalf("deployed must leave the settle alone, got %v, start %v → %v, phase %s", phases(events), start, machine.settleStart, machine.phase)
	}
	if _, outcome := machine.observe(good(71*time.Second, done)); outcome != watchOutcomeVerified {
		t.Fatalf("the original settle window should verify at its end, got %v", outcome)
	}
}

func TestDeployPassDeploymentID(t *testing.T) {
	pass := newDeployPass(watchObservation{ProcessID: "111", MachineName: "m1"})
	entries := func(logs []fakeLogEntry) []map[string]any {
		decoded := make([]map[string]any, 0, len(logs))
		for _, log := range logs {
			decoded = append(decoded, decodeEntry(t, log.json))
		}
		return decoded
	}
	pass.observe(entries([]fakeLogEntry{{0, logJSON(1, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkItems.DiskReadWorkItem", "Preparing", "")}}))
	pass.observe(entries([]fakeLogEntry{
		{0, logJSON(2, "Information", "Umbraco.Deploy.Infrastructure.Disk.DiskService", `Work Status "Completed".`, "")},
		{0, logJSON(3, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkEnvironment", `Deployment "0000aaaa" completed.`, "")},
	}))
	if !pass.completed() || pass.deploymentID() != "0000aaaa" || pass.endedAt != "2026-10-01T10:00:02Z" {
		t.Fatalf("without a beginning the completion line names the deployment: %+v", pass)
	}
	pass.observe(entries([]fakeLogEntry{{0, logJSON(4, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkEnvironment", `Beginning deployment "0000bbbb".`, "")}}))
	if pass.deploymentID() != "0000aaaa" {
		t.Fatalf("a deployment begun after the pass ended is not this one: %+v", pass)
	}
}
