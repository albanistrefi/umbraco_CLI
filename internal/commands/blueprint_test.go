package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const blueprintScaffoldBody = `{
	"documentType": {"collection": null, "icon": "icon-document", "id": "dt-1"},
	"flags": [],
	"id": "scaffold-1",
	"values": [
		{"alias": "title", "culture": null, "editorAlias": "Umbraco.TextBox", "segment": null, "value": "Preset title"},
		{"alias": "body", "culture": null, "editorAlias": "Umbraco.TextBox", "segment": null, "value": "Preset body"}
	],
	"variants": [
		{"createDate": "2026-01-01T00:00:00+00:00", "culture": null, "flags": [], "id": "00000000-0000-0000-0000-000000000000", "name": "Preset name", "segment": null, "state": "Draft", "updateDate": "2026-01-01T00:00:00+00:00"}
	]
}`

func buildBlueprintRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterBlueprint(root, deps)
	RegisterDocument(root, deps)
	return root
}

// blueprintRecorder answers every non-token request with the supplied body
// and records the request line and payload of the last call.
type blueprintRecorder struct {
	method string
	path   string
	query  string
	body   string
}

func blueprintDeps(t *testing.T, recorder *blueprintRecorder, responses map[string]string) Dependencies {
	t.Helper()
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		recorder.method = req.Method
		recorder.path = req.URL.Path
		recorder.query = req.URL.RawQuery
		if req.Body != nil {
			payload, _ := io.ReadAll(req.Body)
			recorder.body = string(payload)
		}
		if response, ok := responses[req.URL.Path]; ok {
			return endpointJSONResponse(http.StatusOK, response), nil
		}
		return endpointJSONResponse(http.StatusOK, `null`), nil
	})
}

