package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

func schemaDiffTestDeps(handler cmdtest.RoundTripper) cmdkit.Dependencies {
	httpClient := &http.Client{Transport: handler}
	output := "json"
	return cmdkit.Dependencies{
		HTTPClient: httpClient,
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

func writeSchemaDiffTestProfile(t *testing.T, profile string, baseURL string) {
	t.Helper()
	if err := config.WriteUserConfigWithOptions(config.LoadOptions{Profile: profile}, config.Config{
		BaseURL:      baseURL,
		ClientID:     profile + "-client",
		ClientSecret: profile + "-secret",
	}); err != nil {
		t.Fatalf("write profile %s: %v", profile, err)
	}
}

func prepareSchemaDiffProfiles(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeSchemaDiffTestProfile(t, "dev", "https://dev.example.test")
	writeSchemaDiffTestProfile(t, "live", "https://live.example.test")
}

func schemaDiffRouteHost(req *http.Request, devBody string, liveBody string) (*http.Response, error) {
	switch req.URL.Host {
	case "dev.example.test":
		return cmdtest.JSONResponse(http.StatusOK, devBody), nil
	case "live.example.test":
		return cmdtest.JSONResponse(http.StatusOK, liveBody), nil
	default:
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	}
}

func schemaDiffFixtureHandler(t *testing.T, devDoc string, liveDoc string, devData string, liveData string) cmdtest.RoundTripper {
	t.Helper()
	return func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"data-text","name":"Textstring"}],"total":1}`), nil
		case "/umbraco/management/api/v1/data-type/data-text":
			return schemaDiffRouteHost(req, devData, liveData)
		case "/umbraco/management/api/v1/tree/document-type/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"doc-home","alias":"home","name":"Home"}],"total":1}`), nil
		case "/umbraco/management/api/v1/document-type/doc-home":
			return schemaDiffRouteHost(req, devDoc, liveDoc)
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	}
}

func decodeSchemaDiffOutput(t *testing.T, output string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode schema diff output: %v\n%s", err, output)
	}
	return payload
}

