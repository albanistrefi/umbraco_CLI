package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

type datatypeRoundTripper func(*http.Request) (*http.Response, error)

func (fn datatypeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func datatypeJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func datatypeDeps(handler datatypeRoundTripper) Dependencies {
	cfg := config.Config{
		BaseURL:      "https://example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	httpClient := &http.Client{Transport: handler}
	output := "json"

	return Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

func TestDatatypeListUsesFilterEndpointWithPagination(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"dt-1","name":"Article Grid"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "list", "--skip", "5", "--take", "20")
	if err != nil {
		t.Fatalf("datatype list failed: %v", err)
	}

	if !strings.Contains(observedPath, "skip=5") || !strings.Contains(observedPath, "take=20") {
		t.Fatalf("expected pagination params on filter endpoint, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype list payload: %v", err)
	}
	if payload["total"] != float64(1) {
		t.Fatalf("unexpected datatype list payload: %+v", payload)
	}
}

func TestDatatypeListFieldsProjectClientSideWithoutQueryParam(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			observedPath = req.URL.String()
			if req.URL.Query().Get("fields") != "" {
				return datatypeJSONResponse(http.StatusNotFound, `null`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"dt-1","name":"Textstring","alias":"textstring","editorAlias":"Umbraco.TextBox"},
				{"id":"dt-2","name":"Rich Text","alias":"richText","editorAlias":"Umbraco.RichText"}
			]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "list", "--first-n", "1", "--fields", "id,name")
	if err != nil {
		t.Fatalf("datatype list --fields failed: %v", err)
	}
	if strings.Contains(observedPath, "fields=") {
		t.Fatalf("expected --fields to stay client-side, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype list payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected --first-n to keep one item, got %+v", payload)
	}
	item := items[0].(map[string]any)
	if len(item) != 2 || item["id"] != "dt-1" || item["name"] != "Textstring" {
		t.Fatalf("expected projected datatype item, got %+v", item)
	}
}

func TestDatatypeSearchFallsBackToFilterEndpointWhenItemSearchIsMissing(t *testing.T) {
	var itemSearchRequests int
	var observedFilterPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/data-type/search":
			itemSearchRequests++
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			observedFilterPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"dt-1","name":"Google Docs"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "search", "--query", "google", "--skip", "2", "--take", "15")
	if err != nil {
		t.Fatalf("datatype search failed: %v", err)
	}

	if itemSearchRequests != 1 {
		t.Fatalf("expected one request to item search endpoint, got %d", itemSearchRequests)
	}
	if !strings.Contains(observedFilterPath, "filter=google") || !strings.Contains(observedFilterPath, "skip=2") || !strings.Contains(observedFilterPath, "take=15") {
		t.Fatalf("expected mapped fallback filter params, got %q", observedFilterPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype search payload: %v", err)
	}
	if payload["total"] != float64(1) {
		t.Fatalf("unexpected datatype search payload: %+v", payload)
	}
}

func TestDatatypeSearchEditorAliasOnlyUsesFilterEndpointAndClientFilters(t *testing.T) {
	var observedPath string
	var itemSearchRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/data-type/search":
			itemSearchRequests++
			return datatypeJSONResponse(http.StatusBadRequest, `{"errors":{"query":["The query field is required"]}}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{
  "total":3,
  "items":[
    {"id":"dt-text","name":"Textstring","editorAlias":"Umbraco.TextBox"},
    {"id":"dt-color","name":"Color","editorAlias":"Umbraco.ColorPicker"},
    {"id":"dt-textarea","name":"Textarea","editorAlias":"Umbraco.TextArea"}
  ]
}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "search", "--editor-alias", "Umbraco.TextBox")
	if err != nil {
		t.Fatalf("datatype search --editor-alias failed: %v", err)
	}
	if itemSearchRequests != 0 {
		t.Fatalf("expected editor-alias-only search to avoid query-required item search endpoint, got %d requests", itemSearchRequests)
	}
	if !strings.Contains(observedPath, "/filter/data-type") || !strings.Contains(observedPath, "filter=Umbraco.TextBox") {
		t.Fatalf("expected filter endpoint with editor alias fallback query, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype search payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one client-filtered item, got %+v", payload)
	}
	item := items[0].(map[string]any)
	if item["id"] != "dt-text" || item["editorAlias"] != "Umbraco.TextBox" {
		t.Fatalf("unexpected filtered item: %+v", item)
	}
	if payload["filteredTotal"] != float64(1) {
		t.Fatalf("expected filteredTotal=1, got %+v", payload)
	}
}

func TestDatatypeSearchQueryAndEditorAliasClientFiltersServerResults(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/data-type/search":
			return datatypeJSONResponse(http.StatusOK, `{
  "total":3,
  "items":[
    {"id":"dt-dropdown","name":"Dropdown","editorAlias":"Umbraco.DropDown.Flexible"},
    {"id":"dt-text","name":"Textstring","editorAlias":"Umbraco.TextBox"},
    {"id":"dt-picker","name":"Picker","editorAlias":"Umbraco.MultiNodeTreePicker"}
  ]
}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "search", "--query", "Umbraco", "--editor-alias", "umbraco.textbox")
	if err != nil {
		t.Fatalf("datatype search query + editor alias failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype search payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one client-filtered item, got %+v", payload)
	}
	item := items[0].(map[string]any)
	if item["id"] != "dt-text" {
		t.Fatalf("unexpected filtered item: %+v", item)
	}
}

func TestDatatypeSearchEditorAliasPaginatesBeforeApplyingUserTake(t *testing.T) {
	var requests []string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			requests = append(requests, req.URL.RawQuery)
			switch req.URL.Query().Get("skip") {
			case "0":
				return datatypeJSONResponse(http.StatusOK, `{"total":401,"items":[{"id":"dt-color","editorAlias":"Umbraco.ColorPicker"}]}`), nil
			case "200":
				return datatypeJSONResponse(http.StatusOK, `{"total":401,"items":[
					{"id":"dt-text-1","editorAlias":"Umbraco.TextBox"},
					{"id":"dt-text-2","editorAlias":"Umbraco.TextBox"}
				]}`), nil
			case "400":
				return datatypeJSONResponse(http.StatusOK, `{"total":401,"items":[{"id":"dt-text-3","editorAlias":"Umbraco.TextBox"}]}`), nil
			default:
				return datatypeJSONResponse(http.StatusOK, `{"total":401,"items":[]}`), nil
			}
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "search", "--editor-alias", "Umbraco.TextBox", "--take", "3")
	if err != nil {
		t.Fatalf("datatype search --editor-alias failed: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected scan to continue until three matches were found, got %d requests: %+v", len(requests), requests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype search payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected three filtered matches, got %+v", payload)
	}
	if payload["filteredTotal"] != float64(3) {
		t.Fatalf("expected filteredTotal=3, got %+v", payload)
	}
}

func TestDatatypeRootUsesTreeRootEndpoint(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/data-type/root":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"root-1","name":"Root"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "root", "--skip", "1", "--take", "10")
	if err != nil {
		t.Fatalf("datatype root failed: %v", err)
	}

	if !strings.Contains(observedPath, "skip=1") || !strings.Contains(observedPath, "take=10") {
		t.Fatalf("expected pagination params on tree root endpoint, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype root payload: %v", err)
	}
	if payload["total"] != float64(1) {
		t.Fatalf("unexpected datatype root payload: %+v", payload)
	}
}

func TestDatatypeExtensionsReadsAliasValueArray(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[
    {"alias":"extensions","value":["Existing.Extension","New.Extension"]},
    {"alias":"toolbar","value":["bold","italic"]}
  ]
}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "extensions", "dt-1")
	if err != nil {
		t.Fatalf("datatype extensions failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype extensions payload: %v", err)
	}
	if payload["name"] != "Rich Text" {
		t.Fatalf("unexpected datatype extensions payload: %+v", payload)
	}

	extensions, ok := payload["extensions"].([]any)
	if !ok || len(extensions) != 2 || extensions[1] != "New.Extension" {
		t.Fatalf("expected extension aliases in payload, got %+v", payload["extensions"])
	}
}