func TestBlueprintListHitsTreeRoot(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/tree/document-blueprint/root": `{"items":[],"total":0}`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "list", "--take", "5"); err != nil {
		t.Fatalf("blueprint list failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/tree/document-blueprint/root" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	if !strings.Contains(recorder.query, "take=5") {
		t.Fatalf("expected pagination in query, got %q", recorder.query)
	}
}

func TestBlueprintChildrenPassesParentID(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/tree/document-blueprint/children": `{"items":[],"total":0}`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "children", "folder-1"); err != nil {
		t.Fatalf("blueprint children failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/tree/document-blueprint/children" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	if !strings.Contains(recorder.query, "parentId=folder-1") {
		t.Fatalf("expected parentId in query, got %q", recorder.query)
	}
}

func TestBlueprintAncestorsPassesDescendantID(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/tree/document-blueprint/ancestors": `[]`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "ancestors", "bp-1"); err != nil {
		t.Fatalf("blueprint ancestors failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/tree/document-blueprint/ancestors" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	if !strings.Contains(recorder.query, "descendantId=bp-1") {
		t.Fatalf("expected descendantId in query, got %q", recorder.query)
	}
}

func TestBlueprintSiblingsWindowsAroundTheTarget(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/tree/document-blueprint/siblings": `{"totalBefore":0,"totalAfter":0,"items":[]}`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "siblings", "bp-1"); err != nil {
		t.Fatalf("blueprint siblings failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/tree/document-blueprint/siblings" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	// The route is windowed, not paginated: the id travels as target and
	// both sides of the window are always sent.
	for _, expected := range []string{"target=bp-1", "before=10", "after=10"} {
		if !strings.Contains(recorder.query, expected) {
			t.Fatalf("expected %q in the siblings query, got %q", expected, recorder.query)
		}
	}
	if strings.Contains(recorder.query, "foldersOnly") {
		t.Fatalf("expected foldersOnly to stay off by default, got %q", recorder.query)
	}

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "siblings", "bp-1", "--before", "2", "--after", "3", "--folders-only"); err != nil {
		t.Fatalf("blueprint siblings with a window failed: %v", err)
	}
	for _, expected := range []string{"before=2", "after=3", "foldersOnly=true"} {
		if !strings.Contains(recorder.query, expected) {
			t.Fatalf("expected %q in the siblings query, got %q", expected, recorder.query)
		}
	}
}

func TestBlueprintItemsSendsRepeatedIDs(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/item/document-blueprint": `[]`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "items", "--ids", "bp-1,bp-2,bp-1"); err != nil {
		t.Fatalf("blueprint items failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/item/document-blueprint" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	if strings.Count(recorder.query, "id=") != 2 {
		t.Fatalf("expected two deduplicated repeated id values, got %q", recorder.query)
	}
	for _, expected := range []string{"id=bp-1", "id=bp-2"} {
		if !strings.Contains(recorder.query, expected) {
			t.Fatalf("expected %q in the items query, got %q", expected, recorder.query)
		}
	}
}

func TestBlueprintItemsRequiresIDs(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "items"); err == nil {
		t.Fatal("expected blueprint items without --ids to fail")
	}
	if recorder.path != "" {
		t.Fatalf("expected no request, got %q", recorder.path)
	}
}

func TestBlueprintAuditLogPaginatesOverTheBlueprintRoute(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/document-blueprint/bp-1/audit-log": `{"items":[],"total":0}`,
	})

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "audit-log", "bp-1", "--take", "5", "--params", `{"orderDirection":"Ascending"}`); err != nil {
		t.Fatalf("blueprint audit-log failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/document-blueprint/bp-1/audit-log" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	for _, expected := range []string{"take=5", "orderDirection=Ascending"} {
		if !strings.Contains(recorder.query, expected) {
			t.Fatalf("expected %q in the audit-log query, got %q", expected, recorder.query)
		}
	}
}

func TestBlueprintCreatePostsPayloadWithGeneratedID(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(
		buildBlueprintRoot(deps),
		"blueprint", "create",
		"--json", `{"documentType":{"id":"dt-1"},"values":[],"variants":[{"name":"Preset"}]}`,
	); err != nil {
		t.Fatalf("blueprint create failed: %v", err)
	}
	if recorder.method != http.MethodPost || recorder.path != "/umbraco/management/api/v1/document-blueprint" {
		t.Fatalf("unexpected request %s %s", recorder.method, recorder.path)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(recorder.body), &body); err != nil {
		t.Fatalf("failed to parse create body: %v", err)
	}
	if id, _ := body["id"].(string); strings.TrimSpace(id) == "" {
		t.Fatalf("expected a CLI-generated id in %q", recorder.body)
	}
}

func TestBlueprintCreateFromDocumentBuildsRequestModel(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(
		buildBlueprintRoot(deps),
		"blueprint", "create-from-document", "doc-1",
		"--name", "zz probe preset",
		"--parent", "folder-1",
		"--id", "bp-1",
	); err != nil {
		t.Fatalf("blueprint create-from-document failed: %v", err)
	}
	if recorder.method != http.MethodPost || recorder.path != "/umbraco/management/api/v1/document-blueprint/from-document" {
		t.Fatalf("unexpected request %s %s", recorder.method, recorder.path)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(recorder.body), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	document, _ := body["document"].(map[string]any)
	parent, _ := body["parent"].(map[string]any)
	if document["id"] != "doc-1" || parent["id"] != "folder-1" || body["name"] != "zz probe preset" || body["id"] != "bp-1" {
		t.Fatalf("unexpected from-document body %q", recorder.body)
	}
}

func TestBlueprintCreateFromDocumentRequiresName(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "create-from-document", "doc-1"); err == nil {
		t.Fatal("expected create-from-document without --name to fail")
	}
	if recorder.path != "" {
		t.Fatalf("expected no request, got %q", recorder.path)
	}
}

func TestBlueprintDeleteRequiresForce(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "delete", "bp-1"); err == nil {
		t.Fatal("expected blueprint delete without --force to fail")
	}
	if recorder.path != "" {
		t.Fatalf("expected no request before the force gate, got %q", recorder.path)
	}

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "delete", "bp-1", "--force"); err != nil {
		t.Fatalf("blueprint delete --force failed: %v", err)
	}
	if recorder.method != http.MethodDelete || recorder.path != "/umbraco/management/api/v1/document-blueprint/bp-1" {
		t.Fatalf("unexpected request %s %s", recorder.method, recorder.path)
	}
}

