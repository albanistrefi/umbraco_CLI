package deploy

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

func buildDeployRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	Register(root, deps)
	return root
}

// writeUda writes a fixture artifact UTF-8 with BOM, exactly as Umbraco
// Deploy writes them — json parsing must strip it.
func writeUda(t *testing.T, dir string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestParseUdi(t *testing.T) {
	kind, guid := parseUdi("umb://data-type/c6bac0dd4ab945b18e30e4b619ee5da3")
	if kind != "data-type" || guid != "c6bac0dd-4ab9-45b1-8e30-e4b619ee5da3" {
		t.Fatalf("unexpected parse: %q %q", kind, guid)
	}
	if kind, guid := parseUdi("not-a-udi"); kind != "" || guid != "" {
		t.Fatalf("expected empty parse, got %q %q", kind, guid)
	}
}

const statusDataTypeUda = `{
  "Name": "Example Text",
  "EditorAlias": "Umbraco.TextArea",
  "EditorUiAlias": "Umb.PropertyEditorUi.TextArea",
  "Configuration": {"maxChars": 500},
  "Udi": "umb://data-type/aaaaaaaa111122223333444444444444",
  "Dependencies": [],
  "__type": "Umbraco.Deploy.Infrastructure,Umbraco.Deploy.Infrastructure.Artifacts.DataTypeArtifact",
  "__version": "18.0.1"
}`

const statusDoctypeUda = `{
  "Name": "Example Block",
  "Alias": "exampleBlock",
  "Icon": "icon-box",
  "Permissions": {"IsElementType": true, "AllowedChildContentTypes": []},
  "PropertyGroups": [
    {"Key": "11111111-1111-1111-1111-111111111111", "Name": "Content", "Alias": "content", "PropertyTypes": [
      {"Key": "22222222-2222-2222-2222-222222222222", "Alias": "heading", "DataType": "umb://data-type/aaaaaaaa111122223333444444444444", "ValueType": "System.String", "Name": "Heading", "SortOrder": 0}
    ]}
  ],
  "PropertyTypes": [
    {"Key": "33333333-3333-3333-3333-333333333333", "Alias": "ungrouped", "DataType": "umb://data-type/aaaaaaaa111122223333444444444444", "ValueType": "System.String", "Name": "Ungrouped", "SortOrder": 5}
  ],
  "Udi": "umb://document-type/bbbbbbbb111122223333444444444444",
  "Dependencies": [{"Udi": "umb://data-type/aaaaaaaa111122223333444444444444", "Ordering": true}],
  "__type": "Umbraco.Deploy.Infrastructure,Umbraco.Deploy.Infrastructure.Artifacts.ContentType.DocumentTypeArtifact",
  "__version": "18.0.1"
}`

// statusAutomationUda is a synthetic automation shaped like the handover's
// redacted sample — deliberately not derived from a real artifact.
const statusAutomationUda = `{
  "Name": "Example Automation",
  "Alias": "exampleAutomation",
  "WorkspaceUdi": "umb://umbraco-automate-workspace/cccccccc111122223333444444444444",
  "Trigger": {"TriggerAlias": "example.trigger", "Settings": {}},
  "Steps": [
    {"Id": "s1", "ActionAlias": "umbracoAutomate.forEach", "Name": "Loop", "Alias": "loop", "ConnectionId": null, "Settings": {}, "InputMappings": {}},
    {"Id": "s2", "ActionAlias": "example.sendMail", "Name": "Send", "Alias": "send", "ConnectionId": null, "Settings": {}, "InputMappings": {}}
  ],
  "Connections": [{"SourceStepId": "s1", "TargetStepId": "s2"}],
  "Udi": "umb://umbraco-automate-automation/dddddddd111122223333444444444444",
  "Dependencies": [{"Udi": "umb://umbraco-automate-workspace/cccccccc111122223333444444444444"}],
  "__type": "Umbraco.Deploy.Automate,Umbraco.Deploy.Automate.Artifacts.AutomateAutomationArtifact",
  "__version": "18.0.1"
}`

const statusRemoteDataType = `{
  "id": "aaaaaaaa-1111-2222-3333-444444444444",
  "name": "Example Text",
  "editorAlias": "Umbraco.TextArea",
  "editorUiAlias": "Umb.PropertyEditorUi.TextArea",
  "values": [
    {"alias": "maxChars", "value": 500},
    {"alias": "umbMigrationV14", "value": "2025-10-07T11:11:35Z"}
  ]
}`

const statusRemoteDoctype = `{
  "id": "bbbbbbbb-1111-2222-3333-444444444444",
  "name": "Example Block",
  "alias": "exampleBlock",
  "icon": "icon-box",
  "isElement": true,
  "properties": [
    {"alias": "heading", "name": "Heading", "sortOrder": 0, "dataType": {"id": "aaaaaaaa-1111-2222-3333-444444444444"}},
    {"alias": "ungrouped", "name": "Ungrouped", "sortOrder": 5, "dataType": {"id": "aaaaaaaa-1111-2222-3333-444444444444"}}
  ]
}`

func deployStatusDeps(t *testing.T, remoteDataType string, remoteDoctype string, automateAvailable bool) cmdkit.Dependencies {
	t.Helper()
	return cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/data-type/aaaaaaaa-1111-2222-3333-444444444444":
			return cmdtest.JSONResponse(http.StatusOK, remoteDataType), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type/bbbbbbbb-1111-2222-3333-444444444444":
			return cmdtest.JSONResponse(http.StatusOK, remoteDoctype), nil
		case strings.HasPrefix(req.URL.Path, "/umbraco/automate/management/api/v1/"):
			if !automateAvailable {
				return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
			}
			if strings.HasSuffix(req.URL.Path, "/automations") {
				return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
			}
			if strings.HasSuffix(req.URL.Path, "/export") {
				return cmdtest.JSONResponse(http.StatusOK, `{"automation":{"name":"Example Automation","alias":"exampleAutomation","trigger":{"triggerAlias":"example.trigger","settings":{}},"steps":[{"id":"s1","actionAlias":"umbracoAutomate.forEach","name":"Loop","alias":"loop","settings":{},"inputMappings":{}},{"id":"s2","actionAlias":"example.sendMail","name":"Send","alias":"send","settings":{},"inputMappings":{}}],"connections":[{"sourceStepId":"s1","targetStepId":"s2"}]}}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"Example Automation","alias":"exampleAutomation"}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})
}

func runDeployStatus(t *testing.T, deps cmdkit.Dependencies, dir string, extra ...string) map[string]any {
	t.Helper()
	args := append([]string{"deploy", "status", "--uda-dir", dir, "--exit-zero"}, extra...)
	output, err := cmdtest.Execute(buildDeployRoot(deps), args...)
	if err != nil {
		t.Fatalf("deploy status failed: %v", err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	return payload
}

func statusByFile(t *testing.T, payload map[string]any) map[string]map[string]any {
	t.Helper()
	byFile := map[string]map[string]any{}
	for _, item := range payload["artifacts"].([]any) {
		entry := item.(map[string]any)
		byFile[entry["file"].(string)] = entry
	}
	return byFile
}

func TestDeployStatusInSyncIgnoresEnvironmentOnlyValues(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "data-type__a.uda", statusDataTypeUda)
	writeUda(t, dir, "document-type__b.uda", statusDoctypeUda)

	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir)
	byFile := statusByFile(t, payload)
	if byFile["data-type__a.uda"]["status"] != "in-sync" {
		t.Fatalf("expected data type in-sync despite env-only umbMigrationV14 value, got %+v", byFile["data-type__a.uda"])
	}
	if byFile["document-type__b.uda"]["status"] != "in-sync" {
		t.Fatalf("expected doctype in-sync (both property collections read), got %+v", byFile["document-type__b.uda"])
	}
}

func TestDeployStatusDetectsConfigurationAndPropertyDrift(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "data-type__a.uda", statusDataTypeUda)
	writeUda(t, dir, "document-type__b.uda", statusDoctypeUda)

	driftedDataType := strings.Replace(statusRemoteDataType, `"value": 500`, `"value": 200`, 1)
	driftedDoctype := strings.Replace(statusRemoteDoctype, `{"alias": "ungrouped", "name": "Ungrouped", "sortOrder": 5, "dataType": {"id": "aaaaaaaa-1111-2222-3333-444444444444"}}`, "", 1)
	driftedDoctype = strings.Replace(driftedDoctype, `}},`, `}}`, 1)
	payload := runDeployStatus(t, deployStatusDeps(t, driftedDataType, driftedDoctype, false), dir)
	byFile := statusByFile(t, payload)

	dataType := byFile["data-type__a.uda"]
	if dataType["status"] != "drifted" || !strings.Contains(jsonString(dataType["diffs"]), "configuration.maxChars") {
		t.Fatalf("expected configuration drift, got %+v", dataType)
	}
	doctype := byFile["document-type__b.uda"]
	if doctype["status"] != "drifted" || !strings.Contains(jsonString(doctype["diffs"]), "ungrouped (missing remotely)") {
		t.Fatalf("expected top-level PropertyTypes to be read and missing remotely, got %+v", doctype)
	}
}

func jsonString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestDeployStatusMissingRemoteAndExitCode(t *testing.T) {
	dir := t.TempDir()
	missing := strings.Replace(statusDataTypeUda, "aaaaaaaa111122223333444444444444", "eeeeeeee111122223333444444444444", 1)
	writeUda(t, dir, "data-type__missing.uda", missing)

	deps := deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false)
	payload := runDeployStatus(t, deps, dir)
	byFile := statusByFile(t, payload)
	if byFile["data-type__missing.uda"]["status"] != "missing-remote" {
		t.Fatalf("expected missing-remote, got %+v", byFile["data-type__missing.uda"])
	}

	_, err := cmdtest.Execute(buildDeployRoot(deps), "deploy", "status", "--uda-dir", dir)
	var drift deployDriftFoundError
	if err == nil || !strings.Contains(err.Error(), "missing") || drift.ExitCode() != 7 {
		t.Fatalf("expected exit-7 drift error without --exit-zero, got %v", err)
	}
}

func TestDeployStatusAutomateUnavailableIsUnknownNeverMissing(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "umbraco-automate-automation__d.uda", statusAutomationUda)

	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir, "--flag-step-alias", "umbracoAutomate.forEach")
	byFile := statusByFile(t, payload)
	automation := byFile["umbraco-automate-automation__d.uda"]
	if automation["status"] != "unknown" || !strings.Contains(automation["reason"].(string), "Automate API unavailable") {
		t.Fatalf("expected unknown (never missing-remote) without Automate, got %+v", automation)
	}
	if !strings.Contains(jsonString(automation["stepAliases"]), "umbracoAutomate.forEach") {
		t.Fatalf("expected step aliases read locally despite unavailable API, got %+v", automation)
	}
	if !strings.Contains(jsonString(automation["flags"]), "matches --flag-step-alias") {
		t.Fatalf("expected control-flow flag, got %+v", automation)
	}
	summary := payload["summary"].(map[string]any)
	if summary["flagged"].(float64) != 1 {
		t.Fatalf("expected flagged count 1, got %+v", summary)
	}
}

func TestDeployStatusAutomateAvailableComparesAndFinds(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "umbraco-automate-automation__d.uda", statusAutomationUda)

	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, true), dir)
	byFile := statusByFile(t, payload)
	if byFile["umbraco-automate-automation__d.uda"]["status"] != "in-sync" {
		t.Fatalf("expected in-sync automation with reachable Automate API, got %+v", byFile["umbraco-automate-automation__d.uda"])
	}
}

func TestDeployStatusParseFailureIsReportedNotDropped(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "data-type__broken.uda", "{not json")

	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir)
	byFile := statusByFile(t, payload)
	if byFile["data-type__broken.uda"]["status"] != "error" {
		t.Fatalf("expected parse error surfaced, got %+v", byFile["data-type__broken.uda"])
	}
}

func TestDeployStatusAutomationExportDriftAndBehavioralFields(t *testing.T) {
	dir := t.TempDir()
	drifted := strings.Replace(statusAutomationUda, `"ActionAlias": "example.sendMail"`, `"ActionAlias": "example.sendSms"`, 1)
	writeUda(t, dir, "umbraco-automate-automation__d.uda", drifted)

	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, true), dir)
	byFile := statusByFile(t, payload)
	automation := byFile["umbraco-automate-automation__d.uda"]
	if automation["status"] != "drifted" || !strings.Contains(jsonString(automation["diffs"]), "step s2.actionAlias") {
		t.Fatalf("expected behavioral step drift via export comparison, got %+v", automation)
	}
}

func TestDeployStatusWorkspaceIdentityMatchIsUnknownNotInSync(t *testing.T) {
	dir := t.TempDir()
	workspace := `{"Name":"Example Workspace","Alias":"exampleWorkspace","Udi":"umb://umbraco-automate-workspace/cccccccc111122223333444444444444","Dependencies":[],"__type":"Umbraco.Deploy.Automate,X","__version":"18.0.1"}`
	writeUda(t, dir, "umbraco-automate-workspace__c.uda", workspace)

	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		case strings.HasSuffix(req.URL.Path, "/automations"):
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"Example Workspace","alias":"exampleWorkspace"}`), nil
		}
	})
	payload := runDeployStatus(t, deps, dir)
	byFile := statusByFile(t, payload)
	workspaceResult := byFile["umbraco-automate-workspace__c.uda"]
	if workspaceResult["status"] != "unknown" || !strings.Contains(workspaceResult["reason"].(string), "identity fields match") {
		t.Fatalf("expected identity-only match to stay unknown, got %+v", workspaceResult)
	}
}

