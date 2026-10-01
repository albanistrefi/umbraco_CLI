package forms

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

// Forms write commands: every mutation must target the Forms Management API
// mount, honour --dry-run, and gate deletes. The fake below answers a fixed
// route table and records every non-token request so tests can assert on
// the exact method, path and body that would reach the server.

const formsTestPrefix = "/umbraco/forms/management/api/v1"

type formsFakeResponse struct {
	status   int
	body     string
	location string
}

type formsFakeRequest struct {
	method string
	path   string
	body   any
}

type formsFake struct {
	mu       sync.Mutex
	routes   map[string]formsFakeResponse
	requests []formsFakeRequest
}

func newFormsFake(routes map[string]formsFakeResponse) *formsFake {
	return &formsFake{routes: routes}
}

func (f *formsFake) deps() cmdkit.Dependencies {
	return cmdtest.ClientDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		var body any
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &body)
			}
		}
		path := strings.TrimPrefix(req.URL.Path, formsTestPrefix)
		f.mu.Lock()
		f.requests = append(f.requests, formsFakeRequest{method: req.Method, path: path, body: body})
		f.mu.Unlock()
		if !strings.HasPrefix(req.URL.Path, formsTestPrefix) {
			return cmdtest.JSONResponse(http.StatusNotFound, `{"title":"wrong API mount"}`), nil
		}
		response, ok := f.routes[req.Method+" "+path]
		if !ok {
			return cmdtest.JSONResponse(http.StatusNotFound, `{"title":"Not found"}`), nil
		}
		resp := cmdtest.JSONResponse(response.status, response.body)
		if response.location != "" {
			resp.Header.Set("Location", response.location)
		}
		return resp, nil
	})
}

// sent returns the recorded requests with the given method.
func (f *formsFake) sent(method string) []formsFakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	matches := []formsFakeRequest{}
	for _, request := range f.requests {
		if request.method == method {
			matches = append(matches, request)
		}
	}
	return matches
}

func (f *formsFake) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return cmdtest.Execute(buildFormsRoot(t, f.deps()), args...)
}

func decodeObject(t *testing.T, output string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode output %q: %v", output, err)
	}
	return payload
}

const (
	formsTestFormID   = "aaaa0001-bbbb-4ccc-8ddd-000000000001"
	formsTestFolderID = "aaaa0002-bbbb-4ccc-8ddd-000000000002"
	formsTestOtherID  = "aaaa0003-bbbb-4ccc-8ddd-000000000003"
)

const formsTestScaffold = `{"id":"` + formsTestFormID + `","unique":"` + formsTestFormID + `","name":"","folderId":null,
	"pages":[{"caption":null,"fieldSets":[]}],
	"formWorkflows":{"onSubmit":[{"id":"00000000-0000-0000-0000-000000000000","name":"Default email"}],"onApprove":[],"onReject":[]}}`

func TestFormsCreateMergesOntoScaffold(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /form/scaffold": {status: http.StatusOK, body: formsTestScaffold},
		"POST /form":         {status: http.StatusCreated, body: ``, location: "https://example.test" + formsTestPrefix + "/form/" + formsTestFormID},
	})
	output, err := fake.run(t, "forms", "create", "--json", `{"name":"Contact","folderId":"`+formsTestFolderID+`","formWorkflows":{"onSubmit":[]}}`)
	if err != nil {
		t.Fatalf("forms create failed: %v", err)
	}
	posts := fake.sent(http.MethodPost)
	if len(posts) != 1 || posts[0].path != "/form" {
		t.Fatalf("expected one POST /form under the Forms prefix, got %+v", posts)
	}
	body := posts[0].body.(map[string]any)
	if body["name"] != "Contact" || body["folderId"] != formsTestFolderID || body["id"] != formsTestFormID {
		t.Fatalf("expected the scaffold id with the caller's name and folder, got %+v", body)
	}
	if pages, _ := body["pages"].([]any); len(pages) != 1 {
		t.Fatalf("expected the scaffold page to survive the merge, got %+v", body["pages"])
	}
	workflows := body["formWorkflows"].(map[string]any)
	if onSubmit, _ := workflows["onSubmit"].([]any); len(onSubmit) != 0 {
		t.Fatalf("expected --json to replace the default workflows, got %+v", workflows)
	}
	if decodeObject(t, output)["id"] != formsTestFormID {
		t.Fatalf("expected the created id in the output, got %s", output)
	}
}

