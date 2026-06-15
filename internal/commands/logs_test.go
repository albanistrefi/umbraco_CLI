package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLogsListUsesV17LogEndpointWithQueryFlags(t *testing.T) {
	var observedPath string
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			observedPath = req.URL.Path
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"level":"Error"}],"total":1}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"logs", "list",
		"--from", "2026-05-11T10:00:00Z",
		"--to", "2026-05-11T10:30:00Z",
		"--level", "Error",
		"--filter-expression", "@Message like '%panic%'",
		"--skip", "5",
		"--take", "25",
	)
	if err != nil {
		t.Fatalf("logs list failed: %v", err)
	}

	if observedPath != "/umbraco/management/api/v1/log-viewer/log" {
		t.Fatalf("expected v17 log endpoint, got %q", observedPath)
	}
	assertQueryValue(t, observedQuery, "startDate", "2026-05-11T10:00:00Z")
	assertQueryValue(t, observedQuery, "endDate", "2026-05-11T10:30:00Z")
	assertQueryValue(t, observedQuery, "filterExpression", "@Message like '%panic%'")
	assertQueryValue(t, observedQuery, "logLevel", "Error")
	assertQueryValue(t, observedQuery, "skip", "5")
	assertQueryValue(t, observedQuery, "take", "25")

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode logs list payload: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected log items payload, got %+v", payload)
	}
}

func TestLogsSearchCombinesFlagsWithParamsBlob(t *testing.T) {
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"level":"Error"}],"total":1}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	// The reported repro: a paging --params blob alongside a --level flag must
	// forward both, not drop the level and return newest-N unfiltered.
	_, err := execute(buildRootWithCollections(t, deps),
		"logs", "search", "--level", "Error", "--params", `{"take":30}`)
	if err != nil {
		t.Fatalf("logs search failed: %v", err)
	}
	assertQueryValue(t, observedQuery, "logLevel", "Error")
	assertQueryValue(t, observedQuery, "take", "30")
}

func TestLogsSearchFlagsOverrideParamsBlobOnConflict(t *testing.T) {
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps),
		"logs", "search", "--filter-expression", "@l = 'Error'", "--params", `{"filterExpression":"ignored","take":200}`)
	if err != nil {
		t.Fatalf("logs search failed: %v", err)
	}
	assertQueryValue(t, observedQuery, "filterExpression", "@l = 'Error'")
	assertQueryValue(t, observedQuery, "take", "200")
}

func TestLogsListFallsBackToLegacyEndpointOnNotFound(t *testing.T) {
	var requests []string
	var legacyQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			requests = append(requests, req.URL.Path)
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/log-viewer":
			requests = append(requests, req.URL.Path)
			legacyQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"level":"Warning"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "list", "--params", `{"level":"Warning","from":"2026-05-11T09:00:00Z","take":10}`)
	if err != nil {
		t.Fatalf("logs list fallback failed: %v", err)
	}

	expected := []string{"/umbraco/management/api/v1/log-viewer/log", "/umbraco/management/api/v1/log-viewer"}
	if strings.Join(requests, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected fallback request order: %+v", requests)
	}
	assertQueryValue(t, legacyQuery, "logLevel", "Warning")
	assertQueryValue(t, legacyQuery, "startDate", "2026-05-11T09:00:00Z")
	assertQueryValue(t, legacyQuery, "take", "10")
}

func TestLogsSearchUsesV17LogEndpointAndFallsBackToLegacySearch(t *testing.T) {
	var requests []string
	var searchQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			requests = append(requests, req.URL.Path)
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/log-viewer/search":
			requests = append(requests, req.URL.Path)
			searchQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"level":"Information"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "search", "--filter-expression", "StartsWith(SourceContext, 'Umbraco')", "--skip", "2")
	if err != nil {
		t.Fatalf("logs search fallback failed: %v", err)
	}

	expected := []string{"/umbraco/management/api/v1/log-viewer/log", "/umbraco/management/api/v1/log-viewer/search"}
	if strings.Join(requests, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected fallback request order: %+v", requests)
	}
	assertQueryValue(t, searchQuery, "filterExpression", "StartsWith(SourceContext, 'Umbraco')")
	assertQueryValue(t, searchQuery, "skip", "2")
}

func TestLogsTemplatesUsesV17MessageTemplateEndpointWithQueryFlags(t *testing.T) {
	var observedPath string
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/message-template":
			observedPath = req.URL.Path
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"messageTemplate":"Template 6"},{"messageTemplate":"Template 7"}],"total":10}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"logs", "templates",
		"--from", "2026-05-01",
		"--to", "2026-05-12",
		"--skip", "5",
		"--take", "5",
	)
	if err != nil {
		t.Fatalf("logs templates failed: %v", err)
	}

	if observedPath != "/umbraco/management/api/v1/log-viewer/message-template" {
		t.Fatalf("expected v17 message-template endpoint, got %q", observedPath)
	}
	assertQueryValue(t, observedQuery, "startDate", "2026-05-01")
	assertQueryValue(t, observedQuery, "endDate", "2026-05-12")
	assertQueryValue(t, observedQuery, "skip", "5")
	assertQueryValue(t, observedQuery, "take", "5")

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode logs templates payload: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected paged message template items, got %+v", payload)
	}
}