func TestDatatypeAddValueAppendsAliasArrayValueWithoutDroppingRequiredFields(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode datatype add-value payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "add-value", "dt-1",
		"--alias", "extensions",
		"--value", "New.Extension",
	)
	if err != nil {
		t.Fatalf("datatype add-value failed: %v", err)
	}

	if observedPutBody["name"] != "Rich Text" || observedPutBody["editorAlias"] != "Umb.PropertyEditorUi.Tiptap" {
		t.Fatalf("expected required fields to be preserved, got %+v", observedPutBody)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("expected one alias entry in values payload, got %+v", observedPutBody["values"])
	}
	valueEntry, ok := values[0].(map[string]any)
	if !ok {
		t.Fatalf("expected alias entry object, got %T", values[0])
	}
	extensions, ok := valueEntry["value"].([]any)
	if !ok || len(extensions) != 2 || extensions[1] != "New.Extension" {
		t.Fatalf("expected appended extension alias, got %+v", valueEntry["value"])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype add-value result: %v", err)
	}
	if payload["action"] != "add" || payload["changed"] != true || payload["value"] != "New.Extension" {
		t.Fatalf("unexpected datatype add-value result: %+v", payload)
	}
}

func TestDatatypeAddValueAvoidsDuplicateEntries(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "add-value", "dt-1",
		"--alias", "extensions",
		"--value", "Existing.Extension",
	)
	if err != nil {
		t.Fatalf("datatype add-value duplicate case failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected duplicate add-value to short-circuit without PUT, got %d writes", putRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode duplicate add-value payload: %v", err)
	}
	if payload["changed"] != false || payload["message"] != "value already present" {
		t.Fatalf("unexpected duplicate add-value payload: %+v", payload)
	}
}