func TestFormsCreateRequiresNameAndBindsCallerID(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /form/scaffold": {status: http.StatusOK, body: formsTestScaffold},
	})
	if _, err := fake.run(t, "forms", "create"); err == nil || !strings.Contains(err.Error(), "requires a form name") {
		t.Fatalf("expected a missing-name error, got %v", err)
	}
	if posts := fake.sent(http.MethodPost); len(posts) != 0 {
		t.Fatalf("expected no POST without a name, got %+v", posts)
	}

	output, err := fake.run(t, "forms", "create", "--dry-run", "--json", `{"name":"Contact","id":"`+formsTestOtherID+`"}`)
	if err != nil {
		t.Fatalf("forms create --dry-run failed: %v", err)
	}
	plan := decodeObject(t, output)
	planned := plan["body"].(map[string]any)
	if planned["id"] != formsTestOtherID || planned["unique"] != formsTestOtherID {
		t.Fatalf("expected the caller's id mirrored into unique, got %+v", planned)
	}
	if !strings.HasSuffix(cmdkit.AsString(plan["path"]), formsTestPrefix+"/form") {
		t.Fatalf("expected the dry-run plan on the Forms prefix, got %+v", plan["path"])
	}
}

func TestFormsCreatePrintTemplate(t *testing.T) {
	fake := newFormsFake(nil)
	output, err := fake.run(t, "forms", "create", "--print-template")
	if err != nil {
		t.Fatalf("forms create --print-template failed: %v", err)
	}
	if !strings.Contains(output, `"folderId"`) || len(fake.sent(http.MethodGet)) != 0 {
		t.Fatalf("expected an offline template naming folderId, got %s", output)
	}
}

func TestFormsUpdateBindsPathID(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /form/" + formsTestFormID: {status: http.StatusOK, body: `{"id":"` + formsTestFormID + `","name":"Old","folderId":"` + formsTestFolderID + `","pages":[{}]}`},
		"PUT /form/" + formsTestFormID: {status: http.StatusOK, body: ``},
	})
	output, err := fake.run(t, "forms", "update", formsTestFormID, "--json", `{"name":"Full replacement"}`)
	if err != nil {
		t.Fatalf("forms update --json failed: %v", err)
	}
	if decodeObject(t, output)["updated"] != true {
		t.Fatalf("expected {updated:true}, got %s", output)
	}
	puts := fake.sent(http.MethodPut)
	if len(puts) != 1 || puts[0].body.(map[string]any)["id"] != formsTestFormID {
		t.Fatalf("expected the path id filled into the body (an id-less PUT creates a new form), got %+v", puts)
	}

	if _, err := fake.run(t, "forms", "update", formsTestFormID, "--merge-json", `{"name":"New"}`); err != nil {
		t.Fatalf("forms update --merge-json failed: %v", err)
	}
	merged := fake.sent(http.MethodPut)[1].body.(map[string]any)
	if merged["name"] != "New" || merged["folderId"] != formsTestFolderID || merged["id"] != formsTestFormID {
		t.Fatalf("expected the merge to keep folderId and id, got %+v", merged)
	}

	_, err = fake.run(t, "forms", "update", formsTestFormID, "--json", `{"id":"`+formsTestOtherID+`","name":"X"}`)
	if err == nil || !strings.Contains(err.Error(), "does not match the id argument") {
		t.Fatalf("expected a mismatching body id to be refused, got %v", err)
	}
	if len(fake.sent(http.MethodPut)) != 2 {
		t.Fatalf("expected no PUT for the mismatching id")
	}
}

func TestFormsDeleteIsForceGated(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"DELETE /form/" + formsTestFormID: {status: http.StatusOK, body: ``},
	})
	if _, err := fake.run(t, "forms", "delete", formsTestFormID); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the force gate, got %v", err)
	}
	if len(fake.sent(http.MethodDelete)) != 0 {
		t.Fatalf("expected no DELETE without --force")
	}
	output, err := fake.run(t, "forms", "delete", formsTestFormID, "--force")
	if err != nil {
		t.Fatalf("forms delete --force failed: %v", err)
	}
	if decodeObject(t, output)["deleted"] != true || len(fake.sent(http.MethodDelete)) != 1 {
		t.Fatalf("expected one DELETE and {deleted:true}, got %s", output)
	}
}