func TestLogsTemplatesFallsBackToLegacyEndpointOnNotFound(t *testing.T) {
	var requests []string
	var legacyQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/message-template":
			requests = append(requests, req.URL.Path)
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/log-viewer/templates":
			requests = append(requests, req.URL.Path)
			legacyQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"messageTemplate":"Legacy"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "templates", "--take", "5")
	if err != nil {
		t.Fatalf("logs templates fallback failed: %v", err)
	}

	expected := []string{"/umbraco/management/api/v1/log-viewer/message-template", "/umbraco/management/api/v1/log-viewer/templates"}
	if strings.Join(requests, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected fallback request order: %+v", requests)
	}
	assertQueryValue(t, legacyQuery, "take", "5")
}

func TestLogsTemplatesFallbackStopsOnNonNotFoundAPIError(t *testing.T) {
	var legacyCalled bool

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/message-template":
			return endpointJSONResponse(http.StatusInternalServerError, `{"title":"boom"}`), nil
		case "/umbraco/management/api/v1/log-viewer/templates":
			legacyCalled = true
			return endpointJSONResponse(http.StatusOK, `{"items":[]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "templates")
	if err == nil {
		t.Fatalf("expected logs templates to fail on non-404 primary error")
	}
	if legacyCalled {
		t.Fatalf("fallback should not run after non-404 API error")
	}
}

func TestLogsTemplatesOutputFormatsRender(t *testing.T) {
	for _, format := range []string{"json", "table", "plain"} {
		format := format
		t.Run(format, func(t *testing.T) {
			outputFormat := format
			deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/umbraco/management/api/v1/security/back-office/token":
					return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
				case "/umbraco/management/api/v1/log-viewer/message-template":
					return endpointJSONResponse(http.StatusOK, `{"items":[{"messageTemplate":"Template"}],"total":1}`), nil
				default:
					return endpointJSONResponse(http.StatusNotFound, `null`), nil
				}
			})
			deps.OutputFlag = &outputFormat

			output, err := execute(buildRootWithCollections(t, deps), "logs", "templates", "--take", "1")
			if err != nil {
				t.Fatalf("logs templates --output %s failed: %v", format, err)
			}
			if strings.TrimSpace(output) == "" {
				t.Fatalf("expected %s output to render response", format)
			}
			if format == "json" {
				var payload map[string]any
				if err := json.Unmarshal([]byte(output), &payload); err != nil {
					t.Fatalf("failed to decode json output: %v", err)
				}
			}
		})
	}
}

func TestLogsFallbackStopsOnNonNotFoundAPIError(t *testing.T) {
	var legacyCalled bool

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			return endpointJSONResponse(http.StatusInternalServerError, `{"title":"boom"}`), nil
		case "/umbraco/management/api/v1/log-viewer":
			legacyCalled = true
			return endpointJSONResponse(http.StatusOK, `{"items":[]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "list")
	if err == nil {
		t.Fatalf("expected logs list to fail on non-404 primary error")
	}
	if legacyCalled {
		t.Fatalf("fallback should not run after non-404 API error")
	}
}

func TestLogsLevelCountExplainsLargeWindowGuard(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/level-count":
			return endpointJSONResponse(http.StatusBadRequest, `{"operationStatus":"CancelledByLogsSizeValidation"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "logs", "level-count", "--from", "2026-05-01T00:00:00Z", "--to", "2026-05-11T00:00:00Z")
	if err == nil {
		t.Fatalf("expected logs level-count to fail")
	}
	if !strings.Contains(err.Error(), "time range is too large") || !strings.Contains(err.Error(), "narrower --from/--to") {
		t.Fatalf("expected friendly log size guard message, got %v", err)
	}
}

func TestLogsLevelsIsHiddenAndUnsupported(t *testing.T) {
	root := buildRootWithCollections(t, makeDeps())
	logs := findChildCommand(root, "logs")
	if logs == nil {
		t.Fatal("missing logs command")
	}
	levels := findChildCommand(logs, "levels")
	if levels == nil {
		t.Fatal("missing hidden logs levels compatibility command")
	}
	if !levels.Hidden {
		t.Fatal("logs levels should be hidden because Umbraco v17 has no levels endpoint")
	}

	_, err := execute(root, "logs", "levels")
	if err == nil || !strings.Contains(err.Error(), "not available in the Umbraco v17 Management API") {
		t.Fatalf("expected unsupported logs levels error, got %v", err)
	}
}

func assertQueryValue(t *testing.T, values map[string][]string, key string, expected string) {
	t.Helper()
	actual := values[key]
	if len(actual) != 1 || actual[0] != expected {
		t.Fatalf("expected query %s=%q, got %+v", key, expected, actual)
	}
}
