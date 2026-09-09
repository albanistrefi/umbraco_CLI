package commands

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const applyFolderUda = `{"Name":"Blocks","Udi":"umb://document-type-container/cccccccc111122223333444444444444","Dependencies":[],"__type":"X","__version":"18.0.1"}`

// applyParentDoctypeUda is allowed as a child of the block; the block's
// artifact references it forward (created later) to exercise deferral.
const applyChildDoctypeUda = `{"Name":"Child","Alias":"child","Icon":"icon-item","Permissions":{"IsElementType":false,"AllowedAtRoot":false,"AllowedChildContentTypes":[]},"PropertyGroups":[],"PropertyTypes":[],"Udi":"umb://document-type/dddddddd111122223333444444444444","Dependencies":[{"Udi":"umb://document-type/bbbbbbbb111122223333444444444444","Ordering":true}],"__type":"X","__version":"18.0.1"}`

func applyBlockUdaWithChild() string {
	return strings.Replace(statusDoctypeUda, `"Permissions": {"IsElementType": true, "AllowedChildContentTypes": []}`, `"Permissions": {"IsElementType": true, "AllowedChildContentTypes": ["umb://document-type/dddddddd111122223333444444444444"]}, "Parent": "umb://document-type-container/cccccccc111122223333444444444444"`, 1)
}

func writeApplyCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("\xEF\xBB\xBF"+content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type applyRecorder struct {
	requests []string
	bodies   map[string]map[string]any
	created  map[string]bool
}

// applyDeps simulates an environment where the data type exists but drifts
// (name differs), and nothing else exists; created entities then become
// readable so verification can pass.
func applyDeps(t *testing.T, rec *applyRecorder) Dependencies {
	t.Helper()
	rec.bodies = map[string]map[string]any{}
	rec.created = map[string]bool{}
	return datatypeDeps(func(req *http.Request) (*http.Response, error) {
		path := strings.TrimPrefix(req.URL.Path, "/umbraco/management/api/v1")
		switch path {
		case "/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"t","expires_in":3600}`), nil
		case "/server/status":
			return datatypeJSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		case "/automations":
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
		if req.Method == http.MethodPost || req.Method == http.MethodPut {
			raw, _ := io.ReadAll(req.Body)
			body := map[string]any{}
			_ = json.Unmarshal(raw, &body)
			rec.requests = append(rec.requests, req.Method+" "+path)
			rec.bodies[req.Method+" "+path] = body
			if req.Method == http.MethodPost {
				id, _ := body["id"].(string)
				rec.created[path+"/"+id] = true
			}
			return datatypeJSONResponse(http.StatusOK, ``), nil
		}
		rec.requests = append(rec.requests, "GET "+path)
		switch path {
		case "/data-type/aaaaaaaa-1111-2222-3333-444444444444":
			name := "Old Name"
			if b, ok := rec.bodies["PUT "+path]; ok {
				name, _ = b["name"].(string)
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"aaaaaaaa-1111-2222-3333-444444444444","name":"`+name+`","editorAlias":"Umbraco.TextArea","editorUiAlias":"Umb.PropertyEditorUi.TextArea","values":[{"alias":"maxChars","value":500}]}`), nil
		case "/document-type/folder/cccccccc-1111-2222-3333-444444444444":
			if rec.created["/document-type/folder/cccccccc-1111-2222-3333-444444444444"] {
				return datatypeJSONResponse(http.StatusOK, `{"id":"cccccccc-1111-2222-3333-444444444444","name":"Blocks"}`), nil
			}
		case "/document-type/bbbbbbbb-1111-2222-3333-444444444444":
			if rec.created["/document-type/bbbbbbbb-1111-2222-3333-444444444444"] {
				body := rec.bodies["PUT "+path]
				if body == nil {
					body = rec.bodies["POST /document-type"]
				}
				// Echo the last written body in response shape.
				allowed, _ := body["allowedDocumentTypes"].([]any)
				encoded, _ := json.Marshal(map[string]any{
					"id": "bbbbbbbb-1111-2222-3333-444444444444", "name": "Example Block", "alias": "exampleBlock", "icon": "icon-box", "isElement": true,
					"allowedDocumentTypes": allowed, "compositions": []any{},
					"properties": []any{
						map[string]any{"alias": "heading", "name": "Heading", "dataType": map[string]any{"id": "aaaaaaaa-1111-2222-3333-444444444444"}, "sortOrder": 0, "validation": map[string]any{"mandatory": false}},
						map[string]any{"alias": "ungrouped", "name": "Ungrouped", "dataType": map[string]any{"id": "aaaaaaaa-1111-2222-3333-444444444444"}, "sortOrder": 5, "validation": map[string]any{"mandatory": false}},
					},
				})
				return datatypeJSONResponse(http.StatusOK, string(encoded)), nil
			}
		case "/document-type/dddddddd-1111-2222-3333-444444444444":
			if rec.created[path] {
				return datatypeJSONResponse(http.StatusOK, `{"id":"dddddddd-1111-2222-3333-444444444444","name":"Child","alias":"child","icon":"icon-item","isElement":false,"allowedAsRoot":false,"allowedDocumentTypes":[],"compositions":[],"properties":[]}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
}

func TestDeployApplyRequiresDryRunOrForce(t *testing.T) {
	dir := writeApplyCorpus(t, map[string]string{"dt.uda": statusDataTypeUda})
	rec := &applyRecorder{}
	if _, err := execute(buildDeployRoot(applyDeps(t, rec)), "deploy", "apply", "--uda-dir", dir); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force gate, got %v", err)
	}
	if len(rec.requests) != 0 {
		t.Fatalf("gate must fire before any request, got %v", rec.requests)
	}
}

func TestDeployApplyDryRunPlansInDependencyOrderWithoutWriting(t *testing.T) {
	dir := writeApplyCorpus(t, map[string]string{
		"z-block.uda":  applyBlockUdaWithChild(),
		"a-child.uda":  applyChildDoctypeUda,
		"m-dt.uda":     statusDataTypeUda,
		"f-folder.uda": applyFolderUda,
		"auto.uda":     statusAutomationUda,
	})
	rec := &applyRecorder{}
	output, err := execute(buildDeployRoot(applyDeps(t, rec)), "deploy", "apply", "--uda-dir", dir, "--dry-run", "--bodies")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	for _, r := range rec.requests {
		if strings.HasPrefix(r, "POST") || strings.HasPrefix(r, "PUT") {
			t.Fatalf("dry-run must not write, saw %s", r)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	plan := payload["plan"].([]any)
	order := []string{}
	for _, raw := range plan {
		e := raw.(map[string]any)
		order = append(order, e["kind"].(string)+":"+e["action"].(string))
	}
	joined := strings.Join(order, " ")
	// data type (Ordering dep of the block) before the block; folder before both; child (Ordering dep on block) after the block.
	want := "document-type-container:create data-type:update document-type:create document-type:create umbraco-automate-automation:unsupported"
	if joined != want {
		t.Fatalf("unexpected plan order:\n got %s\nwant %s", joined, want)
	}
	block := plan[2].(map[string]any)
	if block["name"] != "Example Block" {
		t.Fatalf("expected the block third, got %v", block["name"])
	}
	deferred, _ := block["deferredReferences"].([]any)
	if len(deferred) != 1 || deferred[0] != "dddddddd-1111-2222-3333-444444444444" {
		t.Fatalf("expected the forward child reference deferred, got %v", deferred)
	}
	body := block["body"].(map[string]any)
	if len(body["allowedDocumentTypes"].([]any)) != 0 || body["parent"].(map[string]any)["id"] != "cccccccc-1111-2222-3333-444444444444" {
		t.Fatalf("expected deferred allowed children and folder parent in body, got %v", body)
	}
	props := body["properties"].([]any)
	if len(props) != 2 || props[0].(map[string]any)["container"].(map[string]any)["id"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected grouped property bound to its container, got %v", props)
	}
	if body["containers"].([]any)[0].(map[string]any)["type"] != "Group" {
		t.Fatalf("expected Group container, got %v", body["containers"])
	}
	if payload["summary"].(map[string]any)["create"] != float64(3) {
		t.Fatalf("unexpected summary: %v", payload["summary"])
	}
}

func TestDeployApplyForceWritesBacksUpVerifiesAndFixesUpDeferred(t *testing.T) {
	dir := writeApplyCorpus(t, map[string]string{
		"block.uda":  applyBlockUdaWithChild(),
		"child.uda":  applyChildDoctypeUda,
		"dt.uda":     statusDataTypeUda,
		"folder.uda": applyFolderUda,
	})
	backupDir := filepath.Join(t.TempDir(), "bk")
	rec := &applyRecorder{}
	output, err := execute(buildDeployRoot(applyDeps(t, rec)), "deploy", "apply", "--uda-dir", dir, "--force", "--backup-dir", backupDir)
	if err != nil {
		t.Fatalf("apply failed: %v\n%s", err, output)
	}
	writes := []string{}
	for _, r := range rec.requests {
		if strings.HasPrefix(r, "POST") || strings.HasPrefix(r, "PUT") {
			writes = append(writes, r)
		}
	}
	want := []string{
		"POST /document-type/folder",
		"PUT /data-type/aaaaaaaa-1111-2222-3333-444444444444",
		"POST /document-type",
		"POST /document-type",
		"PUT /document-type/bbbbbbbb-1111-2222-3333-444444444444", // fix-up with the deferred child
	}
	if strings.Join(writes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected write sequence:\n%s", strings.Join(writes, "\n"))
	}
	fixup := rec.bodies["PUT /document-type/bbbbbbbb-1111-2222-3333-444444444444"]
	if len(fixup["allowedDocumentTypes"].([]any)) != 1 {
		t.Fatalf("expected fix-up to restore the allowed child, got %v", fixup["allowedDocumentTypes"])
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "data-type-aaaaaaaa") {
		t.Fatalf("expected one backup for the updated data type, got %v", entries)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(output), &payload)
	summary := payload["summary"].(map[string]any)
	if summary["applied"] != float64(4) || summary["failed"] != float64(0) {
		t.Fatalf("unexpected summary: %v", summary)
	}
}

func TestDeployApplyStopsOnFirstFailureAndExits4(t *testing.T) {
	dir := writeApplyCorpus(t, map[string]string{"dt.uda": statusDataTypeUda, "folder.uda": applyFolderUda})
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		path := strings.TrimPrefix(req.URL.Path, "/umbraco/management/api/v1")
		switch {
		case path == "/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"t","expires_in":3600}`), nil
		case path == "/server/status":
			return datatypeJSONResponse(http.StatusOK, `{}`), nil
		case req.Method == http.MethodPost:
			return datatypeJSONResponse(http.StatusBadRequest, `{"title":"nope"}`), nil
		case path == "/data-type/aaaaaaaa-1111-2222-3333-444444444444":
			return datatypeJSONResponse(http.StatusOK, `{"name":"Old","editorAlias":"Umbraco.TextArea","editorUiAlias":"Umb.PropertyEditorUi.TextArea","values":[]}`), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	output, err := execute(buildDeployRoot(deps), "deploy", "apply", "--uda-dir", dir, "--force", "--no-backup")
	if err == nil {
		t.Fatalf("expected failure exit")
	}
	var coder interface{ ExitCode() int }
	if !errorsAs(err, &coder) || coder.ExitCode() != 4 {
		t.Fatalf("expected exit 4, got %v", err)
	}
	if !strings.Contains(output, `"result": "failed"`) || !strings.Contains(output, `"result": "not-run"`) {
		t.Fatalf("expected failed + not-run entries, got %s", output)
	}
}

func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
