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

func TestLogsSearchAppliesStrictClientSideIncidentFilters(t *testing.T) {
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{
				"items": [
					{
						"timestamp": "2026-06-23T11:38:51Z",
						"level": "Error",
						"renderedMessage": "Needle failure",
						"properties": [
							{"name":"SourceContext","value":"My.Project.Controllers.WidgetController"},
							{"name":"RequestPath","value":"/umbraco/backoffice/api/widget"},
							{"name":"CorrelationId","value":"corr-abc-123"}
						]
					},
					{
						"timestamp": "2026-06-23T13:41:09Z",
						"level": "Error",
						"renderedMessage": "Needle failure",
						"properties": [
							{"name":"SourceContext","value":"My.Project.Controllers.WidgetController"},
							{"name":"RequestPath","value":"/umbraco/backoffice/api/widget"},
							{"name":"CorrelationId","value":"corr-abc-123"}
						]
					},
					{
						"timestamp": "2026-06-23T11:39:00Z",
						"level": "Warning",
						"renderedMessage": "Needle failure",
						"properties": [
							{"name":"SourceContext","value":"Other.Source"},
							{"name":"RequestPath","value":"/umbraco/backoffice/api/widget"},
							{"name":"CorrelationId","value":"corr-abc-123"}
						]
					}
				],
				"total": 10
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"logs", "search",
		"--from", "2026-06-23T11:30:00Z",
		"--to", "2026-06-23T11:45:00Z",
		"--level", "Error",
		"--source-context", "WidgetController",
		"--path", "/widget",
		"--contains", "Needle",
		"--correlation-id", "abc-123",
		"--take", "3",
	)
	if err != nil {
		t.Fatalf("logs search failed: %v", err)
	}
	assertQueryValue(t, observedQuery, "startDate", "2026-06-23T11:30:00Z")
	assertQueryValue(t, observedQuery, "endDate", "2026-06-23T11:45:00Z")
	assertQueryValue(t, observedQuery, "logLevel", "Error")
	assertQueryValue(t, observedQuery, "take", "3")

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode logs search payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one strictly filtered item, got %+v", payload["items"])
	}
	item := items[0].(map[string]any)
	if item["timestamp"] != "2026-06-23T11:38:51Z" {
		t.Fatalf("expected only in-window timestamp, got %+v", item)
	}
	if payload["filteredOut"] != float64(2) || payload["serverReturned"] != float64(3) {
		t.Fatalf("expected filter metadata, got %+v", payload)
	}
	if payload["nextCursor"] != float64(3) || payload["hasMore"] != true {
		t.Fatalf("expected explicit pagination metadata, got %+v", payload)
	}
}

func TestLogsSearchFlatJSONAndRedaction(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			return endpointJSONResponse(http.StatusOK, `{
				"items": [
					{
						"timestamp": "2026-06-23T11:38:51Z",
						"level": "Information",
						"renderedMessage": "User jane@example.com sent Bearer abc.def.ghi",
						"exception": null,
						"properties": [
							{"name":"SourceContext","value":"OpenIddict.Server.OpenIddictServerDispatcher"},
							{"name":"RequestPath","value":"/umbraco/management/api/v1/security/back-office/token"},
							{"name":"ClientSecret","value":"plain-secret"},
							{"name":"Response","value":"{\"access_token\":\"secret-token\",\"client_secret\":\"secret-value\",\"email\":\"jane@example.com\"}"}
						]
					}
				],
				"total": 1
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "logs", "search", "--flat", "--redact-default")
	if err != nil {
		t.Fatalf("logs search failed: %v", err)
	}
	if strings.Contains(output, "jane@example.com") || strings.Contains(output, "secret-token") || strings.Contains(output, "plain-secret") || strings.Contains(output, "abc.def.ghi") {
		t.Fatalf("expected sensitive values to be redacted, got %s", output)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode flat logs payload: %v", err)
	}
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	if item["sourceContext"] != "OpenIddict.Server.OpenIddictServerDispatcher" {
		t.Fatalf("expected flat sourceContext, got %+v", item)
	}
	if item["requestPath"] != "/umbraco/management/api/v1/security/back-office/token" {
		t.Fatalf("expected flat requestPath, got %+v", item)
	}
	properties, ok := item["properties"].(map[string]any)
	if !ok || properties["SourceContext"] == nil {
		t.Fatalf("expected properties object in flat output, got %+v", item["properties"])
	}
}

func TestLogsSearchAroundCursorAndCountBy(t *testing.T) {
	var observedQuery = make(map[string][]string)

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/log-viewer/log":
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{
				"items": [
					{
						"timestamp": "2026-06-23T11:38:51Z",
						"level": "Warning",
						"properties": [
							{"name":"SourceContext","value":"Source.A"},
							{"name":"RequestPath","value":"/umbraco/a"}
						]
					},
					{
						"timestamp": "2026-06-23T11:40:00Z",
						"level": "Warning",
						"properties": [
							{"name":"SourceContext","value":"Source.A"},
							{"name":"RequestPath","value":"/umbraco/b"}
						]
					}
				],
				"total": 60
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"logs", "search",
		"--around", "2026-06-23T11:38:51Z",
		"--minutes", "5",
		"--cursor", "50",
		"--take", "2",
		"--count-by", "source",
	)
	if err != nil {
		t.Fatalf("logs search failed: %v", err)
	}
	assertQueryValue(t, observedQuery, "startDate", "2026-06-23T11:33:51Z")
	assertQueryValue(t, observedQuery, "endDate", "2026-06-23T11:43:51Z")
	assertQueryValue(t, observedQuery, "skip", "50")
	assertQueryValue(t, observedQuery, "take", "2")

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode count payload: %v", err)
	}
	if payload["countBy"] != "source" {
		t.Fatalf("expected source count, got %+v", payload)
	}
	counts := payload["counts"].([]any)
	first := counts[0].(map[string]any)
	if first["key"] != "Source.A" || first["count"] != float64(2) {
		t.Fatalf("expected grouped source count, got %+v", counts)
	}
	if payload["cursor"] != float64(50) || payload["nextCursor"] != float64(52) || payload["hasMore"] != true {
		t.Fatalf("expected terminal cursor metadata, got %+v", payload)
	}
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
