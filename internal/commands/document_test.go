package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestDocumentGetJSONOutputEscapesControlCharacters(t *testing.T) {
	controlString := allJSONControlCharacters() + `"\\emoji: 😀`
	apiPayload := map[string]any{
		"id":   "doc-control",
		"name": "Control Characters",
		"values": []any{
			map[string]any{
				"alias": "richText",
				"value": controlString,
			},
		},
	}
	apiBody, err := json.Marshal(apiPayload)
	if err != nil {
		t.Fatalf("failed to encode API fixture: %v", err)
	}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-control":
			return endpointJSONResponse(http.StatusOK, string(apiBody)), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-control", "-o", "json")
	if err != nil {
		t.Fatalf("document get failed: %v", err)
	}

	if !json.Valid([]byte(output)) {
		t.Fatalf("document get -o json emitted invalid JSON: %q", output)
	}
	assertNoRawControlCharactersInJSONStringTokens(t, output)
	assertStrictJSONParsersAccept(t, output)

	var fromCLI map[string]any
	if err := json.Unmarshal([]byte(output), &fromCLI); err != nil {
		t.Fatalf("failed to decode CLI output: %v", err)
	}
	var fromAPI map[string]any
	if err := json.Unmarshal(apiBody, &fromAPI); err != nil {
		t.Fatalf("failed to decode API fixture: %v", err)
	}
	if !reflect.DeepEqual(fromCLI, fromAPI) {
		t.Fatalf("CLI output changed API semantics:\nCLI: %+v\nAPI: %+v", fromCLI, fromAPI)
	}
}

