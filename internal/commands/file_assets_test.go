package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func buildFileAssetRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterPartialView(root, deps)
	RegisterScript(root, deps)
	RegisterStylesheet(root, deps)
	RegisterStaticFile(root, deps)
	return root
}

func fileAssetDeps(handler func(req *http.Request) (*http.Response, error)) Dependencies {
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

// requestURI captures the raw request target, because the whole point of
// the path encoding is what goes on the wire — a decoded URL.Path would
// hide the difference between the catch-all and rename encodings.
func captureURI(observed *string, body string) func(req *http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		*observed = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, body), nil
	}
}

func TestFileAssetGetEncodesPathWithSeparatorsIntact(t *testing.T) {
	for _, tc := range []struct {
		group    string
		resource string
	}{
		{group: "partial-view", resource: "partial-view"},
		{group: "script", resource: "script"},
		{group: "stylesheet", resource: "stylesheet"},
	} {
		var observed string
		deps := fileAssetDeps(captureURI(&observed, `{"name":"a b.cshtml","path":"/My Folder/a b.cshtml","content":"x"}`))

		if _, err := execute(buildFileAssetRoot(deps), tc.group, "get", "/My Folder/Nested/a b.cshtml"); err != nil {
			t.Fatalf("%s get failed: %v", tc.group, err)
		}
		want := "/umbraco/management/api/v1/" + tc.resource + "/My%20Folder/Nested/a%20b.cshtml"
		if observed != want {
			t.Fatalf("%s get: expected %q, got %q", tc.group, want, observed)
		}
	}
}

func TestFileAssetRenameSendsPathAsOneDoublyEscapedSegment(t *testing.T) {
	// The rename route is not a catch-all: Kestrel decodes percent-escapes
	// once before routing, so the separators must be encoded twice to reach
	// the model binder as a single segment. Verified against Umbraco 18.1.
	var observed string
	var body map[string]any
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		observed = req.URL.RequestURI()
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode rename body: %v", err)
		}
		return endpointJSONResponse(http.StatusCreated, `{"id":"new.cshtml"}`), nil
	})

	if _, err := execute(buildFileAssetRoot(deps), "partial-view", "rename", "/My Folder/a b.cshtml", "--name", "new.cshtml"); err != nil {
		t.Fatalf("partial-view rename failed: %v", err)
	}
	want := "/umbraco/management/api/v1/partial-view/My%2520Folder%252Fa%2520b.cshtml/rename"
	if observed != want {
		t.Fatalf("expected rename path %q, got %q", want, observed)
	}
	if body["name"] != "new.cshtml" {
		t.Fatalf("expected rename body {name}, got %+v", body)
	}
	if _, ok := body["content"]; ok {
		t.Fatalf("rename body must carry name only, got %+v", body)
	}
}

func TestFileAssetChildrenSendsParentPathQueryAndRootShortcut(t *testing.T) {
	var observed string
	deps := fileAssetDeps(captureURI(&observed, `{"items":[],"total":0}`))
	root := buildFileAssetRoot(deps)

	if _, err := execute(root, "stylesheet", "children", "/My Folder"); err != nil {
		t.Fatalf("stylesheet children failed: %v", err)
	}
	if !strings.Contains(observed, "parentPath=%2FMy+Folder") {
		t.Fatalf("expected parentPath query, got %q", observed)
	}

	if _, err := execute(root, "stylesheet", "children", "/"); err != nil {
		t.Fatalf("stylesheet children / failed: %v", err)
	}
	if !strings.Contains(observed, "parentPath=%2F") {
		t.Fatalf("expected root parentPath, got %q", observed)
	}
}

func TestFileAssetCreateBuildsBodyAndReportsFullPath(t *testing.T) {
	var body map[string]any
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		return endpointJSONResponse(http.StatusCreated, `{"id":"app.js"}`), nil
	})

	out, err := execute(buildFileAssetRoot(deps), "script", "create", "--path", "/vendor", "--name", "app.js", "--content", "console.log(1);")
	if err != nil {
		t.Fatalf("script create failed: %v", err)
	}
	if body["name"] != "app.js" || body["content"] != "console.log(1);" {
		t.Fatalf("unexpected create body: %+v", body)
	}
	parent, ok := body["parent"].(map[string]any)
	if !ok || parent["path"] != "/vendor" {
		t.Fatalf("expected parent {path:/vendor}, got %+v", body)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode create result: %v", err)
	}
	if payload["path"] != "/vendor/app.js" {
		t.Fatalf("create must report the full path, got %+v", payload)
	}
}

func TestFileAssetCreateAtRootOmitsParent(t *testing.T) {
	var body map[string]any
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		return endpointJSONResponse(http.StatusCreated, ``), nil
	})

	if _, err := execute(buildFileAssetRoot(deps), "partial-view", "create-folder", "--path", "/", "--name", "Blog"); err != nil {
		t.Fatalf("partial-view create-folder failed: %v", err)
	}
	if _, ok := body["parent"]; ok {
		t.Fatalf("root create must omit parent, got %+v", body)
	}
	if body["name"] != "Blog" {
		t.Fatalf("unexpected create-folder body: %+v", body)
	}
}