func TestSchemaDiffIdenticalProfilesReturnsEqual(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	doc := `{"id":"doc-home","alias":"home","name":"Home","allowedAtRoot":true,"properties":[{"alias":"title","dataType":{"id":"data-text","name":"Textstring"}}]}`
	data := `{"id":"data-text","name":"Textstring","editorAlias":"Umbraco.TextBox","updateDate":"2026-01-01T00:00:00Z"}`
	deps := schemaDiffTestDeps(schemaDiffFixtureHandler(t, doc, doc, data, data))

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live")
	if err != nil {
		t.Fatalf("schema diff identical failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, output)
	if payload["equal"] != true {
		t.Fatalf("expected equal output, got %+v", payload)
	}
}

func TestSchemaDiffChangedDoctypeReturnsNonZeroAndJSON(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	devDoc := `{"id":"dev-doc-home","alias":"home","name":"Home","allowedAtRoot":true,"properties":[{"alias":"title","dataType":{"id":"dev-data-text","name":"Textstring"}}]}`
	liveDoc := `{"id":"live-doc-home","alias":"home","name":"Home","allowedAtRoot":false,"properties":[{"alias":"title","dataType":{"id":"live-data-text","name":"Textstring"}}]}`
	devData := `{"id":"dev-data-text","name":"Textstring","editorAlias":"Umbraco.TextBox"}`
	liveData := `{"id":"live-data-text","name":"Textstring","editorAlias":"Umbraco.TextBox"}`
	deps := schemaDiffTestDeps(schemaDiffFixtureHandler(t, devDoc, liveDoc, devData, liveData))

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live")
	if err == nil || !isSchemaDiffFound(err) {
		t.Fatalf("expected schema differences error, got %v", err)
	}
	payload := decodeSchemaDiffOutput(t, output)
	if payload["equal"] != false {
		t.Fatalf("expected unequal output, got %+v", payload)
	}
	differences := payload["differences"].(map[string]any)
	doctypeDiff := differences["doctype"].(map[string]any)
	changed := doctypeDiff["changed"].([]any)
	if len(changed) != 1 {
		t.Fatalf("expected one changed doctype, got %+v", changed)
	}
	fields := changed[0].(map[string]any)["fields"].([]any)
	if fields[0].(map[string]any)["path"] != "allowedAtRoot" {
		t.Fatalf("expected allowedAtRoot delta, got %+v", fields)
	}
}

func TestSchemaDiffExitZeroSuppressesDifferenceExitCode(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	devDoc := `{"id":"doc-home","alias":"home","name":"Home","allowedAtRoot":true}`
	liveDoc := `{"id":"doc-home","alias":"home","name":"Home","allowedAtRoot":false}`
	data := `{"id":"data-text","name":"Textstring","editorAlias":"Umbraco.TextBox"}`
	deps := schemaDiffTestDeps(schemaDiffFixtureHandler(t, devDoc, liveDoc, data, data))

	if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--exit-zero"); err != nil {
		t.Fatalf("schema diff --exit-zero failed: %v", err)
	}
}

func TestSchemaDiffDatatypeScopeAndIncludeSkipsDoctypeFetch(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	var doctypeFetches int
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"data-text","name":"Textstring"},{"id":"data-other","name":"Other"}],"total":2}`), nil
		case "/umbraco/management/api/v1/data-type/data-text":
			return schemaDiffRouteHost(req,
				`{"id":"data-text","name":"Textstring","editorAlias":"Umbraco.TextBox"}`,
				`{"id":"data-text","name":"Textstring","editorAlias":"Umbraco.TextArea"}`)
		case "/umbraco/management/api/v1/data-type/data-other":
			return schemaDiffRouteHost(req,
				`{"id":"data-other","name":"Other","editorAlias":"Umbraco.TextBox"}`,
				`{"id":"data-other","name":"Other","editorAlias":"Umbraco.TextArea"}`)
		case "/umbraco/management/api/v1/tree/document-type/root", "/umbraco/management/api/v1/document-type/doc-home":
			doctypeFetches++
			return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"doctype fetch should not run"}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "datatype", "--include", "Textstring")
	if err == nil || !isSchemaDiffFound(err) {
		t.Fatalf("expected schema differences error, got %v", err)
	}
	if doctypeFetches != 0 {
		t.Fatalf("expected datatype-only diff to skip doctypes, got %d doctype requests", doctypeFetches)
	}
	payload := decodeSchemaDiffOutput(t, output)
	if got := payload["entities"].([]any); len(got) != 1 || got[0] != "datatype" {
		t.Fatalf("unexpected entity scope: %+v", got)
	}
}

func TestSchemaDiffMissingProfileLabelsEnvSide(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeSchemaDiffTestProfile(t, "dev", "https://dev.example.test")
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type", "/umbraco/management/api/v1/tree/document-type/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "missing")
	if err == nil || !strings.Contains(err.Error(), `envB "missing"`) {
		t.Fatalf("expected envB missing profile error, got %v", err)
	}
}

func TestSchemaDiffHumanOutput(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	doc := `{"id":"doc-home","alias":"home","name":"Home"}`
	data := `{"id":"data-text","name":"Textstring","editorAlias":"Umbraco.TextBox"}`
	outputMode := "plain"
	deps := schemaDiffTestDeps(schemaDiffFixtureHandler(t, doc, doc, data, data))
	deps.EnvOutput = config.OutputPlain
	deps.OutputFlag = &outputMode

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live")
	if err != nil {
		t.Fatalf("schema diff human output failed: %v", err)
	}
	if !strings.Contains(output, "Schema diff dev -> live") || !strings.Contains(output, "No differences") {
		t.Fatalf("unexpected human output: %s", output)
	}
}

func TestSchemaDiffUnknownEntityFails(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "webhook")
	if err == nil || !strings.Contains(err.Error(), `unknown schema diff entity "webhook"`) {
		t.Fatalf("expected unknown entity error, got %v", err)
	}
}

func TestSchemaDiffAddedAndRemovedEntities(t *testing.T) {
	report := computeSchemaDiff("dev", "live",
		[]schemaDiffEntity{{Kind: schemaDiffDoctype, Alias: "old", Name: "Old", Normalized: map[string]any{"alias": "old"}}},
		[]schemaDiffEntity{{Kind: schemaDiffDoctype, Alias: "new", Name: "New", Normalized: map[string]any{"alias": "new"}}},
		schemaDiffOptions{Entities: []schemaDiffEntityKind{schemaDiffDoctype}},
	)
	diff := report.Differences[schemaDiffDoctype]
	if len(diff.Added) != 1 || diff.Added[0].Alias != "new" {
		t.Fatalf("expected added new doctype, got %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Alias != "old" {
		t.Fatalf("expected removed old doctype, got %+v", diff.Removed)
	}
	if report.Counts.Added != 1 || report.Counts.Removed != 1 || report.Equal {
		t.Fatalf("unexpected counts/equality: %+v", report)
	}
}

func TestSchemaDiffFetchErrorLabelsEnvironment(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			if req.URL.Host == "live.example.test" {
				return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"boom"}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "datatype")
	if err == nil || !strings.Contains(fmt.Sprint(err), `envB "live" datatype fetch failed`) {
		t.Fatalf("expected envB datatype fetch error, got %v", err)
	}
}

func TestSchemaDiffTemplateWalksNestedTreeAndDetectsContentChange(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/template/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"tpl-master","alias":"master","name":"Master","hasChildren":true}],"total":1}`), nil
		case "/umbraco/management/api/v1/tree/template/children":
			if req.URL.Query().Get("parentId") != "tpl-master" {
				return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"tpl-home","alias":"homePage","name":"Home Page","hasChildren":false}],"total":1}`), nil
		case "/umbraco/management/api/v1/template/tpl-master":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"tpl-master","alias":"master","name":"Master","content":"<html></html>","masterTemplate":null}`), nil
		case "/umbraco/management/api/v1/template/tpl-home":
			return schemaDiffRouteHost(req,
				`{"id":"tpl-home","alias":"homePage","name":"Home Page","content":"<h1>v1</h1>","masterTemplate":{"id":"tpl-master"}}`,
				`{"id":"tpl-home","alias":"homePage","name":"Home Page","content":"<h1>v2</h1>","masterTemplate":{"id":"tpl-master"}}`)
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "template", "--exit-zero")
	if err != nil {
		t.Fatalf("template diff failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, out)
	if payload["equal"] == true {
		t.Fatalf("expected content change detected, got %s", out)
	}
	if !strings.Contains(out, `"homePage"`) || !strings.Contains(out, "content") {
		t.Fatalf("expected homePage content delta, got %s", out)
	}
	// The nested template only differs in content; the master reference id
	// is identical-by-alias and must not appear as a delta.
	if strings.Contains(out, "tpl-master") {
		t.Fatalf("expected master template id normalized away, got %s", out)
	}
}

func TestSchemaDiffLanguageUsesIsoCodeIdentity(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/language":
			return schemaDiffRouteHost(req,
				`{"items":[{"isoCode":"en-US","name":"English (US)","isDefault":true,"isMandatory":true,"fallbackIsoCode":null}],"total":1}`,
				`{"items":[{"isoCode":"en-US","name":"English (US)","isDefault":true,"isMandatory":false,"fallbackIsoCode":null},{"isoCode":"da-DK","name":"Danish","isDefault":false,"isMandatory":false,"fallbackIsoCode":"en-US"}],"total":2}`)
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "language", "--exit-zero")
	if err != nil {
		t.Fatalf("language diff failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, out)
	counts, _ := payload["counts"].(map[string]any)
	if counts["added"] != float64(1) || counts["changed"] != float64(1) {
		t.Fatalf("expected 1 added (da-DK) and 1 changed (en-US isMandatory), got %s", out)
	}
	if !strings.Contains(out, "da-DK") || !strings.Contains(out, "isMandatory") {
		t.Fatalf("expected iso-code identities and mandatory delta, got %s", out)
	}
}

func TestSchemaDiffDictionaryComparesTranslations(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/dictionary":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"dict-greeting","name":"Greeting"}],"total":1}`), nil
		case "/umbraco/management/api/v1/dictionary/dict-greeting":
			return schemaDiffRouteHost(req,
				`{"id":"dict-greeting","name":"Greeting","translations":[{"isoCode":"en-US","translation":"Hello"}]}`,
				`{"id":"dict-greeting","name":"Greeting","translations":[{"isoCode":"en-US","translation":"Hi there"}]}`)
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "dictionary", "--exit-zero")
	if err != nil {
		t.Fatalf("dictionary diff failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, out)
	if payload["equal"] == true {
		t.Fatalf("expected translation change detected, got %s", out)
	}
	if !strings.Contains(out, "Hi there") {
		t.Fatalf("expected translation delta, got %s", out)
	}
}

func TestSchemaDiffMediatypeMapsDataTypeReferences(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	// The same datatype has different server-assigned IDs per environment;
	// mapping to aliases must keep identical media types equal.
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		dev := strings.Contains(req.URL.Host, "dev")
		dataID := "data-live"
		if dev {
			dataID = "data-dev"
		}
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/filter/data-type":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"`+dataID+`","name":"Upload"}],"total":1}`), nil
		case "/umbraco/management/api/v1/data-type/data-dev", "/umbraco/management/api/v1/data-type/data-live":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"`+dataID+`","name":"Upload","editorAlias":"Umbraco.UploadField","values":[]}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"mt-image","alias":"image","name":"Image"}],"total":1}`), nil
		case "/umbraco/management/api/v1/media-type/mt-image":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"mt-image","alias":"image","name":"Image","properties":[{"alias":"umbracoFile","dataType":{"id":"`+dataID+`"}}]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "mediatype")
	if err != nil {
		t.Fatalf("mediatype diff failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, out)
	if payload["equal"] != true {
		t.Fatalf("expected identical media types after datatype id normalization, got %s", out)
	}
}

func TestSchemaDiffDictionaryDetectsParentMoves(t *testing.T) {
	prepareSchemaDiffProfiles(t)
	// Same key, same translations — but on live it moved under a different
	// parent. Only the overview response carries the parent relationship.
	deps := schemaDiffTestDeps(func(req *http.Request) (*http.Response, error) {
		dev := strings.Contains(req.URL.Host, "dev")
		parentID, parentName := "dict-common", "Common"
		if !dev {
			parentID, parentName = "dict-forms", "Forms"
		}
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/dictionary":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[
				{"id":"`+parentID+`","name":"`+parentName+`","parent":null},
				{"id":"dict-greeting","name":"Greeting","parent":{"id":"`+parentID+`"}}
			],"total":2}`), nil
		case "/umbraco/management/api/v1/dictionary/dict-common", "/umbraco/management/api/v1/dictionary/dict-forms":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"`+parentID+`","name":"`+parentName+`","translations":[]}`), nil
		case "/umbraco/management/api/v1/dictionary/dict-greeting":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"dict-greeting","name":"Greeting","translations":[{"isoCode":"en-US","translation":"Hello"}]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildRootWithCollections(t, deps), "schema", "diff", "dev", "live", "--entity", "dictionary", "--include", "Greeting", "--exit-zero")
	if err != nil {
		t.Fatalf("dictionary parent-move diff failed: %v", err)
	}
	payload := decodeSchemaDiffOutput(t, out)
	if payload["equal"] == true {
		t.Fatalf("expected parent move detected, got %s", out)
	}
	if !strings.Contains(out, "parentName") || !strings.Contains(out, "Forms") {
		t.Fatalf("expected parentName delta Common -> Forms, got %s", out)
	}
}

func TestSchemaDiffFoundErrorCarriesExitCode(t *testing.T) {
	var coder interface{ ExitCode() int }
	if !errors.As(error(schemaDiffFoundError{}), &coder) || coder.ExitCode() != 2 {
		t.Fatalf("expected schema diff differences to map to exit code 2")
	}
}
