package engage

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

const (
	engageTestTokenPath = "/umbraco/management/api/v1/security/back-office/token"
	engageTestGUID      = "6f0d2b1e-3c4a-4f5b-9a8e-1d2c3b4a5f60"
	engageUnavailable   = `{"type":"Error","title":"Umbraco Engage is unavailable","status":409,"detail":"Engage's database is not in the expected state, so this section cannot load."}`
)

// engageTestDeps serves the token route plus the given Engage routes (keyed
// by path below the Engage prefix) and records every Engage request URL.
func engageTestDeps(t *testing.T, routes map[string]func(req *http.Request) *http.Response) (cmdkit.Dependencies, *[]string) {
	t.Helper()
	requests := []string{}
	deps := cmdtest.ClientDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == engageTestTokenPath {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		requests = append(requests, req.Method+" "+req.URL.RequestURI())
		route := strings.TrimPrefix(req.URL.Path, engageAPIPrefix)
		if route == req.URL.Path {
			t.Fatalf("request outside the Engage prefix: %s", req.URL.Path)
		}
		if handler, ok := routes[route]; ok {
			return handler(req), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	})
	return deps, &requests
}

func engageJSON(status int, body string) func(req *http.Request) *http.Response {
	return func(req *http.Request) *http.Response { return cmdtest.JSONResponse(status, body) }
}

func TestEngageListUsesEngagePrefixAndProjectsBareArrays(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/segments/all": engageJSON(http.StatusOK, `[{"id":7,"unique":"`+engageTestGUID+`","name":"Returning visitors","rules":[]}]`),
	})
	output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "segment", "list", "--days", "14", "--fields", "unique,name")
	if err != nil {
		t.Fatalf("engage segment list failed: %v", err)
	}
	if len(*requests) != 1 || (*requests)[0] != "GET "+engageAPIPrefix+"/segments/all?amountOfDays=14" {
		t.Fatalf("expected one GET with only the set flag as a param, got %v", *requests)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("decode: %v\n%s", err, output)
	}
	if len(items) != 1 || len(items[0]) != 2 || items[0]["unique"] != engageTestGUID || items[0]["name"] != "Returning visitors" {
		t.Fatalf("expected projected array, got %+v", items)
	}
}

func TestEngageGetSendsGUIDAsQueryParam(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/persona/details":     engageJSON(http.StatusOK, `{"id":3,"unique":"`+engageTestGUID+`","name":"Persona A"}`),
		"/traffic-filter":      engageJSON(http.StatusOK, `{"id":4,"key":"`+engageTestGUID+`","name":"Office"}`),
		"/goal/details":        engageJSON(http.StatusOK, `{"id":5,"unique":"`+engageTestGUID+`","name":"Signup"}`),
		"/ab-test":             engageJSON(http.StatusOK, `{"id":12,"name":"Hero test"}`),
		"/ab-test-variant/all": engageJSON(http.StatusOK, `[]`),
	})
	for _, args := range [][]string{
		{"engage", "persona", "get", engageTestGUID},
		{"engage", "traffic-filter", "get", engageTestGUID},
		{"engage", "goal", "get", engageTestGUID},
		{"engage", "abtest", "get", "12"},
		{"engage", "abtest", "variants", "12"},
	} {
		if _, err := cmdtest.Execute(buildEngageRoot(t, deps), args...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}
	want := []string{
		"GET " + engageAPIPrefix + "/persona/details?id=" + engageTestGUID,
		"GET " + engageAPIPrefix + "/traffic-filter?key=" + engageTestGUID,
		"GET " + engageAPIPrefix + "/goal/details?id=" + engageTestGUID,
		"GET " + engageAPIPrefix + "/ab-test?id=12",
		"GET " + engageAPIPrefix + "/ab-test-variant/all?abTestId=12",
	}
	if strings.Join(*requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected requests:\n%s\nwant:\n%s", strings.Join(*requests, "\n"), strings.Join(want, "\n"))
	}
}

func TestEngageGetRejectsWrongIDKindWithoutRequest(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)

	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "persona", "get", "3")
	if err == nil || !strings.Contains(err.Error(), "GUID `unique`") || !strings.Contains(err.Error(), "umbraco engage persona list") {
		t.Fatalf("expected a GUID hint naming the list command, got %v", err)
	}
	_, err = cmdtest.Execute(buildEngageRoot(t, deps), "engage", "abtest", "get", engageTestGUID)
	if err == nil || !strings.Contains(err.Error(), "numeric `id`") {
		t.Fatalf("expected a numeric-id hint, got %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("id validation must fail before any request, got %v", *requests)
	}
}