func TestDatatypeAddValueSupportsDryRun(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected write"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "add-value", "dt-1",
		"--alias", "extensions",
		"--value", "New.Extension",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("datatype add-value dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype add-value dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}

	body := payload["body"].(map[string]any)
	values := body["values"].([]any)
	valueEntry := values[0].(map[string]any)
	extensions := valueEntry["value"].([]any)
	if len(extensions) != 2 || extensions[1] != "New.Extension" {
		t.Fatalf("expected dry-run body to include appended value, got %+v", extensions)
	}
}

func TestDatatypeRemoveValueRemovesAliasArrayValueWithoutDroppingRequiredFields(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension","Remove.Me"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode datatype remove-value payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "remove-value", "dt-1",
		"--alias", "extensions",
		"--value", "Remove.Me",
	)
	if err != nil {
		t.Fatalf("datatype remove-value failed: %v", err)
	}

	if observedPutBody["name"] != "Rich Text" || observedPutBody["editorAlias"] != "Umb.PropertyEditorUi.Tiptap" {
		t.Fatalf("expected required fields to be preserved, got %+v", observedPutBody)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("expected one alias entry in values payload, got %+v", observedPutBody["values"])
	}
	valueEntry := values[0].(map[string]any)
	extensions := valueEntry["value"].([]any)
	if len(extensions) != 1 || extensions[0] != "Existing.Extension" {
		t.Fatalf("expected targeted extension alias to be removed, got %+v", extensions)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype remove-value result: %v", err)
	}
	if payload["action"] != "remove" || payload["changed"] != true || payload["value"] != "Remove.Me" {
		t.Fatalf("unexpected datatype remove-value result: %+v", payload)
	}
}

func TestDatatypeRemoveValueLeavesPayloadUnchangedWhenValueIsMissing(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "remove-value", "dt-1",
		"--alias", "extensions",
		"--value", "Missing.Extension",
	)
	if err != nil {
		t.Fatalf("datatype remove-value missing case failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected missing remove-value to short-circuit without PUT, got %d writes", putRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode missing remove-value payload: %v", err)
	}
	if payload["changed"] != false || payload["message"] != "value not present" {
		t.Fatalf("unexpected missing remove-value payload: %+v", payload)
	}
}

func TestDatatypeAddExtensionUsesExtensionsAlias(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode datatype add-extension payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "add-extension", "dt-1", "New.Extension",
	)
	if err != nil {
		t.Fatalf("datatype add-extension failed: %v", err)
	}

	values := observedPutBody["values"].([]any)
	valueEntry := values[0].(map[string]any)
	extensions := valueEntry["value"].([]any)
	if len(extensions) != 2 || extensions[1] != "New.Extension" {
		t.Fatalf("expected add-extension to append using the extensions alias, got %+v", extensions)
	}
}