func TestDeployStatusUnreachableEnvironmentPreservesExitCode(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "data-type__a.uda", statusDataTypeUda)
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return cmdtest.JSONResponse(http.StatusInternalServerError, `{"error":"down"}`), nil
	})
	_, err := cmdtest.Execute(buildDeployRoot(deps), "deploy", "status", "--uda-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "cannot reach the target environment") {
		t.Fatalf("expected pre-flight failure, got %v", err)
	}
}

func TestDeployStatusRejectsMalformedUdiBeforeRequesting(t *testing.T) {
	dir := t.TempDir()
	bad := strings.Replace(statusDataTypeUda, "aaaaaaaa111122223333444444444444", "not-32-hex", 1)
	writeUda(t, dir, "data-type__bad.uda", bad)
	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir)
	byFile := statusByFile(t, payload)
	if byFile["data-type__bad.uda"]["status"] != "error" {
		t.Fatalf("expected malformed udi to be an error, got %+v", byFile["data-type__bad.uda"])
	}
}

func TestDeployStatusTemplateWhitespaceIsSignificant(t *testing.T) {
	dir := t.TempDir()
	template := `{"Name":"Example Template","Alias":"exampleTemplate","Content":"@inherits X\r\n<p>hi</p>\r\n","Udi":"umb://template/eeeeeeee111122223333444444444444","Dependencies":[],"__type":"Umbraco.Deploy.Infrastructure,X","__version":"18.0.1"}`
	writeUda(t, dir, "template__e.uda", template)
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		default:
			// Same content, LF endings, but an extra trailing newline: line
			// endings normalize away, trailing whitespace must not.
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"Example Template","alias":"exampleTemplate","content":"@inherits X\n<p>hi</p>\n\n"}`), nil
		}
	})
	payload := runDeployStatus(t, deps, dir)
	byFile := statusByFile(t, payload)
	if byFile["template__e.uda"]["status"] != "drifted" || !strings.Contains(jsonString(byFile["template__e.uda"]["diffs"]), "content") {
		t.Fatalf("expected trailing-whitespace drift, got %+v", byFile["template__e.uda"])
	}
}

func TestDeployStatusRelationTypeBehavioralDrift(t *testing.T) {
	dir := t.TempDir()
	relation := `{"Name":"Related Media","Alias":"relatedMedia","IsBidirectional":true,"IsDependency":false,"Udi":"umb://relation-type/ffffffff111122223333444444444444","Dependencies":[],"__type":"Umbraco.Deploy.Infrastructure,X","__version":"18.0.1"}`
	writeUda(t, dir, "relation-type__f.uda", relation)
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"Related Media","alias":"relatedMedia","isBidirectional":false,"isDependency":false}`), nil
		}
	})
	payload := runDeployStatus(t, deps, dir)
	byFile := statusByFile(t, payload)
	if byFile["relation-type__f.uda"]["status"] != "drifted" || !strings.Contains(jsonString(byFile["relation-type__f.uda"]["diffs"]), "isBidirectional") {
		t.Fatalf("expected directionality drift, got %+v", byFile["relation-type__f.uda"])
	}
}

