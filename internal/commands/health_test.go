package commands

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

func buildHealthRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterHealth(root, deps)
	return root
}

func TestHealthGroupsListsGroups(t *testing.T) {
	var requestedPath string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"name":"Configuration"}],"total":1}`), nil
		}
	})

	out, err := cmdtest.Execute(buildHealthRoot(deps), "health", "groups")
	if err != nil {
		t.Fatalf("health groups failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/health-check-group" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(out, "Configuration") {
		t.Fatalf("expected group in output, got %s", out)
	}
}

func TestHealthGroupEscapesNameSegment(t *testing.T) {
	var requestedURI string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"Data Integrity","checks":[]}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "group", "Data Integrity"); err != nil {
		t.Fatalf("health group failed: %v", err)
	}
	if requestedURI != "/umbraco/management/api/v1/health-check-group/Data%20Integrity" {
		t.Fatalf("expected escaped group name in path, got %q", requestedURI)
	}
}

func TestHealthRunPostsCheckEndpoint(t *testing.T) {
	var requestedPath, requestedMethod string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			requestedMethod = req.Method
			return cmdtest.JSONResponse(http.StatusOK, `{"checks":[{"name":"Macro errors","results":[]}]}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "run", "Configuration"); err != nil {
		t.Fatalf("health run failed: %v", err)
	}
	if requestedMethod != http.MethodPost || requestedPath != "/umbraco/management/api/v1/health-check-group/Configuration/check" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
}

func TestHealthRunFallsBackToLegacyRunEndpoint(t *testing.T) {
	var legacyPath, legacyMethod string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/health-check-group/Configuration/check":
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		default:
			legacyPath = req.URL.Path
			legacyMethod = req.Method
			return cmdtest.JSONResponse(http.StatusOK, `{"checks":[{"name":"Macro errors","results":[]}]}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "run", "Configuration"); err != nil {
		t.Fatalf("health run failed: %v", err)
	}
	if legacyMethod != http.MethodGet || legacyPath != "/umbraco/management/api/v1/health-check-group/Configuration/run" {
		t.Fatalf("expected legacy fallback GET .../run, got %s %s", legacyMethod, legacyPath)
	}
}

func TestHealthActionPostsPayload(t *testing.T) {
	var requestedPath, requestedMethod, requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			requestedMethod = req.Method
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `{"success":true}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "action", "check-1", "--json", `{"alias":"fixConfig"}`); err != nil {
		t.Fatalf("health action failed: %v", err)
	}
	if requestedMethod != http.MethodPost || requestedPath != "/umbraco/management/api/v1/health-check/execute-action" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(requestedBody, `"fixConfig"`) {
		t.Fatalf("expected payload forwarded, got %q", requestedBody)
	}
	if !strings.Contains(requestedBody, `"healthCheck":{"id":"check-1"}`) {
		t.Fatalf("expected healthCheck reference injected from argument, got %q", requestedBody)
	}
	if !strings.Contains(requestedBody, `"valueRequired":false`) {
		t.Fatalf("expected valueRequired default, got %q", requestedBody)
	}
}

func TestHealthActionKeepsExplicitHealthCheckReference(t *testing.T) {
	var requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `{"success":true}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "action", "check-1", "--json", `{"healthCheck":{"id":"explicit"},"valueRequired":true}`); err != nil {
		t.Fatalf("health action failed: %v", err)
	}
	if !strings.Contains(requestedBody, `"healthCheck":{"id":"explicit"}`) {
		t.Fatalf("expected explicit healthCheck reference preserved, got %q", requestedBody)
	}
	if !strings.Contains(requestedBody, `"valueRequired":true`) {
		t.Fatalf("expected explicit valueRequired preserved, got %q", requestedBody)
	}
}

func TestHealthActionFallsBackToLegacyActionEndpoint(t *testing.T) {
	var legacyPath, legacyMethod, legacyBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/health-check/execute-action":
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		default:
			legacyPath = req.URL.Path
			legacyMethod = req.Method
			body, _ := io.ReadAll(req.Body)
			legacyBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `{"success":true}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "action", "action-1", "--json", `{"alias":"fixConfig"}`); err != nil {
		t.Fatalf("health action failed: %v", err)
	}
	if legacyMethod != http.MethodPost || legacyPath != "/umbraco/management/api/v1/health-check/action-1" {
		t.Fatalf("expected legacy fallback POST /health-check/{id}, got %s %s", legacyMethod, legacyPath)
	}
	if strings.Contains(legacyBody, "healthCheck") {
		t.Fatalf("expected legacy body without injected healthCheck reference, got %q", legacyBody)
	}
}

func TestHealthActionDryRunSkipsRequest(t *testing.T) {
	requests := 0
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		requests++
		return cmdtest.JSONResponse(http.StatusOK, `{}`), nil
	})

	out, err := cmdtest.Execute(buildHealthRoot(deps), "health", "action", "action-1", "--dry-run")
	if err != nil {
		t.Fatalf("health action dry-run failed: %v", err)
	}
	if requests != 0 {
		t.Fatalf("dry-run must not hit the API, saw %d requests", requests)
	}
	if !strings.Contains(out, `"dryRun": true`) && !strings.Contains(out, `"dryRun":true`) {
		t.Fatalf("expected dry-run preview in output, got %s", out)
	}
}

func TestHealthActionRejectsInvalidJSON(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for invalid payload")
		return nil, nil
	})

	if _, err := cmdtest.Execute(buildHealthRoot(deps), "health", "action", "action-1", "--json", "{not-json"); err == nil {
		t.Fatalf("expected error for invalid JSON payload")
	}
}