func TestDatatypeRemoveExtensionUsesExtensionsAlias(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Keep.Extension","Remove.Me"]}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode datatype remove-extension payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "remove-extension", "dt-1", "Remove.Me",
	)
	if err != nil {
		t.Fatalf("datatype remove-extension failed: %v", err)
	}

	values := observedPutBody["values"].([]any)
	valueEntry := values[0].(map[string]any)
	extensions := valueEntry["value"].([]any)
	if len(extensions) != 1 || extensions[0] != "Keep.Extension" {
		t.Fatalf("expected remove-extension to target the extensions alias, got %+v", extensions)
	}
}

func TestDatatypeRemoveValueSupportsDryRun(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension","Remove.Me"]}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected write"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "remove-value", "dt-1",
		"--alias", "extensions",
		"--value", "Remove.Me",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("datatype remove-value dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype remove-value dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}

	body := payload["body"].(map[string]any)
	values := body["values"].([]any)
	valueEntry := values[0].(map[string]any)
	extensions := valueEntry["value"].([]any)
	if len(extensions) != 1 || extensions[0] != "Existing.Extension" {
		t.Fatalf("expected dry-run body to exclude removed value, got %+v", extensions)
	}
}

func TestDatatypeUpdateMergeJSONFetchesCurrentAndSendsMergedPayload(t *testing.T) {
	var putPayload map[string]any
	var getRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				getRequests++
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[
    {"alias":"maxChars","value":120},
    {"alias":"toolbar","value":{"bold":true,"italic":true}}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&putPayload); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	// The configuration-map convenience shape converts to values entries
	// before the merge, then merges by alias against the fetched values.
	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"configuration":{"toolbar":{"italic":false}}}`,
	)
	if err != nil {
		t.Fatalf("datatype merge update failed: %v", err)
	}

	if getRequests != 1 {
		t.Fatalf("expected one fetch of the current datatype, got %d", getRequests)
	}
	if putPayload["name"] != "Rich Text" || putPayload["editorAlias"] != "Umb.PropertyEditorUi.Tiptap" {
		t.Fatalf("expected required fields to be preserved, got %+v", putPayload)
	}
	if _, leaked := putPayload["configuration"]; leaked {
		t.Fatalf("configuration must convert to values, not reach the PUT body: %+v", putPayload)
	}

	values, ok := putPayload["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected both values entries preserved, got %+v", putPayload["values"])
	}
	merged := map[string]any{}
	for _, raw := range values {
		entry := raw.(map[string]any)
		merged[entry["alias"].(string)] = entry["value"]
	}
	if merged["maxChars"] != float64(120) {
		t.Fatalf("expected untouched setting to be preserved, got %+v", merged)
	}
	toolbar, ok := merged["toolbar"].(map[string]any)
	if !ok {
		t.Fatalf("missing merged toolbar value: %+v", merged)
	}
	if toolbar["bold"] != true || toolbar["italic"] != false {
		t.Fatalf("expected nested merge to preserve bold and update italic, got %+v", toolbar)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode datatype merge update result: %v", err)
	}
	if result["updated"] != true {
		t.Fatalf("unexpected update result payload: %+v", result)
	}
}

func TestDatatypeUpdateMergeJSONSupportsDryRunForNestedObjectConfig(t *testing.T) {
	var getRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				getRequests++
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"toolbar","value":{"bold":true,"italic":true}}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected write"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"configuration":{"toolbar":{"italic":false}}}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("datatype merge update dry-run failed: %v", err)
	}

	if getRequests != 1 {
		t.Fatalf("expected dry-run merge update to fetch the current datatype once, got %d", getRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing dry-run body: %+v", payload)
	}
	values, ok := body["values"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("missing dry-run merged values: %+v", body)
	}
	entry := values[0].(map[string]any)
	toolbar, ok := entry["value"].(map[string]any)
	if !ok || toolbar["bold"] != true || toolbar["italic"] != false {
		t.Fatalf("unexpected dry-run merged toolbar payload: %+v", entry)
	}
}

func TestDatatypeUpdateMergeJSONPreservesEditorUiAliasAndOtherUnmentionedFields(t *testing.T) {
	// Regression: --json used to PUT only what the caller passed, which the
	// Management API treats as a full replacement — silently dropping
	// editorUiAlias, items, multiple, and anything else not in the payload.
	// Both --json and --merge-json now route through fetch-and-merge.
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
					"id":"dt-1",
					"name":"Tags",
					"editorAlias":"Umbraco.Tags",
					"editorUiAlias":"Umb.PropertyEditorUi.Tags",
					"values":[
						{"alias":"group","value":"default"},
						{"alias":"storageType","value":"Csv"}
					]
				}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("decode PUT: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	// Caller passes a minimal --merge-json with just the field they want to
	// change. Pre-v0.4.0 this protection lived on --json; it now belongs to
	// --merge-json under the uniform update contract.
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"name":"Renamed Tags"}`,
	)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if observedPutBody["name"] != "Renamed Tags" {
		t.Fatalf("expected the user-supplied name change to survive, got %+v", observedPutBody["name"])
	}
	if observedPutBody["editorAlias"] != "Umbraco.Tags" {
		t.Fatalf("editorAlias must be preserved by the merge, got %+v", observedPutBody["editorAlias"])
	}
	if observedPutBody["editorUiAlias"] != "Umb.PropertyEditorUi.Tags" {
		t.Fatalf("editorUiAlias must be preserved by the merge (the original regression), got %+v", observedPutBody["editorUiAlias"])
	}
	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("both values entries must be preserved, got %+v", observedPutBody["values"])
	}
}