func TestFileAssetCreateAndUpdateReadContentFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "site.css")
	content := "body {\n  color: \"red\";\n}\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var bodies []map[string]any
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		bodies = append(bodies, body)
		return endpointJSONResponse(http.StatusOK, ``), nil
	})
	root := buildFileAssetRoot(deps)

	if _, err := execute(root, "stylesheet", "create", "--path", "/", "--name", "site.css", "--content-file", source); err != nil {
		t.Fatalf("stylesheet create failed: %v", err)
	}
	if _, err := execute(root, "stylesheet", "update", "/site.css", "--content-file", source); err != nil {
		t.Fatalf("stylesheet update failed: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected two requests, got %d", len(bodies))
	}
	if bodies[0]["content"] != content {
		t.Fatalf("create --content-file must send the file verbatim, got %q", bodies[0]["content"])
	}
	if bodies[1]["content"] != content || len(bodies[1]) != 1 {
		t.Fatalf("update body must carry content only, got %+v", bodies[1])
	}
}

func TestFileAssetContentFlagsAreMutuallyExclusiveAndRequired(t *testing.T) {
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request expected, got %s", req.URL.Path)
		return nil, nil
	})
	root := buildFileAssetRoot(deps)

	_, err := execute(root, "script", "create", "--path", "/", "--name", "a.js", "--content", "x", "--content-file", "a.js")
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected --content/--content-file conflict, got %v", err)
	}

	_, err = execute(root, "script", "update", "/a.js")
	if err == nil || !strings.Contains(err.Error(), "requires --content or --content-file") {
		t.Fatalf("expected missing content error, got %v", err)
	}
}

func TestFileAssetGetOutWritesContentVerbatim(t *testing.T) {
	content := "@inherits Umbraco\n<p>hi</p>\n"
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		payload, err := json.Marshal(map[string]any{"name": "a.cshtml", "path": "/a.cshtml", "content": content})
		if err != nil {
			t.Fatalf("marshal fixture: %v", err)
		}
		return endpointJSONResponse(http.StatusOK, string(payload)), nil
	})

	out := filepath.Join(t.TempDir(), "nested", "a.cshtml")
	result, err := execute(buildFileAssetRoot(deps), "partial-view", "get", "/a.cshtml", "--out", out)
	if err != nil {
		t.Fatalf("partial-view get --out failed: %v", err)
	}
	saved, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(saved) != content {
		t.Fatalf("expected the content verbatim, got %q", saved)
	}
	if strings.Contains(result, "inherits") {
		t.Fatalf("--out must print a summary, not the body: %s", result)
	}
}

func TestFileAssetDeletesAreForceGated(t *testing.T) {
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request expected, got %s", req.URL.Path)
		return nil, nil
	})
	root := buildFileAssetRoot(deps)

	for _, args := range [][]string{
		{"partial-view", "delete", "/a.cshtml"},
		{"partial-view", "delete-folder", "/Blog"},
		{"script", "delete", "/a.js"},
		{"stylesheet", "delete-folder", "/theme"},
	} {
		_, err := execute(root, args...)
		if err == nil || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("%v: expected a force gate, got %v", args, err)
		}
	}
}

func TestFileAssetDeleteFolderUsesFolderRoute(t *testing.T) {
	var observed string
	var method string
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		observed = req.URL.RequestURI()
		method = req.Method
		return endpointJSONResponse(http.StatusOK, ``), nil
	})

	if _, err := execute(buildFileAssetRoot(deps), "partial-view", "delete-folder", "/My Folder", "--force"); err != nil {
		t.Fatalf("delete-folder failed: %v", err)
	}
	if method != http.MethodDelete || observed != "/umbraco/management/api/v1/partial-view/folder/My%20Folder" {
		t.Fatalf("unexpected delete-folder request: %s %s", method, observed)
	}
}

func TestFileAssetPathsRejectTraversal(t *testing.T) {
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request expected, got %s", req.URL.Path)
		return nil, nil
	})
	root := buildFileAssetRoot(deps)

	for _, args := range [][]string{
		{"script", "get", "/../appsettings.json"},
		{"script", "delete", "/vendor/../../secret.js", "--force"},
		{"script", "children", "/.."},
	} {
		_, err := execute(root, args...)
		if err == nil || !strings.Contains(err.Error(), "relative segments") {
			t.Fatalf("%v: expected a traversal rejection, got %v", args, err)
		}
	}
}

func TestStaticFileGroupIsReadOnly(t *testing.T) {
	root := buildFileAssetRoot(makeDeps())
	group := findChildCommand(root, "static-file")
	if group == nil {
		t.Fatalf("static-file group not registered")
	}
	names := map[string]bool{}
	for _, child := range group.Commands() {
		names[child.Name()] = true
	}
	for _, want := range []string{"list", "children", "get"} {
		if !names[want] {
			t.Fatalf("static-file is missing %q", want)
		}
	}
	for _, forbidden := range []string{"create", "update", "rename", "delete", "create-folder", "delete-folder"} {
		if names[forbidden] {
			t.Fatalf("static-file must not expose %q: the Management API has no write side for static files", forbidden)
		}
	}
	if len(names) != 3 {
		t.Fatalf("expected exactly list/children/get on static-file, got %v", names)
	}
}

