package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

const (
	updatePropertyTypeID        = "aaaaaaaa-0000-4000-8000-0000000001aa"
	updatePropertyCompositionID = "aaaaaaaa-0000-4000-8000-0000000001bb"
	updatePropertyParentID      = "aaaaaaaa-0000-4000-8000-0000000001cc"
	updatePropertyDataTypeID    = "aaaaaaaa-0000-4000-8000-0000000001dd"
	updatePropertyNewDataTypeID = "aaaaaaaa-0000-4000-8000-0000000001ee"
)

// updatePropertyDoctype is a document type with two own properties and one
// composition; the full shape matters because the PUT must write every
// other field back untouched.
const updatePropertyDoctype = `{
	"id":"` + updatePropertyTypeID + `","alias":"article","name":"Article","description":null,"icon":"icon-document",
	"allowedAsRoot":true,"variesByCulture":false,"variesBySegment":false,"isElement":false,"allowedInLibrary":false,
	"allowedTemplates":[],"defaultTemplate":null,"cleanup":{"preventCleanup":false,"keepAllVersionsNewerThanDays":null,"keepLatestVersionPerDayForDays":null},
	"allowedDocumentTypes":[],"collection":null,
	"compositions":[{"documentType":{"id":"` + updatePropertyCompositionID + `"},"compositionType":"Composition"}],
	"containers":[{"id":"aaaaaaaa-0000-4000-8000-0000000002aa","parent":null,"name":"Content","type":"Group","sortOrder":0}],
	"properties":[
		{"id":"aaaaaaaa-0000-4000-8000-0000000003aa","container":{"id":"aaaaaaaa-0000-4000-8000-0000000002aa"},"sortOrder":0,"alias":"title","name":"Title","description":null,
		 "dataType":{"id":"` + updatePropertyDataTypeID + `"},"variesByCulture":false,"variesBySegment":false,
		 "validation":{"mandatory":true,"mandatoryMessage":"Required","regEx":null,"regExMessage":null},"appearance":{"labelOnTop":false}},
		{"id":"aaaaaaaa-0000-4000-8000-0000000003bb","container":{"id":"aaaaaaaa-0000-4000-8000-0000000002aa"},"sortOrder":1,"alias":"teaser","name":"Teaser","description":"Short intro",
		 "dataType":{"id":"` + updatePropertyDataTypeID + `"},"variesByCulture":true,"variesBySegment":false,
		 "validation":{"mandatory":false,"mandatoryMessage":null,"regEx":null,"regExMessage":null},"appearance":{"labelOnTop":true}}
	]}`

const updatePropertyComposition = `{
	"id":"` + updatePropertyCompositionID + `","alias":"seoSettings","name":"SEO Settings",
	"compositions":[{"documentType":{"id":"` + updatePropertyParentID + `"},"compositionType":"Composition"}],
	"properties":[{"id":"aaaaaaaa-0000-4000-8000-0000000003cc","alias":"seoTitle","name":"SEO title"}]}`

const updatePropertyNestedComposition = `{
	"id":"` + updatePropertyParentID + `","alias":"metaBase","name":"Meta Base","compositions":[],
	"properties":[{"id":"aaaaaaaa-0000-4000-8000-0000000003dd","alias":"metaKeywords","name":"Meta keywords"}]}`

// updatePropertyServer fakes the document-type endpoints: GETs return the
// stored type, a PUT replaces it (unless ignorePut, a server that "accepts"
// and keeps the old record), and every PUT body is recorded.
type updatePropertyServer struct {
	t         *testing.T
	stored    string
	ignorePut bool
	puts      []map[string]any
}

func (s *updatePropertyServer) deps() cmdkit.Dependencies {
	return cmdtest.ClientDeps(func(req *http.Request) (*http.Response, error) {
		const prefix = "/umbraco/management/api/v1/document-type/"
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == prefix+updatePropertyTypeID && req.Method == http.MethodGet:
			return cmdtest.JSONResponse(http.StatusOK, s.stored), nil
		case req.URL.Path == prefix+updatePropertyCompositionID && req.Method == http.MethodGet:
			return cmdtest.JSONResponse(http.StatusOK, updatePropertyComposition), nil
		case req.URL.Path == prefix+updatePropertyParentID && req.Method == http.MethodGet:
			return cmdtest.JSONResponse(http.StatusOK, updatePropertyNestedComposition), nil
		case req.URL.Path == prefix+updatePropertyTypeID && req.Method == http.MethodPut:
			raw, _ := io.ReadAll(req.Body)
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				s.t.Fatalf("PUT body is not JSON: %v", err)
			}
			s.puts = append(s.puts, body)
			if !s.ignorePut {
				s.stored = string(raw)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			s.t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})
}

func decodeUpdatePropertyFixture(t *testing.T, raw string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return decoded
}