func TestEngageUnavailableKeepsAPIErrorAndAddsHint(t *testing.T) {
	deps, _ := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/persona/all": engageJSON(http.StatusConflict, engageUnavailable),
	})
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "persona", "list")
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected the 409 APIError to survive (exit code 4), got %v", err)
	}
	if !strings.Contains(err.Error(), "schema alignment") || !strings.Contains(err.Error(), "umbraco engage status") {
		t.Fatalf("expected the unavailable hint in the error, got %v", err)
	}
}

func TestEngageMissingRouteHintsEngageNotInstalled(t *testing.T) {
	deps, _ := engageTestDeps(t, nil)
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "config")
	if err == nil || !strings.Contains(err.Error(), "Umbraco Engage may not be installed") {
		t.Fatalf("expected a not-installed hint on a bare 404, got %v", err)
	}
}

func engageStatusRoutes(probe func(req *http.Request) *http.Response) map[string]func(*http.Request) *http.Response {
	return map[string]func(*http.Request) *http.Response{
		"/package":          engageJSON(http.StatusOK, `{"version":"18.1.0","licenseStatus":"Valid","isPackageEnabled":true}`),
		"/main-switch":      engageJSON(http.StatusOK, `{"on":true}`),
		"/add-ons":          engageJSON(http.StatusOK, `{"umbracoForms":true,"umbracoCommerce":false}`),
		engageDataProbePath: probe,
	}
}

func TestEngageStatusReportsAvailability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		probe     func(*http.Request) *http.Response
		available bool
	}{
		{"available", engageJSON(http.StatusOK, `[]`), true},
		{"unavailable", engageJSON(http.StatusConflict, engageUnavailable), false},
	} {
		deps, _ := engageTestDeps(t, engageStatusRoutes(tc.probe))
		output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "status")
		if err != nil {
			t.Fatalf("%s: engage status failed: %v", tc.name, err)
		}
		var status map[string]any
		if err := json.Unmarshal([]byte(output), &status); err != nil {
			t.Fatalf("%s: decode: %v", tc.name, err)
		}
		if status["dataAvailable"] != tc.available {
			t.Fatalf("%s: dataAvailable = %v, want %v", tc.name, status["dataAvailable"], tc.available)
		}
		if status["package"].(map[string]any)["version"] != "18.1.0" || status["mainSwitch"].(map[string]any)["on"] != true {
			t.Fatalf("%s: expected package and main switch in status, got %+v", tc.name, status)
		}
		unavailable, hasUnavailable := status["unavailable"].(map[string]any)
		if tc.available == hasUnavailable {
			t.Fatalf("%s: unavailable block present = %v, want %v", tc.name, hasUnavailable, !tc.available)
		}
		if !tc.available && unavailable["title"] != "Umbraco Engage is unavailable" {
			t.Fatalf("%s: expected the server title, got %+v", tc.name, unavailable)
		}
	}
}

func TestEngageStatusReturnsOtherProbeFailures(t *testing.T) {
	deps, _ := engageTestDeps(t, engageStatusRoutes(engageJSON(http.StatusInternalServerError, `{"title":"boom"}`)))
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "status")
	if !api.IsStatus(err, http.StatusInternalServerError) {
		t.Fatalf("expected the 500 to be returned as an API error, got %v", err)
	}
}

