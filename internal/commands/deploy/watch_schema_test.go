package deploy

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

const (
	schemaLandTick = 3
	dataTypePath   = "/umbraco/management/api/v1/data-type/aaaaaaaa-1111-2222-3333-444444444444"
	doctypePath    = "/umbraco/management/api/v1/document-type/bbbbbbbb-1111-2222-3333-444444444444"
)

var driftedRemoteDataType = strings.Replace(statusRemoteDataType, `"value": 500`, `"value": 200`, 1)

// deployPassLogs is Deploy's schema pass as logged by the new process on
// 01-10: the disk work item starting, then DiskService's work status.
func deployPassLogs(startTick, endTick int, workStatus string) []fakeLogEntry {
	return []fakeLogEntry{
		{startTick, logJSON(20, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkEnvironment", `Beginning deployment "9bfb818b".`, "")},
		{startTick, logJSON(21, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkItems.DiskReadWorkItem", "Preparing", "")},
		{endTick, logJSON(30, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkItems.DiskReadWorkItem", "Complete", "")},
		{endTick, logJSON(31, "Information", "Umbraco.Deploy.Infrastructure.Disk.DiskService", fmt.Sprintf(`Work Status %q.`, workStatus), "")},
	}
}

type schemaScenario struct {
	// dataType answers the data type GET per tick (status code, body).
	dataType          func(tick int) (int, string)
	automateAvailable bool
	automationExists  bool
	logs              []fakeLogEntry
}

func (sc schemaScenario) env() *fakeDeployEnv {
	env := &fakeDeployEnv{downTicks: map[int]bool{1: true}, landTick: schemaLandTick, entries: sc.logs}
	env.routes = func(tick int, req *http.Request) *http.Response {
		switch {
		case req.URL.Path == dataTypePath:
			status, body := sc.dataType(tick)
			return cmdtest.JSONResponse(status, body)
		case req.URL.Path == doctypePath:
			return cmdtest.JSONResponse(http.StatusOK, statusRemoteDoctype)
		case strings.HasPrefix(req.URL.Path, "/umbraco/automate/management/api/v1/"):
			if !sc.automateAvailable {
				return cmdtest.JSONResponse(http.StatusNotFound, `null`)
			}
			if strings.HasSuffix(req.URL.Path, "/automations") {
				return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`)
			}
			if !sc.automationExists {
				return cmdtest.JSONResponse(http.StatusNotFound, `null`)
			}
		}
		return nil
	}
	return env
}

func schemaDir(t *testing.T, withAutomation bool) string {
	t.Helper()
	dir := t.TempDir()
	writeUda(t, dir, "data-type__a.uda", statusDataTypeUda)
	writeUda(t, dir, "document-type__b.uda", statusDoctypeUda)
	if withAutomation {
		writeUda(t, dir, "umbraco-automate-automation__d.uda", statusAutomationUda)
	}
	return dir
}

func indexOf(lines []map[string]any, match func(map[string]any) bool) int {
	for i, line := range lines {
		if match(line) {
			return i
		}
	}
	return -1
}

func isPhase(name string) func(map[string]any) bool {
	return func(line map[string]any) bool { return line["type"] == "phase" && line["phase"] == name }
}

func summaryOf(t *testing.T, lines []map[string]any) map[string]any {
	t.Helper()
	i := indexOf(lines, func(line map[string]any) bool { return line["type"] == "schema-summary" })
	if i < 0 {
		t.Fatalf("no schema-summary line: %v", lines)
	}
	return lines[i]
}

func TestWatchSchemaConfirmsTrackedArtifactsAfterDeployPass(t *testing.T) {
	sc := schemaScenario{
		// Drifted until Deploy's pass has run (tick 5), in sync after.
		dataType: func(tick int) (int, string) {
			if tick >= 5 {
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		logs: deployPassLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if err != nil {
		t.Fatalf("expected verified, got %v", err)
	}

	baseline := lines[indexOf(lines, isPhase("baseline"))]["detail"].(map[string]any)["schema"].(map[string]any)
	tracking := baseline["tracking"].([]any)
	if len(tracking) != 1 || tracking[0].(map[string]any)["file"] != "data-type__a.uda" || baseline["inSync"] != float64(1) {
		t.Fatalf("baseline should track the drifted data type only: %v", baseline)
	}

	waiting := indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema-pass" && l["status"] == "waiting" })
	started := indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema-pass" && l["status"] == "started" })
	ended := indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema-pass" && l["status"] == "ended" })
	confirmed := indexOf(lines, func(l map[string]any) bool {
		return l["type"] == "schema" && l["file"] == "data-type__a.uda" && l["status"] == "in-sync" && l["previous"] == "drifted"
	})
	verified := indexOf(lines, isPhase("verified"))
	serving := indexOf(lines, isPhase("serving"))
	// Schema-pass lines come from the log follower and the re-check from
	// the probe loop, which run independently: their relative order is the
	// order they were read in, not fixed.
	if serving < 0 || waiting < serving || started < waiting || ended < started || confirmed < waiting || verified < confirmed {
		t.Fatalf("expected serving < waiting < started < ended, and waiting < confirmed < verified, got %d %d %d %d %d %d:\n%v", serving, waiting, started, ended, confirmed, verified, lines)
	}
	summary := summaryOf(t, lines)
	if summary["status"] != "confirmed" || len(summary["detail"].(map[string]any)["confirmed"].([]any)) != 1 {
		t.Fatalf("unexpected summary %v", summary)
	}
}

func TestWatchSchemaInSyncBeforePassEndSettlesWithoutIt(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick >= schemaLandTick {
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if err != nil {
		t.Fatalf("expected verified, got %v", err)
	}
	if summaryOf(t, lines)["status"] != "confirmed" || indexOf(lines, isPhase("verified")) < 0 {
		t.Fatalf("all tracked artifacts in sync is confirmation enough: %v", lines)
	}
}

func TestWatchSchemaArtifactDeploySkippedIsUnconfirmedExit7(t *testing.T) {
	// 15-09: Deploy completed its pass but skipped Automate artifacts new to
	// live, which stayed missing.
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick >= 5 {
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		automateAvailable: true,
		automationExists:  false,
		logs:              deployPassLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, true))
	if exitCode(err) != 7 || !strings.Contains(fmt.Sprint(err), "1 artifacts this deploy should have changed are still drifted or missing") {
		t.Fatalf("expected exit 7 naming the unconfirmed artifact, got %d %v", exitCode(err), err)
	}
	if indexOf(lines, isPhase("verified")) < 0 {
		t.Fatalf("the environment itself is verified: %v", phasesOf(lines))
	}
	detail := summaryOf(t, lines)["detail"].(map[string]any)
	unconfirmed := detail["unconfirmed"].([]any)
	if summaryOf(t, lines)["status"] != "unconfirmed" || len(unconfirmed) != 1 || unconfirmed[0].(map[string]any)["file"] != "umbraco-automate-automation__d.uda" || unconfirmed[0].(map[string]any)["status"] != "missing-remote" {
		t.Fatalf("unexpected summary %v", detail)
	}
	if len(detail["confirmed"].([]any)) != 1 {
		t.Fatalf("the data type should be confirmed: %v", detail)
	}
}

func TestWatchSchemaUnknownAutomateIsUnverifiableNeverInSyncOrFailed(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick >= schemaLandTick {
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		automateAvailable: false,
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, true))
	if err != nil {
		t.Fatalf("an unknown artifact must not fail the watch: %v", err)
	}
	baseline := lines[indexOf(lines, isPhase("baseline"))]["detail"].(map[string]any)["schema"].(map[string]any)
	unverifiable := baseline["unverifiable"].([]any)
	if len(unverifiable) != 1 || unverifiable[0].(map[string]any)["status"] != "unknown" || len(baseline["tracking"].([]any)) != 1 {
		t.Fatalf("Automate artifact should be unverifiable, not tracked: %v", baseline)
	}
	detail := summaryOf(t, lines)["detail"].(map[string]any)
	for _, bucket := range []string{"confirmed", "unconfirmed", "unknown"} {
		for _, item := range detail[bucket].([]any) {
			if item.(map[string]any)["file"] == "umbraco-automate-automation__d.uda" {
				t.Fatalf("the unverifiable artifact landed in %s: %v", bucket, detail)
			}
		}
	}
	if len(detail["unverifiable"].([]any)) != 1 {
		t.Fatalf("summary should list it as unverifiable: %v", detail)
	}
}

func TestWatchSchemaPassEndNeverSeenTimesOutNotVerified(t *testing.T) {
	// The only Work Status entry is from the old process (before the
	// restart), which must not count as this deploy's pass.
	oldProcess := `{"timestamp":"2026-10-01T10:00:01Z","level":"Information","renderedMessage":"Work Status \"Completed\".","properties":[{"name":"SourceContext","value":"Umbraco.Deploy.Infrastructure.Disk.DiskService"},{"name":"ProcessId","value":"111"},{"name":"MachineName","value":"m1"}]}`
	sc := schemaScenario{
		dataType: func(int) (int, string) { return http.StatusOK, driftedRemoteDataType },
		// Logged after baseline, before the restart lands at tick 3.
		logs: []fakeLogEntry{{2, oldProcess}},
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false), "--timeout", "150ms")
	if exitCode(err) != 6 || !strings.Contains(fmt.Sprint(err), "schema pass end not observed") {
		t.Fatalf("expected timeout naming the schema, got %d %v", exitCode(err), err)
	}
	if indexOf(lines, isPhase("verified")) >= 0 {
		t.Fatalf("verified without schema confirmation: %v", phasesOf(lines))
	}
	if indexOf(lines, func(l map[string]any) bool { return l["type"] == "schema-pass" && l["status"] == "ended" }) >= 0 {
		t.Fatalf("an old-process work status was taken as this deploy's pass")
	}
	pendingSummary := summaryOf(t, lines)
	pendingDetail := pendingSummary["detail"].(map[string]any)
	if pendingSummary["status"] != "pending" || pendingDetail["unconfirmed"] != nil || len(pendingDetail["pending"].([]any)) != 1 {
		t.Fatalf("summary must say pending and judge nothing: %v", pendingSummary)
	}
}

func TestWatchSchemaFailedWorkStatusExits7(t *testing.T) {
	sc := schemaScenario{
		dataType: func(int) (int, string) { return http.StatusOK, driftedRemoteDataType },
		logs:     deployPassLogs(4, 5, "Failed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if exitCode(err) != 7 || !strings.Contains(fmt.Sprint(err), `work status "Failed"`) {
		t.Fatalf("expected exit 7 naming the work status, got %d %v", exitCode(err), err)
	}
	if summaryOf(t, lines)["detail"].(map[string]any)["deployPass"].(map[string]any)["workStatus"] != "Failed" {
		t.Fatalf("summary should carry the work status: %v", summaryOf(t, lines))
	}
}

func TestWatchSchemaTransientUnknownAfterPassIsRetried(t *testing.T) {
	sc := schemaScenario{
		// One failed read right after the pass, then the real answer.
		dataType: func(tick int) (int, string) {
			switch {
			case tick == 5:
				return http.StatusInternalServerError, `{"title":"busy"}`
			case tick >= 6:
				return http.StatusOK, statusRemoteDataType
			}
			return http.StatusOK, driftedRemoteDataType
		},
		logs: deployPassLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if err != nil || summaryOf(t, lines)["status"] != "confirmed" {
		t.Fatalf("a transient read failure must be retried, got %v %v", err, summaryOf(t, lines))
	}
}

func TestWatchSchemaUnknownThenStillDriftedSettlesAsUnconfirmed(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick == 5 {
				return http.StatusInternalServerError, `{"title":"busy"}`
			}
			return http.StatusOK, driftedRemoteDataType
		},
		logs: deployPassLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if exitCode(err) != 7 || summaryOf(t, lines)["status"] != "unconfirmed" {
		t.Fatalf("a definitive answer after a failed read must settle (exit 7), got %d %v %v", exitCode(err), err, summaryOf(t, lines))
	}
}

func TestWatchSchemaPermanentUnknownAfterPassIsReportedNotFailed(t *testing.T) {
	sc := schemaScenario{
		dataType: func(tick int) (int, string) {
			if tick >= 5 {
				return http.StatusInternalServerError, `{"title":"broken"}`
			}
			return http.StatusOK, driftedRemoteDataType
		},
		logs: deployPassLogs(4, 5, "Completed"),
	}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if err != nil {
		t.Fatalf("unknown is not failure: %v", err)
	}
	summary := summaryOf(t, lines)
	unknown := summary["detail"].(map[string]any)["unknown"].([]any)
	if summary["status"] != "partly-unknown" || len(unknown) != 1 || unknown[0].(map[string]any)["baseline"] != "drifted" {
		t.Fatalf("expected the artifact reported as unknown: %v", summary)
	}
}

func TestWatchSchemaUsageErrorsBeforeAnyRequest(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request may be sent, got %s", req.URL)
		return nil, nil
	})
	for _, dir := range []string{"/nonexistent/uda", t.TempDir()} {
		_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--uda-dir", dir)
		if exitCode(err) != 1 {
			t.Fatalf("%s: expected a usage error, got %v", dir, err)
		}
	}
}

func TestWatchWithoutUdaDirIsUnchanged(t *testing.T) {
	lines, err := runWatch(t, greenDeploy())
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	for _, line := range lines {
		if strings.HasPrefix(fmt.Sprint(line["type"]), "schema") {
			t.Fatalf("schema output without --uda-dir: %v", line)
		}
	}
}

func TestWatchSchemaNothingTrackedSaysWhyThePassIsNotAwaited(t *testing.T) {
	// 08-10: every artifact in sync at baseline, and the new process
	// logged no schema pass.
	sc := schemaScenario{dataType: func(int) (int, string) { return http.StatusOK, statusRemoteDataType }}
	lines, err := runWatch(t, sc.env(), "--uda-dir", schemaDir(t, false))
	if err != nil {
		t.Fatalf("expected verified, got %v", err)
	}
	summary := summaryOf(t, lines)
	detail := summary["detail"].(map[string]any)
	pass := detail["deployPass"].(map[string]any)
	if summary["status"] != "nothing-to-confirm" || pass["started"] != false || pass["ended"] != false {
		t.Fatalf("unexpected summary %v", summary)
	}
	if !strings.Contains(fmt.Sprint(detail["reason"]), "no schema to confirm") {
		t.Fatalf("the summary should say why no pass was awaited: %v", detail)
	}
}
