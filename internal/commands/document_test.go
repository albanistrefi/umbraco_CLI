package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentSearchUsesItemSearchEndpointAndFallsBack(t *testing.T) {
	var requests []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/document/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1","name":"Toxic"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "search", "--query", "Toxic", "--skip", "0", "--take", "25")
	if err != nil {
		t.Fatalf("document search failed: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 search attempts, got %+v", requests)
	}
	if !strings.Contains(requests[0], "/item/document/search") || !strings.Contains(requests[0], "query=Toxic") {
		t.Fatalf("unexpected primary document search request: %q", requests[0])
	}
	if !strings.Contains(requests[1], "/document/search") {
		t.Fatalf("unexpected fallback document search request: %q", requests[1])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode document search payload: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected document search payload, got %+v", payload)
	}
}

func TestDocumentSearchSupportsUnderShortcut(t *testing.T) {
	var observedPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			observedPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1","name":"Toxic"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"document", "search",
		"--query", "Toxic",
		"--under", "partners-root",
		"--skip", "0",
		"--take", "25",
	)
	if err != nil {
		t.Fatalf("document search --under failed: %v", err)
	}

	if !strings.Contains(observedPath, "parentId=partners-root") {
		t.Fatalf("expected --under to map to parentId, got %q", observedPath)
	}
}

func TestDocumentTreeCommandsPreferTreeEndpoints(t *testing.T) {
	var rootPath string
	var childrenPath string
	var ancestorsPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			rootPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"root-1"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			childrenPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"child-1"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/ancestors":
			ancestorsPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"ancestor-1"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "root", "--fields", "id,name"); err != nil {
		t.Fatalf("document root failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "document", "children", "parent-1", "--fields", "id,name"); err != nil {
		t.Fatalf("document children failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "document", "ancestors", "doc-1"); err != nil {
		t.Fatalf("document ancestors failed: %v", err)
	}

	if !strings.Contains(rootPath, "/tree/document/root") || !strings.Contains(rootPath, "fields=id%2Cname") {
		t.Fatalf("unexpected document root path: %q", rootPath)
	}
	if !strings.Contains(childrenPath, "/tree/document/children") || !strings.Contains(childrenPath, "parentId=parent-1") {
		t.Fatalf("unexpected document children path: %q", childrenPath)
	}
	if !strings.Contains(ancestorsPath, "/tree/document/ancestors") || !strings.Contains(ancestorsPath, "descendantId=doc-1") {
		t.Fatalf("unexpected document ancestors path: %q", ancestorsPath)
	}
}

func TestDocumentUpdateMergeJSONFetchesAndMergesCurrentDocument(t *testing.T) {
	var observedPutBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Toxic",
  "documentType":{"id":"type-1"},
  "values":[
    {"alias":"title","value":"Old title"},
    {"alias":"summary","value":"Keep me"}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode merged document payload: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "update", "doc-1", "--merge-json", `{"values":[{"alias":"title","value":"New title"}]}`)
	if err != nil {
		t.Fatalf("document update --merge-json failed: %v", err)
	}

	if observedPutBody["name"] != "Toxic" {
		t.Fatalf("expected merged document to preserve root fields, got %+v", observedPutBody)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected merged values payload, got %+v", observedPutBody["values"])
	}

	firstValue, _ := values[0].(map[string]any)
	secondValue, _ := values[1].(map[string]any)
	if firstValue["value"] != "New title" || secondValue["value"] != "Keep me" {
		t.Fatalf("expected alias-based merge to preserve untouched values, got %+v", observedPutBody["values"])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode document update result: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected document update result: %+v", payload)
	}
}

func TestDocumentBulkUpdateDryRunUsesExplicitIDsAndSkipsNoOps(t *testing.T) {
	var putRequests int

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
		case "/umbraco/management/api/v1/document/doc-2":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-2",
  "name":"Partner B",
  "values":[{"alias":"title","value":"New title"}]
}`), nil
		default:
			if req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				putRequests++
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "bulk-update",
		"--id", "doc-1",
		"--id", "doc-2",
		"--merge-json", `{"values":[{"alias":"title","value":"New title"}]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document bulk-update failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run bulk update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentBulkUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode bulk update result: %v", err)
	}
	if payload.Total != 2 || payload.Updated != 1 || payload.Skipped != 1 || payload.Failed != 0 {
		t.Fatalf("unexpected bulk update summary: %+v", payload)
	}
}

func TestDocumentBulkUpdateLoadsIDsFromFile(t *testing.T) {
	idFile := filepath.Join(t.TempDir(), "ids.txt")
	if err := os.WriteFile(idFile, []byte("doc-1\n\ndoc-2\n"), 0o644); err != nil {
		t.Fatalf("failed to write id file: %v", err)
	}

	var putRequests int
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			if req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				putRequests++
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `{"id":"doc","name":"Doc","values":[]}`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "bulk-update",
		"--id-file", idFile,
		"--json", `{"values":[]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document bulk-update with id file failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run id-file bulk update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentBulkUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode bulk update id-file result: %v", err)
	}
	if payload.Total != 2 || payload.Updated != 2 {
		t.Fatalf("unexpected id-file bulk update summary: %+v", payload)
	}
}
