package deploy

import (
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

// Both shapes below are from a local 18.2 instance whose database the
// artifacts were written from: before the fix every such document type and
// relation type reported drift on every environment, the source included.

func refsStatusDeps(remote map[string]string) func(req *http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case cmdtest.TokenPath:
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		}
		if body, ok := remote[req.URL.Path]; ok {
			return cmdtest.JSONResponse(http.StatusOK, body), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	}
}

func TestDeployStatusDocumentTypeBareGUIDListViewMatchesCollection(t *testing.T) {
	dir := t.TempDir()
	withListView := strings.Replace(statusDoctypeUda, `"Icon": "icon-box",`, `"Icon": "icon-box", "ListView": "c0808dd3-8133-4e4b-8ce8-e2bea84a96a4",`, 1)
	writeUda(t, dir, "document-type__b.uda", withListView)

	for _, tc := range []struct {
		collection string
		want       string
	}{
		{`{"id": "c0808dd3-8133-4e4b-8ce8-e2bea84a96a4"}`, "in-sync"},
		{`{"id": "dddddddd-8133-4e4b-8ce8-e2bea84a96a4"}`, "drifted"},
	} {
		remote := strings.Replace(statusRemoteDoctype, `"isElement": true,`, `"isElement": true, "collection": `+tc.collection+`,`, 1)
		deps := cmdtest.Deps(refsStatusDeps(map[string]string{"/umbraco/management/api/v1/document-type/bbbbbbbb-1111-2222-3333-444444444444": remote}))
		entry := statusByFile(t, runDeployStatus(t, deps, dir))["document-type__b.uda"]
		if entry["status"] != tc.want {
			t.Fatalf("collection %s: want %s, got %+v", tc.collection, tc.want, entry)
		}
		if tc.want == "drifted" && !strings.Contains(jsonString(entry["diffs"]), "collection") {
			t.Fatalf("expected a collection diff, got %+v", entry)
		}
	}
}

func TestDeployStatusRelationTypeObjectTypesReadFromParentAndChildObject(t *testing.T) {
	dir := t.TempDir()
	relation := `{"Name":"Relate Parent Document On Delete","Alias":"relateParentDocumentOnDelete","IsBidirectional":false,"IsDependency":false,"ParentObjectType":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","ChildObjectType":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","Udi":"umb://relation-type/0cc3507c66ab309189133d998148e423","Dependencies":[],"__type":"Umbraco.Deploy.Infrastructure,X","__version":"18.0.1"}`
	writeUda(t, dir, "relation-type__0c.uda", relation)
	path := "/umbraco/management/api/v1/relation-type/0cc3507c-66ab-3091-8913-3d998148e423"

	for _, tc := range []struct {
		name   string
		remote string
		want   string
		diff   string
	}{
		{"18.2 shape, same types", `{"name":"Relate Parent Document On Delete","alias":"relateParentDocumentOnDelete","isBidirectional":false,"isDependency":false,"parentObject":{"id":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","name":"Document"},"childObject":{"id":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","name":"Document"}}`, "in-sync", ""},
		{"18.2 shape, other child", `{"name":"Relate Parent Document On Delete","alias":"relateParentDocumentOnDelete","isBidirectional":false,"isDependency":false,"parentObject":{"id":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","name":"Document"},"childObject":{"id":"b796f64c-1f99-4ffb-b886-4bf4bc011a9c","name":"Media"}}`, "drifted", "childObjectType"},
		{"flat fallback", `{"name":"Relate Parent Document On Delete","alias":"relateParentDocumentOnDelete","isBidirectional":false,"isDependency":false,"parentObjectType":"c66ba18e-eaf3-4cff-8a22-41b16d66a972","childObjectType":"c66ba18e-eaf3-4cff-8a22-41b16d66a972"}`, "in-sync", ""},
	} {
		deps := cmdtest.Deps(refsStatusDeps(map[string]string{path: tc.remote}))
		entry := statusByFile(t, runDeployStatus(t, deps, dir))["relation-type__0c.uda"]
		if entry["status"] != tc.want || (tc.diff != "" && !strings.Contains(jsonString(entry["diffs"]), tc.diff)) {
			t.Fatalf("%s: want %s %s, got %+v", tc.name, tc.want, tc.diff, entry)
		}
	}
}