func TestDoctypeUpdatePropertyChangesOnlyTheRequestedField(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	out, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "teaser", "--description", "Shown in listings")
	if err != nil {
		t.Fatalf("doctype update-property failed: %v", err)
	}
	if len(server.puts) != 1 {
		t.Fatalf("expected exactly one PUT, got %d", len(server.puts))
	}
	expected := decodeUpdatePropertyFixture(t, updatePropertyDoctype)
	expected["properties"].([]any)[1].(map[string]any)["description"] = "Shown in listings"
	if !reflect.DeepEqual(server.puts[0], expected) {
		got, _ := json.Marshal(server.puts[0])
		want, _ := json.Marshal(expected)
		t.Fatalf("PUT body must equal the fetched type with only teaser.description changed\n got: %s\nwant: %s", got, want)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	changes := result["changes"].(map[string]any)
	if result["verified"] != true || result["changed"] != true || len(changes) != 1 {
		t.Fatalf("unexpected result: %s", out)
	}
	if change := changes["description"].(map[string]any); change["from"] != "Short intro" || change["to"] != "Shown in listings" {
		t.Fatalf("expected the description change described, got %+v", change)
	}
}

func TestDoctypeUpdatePropertyChangesNameMandatoryAndDataType(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	_, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID,
		"--alias", "title", "--name", "Headline", "--no-mandatory", "--data-type", updatePropertyNewDataTypeID)
	if err != nil {
		t.Fatalf("doctype update-property failed: %v", err)
	}
	expected := decodeUpdatePropertyFixture(t, updatePropertyDoctype)
	title := expected["properties"].([]any)[0].(map[string]any)
	title["name"] = "Headline"
	title["dataType"] = map[string]any{"id": updatePropertyNewDataTypeID}
	title["validation"].(map[string]any)["mandatory"] = false
	if !reflect.DeepEqual(server.puts[0], expected) {
		got, _ := json.Marshal(server.puts[0])
		t.Fatalf("PUT body changed more than title's name, data type and mandatory flag: %s", got)
	}
}

func TestDoctypeUpdatePropertyRejectsUnknownAlias(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	_, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "subtitle", "--name", "Subtitle")
	if err == nil || !strings.Contains(err.Error(), `has no property with alias "subtitle"`) || len(server.puts) != 0 {
		t.Fatalf("expected an unknown-alias error and no write, got err=%v puts=%d", err, len(server.puts))
	}
	_, err = cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "Teaser", "--name", "Intro")
	if err == nil || !strings.Contains(err.Error(), `did you mean "teaser"`) || len(server.puts) != 0 {
		t.Fatalf("expected a case hint and no write, got err=%v puts=%d", err, len(server.puts))
	}
}

func TestDoctypeUpdatePropertyNamesTheCompositionThatOwnsTheProperty(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	_, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "seoTitle", "--description", "x")
	if err == nil || len(server.puts) != 0 {
		t.Fatalf("expected a composition error and no write, got err=%v puts=%d", err, len(server.puts))
	}
	for _, want := range []string{`does not own property "seoTitle"`, `composition "SEO Settings"`, "alias seoSettings", updatePropertyCompositionID, "update-property " + updatePropertyCompositionID + " --alias seoTitle"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in the composition error, got %v", want, err)
		}
	}

	_, err = cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "metaKeywords", "--description", "x")
	if err == nil || !strings.Contains(err.Error(), `composition "Meta Base"`) || !strings.Contains(err.Error(), `through "SEO Settings"`) || !strings.Contains(err.Error(), updatePropertyParentID) {
		t.Fatalf("expected the nested composition and the path to it named, got %v", err)
	}
}

func TestDoctypeUpdatePropertyNamesTheParentForAnInheritedProperty(t *testing.T) {
	stored := strings.Replace(updatePropertyDoctype, `"compositionType":"Composition"`, `"compositionType":"Inheritance"`, 1)
	server := &updatePropertyServer{t: t, stored: stored}
	_, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "seoTitle", "--no-mandatory")
	if err == nil || !strings.Contains(err.Error(), `inherited from parent document type "SEO Settings"`) || len(server.puts) != 0 {
		t.Fatalf("expected the parent type named and no write, got err=%v puts=%d", err, len(server.puts))
	}

	_, err = cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "metaKeywords", "--no-mandatory")
	if err == nil || !strings.Contains(err.Error(), `comes from composition "Meta Base"`) || strings.Contains(err.Error(), "parent document type") || len(server.puts) != 0 {
		t.Fatalf("expected the parent's composition named as a composition, not as the parent, got err=%v puts=%d", err, len(server.puts))
	}
}

