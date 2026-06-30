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

type endpointRoundTripper func(*http.Request) (*http.Response, error)

func (fn endpointRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func endpointJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func endpointDeps(handler endpointRoundTripper) Dependencies {
	cfg := config.Config{
		BaseURL:      "https://example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	httpClient := &http.Client{Transport: handler}
	output := "json"

	return Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		Config:     cfg,
		HTTPClient: httpClient,
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

func TestTemplateRootUsesTreeEndpointAndFallsBack(t *testing.T) {
	var requests []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/template/root":
			requests = append(requests, req.URL.Path)
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/template/root":
			requests = append(requests, req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"tpl-1"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "template", "root")
	if err != nil {
		t.Fatalf("template root failed: %v", err)
	}

	if len(requests) != 2 || requests[0] != "/umbraco/management/api/v1/tree/template/root" || requests[1] != "/umbraco/management/api/v1/template/root" {
		t.Fatalf("unexpected request order: %+v", requests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode template root payload: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected template root payload, got %+v", payload)
	}
}

func TestTemplateSearchUsesItemSearchEndpoint(t *testing.T) {
	var observedPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/template/search":
			observedPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"tpl-1","name":"Partner Page"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "template", "search", "--query", "Partner Page")
	if err != nil {
		t.Fatalf("template search failed: %v", err)
	}

	if !strings.Contains(observedPath, "/item/template/search") || !strings.Contains(observedPath, "query=Partner+Page") {
		t.Fatalf("unexpected template search path: %q", observedPath)
	}
}

func TestDoctypeRootChildrenAndSearchUseTreeAndItemEndpoints(t *testing.T) {
	var rootPath string
	var childrenPath string
	var searchPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document-type/root":
			rootPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"dt-root"}]}`), nil
		case "/umbraco/management/api/v1/tree/document-type/children":
			childrenPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"dt-child"}]}`), nil
		case "/umbraco/management/api/v1/item/document-type/search":
			searchPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"dt-1"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "root"); err != nil {
		t.Fatalf("doctype root failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "children", "parent-1"); err != nil {
		t.Fatalf("doctype children failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "search", "--query", "partnerPage"); err != nil {
		t.Fatalf("doctype search failed: %v", err)
	}

	if rootPath != "https://example.test/umbraco/management/api/v1/tree/document-type/root" {
		t.Fatalf("unexpected doctype root path: %q", rootPath)
	}
	if !strings.Contains(childrenPath, "/tree/document-type/children") || !strings.Contains(childrenPath, "parentId=parent-1") {
		t.Fatalf("unexpected doctype children path: %q", childrenPath)
	}
	if !strings.Contains(searchPath, "/item/document-type/search") || !strings.Contains(searchPath, "query=partnerPage") {
		t.Fatalf("unexpected doctype search path: %q", searchPath)
	}
}

func TestServerCommandsPreferLongRouteNames(t *testing.T) {
	var observed []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/server/information",
			"/umbraco/management/api/v1/server/configuration",
			"/umbraco/management/api/v1/server/troubleshooting":
			observed = append(observed, req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	for _, args := range [][]string{
		{"server", "info"},
		{"server", "config"},
		{"server", "troubleshoot"},
	} {
		if _, err := execute(buildRootWithCollections(t, deps), args...); err != nil {
			t.Fatalf("%s failed: %v", strings.Join(args, " "), err)
		}
	}

	expected := []string{
		"/umbraco/management/api/v1/server/information",
		"/umbraco/management/api/v1/server/configuration",
		"/umbraco/management/api/v1/server/troubleshooting",
	}
	if len(observed) != len(expected) {
		t.Fatalf("unexpected server requests: %+v", observed)
	}
	for index, path := range expected {
		if observed[index] != path {
			t.Fatalf("expected %q at index %d, got %+v", path, index, observed)
		}
	}
}