func TestDatatypeUpdateRejectsJSONAndMergeJSONTogether(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	_, err := execute(
		root,
		"datatype", "update", "dt-1",
		"--json", `{"name":"Full"}`,
		"--merge-json", `{"configuration":{"toolbar":{"italic":false}}}`,
	)
	if err == nil {
		t.Fatalf("expected datatype update to reject simultaneous --json and --merge-json")
	}
	if !strings.Contains(err.Error(), "exactly one of --json (full replacement) or --merge-json (fetch and merge)") {
		t.Fatalf("unexpected merge-json validation error: %v", err)
	}
}

func TestDatatypeUpdateJSONReplacesWithoutFetching(t *testing.T) {
	var observedPutBody map[string]any
	var observedGets int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				observedGets++
				return datatypeJSONResponse(http.StatusOK, `{"id":"dt-1","name":"Old"}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("decode PUT: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, ``), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--json", `{"name":"Full Replacement","editorAlias":"Umbraco.Tags"}`,
	)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	// --json is a wholesale replacement: no fetch, body sent verbatim.
	if observedGets != 0 {
		t.Fatalf("expected no fetch for --json replacement, got %d GETs", observedGets)
	}
	if observedPutBody["name"] != "Full Replacement" || len(observedPutBody) != 2 {
		t.Fatalf("expected verbatim replacement body, got %+v", observedPutBody)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["updated"] != true {
		t.Fatalf("expected empty 204 success to coalesce to {updated:true}, got %+v", payload)
	}
}

func TestDatatypeUpdateMergeJSONMergesValuesByAliasAndPreservesRequiredFields(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[
    {"alias":"extensions","value":["Existing.Extension"]},
    {"alias":"toolbar","value":["bold","italic"]}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode merged datatype payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"values":[{"alias":"extensions","value":["Existing.Extension","New.Extension"]}]}`,
	)
	if err != nil {
		t.Fatalf("datatype merge update failed: %v", err)
	}

	if observedPutBody["name"] != "Rich Text" || observedPutBody["editorAlias"] != "Umb.PropertyEditorUi.Tiptap" {
		t.Fatalf("expected required fields to be preserved, got %+v", observedPutBody)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected merged values array, got %+v", observedPutBody["values"])
	}

	var extensionsValue []any
	var toolbarValue []any
	for _, item := range values {
		itemMap, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected value item to be an object, got %T", item)
		}
		switch itemMap["alias"] {
		case "extensions":
			extensionsValue, _ = itemMap["value"].([]any)
		case "toolbar":
			toolbarValue, _ = itemMap["value"].([]any)
		}
	}

	if len(extensionsValue) != 2 || extensionsValue[1] != "New.Extension" {
		t.Fatalf("expected extensions alias to be updated, got %+v", extensionsValue)
	}
	if len(toolbarValue) != 2 || toolbarValue[0] != "bold" {
		t.Fatalf("expected unrelated alias entries to be preserved, got %+v", toolbarValue)
	}
}