func TestDoctypeUpdatePropertyIsANoOpWhenNothingChanges(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	backupPath := filepath.Join(t.TempDir(), "noop.backup.json")
	out, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID,
		"--alias", "title", "--name", "Title", "--mandatory", "--data-type", strings.ToUpper(updatePropertyDataTypeID), "--backup="+backupPath)
	if err != nil {
		t.Fatalf("doctype update-property no-op failed: %v", err)
	}
	if len(server.puts) != 0 {
		t.Fatalf("expected no PUT when nothing changes, got %d", len(server.puts))
	}
	if !strings.Contains(out, `"changed": false`) || !strings.Contains(out, "already has the requested values") {
		t.Fatalf("expected a clear no-op message, got %s", out)
	}
	if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no backup for a no-op, stat err=%v", statErr)
	}
	out, err = cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "title", "--description", "")
	if err != nil || len(server.puts) != 0 || !strings.Contains(out, `"changed": false`) {
		t.Fatalf("expected clearing a null description to be a no-op, got err=%v puts=%d out=%s", err, len(server.puts), out)
	}
}

func TestDoctypeUpdatePropertyDryRunSendsNothing(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	backupPath := filepath.Join(t.TempDir(), "dryrun.backup.json")
	out, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID,
		"--alias", "teaser", "--description", "Shown in listings", "--dry-run", "--backup="+backupPath)
	if err != nil {
		t.Fatalf("doctype update-property --dry-run failed: %v", err)
	}
	if len(server.puts) != 0 {
		t.Fatalf("dry-run must not send the PUT, got %d", len(server.puts))
	}
	if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run must not write a backup, stat err=%v", statErr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	plan := result["update"].(map[string]any)
	if plan["dryRun"] != true || plan["method"] != http.MethodPut || plan["path"] != "/umbraco/management/api/v1/document-type/"+updatePropertyTypeID {
		t.Fatalf("expected the planned PUT, got %+v", plan)
	}
	expected := decodeUpdatePropertyFixture(t, updatePropertyDoctype)
	expected["properties"].([]any)[1].(map[string]any)["description"] = "Shown in listings"
	if !reflect.DeepEqual(plan["body"], expected) {
		got, _ := json.Marshal(plan["body"])
		t.Fatalf("planned body must differ from the fetched type only in teaser.description, got %s", got)
	}
}

func TestDoctypeUpdatePropertyWritesBackupBeforeTheChange(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype}
	backupPath := filepath.Join(t.TempDir(), "article.backup.json")
	out, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID,
		"--alias", "title", "--no-mandatory", "--backup="+backupPath)
	if err != nil {
		t.Fatalf("doctype update-property --backup failed: %v", err)
	}
	if !strings.Contains(out, `"backup": "`+backupPath+`"`) {
		t.Fatalf("expected the backup path in the result, got %s", out)
	}
	envelope, err := cmdkit.ReadBackup(backupPath, "doctype")
	if err != nil || envelope.ID != updatePropertyTypeID || envelope.Path != "/document-type/"+updatePropertyTypeID {
		t.Fatalf("expected a doctype backup envelope, got err=%v envelope=%+v", err, envelope)
	}
	if !reflect.DeepEqual(envelope.Entity, decodeUpdatePropertyFixture(t, updatePropertyDoctype)) {
		saved, _ := json.Marshal(envelope.Entity)
		t.Fatalf("expected the pre-change type (title still mandatory) in the backup, got %s", saved)
	}
}

func TestDoctypeUpdatePropertyFailsWhenTheChangeDoesNotStick(t *testing.T) {
	server := &updatePropertyServer{t: t, stored: updatePropertyDoctype, ignorePut: true}
	_, err := cmdtest.Execute(buildRootWithCollections(t, server.deps()), "doctype", "update-property", updatePropertyTypeID, "--alias", "teaser", "--name", "Intro")
	if err == nil || !strings.Contains(err.Error(), `property "teaser" still differs in name`) || len(server.puts) != 1 {
		t.Fatalf("expected the verify step to fail, got err=%v puts=%d", err, len(server.puts))
	}
}

func TestDoctypeUpdatePropertyValidatesFlagsBeforeAnyRequest(t *testing.T) {
	deps := cmdtest.ClientDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no request expected, got %s %s", req.Method, req.URL.Path)
		return nil, nil
	})
	cases := map[string][]string{
		"needs at least one change":  {"--alias", "title"},
		"mutually exclusive":         {"--alias", "title", "--mandatory", "--no-mandatory"},
		"--data-type must be a data": {"--alias", "title", "--data-type", "not-a-guid"},
		"--name cannot be empty":     {"--alias", "title", "--name", " "},
		"missing required option":    {"--name", "Title"},
	}
	for want, flags := range cases {
		args := append([]string{"doctype", "update-property", updatePropertyTypeID}, flags...)
		if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), args...); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v: expected error containing %q, got %v", flags, want, err)
		}
	}
}