func TestParseUdiKeepsNonGUIDIdentifiers(t *testing.T) {
	kind, id := parseUdi("umb://language/en-US")
	if kind != "language" || id != "en-US" {
		t.Fatalf("expected language ISO identifier kept verbatim, got %q %q", kind, id)
	}
}

func TestDeployStatusLanguageComparesByIsoCode(t *testing.T) {
	dir := t.TempDir()
	lang := `{"Name":"English (United States)","IsoCode":"en-US","IsDefault":true,"IsMandatory":false,"Udi":"umb://language/en-US","Dependencies":[],"__type":"Umbraco.Deploy.Infrastructure,X","__version":"18.0.1"}`
	writeUda(t, dir, "language__en-US.uda", lang)
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/server/status":
			return cmdtest.JSONResponse(http.StatusOK, `{"serverStatus":"Run"}`), nil
		case "/umbraco/management/api/v1/language/en-US":
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"English (United States)","isoCode":"en-US","isDefault":false,"isMandatory":false,"fallbackIsoCode":null}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	payload := runDeployStatus(t, deps, dir)
	byFile := statusByFile(t, payload)
	language := byFile["language__en-US.uda"]
	if language["status"] != "drifted" || !strings.Contains(jsonString(language["diffs"]), "isDefault") {
		t.Fatalf("expected language compared with isDefault drift, got %+v", language)
	}
}