func TestDatatypeUpdateMergeJSONSupportsDryRunForAliasValues(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "values":[{"alias":"extensions","value":["Existing.Extension"]}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected method"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"values":[{"alias":"extensions","value":["Existing.Extension","New.Extension"]}]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("datatype merge dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode datatype merge dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}

	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected dry-run body to be an object, got %+v", payload["body"])
	}
	if body["name"] != "Rich Text" || body["editorAlias"] != "Umb.PropertyEditorUi.Tiptap" {
		t.Fatalf("expected merged dry-run payload to preserve required fields, got %+v", body)
	}
}

func TestDatatypeCreateConvertsConfigurationToValues(t *testing.T) {
	// The API silently ignores an unknown configuration key, so settings
	// passed that way used to vanish while creation reported success.
	output, err := execute(buildRootWithCollections(t, makeDeps()),
		"datatype", "create", "--dry-run",
		"--json", `{"name":"Tags","editorAlias":"Umbraco.Tags","editorUiAlias":"Umb.PropertyEditorUi.Tags","configuration":{"storageType":"Json","group":"default"}}`,
	)
	if err != nil {
		t.Fatalf("datatype create dry-run failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode dry-run payload: %v", err)
	}
	body := payload["body"].(map[string]any)
	if _, leaked := body["configuration"]; leaked {
		t.Fatalf("configuration must convert to values, got %+v", body)
	}
	values, ok := body["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected two converted values entries, got %+v", body["values"])
	}
	first := values[0].(map[string]any)
	if first["alias"] != "group" || first["value"] != "default" {
		t.Fatalf("expected deterministic alias-sorted conversion, got %+v", values)
	}

	_, err = execute(buildRootWithCollections(t, makeDeps()),
		"datatype", "create", "--dry-run",
		"--json", `{"name":"Tags","editorAlias":"Umbraco.Tags","configuration":{"a":1},"values":[{"alias":"b","value":2}]}`,
	)
	if err == nil || !strings.Contains(err.Error(), "mixes configuration and values") {
		t.Fatalf("expected mixed shapes to be rejected, got %v", err)
	}
}

func TestDatatypeUpdateMergeFoldsLegacyConfigurationResponses(t *testing.T) {
	// Defense in depth: no supported Management API returns a configuration
	// map for data types, but if a non-standard response carries one, the
	// merged body must fold it into values (patch wins on collisions) and
	// never PUT the configuration key.
	var putPayload map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/data-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "name":"Rich Text",
  "editorAlias":"Umb.PropertyEditorUi.Tiptap",
  "configuration":{"maxChars":120,"toolbar":{"bold":true,"italic":true}}
}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&putPayload); err != nil {
				t.Fatalf("decode put payload: %v", err)
			}
			return datatypeJSONResponse(http.StatusOK, ``), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"datatype", "update", "dt-1",
		"--merge-json", `{"configuration":{"toolbar":{"italic":false}}}`); err != nil {
		t.Fatalf("datatype merge update failed: %v", err)
	}

	if _, leaked := putPayload["configuration"]; leaked {
		t.Fatalf("configuration must never reach the PUT body, got %+v", putPayload)
	}
	values, ok := putPayload["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected folded values entries, got %+v", putPayload["values"])
	}
	settings := map[string]any{}
	for _, raw := range values {
		entry := raw.(map[string]any)
		settings[entry["alias"].(string)] = entry["value"]
	}
	if settings["maxChars"] != float64(120) {
		t.Fatalf("expected untouched legacy setting folded into values, got %+v", settings)
	}
	toolbar, ok := settings["toolbar"].(map[string]any)
	if !ok || toolbar["italic"] != false || toolbar["bold"] != true {
		t.Fatalf("expected the patch deep-merged over the legacy setting (italic updated, bold preserved), got %+v", settings)
	}
}