func TestBlueprintDeleteFolderRequiresForce(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "delete-folder", "folder-1"); err == nil {
		t.Fatal("expected blueprint delete-folder without --force to fail")
	}

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "delete-folder", "folder-1", "--force"); err != nil {
		t.Fatalf("blueprint delete-folder --force failed: %v", err)
	}
	if recorder.method != http.MethodDelete || recorder.path != "/umbraco/management/api/v1/document-blueprint/folder/folder-1" {
		t.Fatalf("unexpected request %s %s", recorder.method, recorder.path)
	}
}

func TestBlueprintCreateFolderPostsAndReadsBack(t *testing.T) {
	var paths []string
	var createBody string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/umbraco/management/api/v1/document-blueprint/folder":
			paths = append(paths, req.Method+" "+req.URL.Path)
			payload, _ := io.ReadAll(req.Body)
			createBody = string(payload)
			return endpointJSONResponse(http.StatusCreated, `null`), nil
		default:
			paths = append(paths, req.Method+" "+req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"id":"11111111-1111-1111-1111-111111111111","name":"zz probe folder"}`), nil
		}
	})

	out, err := execute(
		buildBlueprintRoot(deps),
		"blueprint", "create-folder",
		"--name", "zz probe folder",
		"--id", "11111111-1111-1111-1111-111111111111",
	)
	if err != nil {
		t.Fatalf("blueprint create-folder failed: %v", err)
	}
	if len(paths) != 2 || paths[0] != "POST /umbraco/management/api/v1/document-blueprint/folder" {
		t.Fatalf("unexpected request sequence %v", paths)
	}
	if paths[1] != "GET /umbraco/management/api/v1/document-blueprint/folder/11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected the folder to be read back, got %v", paths)
	}
	if !strings.Contains(createBody, `"name":"zz probe folder"`) {
		t.Fatalf("unexpected folder body %q", createBody)
	}
	if !strings.Contains(out, `"created": true`) {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestBlueprintMoveUsesTargetShortcut(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "blueprint", "move", "bp-1", "--to", "folder-1"); err != nil {
		t.Fatalf("blueprint move failed: %v", err)
	}
	if recorder.method != http.MethodPut || recorder.path != "/umbraco/management/api/v1/document-blueprint/bp-1/move" {
		t.Fatalf("unexpected request %s %s", recorder.method, recorder.path)
	}
	if !strings.Contains(recorder.body, `"target":{"id":"folder-1"}`) {
		t.Fatalf("unexpected move body %q", recorder.body)
	}
}

func TestBlueprintScaffoldReadsScaffoldRoute(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, map[string]string{
		"/umbraco/management/api/v1/document-blueprint/bp-1/scaffold": blueprintScaffoldBody,
	})

	out, err := execute(buildBlueprintRoot(deps), "blueprint", "scaffold", "bp-1")
	if err != nil {
		t.Fatalf("blueprint scaffold failed: %v", err)
	}
	if recorder.path != "/umbraco/management/api/v1/document-blueprint/bp-1/scaffold" {
		t.Fatalf("unexpected path %q", recorder.path)
	}
	if !strings.Contains(out, "Preset title") {
		t.Fatalf("expected the scaffold values in the output, got %q", out)
	}
}

func TestDocumentCreateFromBlueprintMergesScaffold(t *testing.T) {
	var scaffoldRequested bool
	var createBody string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-blueprint/bp-1/scaffold":
			scaffoldRequested = true
			return endpointJSONResponse(http.StatusOK, blueprintScaffoldBody), nil
		default:
			payload, _ := io.ReadAll(req.Body)
			createBody = string(payload)
			return endpointJSONResponse(http.StatusCreated, `null`), nil
		}
	})

	if _, err := execute(
		buildBlueprintRoot(deps),
		"document", "create",
		"--from-blueprint", "bp-1",
		"--parent", "parent-1",
		"--json", `{"variants":[{"name":"zz probe from blueprint"}],"values":[{"alias":"title","value":"Overridden title"}]}`,
	); err != nil {
		t.Fatalf("document create --from-blueprint failed: %v", err)
	}
	if !scaffoldRequested {
		t.Fatal("expected the blueprint scaffold to be fetched")
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(createBody), &body); err != nil {
		t.Fatalf("failed to parse create body: %v", err)
	}
	documentType, _ := body["documentType"].(map[string]any)
	if documentType["id"] != "dt-1" || len(documentType) != 1 {
		t.Fatalf("expected documentType reduced to {id}, got %v", documentType)
	}
	if _, present := body["template"]; !present {
		t.Fatalf("expected the required template key to be seeded, got %q", createBody)
	}
	if _, present := body["flags"]; present {
		t.Fatalf("expected response-only flags to be dropped, got %q", createBody)
	}
	parent, _ := body["parent"].(map[string]any)
	if parent["id"] != "parent-1" {
		t.Fatalf("expected --parent to fill parent, got %v", body["parent"])
	}

	values, _ := body["values"].([]any)
	if len(values) != 2 {
		t.Fatalf("expected both scaffold values, got %v", values)
	}
	byAlias := map[string]map[string]any{}
	for _, value := range values {
		entry, _ := value.(map[string]any)
		alias, _ := entry["alias"].(string)
		byAlias[alias] = entry
	}
	if byAlias["title"]["value"] != "Overridden title" {
		t.Fatalf("expected --json to override the scaffold value, got %v", byAlias["title"])
	}
	if byAlias["body"]["value"] != "Preset body" {
		t.Fatalf("expected the untouched scaffold value to survive, got %v", byAlias["body"])
	}
	if _, present := byAlias["title"]["editorAlias"]; present {
		t.Fatalf("expected editorAlias to be stripped from values, got %v", byAlias["title"])
	}

	variants, _ := body["variants"].([]any)
	if len(variants) != 1 {
		t.Fatalf("expected one variant, got %v", variants)
	}
	variant, _ := variants[0].(map[string]any)
	if variant["name"] != "zz probe from blueprint" {
		t.Fatalf("expected --json to name the document, got %v", variant)
	}
	if _, present := variant["state"]; present {
		t.Fatalf("expected response-only variant fields to be dropped, got %v", variant)
	}
}

// Regression: --json on top of a variant blueprint's scaffold used to
// replace the whole variants array, so renaming one culture created a
// document missing every other culture.
func TestDocumentCreateFromBlueprintMergesVariantsPerCulture(t *testing.T) {
	const variantScaffoldBody = `{
	"documentType": {"id": "dt-1"},
	"values": [
		{"alias": "title", "culture": "en-US", "segment": null, "editorAlias": "Umbraco.TextBox", "value": "English preset"},
		{"alias": "title", "culture": "da-DK", "segment": null, "editorAlias": "Umbraco.TextBox", "value": "Danish preset"}
	],
	"variants": [
		{"culture": "en-US", "segment": null, "name": "English preset name", "state": "Draft"},
		{"culture": "da-DK", "segment": null, "name": "Danish preset name", "state": "Draft"}
	]
}`

	var createBody string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-blueprint/bp-1/scaffold":
			return endpointJSONResponse(http.StatusOK, variantScaffoldBody), nil
		default:
			payload, _ := io.ReadAll(req.Body)
			createBody = string(payload)
			return endpointJSONResponse(http.StatusCreated, `null`), nil
		}
	})

	if _, err := execute(
		buildBlueprintRoot(deps),
		"document", "create", "--from-blueprint", "bp-1",
		"--json", `{"variants":[{"culture":"da-DK","name":"zz probe Danish name"}]}`,
	); err != nil {
		t.Fatalf("document create --from-blueprint with a variant patch failed: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(createBody), &body); err != nil {
		t.Fatalf("failed to parse create body: %v", err)
	}
	variants, ok := body["variants"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("expected both cultures in the create payload, got %q", createBody)
	}
	byCulture := map[string]map[string]any{}
	for _, variant := range variants {
		object, _ := variant.(map[string]any)
		culture, _ := object["culture"].(string)
		byCulture[culture] = object
	}
	if byCulture["da-DK"]["name"] != "zz probe Danish name" {
		t.Fatalf("expected --json to rename the Danish variant, got %+v", byCulture["da-DK"])
	}
	if byCulture["en-US"]["name"] != "English preset name" {
		t.Fatalf("expected the English variant to survive the merge, got %+v", byCulture["en-US"])
	}
	// The scaffold normalisation still runs first, so response-only
	// variant fields never reach the create payload.
	if _, present := byCulture["en-US"]["state"]; present {
		t.Fatalf("expected response-only variant fields to stay dropped, got %+v", byCulture["en-US"])
	}
}

func TestDocumentCreateFromBlueprintKeepsScaffoldName(t *testing.T) {
	var createBody string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-blueprint/bp-1/scaffold":
			return endpointJSONResponse(http.StatusOK, blueprintScaffoldBody), nil
		default:
			payload, _ := io.ReadAll(req.Body)
			createBody = string(payload)
			return endpointJSONResponse(http.StatusCreated, `null`), nil
		}
	})

	// No --json at all: the scaffold alone must be a usable create payload.
	if _, err := execute(buildBlueprintRoot(deps), "document", "create", "--from-blueprint", "bp-1"); err != nil {
		t.Fatalf("document create --from-blueprint without --json failed: %v", err)
	}
	if !strings.Contains(createBody, `"name":"Preset name"`) {
		t.Fatalf("expected the scaffold variant name to be used, got %q", createBody)
	}
}

func TestDocumentCreateFromBlueprintRejectsNamelessScaffold(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-blueprint/bp-1/scaffold":
			return endpointJSONResponse(http.StatusOK, `{"documentType":{"id":"dt-1"},"values":[],"variants":[]}`), nil
		default:
			t.Errorf("unexpected request to %s", req.URL.Path)
			return endpointJSONResponse(http.StatusCreated, `null`), nil
		}
	})

	_, err := execute(buildBlueprintRoot(deps), "document", "create", "--from-blueprint", "bp-1")
	if err == nil {
		t.Fatal("expected a nameless scaffold to be rejected before the POST")
	}
	if !strings.Contains(err.Error(), "variants") {
		t.Fatalf("expected the error to name the missing variant name, got %v", err)
	}
}

// The scaffold fetch is a network round trip, so anything decidable from
// the command line alone must fail before it.
func TestDocumentCreateFromBlueprintValidatesLocallyBeforeFetching(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "malformed --json",
			args:     []string{"document", "create", "--from-blueprint", "bp-1", "--json", `{bad`},
			contains: "JSON",
		},
		{
			name:     "--culture without --publish",
			args:     []string{"document", "create", "--from-blueprint", "bp-1", "--culture", "en-US"},
			contains: "--culture requires --publish",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var requested []string
			deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
				requested = append(requested, req.URL.Path)
				return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
			})

			_, err := execute(buildBlueprintRoot(deps), testCase.args...)
			if err == nil {
				t.Fatal("expected the command to fail before any request")
			}
			if !strings.Contains(err.Error(), testCase.contains) {
				t.Fatalf("expected the error to mention %q, got %v", testCase.contains, err)
			}
			if len(requested) != 0 {
				t.Fatalf("expected no request, got %v", requested)
			}
		})
	}
}

func TestDocumentCreateStillRequiresJSONWithoutBlueprint(t *testing.T) {
	recorder := &blueprintRecorder{}
	deps := blueprintDeps(t, recorder, nil)

	if _, err := execute(buildBlueprintRoot(deps), "document", "create"); err == nil {
		t.Fatal("expected document create without --json to fail")
	}
	if recorder.path != "" {
		t.Fatalf("expected no request, got %q", recorder.path)
	}
}
