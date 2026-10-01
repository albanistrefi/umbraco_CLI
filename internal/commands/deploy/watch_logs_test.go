package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

// fakeDeployEnv scripts an environment through a deploy, one tick per
// unauthenticated token probe (the watch's first request each poll):
// tick 0 is the baseline, ticks in downTicks answer 503 everywhere, and
// from landTick on the process id changes. Log entries become visible at
// their tick.
type fakeDeployEnv struct {
	mu        sync.Mutex
	tick      int
	downTicks map[int]bool
	landTick  int
	// tailBroken makes every tail read fail, as if the log viewer were
	// unusable, while the single-entry landing probe keeps working.
	tailBroken bool
	entries    []fakeLogEntry
	tailReads  int
}

type fakeLogEntry struct {
	tick int
	json string
}

const fakeBaselineAt = "2026-10-01T10:00:00Z"

func logJSON(second int, level string, source string, message string, extra string) string {
	body := fmt.Sprintf(`"timestamp":"2026-10-01T10:00:%02dZ","level":%q,"renderedMessage":%q,"properties":[{"name":"SourceContext","value":%q},{"name":"ProcessId","value":"222"},{"name":"MachineName","value":"m1"}]`, second, level, message, source)
	if extra != "" {
		body += "," + extra
	}
	return "{" + body + "}"
}

func (e *fakeDeployEnv) newest(tick int) string {
	if tick >= e.landTick {
		return `{"timestamp":"2026-10-01T10:00:59Z","level":"Information","renderedMessage":"x","properties":[{"name":"ProcessId","value":"222"},{"name":"MachineName","value":"m1"}]}`
	}
	return `{"timestamp":"` + fakeBaselineAt + `","level":"Information","renderedMessage":"baseline","properties":[{"name":"ProcessId","value":"111"},{"name":"MachineName","value":"m1"}]}`
}