func TestDocumentCopyPublishPublishesCopiedDocument(t *testing.T) {
	var publishPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/source-1/copy":
			return endpointJSONResponse(http.StatusOK, `{"id":"copy-1","name":"Copied"}`), nil
		case "/umbraco/management/api/v1/document/copy-1/publish":
			publishPath = req.URL.Path
			return endpointJSONResponse(http.StatusOK, `{"published":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "copy", "source-1", "--to", "parent-1", "--publish")
	if err != nil {
		t.Fatalf("document copy --publish failed: %v", err)
	}
	if publishPath != "/umbraco/management/api/v1/document/copy-1/publish" {
		t.Fatalf("expected copied document to be published, got %q", publishPath)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode copy publish result: %v", err)
	}
	if result["copied"] == nil || result["published"] == nil {
		t.Fatalf("unexpected copy publish result: %+v", result)
	}
}

func allJSONControlCharacters() string {
	runes := make([]rune, 0, 32)
	for value := rune(0); value <= 0x1f; value++ {
		runes = append(runes, value)
	}
	return string(runes)
}

func assertNoRawControlCharactersInJSONStringTokens(t *testing.T, raw string) {
	t.Helper()

	inString := false
	escaped := false
	for index, r := range raw {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString && r <= 0x1f:
			t.Fatalf("raw control character U+%04X found inside JSON string at byte %d", r, index)
		}
	}
	if inString {
		t.Fatal("unterminated JSON string")
	}
}

func assertStrictJSONParsersAccept(t *testing.T, raw string) {
	t.Helper()

	parsers := []struct {
		name string
		args []string
	}{
		{name: "python3", args: []string{"-c", "import json,sys; json.load(sys.stdin)"}},
		{name: "node", args: []string{"-e", "JSON.parse(require('fs').readFileSync(0, 'utf8'))"}},
		{name: "jq", args: []string{"."}},
	}
	for _, parser := range parsers {
		if _, err := exec.LookPath(parser.name); err != nil {
			t.Logf("%s not found; skipping external JSON parser check", parser.name)
			continue
		}
		cmd := exec.Command(parser.name, parser.args...)
		cmd.Stdin = strings.NewReader(raw)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s rejected CLI JSON output: %v: %s", parser.name, err, stderr.String())
		}
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

	if !strings.Contains(rootPath, "/tree/document/root") || strings.Contains(rootPath, "fields=") {
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

func TestDocumentUpdateMergeJSONAllowsExistingControlCharactersInFetchedDocument(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[
    {"alias":"bodyText","value":"line1\nline2"},
    {"alias":"title","value":"Old title"}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--merge-json", `{"values":[{"alias":"skills","value":[{"type":"document","unique":"62689bb1-3a4d-478f-a7b1-1c0e560d4748"}]}]}`,
	)
	if err != nil {
		t.Fatalf("document update --merge-json with existing control characters failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode merge-json regression payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected merge-json regression payload: %+v", payload)
	}
}

func TestDocumentUpdatePropertyTargetsPropertiesEndpoint(t *testing.T) {
	var observedGetCount int
	var observedPutPath string
	var observedPutBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				observedGetCount++
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			if req.Method == http.MethodPut {
				observedPutPath = req.URL.Path
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

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
	)
	if err != nil {
		t.Fatalf("document property update failed: %v", err)
	}

	if observedGetCount != 1 {
		t.Fatalf("expected one GET before property merge update, got %d", observedGetCount)
	}
	if observedPutPath != "/umbraco/management/api/v1/document/doc-1" {
		t.Fatalf("unexpected merged document update path: %q", observedPutPath)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("unexpected properties payload: %+v", observedPutBody)
	}
	var foundSkills bool
	for _, item := range values {
		valueEntry, _ := item.(map[string]any)
		if valueEntry["alias"] == "skills" && valueEntry["value"] == "C#;Go" {
			foundSkills = true
		}
	}
	if !foundSkills {
		t.Fatalf("expected merged property value entry, got %+v", observedPutBody)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode property update output: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected property update result: %+v", payload)
	}
}

func TestDocumentUpdateSaveAndPublishDryRunReturnsBothSteps(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
		"--save-and-publish",
		"--culture", "en-US",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document save-and-publish dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode save-and-publish output: %v", err)
	}
	if payload["saveAndPublish"] != true {
		t.Fatalf("expected saveAndPublish marker, got %+v", payload)
	}

	updated, ok := payload["updated"].(map[string]any)
	if !ok {
		t.Fatalf("missing updated dry-run payload: %+v", payload)
	}
	published, ok := payload["published"].(map[string]any)
	if !ok {
		t.Fatalf("missing published dry-run payload: %+v", payload)
	}

	if updated["path"] != "/umbraco/management/api/v1/document/doc-1" {
		t.Fatalf("unexpected update dry-run path: %+v", updated)
	}
	if published["path"] != "/umbraco/management/api/v1/document/doc-1/publish" {
		t.Fatalf("unexpected publish dry-run path: %+v", published)
	}
	body, _ := published["body"].(map[string]any)
	cultures, _ := body["cultures"].([]any)
	if len(cultures) != 1 || cultures[0] != "en-US" {
		t.Fatalf("expected publish culture in dry-run body, got %+v", body)
	}
}

func TestDocumentPublishDryRunDefaultsToInvariantPublishSchedule(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "publish", "doc-1",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document publish dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode publish dry-run payload: %v", err)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing publish body in dry-run payload: %+v", payload)
	}
	publishSchedules, ok := body["publishSchedules"].([]any)
	if !ok || len(publishSchedules) != 1 {
		t.Fatalf("expected invariant publishSchedules payload, got %+v", body)
	}
	entry, ok := publishSchedules[0].(map[string]any)
	if !ok || entry["culture"] != nil {
		t.Fatalf("expected publishSchedules culture=null, got %+v", publishSchedules[0])
	}
}

func TestDocumentUpdateSaveAndPublishDryRunDefaultsToInvariantPublishSchedule(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
		"--save-and-publish",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document save-and-publish invariant dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode invariant save-and-publish output: %v", err)
	}
	published, ok := payload["published"].(map[string]any)
	if !ok {
		t.Fatalf("missing published dry-run payload: %+v", payload)
	}
	body, ok := published["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing publish body: %+v", published)
	}
	publishSchedules, ok := body["publishSchedules"].([]any)
	if !ok || len(publishSchedules) != 1 {
		t.Fatalf("expected invariant publishSchedules payload, got %+v", body)
	}
	entry, ok := publishSchedules[0].(map[string]any)
	if !ok || entry["culture"] != nil {
		t.Fatalf("expected publishSchedules culture=null, got %+v", publishSchedules[0])
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

func TestDocumentCSVUpdateDryRunUsesMappedProperties(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "partners.csv")
	if err := os.WriteFile(csvPath, []byte("id,skills\npartner-1,C#;Go\npartner-2,\n"), 0o644); err != nil {
		t.Fatalf("failed to write CSV fixture: %v", err)
	}

	var putRequests int
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/partner-1":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"partner-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
		case "/umbraco/management/api/v1/document/partner-2":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"partner-2",
  "name":"Partner B",
  "values":[{"alias":"title","value":"Old title"}]
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
		"document", "csv-update",
		"--file", csvPath,
		"--property", "skills",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document csv-update failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run CSV update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentCSVUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode CSV update result: %v", err)
	}
	if payload.TotalRows != 2 || payload.Updated != 1 || payload.Skipped != 1 || payload.Failed != 0 {
		t.Fatalf("unexpected CSV update summary: %+v", payload)
	}
}

func TestDocumentCSVUpdateRejectsDuplicateIDs(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "partners.csv")
	if err := os.WriteFile(csvPath, []byte("id,skills\npartner-1,C#\npartner-1,Go\n"), 0o644); err != nil {
		t.Fatalf("failed to write CSV fixture: %v", err)
	}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/partner-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"partner-1","name":"Partner A","values":[]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "csv-update",
		"--file", csvPath,
		"--property", "skills",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document csv-update duplicate case failed: %v", err)
	}

	var payload documentCSVUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode CSV duplicate result: %v", err)
	}
	if payload.Failed != 1 {
		t.Fatalf("expected one failed duplicate row, got %+v", payload)
	}
}