func TestDeployStatusKindFilterExcludesErroredOutOfScopeArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeUda(t, dir, "data-type__a.uda", statusDataTypeUda)
	writeUda(t, dir, "broken__x.uda", "{not json")
	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir, "--kind", "data-type")
	summary := payload["summary"].(map[string]any)
	if summary["total"].(float64) != 1 || summary["errors"].(float64) != 0 {
		t.Fatalf("expected filtered run to exclude out-of-scope errored artifacts, got %+v", summary)
	}
}

func TestDeployStatusNonGUIDOnGUIDRouteIsError(t *testing.T) {
	dir := t.TempDir()
	bad := strings.Replace(statusDataTypeUda, "umb://data-type/aaaaaaaa111122223333444444444444", "umb://data-type/en-US", 1)
	writeUda(t, dir, "data-type__bad.uda", bad)
	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false), dir)
	byFile := statusByFile(t, payload)
	if byFile["data-type__bad.uda"]["status"] != "error" || !strings.Contains(byFile["data-type__bad.uda"]["reason"].(string), "requires a GUID") {
		t.Fatalf("expected GUID-route guard, got %+v", byFile["data-type__bad.uda"])
	}
}

func TestDeployStatusAutomateNonGUIDIsErrorNotRequest(t *testing.T) {
	dir := t.TempDir()
	bad := strings.Replace(statusAutomationUda, "umb://umbraco-automate-automation/dddddddd111122223333444444444444", "umb://umbraco-automate-automation/not-a-guid", 1)
	writeUda(t, dir, "umbraco-automate-automation__bad.uda", bad)
	payload := runDeployStatus(t, deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, true), dir)
	byFile := statusByFile(t, payload)
	automation := byFile["umbraco-automate-automation__bad.uda"]
	if automation["status"] != "error" || !strings.Contains(automation["reason"].(string), "requires a GUID") {
		t.Fatalf("expected GUID guard before automate dispatch, got %+v", automation)
	}
}