func TestStaticFileGetUsesItemEndpoint(t *testing.T) {
	var observed string
	deps := fileAssetDeps(captureURI(&observed, `[{"name":"RTE.css","path":"/wwwroot/css/RTE.css","isFolder":false}]`))

	if _, err := execute(buildFileAssetRoot(deps), "static-file", "get", "/wwwroot/css/RTE.css"); err != nil {
		t.Fatalf("static-file get failed: %v", err)
	}
	want := "/umbraco/management/api/v1/item/static-file?path=%2Fwwwroot%2Fcss%2FRTE.css"
	if observed != want {
		t.Fatalf("expected %q, got %q", want, observed)
	}
}

func TestStaticFileGetUnwrapsTheMatchingItem(t *testing.T) {
	// /item/static-file takes a repeatable path parameter and answers with
	// an array. A get of one path must return that entry, not a
	// one-element array, so --fields and downstream parsing see an object.
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `[
			{"name":"other.css","path":"/wwwroot/css/other.css","isFolder":false},
			{"name":"RTE.css","path":"/wwwroot/css/RTE.css","isFolder":false}
		]`), nil
	})

	out, err := execute(buildFileAssetRoot(deps), "static-file", "get", "/wwwroot/css/RTE.css", "--fields", "name")
	if err != nil {
		t.Fatalf("static-file get failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("expected a single object, got %s (%v)", out, err)
	}
	if payload["name"] != "RTE.css" {
		t.Fatalf("expected the requested entry, got %+v", payload)
	}
	if _, ok := payload["path"]; ok {
		t.Fatalf("--fields must project the unwrapped item, got %+v", payload)
	}
}

func TestStaticFileGetReportsMissingPath(t *testing.T) {
	// An unknown path answers 200 with [], which must not read as success.
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `[]`), nil
	})

	out, err := execute(buildFileAssetRoot(deps), "static-file", "get", "/wwwroot/css/nope.css")
	if err == nil {
		t.Fatalf("expected a not-found error, got output %s", out)
	}
	if !strings.Contains(err.Error(), "static file /wwwroot/css/nope.css not found") {
		t.Fatalf("expected the path in the error, got %v", err)
	}
}

func TestFileAssetPathsRejectBackslashes(t *testing.T) {
	// Only "/" is split on, so a backslash would sneak ".." past the
	// relative-segment check on a Windows-hosted instance.
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request expected, got %s", req.URL.Path)
		return nil, nil
	})
	root := buildFileAssetRoot(deps)

	for _, args := range [][]string{
		{"script", "get", `/vendor\..\secret.js`},
		{"partial-view", "delete", `/Blog\..\..\appsettings.json`, "--force"},
		{"stylesheet", "children", `/theme\..`},
		{"static-file", "get", `/wwwroot\..\appsettings.json`},
	} {
		_, err := execute(root, args...)
		if err == nil || !strings.Contains(err.Error(), "backslashes are not allowed") {
			t.Fatalf("%v: expected a backslash rejection, got %v", args, err)
		}
	}
}

func TestFileAssetSnippetsOnlyOnPartialView(t *testing.T) {
	root := buildFileAssetRoot(makeDeps())
	for _, group := range []string{"script", "stylesheet", "static-file"} {
		command := findChildCommand(root, group)
		if command == nil {
			t.Fatalf("%s group not registered", group)
		}
		for _, child := range command.Commands() {
			if child.Name() == "snippets" || child.Name() == "snippet" {
				t.Fatalf("%s must not expose %q: snippets are a partial-view-only catalogue", group, child.Name())
			}
		}
	}
	partialView := findChildCommand(root, "partial-view")
	found := 0
	for _, child := range partialView.Commands() {
		if child.Name() == "snippets" || child.Name() == "snippet" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected partial-view snippets and snippet, found %d", found)
	}
}

func TestFileAssetMutationsSupportDryRun(t *testing.T) {
	deps := fileAssetDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("dry run must not issue a request, got %s %s", req.Method, req.URL.Path)
		return nil, nil
	})
	root := buildFileAssetRoot(deps)

	for _, args := range [][]string{
		{"partial-view", "create", "--path", "/", "--name", "a.cshtml", "--content", "x", "--dry-run"},
		{"partial-view", "update", "/a.cshtml", "--content", "x", "--dry-run"},
		{"partial-view", "rename", "/a.cshtml", "--name", "b.cshtml", "--dry-run"},
		{"partial-view", "delete", "/a.cshtml", "--dry-run"},
		{"partial-view", "create-folder", "--path", "/", "--name", "Blog", "--dry-run"},
		{"partial-view", "delete-folder", "/Blog", "--dry-run"},
	} {
		out, err := execute(root, args...)
		if err != nil {
			t.Fatalf("%v: dry run failed: %v", args, err)
		}
		if !strings.Contains(out, `"dryRun": true`) {
			t.Fatalf("%v: expected a dry-run plan, got %s", args, out)
		}
	}
}