func (e *fakeDeployEnv) handler() cmdtest.RoundTripper {
	return func(req *http.Request) (*http.Response, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if req.URL.Path == cmdtest.TokenPath {
			body := ""
			if req.Body != nil {
				raw, _ := io.ReadAll(req.Body)
				body = string(raw)
			}
			if body == "" { // the watch's unauthenticated liveness probe
				e.tick++
				if e.downTicks[e.tick-1] {
					return cmdtest.JSONResponse(http.StatusServiceUnavailable, `{}`), nil
				}
				return cmdtest.JSONResponse(http.StatusBadRequest, `{}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		tick := e.tick - 1
		if e.downTicks[tick] {
			return cmdtest.JSONResponse(http.StatusServiceUnavailable, `{}`), nil
		}
		switch req.URL.Path {
		case "/":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("home"))}, nil
		case "/umbraco/management/api/v1/log-viewer/log":
			if req.URL.Query().Get("take") == "1" {
				return cmdtest.JSONResponse(http.StatusOK, `{"total":1,"items":[`+e.newest(tick)+`]}`), nil
			}
			e.tailReads++
			if e.tailBroken {
				return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"log viewer broke"}`), nil
			}
			visible := []string{e.newest(0)}
			for i := len(e.entries) - 1; i >= 0; i-- { // newest first, like the server
				if e.entries[i].tick <= tick {
					visible = append([]string{e.entries[i].json}, visible...)
				}
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"total":`+fmt.Sprint(len(visible))+`,"items":[`+strings.Join(visible, ",")+`]}`), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `{}`), nil
	}
}

// deployLogs is what Umbraco 18.2 logged during the 01-10 live deploy, plus
// the three known noise entries and one unrelated error.
func deployLogs(landTick int) []fakeLogEntry {
	return []fakeLogEntry{
		{landTick, logJSON(1, "Information", "Umbraco.Cms.Core.Runtime.MainDom", "Acquiring MainDom.", "")},
		{landTick, logJSON(2, "Information", "Umbraco.Cms.Infrastructure.Migrations.MigrationPlanExecutor", `Starting '"UmbracoForms"'...`, "")},
		{landTick, logJSON(3, "Error", "Microsoft.Extensions.Diagnostics.HealthChecks.DefaultHealthCheckService", `Health check "umbraco-ready" with status "Unhealthy" completed after 0.0033ms with message '"Umbraco is not yet ready. Level: Upgrading"'`, "")},
		{landTick, logJSON(4, "Information", "Umbraco.Cms.Infrastructure.Examine.DeliveryApiContentIndexPopulator", "The Delivery API is not enabled, no indexing will performed for the Delivery API content index.", "")},
		{landTick, logJSON(5, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkEnvironment", `Beginning deployment "264b90a9".`, "")},
		{landTick, logJSON(6, "Information", "Umbraco.Deploy.Infrastructure.Work.WorkItems.DiskReadWorkItem", "Suspend indexers.", "")},
		{landTick, logJSON(7, "Information", "object", "Resume indexers (rebuild:True).", "")},
		{landTick, logJSON(8, "Information", "object", "Resume scheduled publishing.", "")},
		{landTick, logJSON(9, "Error", "WorkflowCore.Services.BackgroundTasks.RunnablePoller", "Error executing workflow lock", `"exception":"Microsoft.Data.SqlClient.SqlException (0x80131904): Violation of PRIMARY KEY constraint 'PK_umbracoAutomateWorkflowLock'. Cannot insert duplicate key.\r\n   at Foo"`)},
		{landTick, logJSON(10, "Information", "OpenIddict.Server.OpenIddictServerDispatcher", "The token request was successfully validated.", "")},
		{landTick, logJSON(11, "Error", "UmbracoDotCom.Web.Services.PartnerMemberService", "Lookup failed for jane.doe@example.com", `"exception":"System.InvalidOperationException: boom\r\n   at Bar"`)},
		// A server that sends the text only as "message" (the field a
		// hand-built monitor missed on 01-10) is matched all the same.
		{landTick, `{"timestamp":"2026-10-01T10:00:12Z","level":"Information","message":"Released (\"environment\")","properties":[{"name":"SourceContext","value":"Umbraco.Cms.Core.Runtime.MainDom"}]}`},
	}
}

func runWatch(t *testing.T, env *fakeDeployEnv, args ...string) ([]map[string]any, error) {
	t.Helper()
	deps := cmdtest.Deps(env.handler())
	base := []string{"deploy", "watch", "--json", "--interval", "1ms", "--settle", "0", "--skip-index-verify", "--escalation", "1h", "--heartbeat", "0"}
	out, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), append(base, args...)...)
	lines := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var decoded map[string]any
		if jsonErr := json.Unmarshal([]byte(line), &decoded); jsonErr != nil {
			t.Fatalf("not NDJSON: %q (%v)", line, jsonErr)
		}
		lines = append(lines, decoded)
	}
	return lines, err
}

func phasesOf(lines []map[string]any) []string {
	phases := make([]string, 0)
	for _, line := range lines {
		if line["type"] == "phase" {
			phases = append(phases, fmt.Sprint(line["phase"]))
		}
	}
	return phases
}

func ofType(lines []map[string]any, kind string) []map[string]any {
	result := make([]map[string]any, 0)
	for _, line := range lines {
		if line["type"] == kind {
			result = append(result, line)
		}
	}
	return result
}

func greenDeploy() *fakeDeployEnv {
	return &fakeDeployEnv{downTicks: map[int]bool{1: true, 2: true}, landTick: 3, entries: deployLogs(3)}
}

func TestWatchLogsEmitsDeployEntriesTypedApartFromPhases(t *testing.T) {
	lines, err := runWatch(t, greenDeploy(), "--logs")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}

	for _, line := range lines {
		switch line["type"] {
		case "phase":
		case "log", "log-monitor":
			if _, has := line["phase"]; has {
				t.Fatalf("a %s line carries a phase key: %v", line["type"], line)
			}
		default:
			t.Fatalf("line without a known type: %v", line)
		}
	}

	type want struct{ category, message string }
	expected := []want{
		{"lifecycle", "Acquiring MainDom."},
		{"migration", `Starting '"UmbracoForms"'...`},
		{"deploy", `Beginning deployment "264b90a9".`},
		{"indexer", "Suspend indexers."},
		{"indexer", "Resume indexers (rebuild:True)."},
		{"deploy", "Resume scheduled publishing."},
		{"level", "Lookup failed for [redacted-email]"},
		{"lifecycle", `Released ("environment")`},
	}
	logs := ofType(lines, "log")
	if len(logs) != len(expected) {
		t.Fatalf("expected %d log events, got %d: %v", len(expected), len(logs), logs)
	}
	for i, w := range expected {
		if logs[i]["category"] != w.category || logs[i]["message"] != w.message {
			t.Fatalf("log event %d: want %s %q, got %v", i, w.category, w.message, logs[i])
		}
	}
	if logs[6]["exception"] != "System.InvalidOperationException: boom" || logs[6]["level"] != "Error" {
		t.Fatalf("error entry should carry level and the exception's first line: %v", logs[6])
	}

	monitor := ofType(lines, "log-monitor")
	statuses := make([]string, 0, len(monitor))
	for _, event := range monitor {
		statuses = append(statuses, fmt.Sprint(event["status"]))
	}
	if strings.Join(statuses, ",") != "unavailable,resumed,stopped" {
		t.Fatalf("unexpected monitor statuses %v", statuses)
	}
	last := lines[len(lines)-1]
	if last["type"] != "log-monitor" || last["status"] != "stopped" {
		t.Fatalf("the monitor must stop with the watch, last line %v", last)
	}
	detail := last["detail"].(map[string]any)
	excluded := detail["excluded"].(map[string]any)
	for _, name := range []string{"delivery-api-disabled", "ready-probe-upgrading", "automate-workflow-lock-pk"} {
		if excluded[name] != float64(1) {
			t.Fatalf("expected %s excluded once, got %v", name, excluded)
		}
	}
	if detail["phase"] != "verified" || detail["read"] != float64(12) {
		t.Fatalf("unexpected stopped detail %v", detail)
	}
	// Baseline entries are history: the one at the baseline instant is not replayed.
	for _, event := range logs {
		if event["message"] == "baseline" {
			t.Fatalf("baseline entry replayed: %v", event)
		}
	}
}

func TestWatchLogsNeverChangePhasesOrExitCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  func() *fakeDeployEnv
		args []string
		exit int
	}{
		{name: "verified", env: greenDeploy, exit: 0},
		{name: "verified with a broken log viewer", env: func() *fakeDeployEnv {
			env := greenDeploy()
			env.tailBroken = true
			return env
		}, exit: 0},
		{name: "timeout", env: func() *fakeDeployEnv {
			return &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, entries: deployLogs(1)}
		}, args: []string{"--timeout", "40ms"}, exit: 6},
		{name: "failed", env: func() *fakeDeployEnv {
			down := map[int]bool{}
			for i := 1; i < 100000; i++ {
				down[i] = true
			}
			return &fakeDeployEnv{downTicks: down, landTick: 1 << 30, entries: deployLogs(1)}
		}, args: []string{"--escalation", "1ms"}, exit: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			without, errWithout := runWatch(t, tc.env(), tc.args...)
			with, errWith := runWatch(t, tc.env(), append([]string{"--logs"}, tc.args...)...)
			if exitCode(errWithout) != tc.exit || exitCode(errWith) != tc.exit {
				t.Fatalf("exit codes: without --logs %d (%v), with %d (%v), want %d", exitCode(errWithout), errWithout, exitCode(errWith), errWith, tc.exit)
			}
			if strings.Join(phasesOf(without), ",") != strings.Join(phasesOf(with), ",") {
				t.Fatalf("phases differ:\n without %v\n with    %v", phasesOf(without), phasesOf(with))
			}
			last := with[len(with)-1]
			if last["type"] != "log-monitor" || last["status"] != "stopped" {
				t.Fatalf("monitor did not stop with the watch: %v", last)
			}
			if len(ofType(without, "log")) != 0 || len(ofType(without, "log-monitor")) != 0 {
				t.Fatalf("log events without --logs")
			}
		})
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if coder, ok := err.(interface{ ExitCode() int }); ok {
		return coder.ExitCode()
	}
	return 1
}

func TestWatchLogsBrokenViewerIsReportedOnce(t *testing.T) {
	env := greenDeploy()
	env.tailBroken = true
	lines, err := runWatch(t, env, "--logs")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	unavailable := 0
	for _, event := range ofType(lines, "log-monitor") {
		if event["status"] == "unavailable" {
			unavailable++
		}
	}
	if unavailable != 1 {
		t.Fatalf("expected one unavailable event, got %d: %v", unavailable, lines)
	}
	if env.tailReads < 2 {
		t.Fatalf("the monitor should keep trying after a failure, read %d times", env.tailReads)
	}
}

func TestWatchLogsBurstBeyondThePageCapIsAReportedGap(t *testing.T) {
	env := greenDeploy()
	env.entries = nil
	for i := 0; i < 600; i++ {
		ts := fmt.Sprintf("2026-10-01T10:%02d:%02dZ", 1+i/60, i%60)
		env.entries = append(env.entries, fakeLogEntry{env.landTick, `{"timestamp":"` + ts + `","level":"Error","renderedMessage":"burst","properties":[]}`})
	}
	lines, err := runWatch(t, env, "--logs")
	if err != nil {
		t.Fatalf("a log burst must not fail the watch: %v", err)
	}
	var gap map[string]any
	for _, event := range ofType(lines, "log-monitor") {
		if event["status"] == "gap" {
			gap = event
		}
	}
	if gap == nil {
		t.Fatalf("expected a gap event, got %v", ofType(lines, "log-monitor"))
	}
	detail := gap["detail"].(map[string]any)
	if detail["from"] == nil || detail["to"] == nil || !strings.Contains(fmt.Sprint(detail["reason"]), "were not read") {
		t.Fatalf("gap event must say what was skipped: %v", gap)
	}
	if phases := phasesOf(lines); phases[len(phases)-1] != "verified" {
		t.Fatalf("phases changed by the gap: %v", phases)
	}
}

func TestWatchLogsExclusionsAreExtendableAndOverridable(t *testing.T) {
	env := greenDeploy()
	lines, err := runWatch(t, env, "--logs", "--logs-level", "Information", "--logs-keep", "delivery-api-disabled", "--logs-exclude", "openiddict", "--logs-match", "zzz-never")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	messages := map[string]string{}
	for _, event := range ofType(lines, "log") {
		messages[fmt.Sprint(event["message"])] = fmt.Sprint(event["category"])
	}
	if messages["The Delivery API is not enabled, no indexing will performed for the Delivery API content index."] != "level" {
		t.Fatalf("--logs-keep delivery-api-disabled with --logs-level Information should emit it: %v", messages)
	}
	if _, has := messages["The token request was successfully validated."]; has {
		t.Fatalf("--logs-exclude openiddict should drop the OpenIddict entry")
	}
	stopped := lines[len(lines)-1]["detail"].(map[string]any)["excluded"].(map[string]any)
	if stopped["custom:openiddict"] != float64(1) || stopped["delivery-api-disabled"] != nil || stopped["ready-probe-upgrading"] != float64(1) {
		t.Fatalf("unexpected exclusion counts %v", stopped)
	}

	// --logs-keep all with the default level re-admits the two Error-level noise entries.
	lines, err = runWatch(t, greenDeploy(), "--logs", "--logs-keep", "all")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	levels := 0
	for _, event := range ofType(lines, "log") {
		if event["category"] == "level" {
			levels++
		}
	}
	if levels != 3 {
		t.Fatalf("expected the two noise errors plus the real one, got %d level events", levels)
	}
}

func TestWatchLogsMatchAddsEntries(t *testing.T) {
	lines, err := runWatch(t, greenDeploy(), "--logs", "--logs-match", "TOKEN REQUEST")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	found := false
	for _, event := range ofType(lines, "log") {
		if event["category"] == "match" && event["message"] == "The token request was successfully validated." {
			found = true
		}
	}
	if !found {
		t.Fatalf("--logs-match should emit the matching entry: %v", ofType(lines, "log"))
	}
}

func TestWatchLogsFlagValidation(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request may be sent on a usage error, got %s", req.URL)
		return nil, nil
	})
	for _, args := range [][]string{
		{"--logs-match", "x"},
		{"--logs-exclude", "x"},
		{"--logs-keep", "all"},
		{"--logs-level", "Warning"},
		{"--logs", "--logs-keep", "bogus"},
		{"--logs", "--logs-level", "Loud"},
	} {
		_, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), append([]string{"deploy", "watch"}, args...)...)
		if err == nil || exitCode(err) != 1 {
			t.Fatalf("%v: expected a usage error, got %v", args, err)
		}
	}
}

func TestWatchTextOutputKeepsPhaseLinesAndAddsLogLines(t *testing.T) {
	env := greenDeploy()
	deps := cmdtest.Deps(env.handler())
	out, err := cmdtest.Execute(cmdtest.BuildRoot(t, deps, Register), "deploy", "watch", "--interval", "1ms", "--settle", "0", "--skip-index-verify", "--heartbeat", "0", "--logs")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	if !strings.Contains(out, " log [lifecycle] Information Umbraco.Cms.Core.Runtime.MainDom: Acquiring MainDom.\n") {
		t.Fatalf("missing text log line:\n%s", out)
	}
	if !strings.Contains(out, " verified — ") || strings.Contains(out, "type=") {
		t.Fatalf("phase lines changed:\n%s", out)
	}
}