func TestDeployStatusDriftErrorQuietOnlyUnderExplicitJSON(t *testing.T) {
	dir := t.TempDir()
	missing := strings.Replace(statusDataTypeUda, "aaaaaaaa111122223333444444444444", "eeeeeeee111122223333444444444444", 1)
	writeUda(t, dir, "data-type__missing.uda", missing)
	deps := deployStatusDeps(t, statusRemoteDataType, statusRemoteDoctype, false)

	_, err := cmdtest.Execute(buildDeployRoot(deps), "deploy", "status", "--uda-dir", dir, "-o", " JSON ")
	var drift deployDriftFoundError
	if err == nil || !errorsAsDrift(err, &drift) || !drift.QuietExit() || drift.ExitCode() != 7 {
		t.Fatalf("expected quiet exit-7 drift error under explicit -o json, got %v", err)
	}

	_, err = cmdtest.Execute(buildDeployRoot(deps), "deploy", "status", "--uda-dir", dir, "-o", "table")
	if err == nil || !errorsAsDrift(err, &drift) || drift.QuietExit() {
		t.Fatalf("expected audible drift error under -o table, got %v", err)
	}
}

func errorsAsDrift(err error, target *deployDriftFoundError) bool {
	d, ok := err.(deployDriftFoundError)
	if ok {
		*target = d
	}
	return ok
}
