package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

func healthAllDeps(t *testing.T, groups string, check func(path string) *http.Response) (*[]string, *cmdtest.RoundTripper) {
	t.Helper()
	var ran []string
	handler := cmdtest.RoundTripper(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == cmdtest.TokenPath:
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/health-check-group":
			return cmdtest.JSONResponse(http.StatusOK, groups), nil
		case strings.HasSuffix(req.URL.Path, "/check") && req.Method == http.MethodPost:
			ran = append(ran, req.URL.Path)
			return check(req.URL.Path), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	})
	return &ran, &handler
}

const twoGroups = `{"items":[{"name":"Configuration"},{"name":"Data Integrity"}],"total":2}`

func TestHealthRunAllRunsEveryGroupWithASummary(t *testing.T) {
	ran, handler := healthAllDeps(t, twoGroups, func(path string) *http.Response {
		if strings.Contains(path, "Configuration") {
			return cmdtest.JSONResponse(http.StatusOK, `{"checks":[{"id":"a","results":[{"resultType":"Success","message":"ok"},{"resultType":"Warning","message":"hm"}]}]}`)
		}
		return cmdtest.JSONResponse(http.StatusOK, `{"checks":[{"id":"b","results":[{"resultType":"Error","message":"bad"}]}]}`)
	})
	out, err := cmdtest.Execute(buildHealthRoot(cmdtest.Deps(*handler)), "health", "run", "--all")
	if err != nil {
		t.Fatalf("health run --all failed: %v", err)
	}
	if strings.Join(*ran, ",") != "/umbraco/management/api/v1/health-check-group/Configuration/check,/umbraco/management/api/v1/health-check-group/Data Integrity/check" {
		t.Fatalf("unexpected runs %v", *ran)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	summary := payload["summary"].(map[string]any)
	results := summary["results"].(map[string]any)
	if summary["groups"] != float64(2) || summary["ran"] != float64(2) || summary["failed"] != float64(0) ||
		results["Success"] != float64(1) || results["Warning"] != float64(1) || results["Error"] != float64(1) {
		t.Fatalf("unexpected summary %v", summary)
	}
	groups := payload["groups"].([]any)
	if groups[1].(map[string]any)["name"] != "Data Integrity" || len(groups[1].(map[string]any)["checks"].([]any)) != 1 {
		t.Fatalf("unexpected groups %v", groups)
	}
}

func TestHealthRunAllReportsAFailedGroupAndExits4(t *testing.T) {
	_, handler := healthAllDeps(t, twoGroups, func(path string) *http.Response {
		if strings.Contains(path, "Configuration") {
			return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"boom"}`)
		}
		return cmdtest.JSONResponse(http.StatusOK, `{"checks":[]}`)
	})
	out, err := cmdtest.Execute(buildHealthRoot(cmdtest.Deps(*handler)), "health", "run", "--all")
	coder, ok := err.(interface{ ExitCode() int })
	if !ok || coder.ExitCode() != 4 {
		t.Fatalf("expected exit 4, got %v", err)
	}
	var payload map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("results must still be printed: %v\n%s", jsonErr, out)
	}
	first := payload["groups"].([]any)[0].(map[string]any)
	if first["name"] != "Configuration" || !strings.Contains(first["error"].(string), "500") || payload["summary"].(map[string]any)["ran"] != float64(1) {
		t.Fatalf("the failed group must be reported in place: %v", payload)
	}
}

func TestHealthRunArgumentRules(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request on a usage error, got %s", req.URL)
		return nil, nil
	})
	for _, args := range [][]string{{"health", "run"}, {"health", "run", "Configuration", "--all"}, {"health", "run", "a", "b"}} {
		if _, err := cmdtest.Execute(buildHealthRoot(deps), args...); err == nil {
			t.Fatalf("%v: expected a usage error", args)
		}
	}
}