func TestFormsCopyBuildsBodyFromFlags(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"POST /form/" + formsTestFormID + "/copy": {status: http.StatusCreated, body: ``, location: "https://example.test" + formsTestPrefix + "/form/" + formsTestOtherID},
	})
	output, err := fake.run(t, "forms", "copy", formsTestFormID, "--name", "Copy", "--to", formsTestFolderID, "--copy-workflows")
	if err != nil {
		t.Fatalf("forms copy failed: %v", err)
	}
	body := fake.sent(http.MethodPost)[0].body.(map[string]any)
	if body["newName"] != "Copy" || body["copyToFolderId"] != formsTestFolderID || body["copyWorkflows"] != true {
		t.Fatalf("unexpected copy body %+v", body)
	}
	result := decodeObject(t, output)
	if result["id"] != formsTestOtherID || result["sourceId"] != formsTestFormID || result["copied"] != true || result["name"] != "Copy" {
		t.Fatalf("expected the new id and source in the result, got %+v", result)
	}

	if _, err := fake.run(t, "forms", "copy", formsTestFormID, "--json", `{"copyWorkflows":false}`, "--name", "X"); err == nil {
		t.Fatalf("expected --json combined with --name to be refused")
	}
	if _, err := fake.run(t, "forms", "copy", formsTestFormID, "--to", "not-a-guid"); err == nil {
		t.Fatalf("expected a non-GUID --to to be refused")
	}
}

func TestFormsMoveBodies(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"PUT /form/" + formsTestFormID + "/move":     {status: http.StatusOK, body: ``},
		"PUT /folder/" + formsTestFolderID + "/move": {status: http.StatusOK, body: ``},
	})
	output, err := fake.run(t, "forms", "move", formsTestFormID, "--to-root")
	if err != nil {
		t.Fatalf("forms move --to-root failed: %v", err)
	}
	if decodeObject(t, output)["moved"] != true {
		t.Fatalf("expected {moved:true}, got %s", output)
	}
	if _, err := fake.run(t, "forms", "move-folder", formsTestFolderID, "--to", formsTestOtherID); err != nil {
		t.Fatalf("forms move-folder failed: %v", err)
	}
	puts := fake.sent(http.MethodPut)
	root := puts[0].body.(map[string]any)
	if value, ok := root["parentId"]; !ok || value != nil {
		t.Fatalf("expected parentId null for --to-root, got %+v", root)
	}
	if puts[1].body.(map[string]any)["parentId"] != formsTestOtherID {
		t.Fatalf("expected parentId from --to, got %+v", puts[1].body)
	}
	for _, args := range [][]string{
		{"forms", "move", formsTestFormID},
		{"forms", "move", formsTestFormID, "--to", formsTestFolderID, "--to-root"},
		{"forms", "move", formsTestFormID, "--to", "folder-name"},
		{"forms", "move", formsTestFormID, "--json", `{}`, "--dry-run"},
		{"forms", "move-folder", formsTestFolderID, "--json", `{"parentId":"folder-name"}`, "--dry-run"},
		{"forms", "move-folder", formsTestFolderID, "--json", `{"parentId":7}`, "--dry-run"},
	} {
		if _, err := fake.run(t, args...); err == nil {
			t.Fatalf("expected %v to be refused", args)
		}
	}
	if len(fake.sent(http.MethodPut)) != 2 {
		t.Fatalf("refused moves must not send a request, got %d PUTs", len(fake.sent(http.MethodPut)))
	}
	if _, err := fake.run(t, "forms", "move", formsTestFormID, "--json", `{"parentId":null}`); err != nil {
		t.Fatalf("an explicit null parentId must still move to the root: %v", err)
	}
	if body := fake.sent(http.MethodPut)[2].body.(map[string]any); body["parentId"] != nil {
		t.Fatalf("expected the raw null parentId sent, got %+v", body)
	}
}

func TestFormsCopyWorkflowsBody(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"POST /form/" + formsTestFormID + "/copy-workflows": {status: http.StatusOK, body: ``},
	})
	output, err := fake.run(t, "forms", "copy-workflows", formsTestFormID, "--to", formsTestOtherID, "--workflow-ids", formsTestFolderID)
	if err != nil {
		t.Fatalf("forms copy-workflows failed: %v", err)
	}
	body := fake.sent(http.MethodPost)[0].body.(map[string]any)
	ids, _ := body["workflowIds"].([]any)
	if body["destinationId"] != formsTestOtherID || len(ids) != 1 || ids[0] != formsTestFolderID {
		t.Fatalf("unexpected copy-workflows body %+v", body)
	}
	if decodeObject(t, output)["copied"] != true {
		t.Fatalf("expected {copied:true}, got %s", output)
	}
	if _, err := fake.run(t, "forms", "copy-workflows", formsTestFormID, "--workflow-ids", formsTestFolderID); err == nil {
		t.Fatalf("expected a missing --to to be refused")
	}
	if _, err := fake.run(t, "forms", "copy-workflows", formsTestFormID, "--to", formsTestOtherID); err == nil {
		t.Fatalf("expected missing --workflow-ids to be refused")
	}
}