func TestEngageAnalyticsQueryBuildsBackOfficeBody(t *testing.T) {
	var body map[string]any
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/analytics/query": func(req *http.Request) *http.Response {
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"columns":[],"rows":[],"currentPage":1,"rowsPerPage":20,"totalRows":0,"totalPages":0,"fromRow":0,"toRow":0,"reportsExist":true}`)
		},
	})
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "query",
		"--metrics", "Pageviews, SESSIONS", "--dimensions", "pagepath,nodeid",
		"--from", "2026-09-01", "--to", "2026-10-01T00:00:00Z",
		"--filter", "deviceCategory=='mobile'", "--node", engageTestGUID, "--include-subpages", "--culture", "en-US",
		"--page-size", "20")
	if err != nil {
		t.Fatalf("analytics query failed: %v", err)
	}
	if len(*requests) != 1 || !strings.HasPrefix((*requests)[0], "POST "+engageAPIPrefix+"/analytics/query") {
		t.Fatalf("expected one POST to /analytics/query, got %v", *requests)
	}
	want := map[string]any{
		"metrics":         []any{"pageviews", "sessions"},
		"dimensions":      []any{"pagePath", "NodeId"},
		"startDate":       "2026-09-01",
		"endDate":         "2026-10-01",
		"filter":          "deviceCategory=='mobile';NodeId=='" + engageTestGUID + "+';NodeCulture=='en-US'",
		"sort":            "pagePath",
		"page":            float64(1),
		"pageSize":        float64(20),
		"ascending":       false,
		"realtime":        false,
		"includeSubpages": true,
	}
	gotJSON, _ := json.Marshal(body)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("unexpected body:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestEngageAnalyticsQueryDefaultsToLast30InclusiveDays(t *testing.T) {
	for _, tc := range []struct {
		now        time.Time
		start, end string
	}{
		{time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC), "2026-09-02", "2026-10-01"},
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "2026-09-02", "2026-10-01"},
		{time.Date(2026, 10, 1, 23, 59, 59, 0, time.UTC), "2026-09-02", "2026-10-01"},
		// 01:00 at UTC+2 is still 30 September in UTC.
		{time.Date(2026, 10, 1, 1, 0, 0, 0, time.FixedZone("UTC+2", 2*3600)), "2026-09-01", "2026-09-30"},
		{time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC), "2024-02-01", "2024-03-01"},
	} {
		body, err := buildEngageAnalyticsQuery(engageAnalyticsQueryInput{Metrics: "pageviews", Page: 1, PageSize: 100}, tc.now)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if body["startDate"] != tc.start || body["endDate"] != tc.end {
			t.Fatalf("now %s: default range %v .. %v, want %s .. %s", tc.now, body["startDate"], body["endDate"], tc.start, tc.end)
		}
		start, _ := time.Parse(engageDayLayout, body["startDate"].(string))
		end, _ := time.Parse(engageDayLayout, body["endDate"].(string))
		if days := int(end.Sub(start).Hours()/24) + 1; days != 30 {
			t.Fatalf("now %s: default range covers %d inclusive days, want 30", tc.now, days)
		}
	}
	body, err := buildEngageAnalyticsQuery(engageAnalyticsQueryInput{Metrics: "pageviews", Page: 1, PageSize: 100}, time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, ok := body["sort"]; ok {
		t.Fatalf("no dimensions means no default sort, got %v", body["sort"])
	}
	if dims, ok := body["dimensions"].([]string); !ok || len(dims) != 0 {
		t.Fatalf("dimensions must be sent as an empty array, got %#v", body["dimensions"])
	}
}

func TestEngageAnalyticsQueryRejectsBadInputLocally(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--dimensions", "date"}, "missing required option: --metrics"},
		{[]string{"--metrics", " , ,"}, "missing required option: --metrics"},
		{[]string{"--metrics", "pageviews,clicks"}, "unknown name(s) clicks"},
		{[]string{"--metrics", "pageviews", "--from", "01-09-2026"}, "--from must be a day as YYYY-MM-DD"},
		{[]string{"--metrics", "pageviews", "--culture", "en-US"}, "--culture requires --node"},
		{[]string{"--metrics", "pageviews", "--page", "0"}, "--page is 1-based"},
		{[]string{"--json", `{"metrics":["pageviews"]}`, "--metrics", "sessions"}, "cannot be combined with --metrics"},
	} {
		_, err := cmdtest.Execute(buildEngageRoot(t, deps), append([]string{"engage", "analytics", "query"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: expected error containing %q, got %v", tc.args, tc.want, err)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("invalid input must not reach the API, got %v", *requests)
	}
}

func TestEngageAnalyticsQueryRefusesATimeOfDayEngageWouldDrop(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)
	for _, tc := range []struct {
		flag, value string
		want        []string
	}{
		{"--to", "2025-11-03T12:00:00Z", []string{`--to "2025-11-03T12:00:00Z" is not a whole day`, "both ends inclusive", "ignores the time of day", "YYYY-MM-DD"}},
		{"--from", "2025-11-03T23:59:59.5Z", []string{"is not a whole day"}},
		{"--to", "2025-11-03T23:30:00-05:00", []string{"is not a whole day", "UTC date (2025-11-04)"}},
		{"--from", "2025-11-03T00:00:00+02:00", []string{"is not a whole day", "UTC date (2025-11-02)"}},
		{"--to", "2025-11-02T19:00:00-05:00", []string{"is not a whole day", "UTC date (2025-11-03)"}},
	} {
		_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "query", "--metrics", "pageviews", "--from", "2025-11-01", tc.flag, tc.value)
		if err == nil {
			t.Fatalf("%s %s: expected a refusal", tc.flag, tc.value)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s %s: expected %q in %v", tc.flag, tc.value, want, err)
			}
		}
		if coder, ok := err.(interface{ ExitCode() int }); ok {
			t.Fatalf("%s %s: a refused value is a usage error (exit 1), got exit %d", tc.flag, tc.value, coder.ExitCode())
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("refused values must not reach the API, got %v", *requests)
	}
	for value, want := range map[string]string{
		"2025-11-03":                    "2025-11-03",
		"2025-11-03T00:00:00Z":          "2025-11-03",
		"2025-11-03T00:00:00.000+00:00": "2025-11-03",
	} {
		body, err := buildEngageAnalyticsQuery(engageAnalyticsQueryInput{Metrics: "pageviews", From: value, To: value, Page: 1, PageSize: 100}, time.Now())
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if body["startDate"] != want || body["endDate"] != want {
			t.Fatalf("%s: sent %v .. %v, want %s", value, body["startDate"], body["endDate"], want)
		}
	}
}

func TestEngageAnalyticsQueryServerErrorKeepsStatusAndAddsHint(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		toggle string
	}{
		{[]string{"--metrics", "pageviews", "--dimensions", "visitorType"}, "add --realtime"},
		{[]string{"--metrics", "totalEvents", "--realtime"}, "drop --realtime"},
		{[]string{"--json", `{"metrics":["totalEvents"],"dimensions":[],"realtime":true}`}, "drop --realtime"},
	} {
		deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
			"/analytics/query": engageJSON(http.StatusInternalServerError, `null`),
		})
		_, err := cmdtest.Execute(buildEngageRoot(t, deps), append([]string{"engage", "analytics", "query"}, tc.args...)...)
		var apiErr *api.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError || apiErr.Payload != nil {
			t.Fatalf("%v: expected the 500 APIError with its null body to survive, got %v", tc.args, err)
		}
		if coder, ok := err.(interface{ ExitCode() int }); !ok || coder.ExitCode() != 4 {
			t.Fatalf("%v: expected exit code 4, got %v", tc.args, err)
		}
		for _, want := range []string{
			"API 500 POST " + engageAPIPrefix + "/analytics/query: null. Hint: ",
			"Engage failed on this combination of metrics, dimensions and filter",
			"server error, not a validation result",
			"Try fewer dimensions, drop the filter, or " + tc.toggle,
			"'umbraco engage analytics query --help'",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%v: expected %q in %v", tc.args, want, err)
			}
		}
		if len(*requests) != 1 {
			t.Fatalf("%v: expected one request (no retry), got %v", tc.args, *requests)
		}
	}
}

func TestEngageAnalyticsQueryHintsOnlyServerErrors(t *testing.T) {
	deps, _ := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/analytics/query": engageJSON(http.StatusBadRequest, `{"title":"Bad Request","status":400}`),
	})
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "query", "--metrics", "pageviews")
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || apiErr.Hint != "" {
		t.Fatalf("expected the 400 without a hint, got %v", err)
	}
}

func TestEngageAnalyticsDistinctServerErrorKeepsStatusAndAddsHint(t *testing.T) {
	deps, _ := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/analytics/distinct": engageJSON(http.StatusInternalServerError, `null`),
	})
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "distinct", "--dimension", "visitorType")
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError || apiErr.Payload != nil {
		t.Fatalf("expected the 500 APIError with its null body to survive, got %v", err)
	}
	if coder, ok := err.(interface{ ExitCode() int }); !ok || coder.ExitCode() != 4 {
		t.Fatalf("expected exit code 4, got %v", err)
	}
	for _, want := range []string{
		"API 500 GET " + engageAPIPrefix + "/analytics/distinct?dimension=visitorType: null. Hint: ",
		"server error, not a validation result",
		"'umbraco engage analytics distinct --help'",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %v", want, err)
		}
	}
}

func TestEngageDateHelpDescribesEngageSemantics(t *testing.T) {
	help := func(args ...string) string {
		t.Helper()
		output, err := cmdtest.Execute(buildEngageRoot(t, cmdtest.MakeDeps()), append(args, "--help")...)
		if err != nil {
			t.Fatalf("%v --help failed: %v", args, err)
		}
		return strings.Join(strings.Fields(output), " ")
	}
	query := help("engage", "analytics", "query")
	for _, want := range []string{
		"--from and --to are whole days, both inclusive",
		"ignores any time of day",
		"--from 2026-10-01 --to 2026-10-01 is that one day",
		"accepted only at midnight UTC",
		"the last 30 days: today (UTC) and the 29 days before",
		"First day of the range, inclusive",
		"Last day of the range, inclusive",
		"Engage 18.1.0 answers some combinations with HTTP 500",
		"the visitorType and usertype dimensions",
		"With --realtime, the totalEvents metric",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("analytics query help lacks %q:\n%s", want, query)
		}
	}
	for _, stale := range []string{"reads as midnight", "pass the next day", "ending now"} {
		if strings.Contains(query, stale) {
			t.Fatalf("analytics query help still says %q", stale)
		}
	}
	if distinct := help("engage", "analytics", "distinct"); !strings.Contains(distinct, "Engage 18.1.0 answers HTTP 500 for the visitorType and usertype dimensions") {
		t.Fatalf("analytics distinct help lacks the known failures:\n%s", distinct)
	}
	annotation := help("engage", "annotation", "list")
	for _, want := range []string{
		"are instants, unlike the whole days of 'analytics query'",
		"--to 2026-09-30 leaves out annotations made on the 30th",
		"Engage 18.1.0 answers HTTP 500 unless both are given",
	} {
		if !strings.Contains(annotation, want) {
			t.Fatalf("annotation list help lacks %q:\n%s", want, annotation)
		}
	}
}

func TestEngageAnalyticsQueryDryRunPrintsPlanWithoutRequest(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)
	output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "query", "--json", `{"metrics":["pageviews"],"dimensions":[]}`, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("dry-run must not perform requests, got %v", *requests)
	}
	if !strings.Contains(output, engageAPIPrefix+"/analytics/query") || !strings.Contains(output, `"pageviews"`) {
		t.Fatalf("expected the planned POST with the verbatim body, got %s", output)
	}
}

func TestEngageAnalyticsDistinctCanonicalisesDimension(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/analytics/distinct": engageJSON(http.StatusOK, `["Denmark","Netherlands"]`),
	})
	if _, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "analytics", "distinct", "--dimension", "COUNTRY"); err != nil {
		t.Fatalf("distinct failed: %v", err)
	}
	if len(*requests) != 1 || (*requests)[0] != "GET "+engageAPIPrefix+"/analytics/distinct?dimension=country" {
		t.Fatalf("unexpected requests %v", *requests)
	}
}

func TestEngageAnnotationListRoutes(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/annotations/all":    engageJSON(http.StatusOK, `[]`),
		"/annotations/global": engageJSON(http.StatusOK, `[]`),
		"/annotations/page":   engageJSON(http.StatusOK, `[]`),
	})
	for _, args := range [][]string{
		{"--from", "2026-09-01"},
		{"--global"},
		{"--node", engageTestGUID, "--culture", "en-US"},
	} {
		if _, err := cmdtest.Execute(buildEngageRoot(t, deps), append([]string{"engage", "annotation", "list"}, args...)...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}
	want := []string{
		"GET " + engageAPIPrefix + "/annotations/all?from=2026-09-01",
		"GET " + engageAPIPrefix + "/annotations/global",
		"GET " + engageAPIPrefix + "/annotations/page?culture=en-US&unique=" + engageTestGUID,
	}
	if strings.Join(*requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected requests:\n%s", strings.Join(*requests, "\n"))
	}
	if _, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "annotation", "list", "--global", "--node", engageTestGUID); err == nil {
		t.Fatalf("--global with --node must be refused")
	}
}

func TestEngageNameCataloguesAreUniqueAndExcludeSentinel(t *testing.T) {
	for name, list := range map[string][]string{"metrics": engageMetrics, "dimensions": engageDimensions} {
		seen := map[string]bool{}
		for _, entry := range list {
			key := strings.ToLower(entry)
			if seen[key] || key == "undefined" {
				t.Fatalf("%s catalogue has a duplicate or sentinel entry %q", name, entry)
			}
			seen[key] = true
		}
	}
}
