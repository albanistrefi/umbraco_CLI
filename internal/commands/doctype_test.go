package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDoctypeListSupportsFieldsAndReadTriage(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document-type/root":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"dt-1","name":"Article","alias":"article","icon":"icon-document"},
				{"id":"dt-2","name":"Product","alias":"product","icon":"icon-box"}
			]}`), nil
		case "/umbraco/management/api/v1/document-type":
			t.Fatalf("doctype list should prefer the v17 tree root endpoint over /document-type")
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "doctype", "list", "--first-n", "1", "--fields", "id,name")
	if err != nil {
		t.Fatalf("doctype list --first-n --fields failed: %v", err)
	}
	if strings.Contains(observedPath, "fields=") {
		t.Fatalf("expected --fields to stay client-side, got %q", observedPath)
	}
	if !strings.Contains(observedPath, "/tree/document-type/root") {
		t.Fatalf("expected doctype list to use tree root endpoint, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode doctype list payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one returned item, got %+v", payload)
	}
	item := items[0].(map[string]any)
	if len(item) != 2 || item["id"] != "dt-1" || item["name"] != "Article" {
		t.Fatalf("expected projected doctype item, got %+v", item)
	}
	if payload["returned"] != float64(1) {
		t.Fatalf("expected returned=1, got %+v", payload)
	}
}

func TestDoctypeListRecursiveTypesOnlyWalksFolders(t *testing.T) {
	childRequests := map[string]int{}

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document-type/root":
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"folder-1","name":"Compositions","isFolder":true,"icon":"icon-folder"},
				{"id":"dt-article","name":"Article","alias":"article","icon":"icon-document"}
			]}`), nil
		case "/umbraco/management/api/v1/tree/document-type/children":
			parentID := req.URL.Query().Get("parentId")
			childRequests[parentID]++
			switch parentID {
			case "folder-1":
				return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
					{"id":"folder-2","name":"Nested","isFolder":true,"icon":"icon-folder"},
					{"id":"dt-seo","name":"SEO","alias":"seo","icon":"icon-document"}
				]}`), nil
			case "folder-2":
				return datatypeJSONResponse(http.StatusOK, `{"total":1,"items":[
					{"id":"dt-card","name":"Card","alias":"card","icon":"icon-document"}
				]}`), nil
			default:
				return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
			}
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "doctype", "list", "--recursive", "--types-only", "--fields", "id,name,alias")
	if err != nil {
		t.Fatalf("doctype list --recursive --types-only failed: %v", err)
	}
	if childRequests["folder-1"] != 1 || childRequests["folder-2"] != 1 {
		t.Fatalf("expected recursive folder walk, got requests %+v", childRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode recursive doctype list payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected three document types and no folders, got %+v", payload)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["alias"] == nil {
			t.Fatalf("expected folders to be excluded from types-only output, got %+v", payload)
		}
	}
}

func TestDoctypeListRecursiveFirstNStopsWalkingAfterLimit(t *testing.T) {
	childRequests := map[string]int{}

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document-type/root":
			return datatypeJSONResponse(http.StatusOK, `{"total":3,"items":[
				{"id":"folder-1","name":"Compositions","isFolder":true,"icon":"icon-folder"},
				{"id":"folder-2","name":"Unused","isFolder":true,"icon":"icon-folder"},
				{"id":"dt-article","name":"Article","alias":"article","icon":"icon-document"}
			]}`), nil
		case "/umbraco/management/api/v1/tree/document-type/children":
			parentID := req.URL.Query().Get("parentId")
			childRequests[parentID]++
			switch parentID {
			case "folder-1":
				return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
					{"id":"dt-seo","name":"SEO","alias":"seo","icon":"icon-document"},
					{"id":"dt-card","name":"Card","alias":"card","icon":"icon-document"}
				]}`), nil
			case "folder-2":
				return datatypeJSONResponse(http.StatusOK, `{"total":1,"items":[
					{"id":"dt-unused","name":"Unused","alias":"unused","icon":"icon-document"}
				]}`), nil
			default:
				return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
			}
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "doctype", "list", "--recursive", "--first-n", "2", "--fields", "id,name")
	if err != nil {
		t.Fatalf("doctype list --recursive --first-n failed: %v", err)
	}
	if childRequests["folder-1"] != 1 {
		t.Fatalf("expected one request for the first folder, got requests %+v", childRequests)
	}
	if childRequests["folder-2"] != 0 {
		t.Fatalf("expected traversal to stop before the second folder, got requests %+v", childRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode recursive doctype list payload: %v", err)
	}
	if payload["returned"] != float64(2) {
		t.Fatalf("expected returned=2, got %+v", payload)
	}
	items := payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected two returned items, got %+v", payload)
	}
	if items[0].(map[string]any)["id"] != "folder-1" || items[1].(map[string]any)["id"] != "dt-seo" {
		t.Fatalf("expected traversal to return first folder and first child, got %+v", payload)
	}
}

func TestDoctypeGetFolderIDReturnsFolderDiagnostic(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/f0f0f0f0-0000-4000-8000-000000000001", "/umbraco/management/api/v1/document-type/f0f0f0f0-0000-4000-8000-000000000002":
			return datatypeJSONResponse(http.StatusNotFound, `{"title":"Not Found"}`), nil
		case "/umbraco/management/api/v1/document-type/folder/f0f0f0f0-0000-4000-8000-000000000001":
			return datatypeJSONResponse(http.StatusOK, `{"id":"f0f0f0f0-0000-4000-8000-000000000001","name":"Compositions"}`), nil
		case "/umbraco/management/api/v1/tree/document-type/children":
			// The tree answers 200 with an empty page for any parentId; it must
			// no longer be what decides "folder".
			return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "doctype", "get", "f0f0f0f0-0000-4000-8000-000000000001")
	if err == nil || !strings.Contains(err.Error(), "is a folder, not a document type") {
		t.Fatalf("expected folder-specific diagnostic, got %v", err)
	}

	_, err = execute(buildRootWithCollections(t, deps), "doctype", "get", "f0f0f0f0-0000-4000-8000-000000000002")
	if err == nil || strings.Contains(err.Error(), "is a folder") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected the real 404 for a missing document type, got %v", err)
	}
}

func TestDoctypeUpdateMergeJSONFetchesCurrentAndSendsMergedPayload(t *testing.T) {
	var putPayload map[string]any
	var getRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				getRequests++
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "icon":"icon-document",
  "properties":[
    {"alias":"title","name":"Title","dataType":{"id":"dt-text"}},
    {"alias":"body","name":"Body","dataType":{"id":"dt-rte"}}
  ],
  "containers":[
    {"id":"c-1","name":"Content","type":"Tab","sortOrder":0}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&putPayload); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "update", "dt-1",
		"--merge-json", `{"properties":[{"alias":"title","name":"Headline"}]}`,
	)
	if err != nil {
		t.Fatalf("doctype merge update failed: %v", err)
	}

	if getRequests != 1 {
		t.Fatalf("expected one fetch of the current doctype, got %d", getRequests)
	}
	if putPayload["alias"] != "partnerPage" || putPayload["icon"] != "icon-document" {
		t.Fatalf("expected required fields to be preserved, got %+v", putPayload)
	}

	properties, ok := putPayload["properties"].([]any)
	if !ok || len(properties) != 2 {
		t.Fatalf("expected merged properties array, got %+v", putPayload["properties"])
	}

	var titleEntry map[string]any
	var bodyEntry map[string]any
	for _, item := range properties {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected property entry to be an object, got %T", item)
		}
		switch entry["alias"] {
		case "title":
			titleEntry = entry
		case "body":
			bodyEntry = entry
		}
	}
	if titleEntry == nil || titleEntry["name"] != "Headline" {
		t.Fatalf("expected title alias to be merged with new name, got %+v", titleEntry)
	}
	titleDataType, ok := titleEntry["dataType"].(map[string]any)
	if !ok || titleDataType["id"] != "dt-text" {
		t.Fatalf("expected title dataType to be preserved by merge, got %+v", titleEntry["dataType"])
	}
	if bodyEntry == nil || bodyEntry["name"] != "Body" {
		t.Fatalf("expected unrelated property to be preserved unchanged, got %+v", bodyEntry)
	}

	containers, ok := putPayload["containers"].([]any)
	if !ok || len(containers) != 1 {
		t.Fatalf("expected containers to be preserved, got %+v", putPayload["containers"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode doctype merge update result: %v", err)
	}
	if result["updated"] != true {
		t.Fatalf("unexpected update result payload: %+v", result)
	}
}

func TestDoctypeCreateElementFlagSetsIsElementTrue(t *testing.T) {
	var postPayload map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type":
			if req.Method != http.MethodPost {
				return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&postPayload); err != nil {
				t.Fatalf("failed to decode create payload: %v", err)
			}
			return datatypeJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	// --element should win over an explicit isElement:false in --json so agents
	// can opt in with a single flag instead of editing the JSON payload.
	if _, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "create",
		"--element",
		"--json", `{"name":"HeroBlock","alias":"heroBlock","isElement":false}`,
	); err != nil {
		t.Fatalf("doctype create --element failed: %v", err)
	}
	if postPayload["isElement"] != true {
		t.Fatalf("expected --element to force isElement=true, got %+v", postPayload["isElement"])
	}
}

func TestDoctypeCreateNormalizesDataTypeIDAndReturnsCreatedIdentity(t *testing.T) {
	var postPayload map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type":
			if req.Method != http.MethodPost {
				return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&postPayload); err != nil {
				t.Fatalf("failed to decode create payload: %v", err)
			}
			return datatypeJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "create",
		"--json", `{"name":"Article","alias":"article","properties":[{"name":"Title","alias":"title","dataTypeId":"dt-text"}]}`,
	)
	if err != nil {
		t.Fatalf("doctype create failed: %v", err)
	}

	if postPayload["id"] == "" {
		t.Fatalf("expected CLI to generate an id, got %+v", postPayload)
	}
	properties := postPayload["properties"].([]any)
	property := properties[0].(map[string]any)
	if _, exists := property["dataTypeId"]; exists {
		t.Fatalf("expected dataTypeId shortcut to be removed, got %+v", property)
	}
	dataType := property["dataType"].(map[string]any)
	if dataType["id"] != "dt-text" {
		t.Fatalf("expected nested dataType id, got %+v", property)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode create result: %v", err)
	}
	if result["id"] != postPayload["id"] || result["name"] != "Article" || result["alias"] != "article" {
		t.Fatalf("expected minimal created identity, got %+v", result)
	}
}

func TestDoctypeUpdateMergeJSONSupportsDryRun(t *testing.T) {
	var getRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				getRequests++
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "icon":"icon-document",
  "properties":[{"alias":"title","name":"Title"}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected write"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "update", "dt-1",
		"--merge-json", `{"properties":[{"alias":"title","name":"Headline"}]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("doctype merge update dry-run failed: %v", err)
	}

	if getRequests != 1 {
		t.Fatalf("expected dry-run merge update to fetch the current doctype once, got %d", getRequests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing dry-run body: %+v", payload)
	}
	properties, ok := body["properties"].([]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("missing merged properties in dry-run body: %+v", body)
	}
	entry := properties[0].(map[string]any)
	if entry["name"] != "Headline" {
		t.Fatalf("unexpected dry-run merged property: %+v", entry)
	}
}

func TestDocumentTypeAliasRoutesToDoctypeCommand(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type":
			observedPath = req.URL.Path
			return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document-type", "list"); err != nil {
		t.Fatalf("document-type alias failed: %v", err)
	}
	if observedPath != "/umbraco/management/api/v1/document-type" {
		t.Fatalf("expected document-type alias to hit /document-type, got %q", observedPath)
	}
}

func TestDoctypeAddPropertyAppendsPropertyUnderResolvedContainer(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "icon":"icon-document",
  "properties":[
    {"alias":"title","name":"Title","container":{"id":"c-1"},"sortOrder":0,"dataType":{"id":"dt-text"}}
  ],
  "containers":[
    {"id":"c-1","name":"Content","type":"Tab","sortOrder":0}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "subtitle",
		"--name", "Subtitle",
		"--data-type", "dt-text",
		"--container", "content",
	)
	if err != nil {
		t.Fatalf("doctype add-property failed: %v", err)
	}

	if observedPutBody["alias"] != "partnerPage" || observedPutBody["icon"] != "icon-document" {
		t.Fatalf("expected required doctype fields to be preserved, got %+v", observedPutBody)
	}

	properties, ok := observedPutBody["properties"].([]any)
	if !ok || len(properties) != 2 {
		t.Fatalf("expected appended property to produce two entries, got %+v", observedPutBody["properties"])
	}

	var subtitle map[string]any
	for _, item := range properties {
		entry := item.(map[string]any)
		if entry["alias"] == "subtitle" {
			subtitle = entry
		}
	}
	if subtitle == nil {
		t.Fatalf("expected subtitle property to be appended, got %+v", properties)
	}
	if subtitle["name"] != "Subtitle" {
		t.Fatalf("unexpected appended property name: %+v", subtitle)
	}
	if id, _ := subtitle["id"].(string); id == "" {
		t.Fatalf("expected new property to have a generated id, got %+v", subtitle)
	}
	if container, _ := subtitle["container"].(map[string]any); container == nil || container["id"] != "c-1" {
		t.Fatalf("expected container id to be resolved from alias, got %+v", subtitle["container"])
	}
	if dataType, _ := subtitle["dataType"].(map[string]any); dataType == nil || dataType["id"] != "dt-text" {
		t.Fatalf("unexpected data type reference: %+v", subtitle["dataType"])
	}
	if sortOrder, _ := subtitle["sortOrder"].(float64); sortOrder != 1 {
		t.Fatalf("expected sortOrder to follow existing properties, got %v", subtitle["sortOrder"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode add-property result: %v", err)
	}
	if result["updated"] != true {
		t.Fatalf("unexpected add-property result payload: %+v", result)
	}
}

func TestDoctypeAddPropertyRejectsUnknownContainer(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "subtitle",
		"--name", "Subtitle",
		"--data-type", "dt-text",
		"--container", "missing",
	)
	if err == nil {
		t.Fatalf("expected add-property to fail when the container name is not found")
	}
	if !strings.Contains(err.Error(), "no container named") {
		t.Fatalf("unexpected container resolution error: %v", err)
	}
	if putRequests != 0 {
		t.Fatalf("expected unknown container to short-circuit before PUT, got %d writes", putRequests)
	}
}

func TestDoctypeAddPropertyRejectsDuplicateAlias(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[{"alias":"title","name":"Title","container":{"id":"c-1"}}],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "title",
		"--name", "Title",
		"--data-type", "dt-text",
		"--container", "content",
	)
	if err == nil {
		t.Fatalf("expected add-property to fail when the alias is already in use")
	}
	if !strings.Contains(err.Error(), "already has a property") {
		t.Fatalf("unexpected duplicate alias error: %v", err)
	}
	if putRequests != 0 {
		t.Fatalf("expected duplicate alias to short-circuit before PUT, got %d writes", putRequests)
	}
}

func TestDoctypeAddPropertyRejectsAmbiguousContainerName(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[],
  "containers":[
    {"id":"c-1","name":"Content","type":"Tab","sortOrder":0},
    {"id":"c-2","name":"Content","type":"Group","parent":{"id":"c-1"},"sortOrder":0}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "subtitle",
		"--name", "Subtitle",
		"--data-type", "dt-text",
		"--container", "Content",
	)
	if err == nil {
		t.Fatalf("expected ambiguous container name to fail")
	}
	if !strings.Contains(err.Error(), "multiple containers named") {
		t.Fatalf("unexpected ambiguity error: %v", err)
	}
	if putRequests != 0 {
		t.Fatalf("expected ambiguous container to short-circuit before PUT, got %d writes", putRequests)
	}
}

func TestDoctypeAddPropertySupportsDryRun(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"unexpected write"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "subtitle",
		"--name", "Subtitle",
		"--data-type", "dt-text",
		"--container", "content",
		"--mandatory",
		"--description", "Shown under the title",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("doctype add-property dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode add-property dry-run payload: %v", err)
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true, got %+v", payload)
	}

	body := payload["body"].(map[string]any)
	properties := body["properties"].([]any)
	if len(properties) != 1 {
		t.Fatalf("expected dry-run body to include the new property, got %+v", properties)
	}
	added := properties[0].(map[string]any)
	if added["description"] != "Shown under the title" {
		t.Fatalf("expected description to be carried into payload, got %+v", added)
	}
	validation := added["validation"].(map[string]any)
	if validation["mandatory"] != true {
		t.Fatalf("expected --mandatory to set validation.mandatory=true, got %+v", validation)
	}
}

func TestDoctypeAddContainerAppendsTabAtRoot(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				// After the PUT the verification read must see the saved
				// state, container included.
				if observedPutBody != nil {
					saved, _ := json.Marshal(observedPutBody)
					return datatypeJSONResponse(http.StatusOK, string(saved)), nil
				}
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "icon":"icon-document",
  "properties":[],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-container", "dt-1",
		"--name", "SEO",
		"--type", "Tab",
	)
	if err != nil {
		t.Fatalf("doctype add-container failed: %v", err)
	}

	if observedPutBody["alias"] != "partnerPage" || observedPutBody["icon"] != "icon-document" {
		t.Fatalf("expected required doctype fields to be preserved, got %+v", observedPutBody)
	}

	containers, ok := observedPutBody["containers"].([]any)
	if !ok || len(containers) != 2 {
		t.Fatalf("expected appended container to produce two entries, got %+v", observedPutBody["containers"])
	}

	var seo map[string]any
	for _, item := range containers {
		entry := item.(map[string]any)
		if entry["name"] == "SEO" {
			seo = entry
		}
	}
	if seo == nil {
		t.Fatalf("expected SEO container to be appended, got %+v", containers)
	}
	if seo["type"] != "Tab" {
		t.Fatalf("expected normalized Tab type, got %+v", seo["type"])
	}
	if id, _ := seo["id"].(string); id == "" {
		t.Fatalf("expected new container to have a generated id, got %+v", seo)
	}
	if seo["parent"] != nil {
		t.Fatalf("expected root container to have nil parent, got %+v", seo["parent"])
	}
	if sortOrder, _ := seo["sortOrder"].(float64); sortOrder != 1 {
		t.Fatalf("expected sortOrder to follow the existing tab, got %v", seo["sortOrder"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode add-container result: %v", err)
	}
	if result["updated"] != true {
		t.Fatalf("unexpected add-container result payload: %+v", result)
	}
}

func TestDoctypeAddContainerResolvesParentByName(t *testing.T) {
	var observedPutBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				if observedPutBody != nil {
					saved, _ := json.Marshal(observedPutBody)
					return datatypeJSONResponse(http.StatusOK, string(saved)), nil
				}
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-container", "dt-1",
		"--name", "Hero",
		"--type", "group",
		"--parent", "content",
	)
	if err != nil {
		t.Fatalf("doctype add-container with parent failed: %v", err)
	}

	containers := observedPutBody["containers"].([]any)
	if len(containers) != 2 {
		t.Fatalf("expected appended container, got %+v", containers)
	}
	var hero map[string]any
	for _, item := range containers {
		entry := item.(map[string]any)
		if entry["name"] == "Hero" {
			hero = entry
		}
	}
	if hero == nil {
		t.Fatalf("expected Hero container, got %+v", containers)
	}
	if hero["type"] != "Group" {
		t.Fatalf("expected normalized Group type from lowercase input, got %+v", hero["type"])
	}
	parent, _ := hero["parent"].(map[string]any)
	if parent == nil || parent["id"] != "c-1" {
		t.Fatalf("expected parent id to be resolved from name, got %+v", hero["parent"])
	}
}

func TestDoctypeAddContainerRejectsBadType(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-container", "dt-1",
		"--name", "Whatever",
		"--type", "Section",
	)
	if err == nil {
		t.Fatalf("expected unsupported container type to fail")
	}
	if !strings.Contains(err.Error(), "must be Tab or Group") {
		t.Fatalf("unexpected type validation error: %v", err)
	}
}

func TestDoctypeAddContainerRejectsDuplicateName(t *testing.T) {
	var putRequests int

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "properties":[],
  "containers":[{"id":"c-1","name":"Content","type":"Tab","sortOrder":0}]
}`), nil
			}
			if req.Method == http.MethodPut {
				putRequests++
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-container", "dt-1",
		"--name", "Content",
		"--type", "Tab",
	)
	if err == nil {
		t.Fatalf("expected duplicate container name to fail")
	}
	if !strings.Contains(err.Error(), "already has a container named") {
		t.Fatalf("unexpected duplicate container error: %v", err)
	}
	if putRequests != 0 {
		t.Fatalf("expected duplicate container to short-circuit before PUT, got %d writes", putRequests)
	}
}

func TestDoctypeUpdateRejectsJSONAndMergeJSONTogether(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	_, err := execute(
		root,
		"doctype", "update", "dt-1",
		"--json", `{"name":"Full"}`,
		"--merge-json", `{"properties":[]}`,
	)
	if err == nil {
		t.Fatalf("expected doctype update to reject simultaneous --json and --merge-json")
	}
	if !strings.Contains(err.Error(), "exactly one of --json (full replacement) or --merge-json (fetch and merge)") {
		t.Fatalf("unexpected merge-json validation error: %v", err)
	}
}

const reorderDoctypePayload = `{
  "id":"dt-1",
  "alias":"partnerPage",
  "name":"Partner Page",
  "icon":"icon-document",
  "properties":[
    {"alias":"title","name":"Title","container":{"id":"c-1"},"sortOrder":0,"dataType":{"id":"dt-text"}},
    {"alias":"subtitle","name":"Subtitle","container":{"id":"c-1"},"sortOrder":1,"dataType":{"id":"dt-text"}},
    {"alias":"body","name":"Body","container":{"id":"c-1"},"sortOrder":2,"dataType":{"id":"dt-rte"}},
    {"alias":"seoTitle","name":"SEO Title","container":{"id":"c-2"},"sortOrder":0,"dataType":{"id":"dt-text"}}
  ],
  "containers":[
    {"id":"c-1","name":"Content","type":"Tab","sortOrder":0},
    {"id":"c-2","name":"SEO","type":"Tab","sortOrder":1}
  ]
}`

func reorderDoctypeDeps(t *testing.T, observed *map[string]any) Dependencies {
	t.Helper()
	return datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, reorderDoctypePayload), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(observed); err != nil {
					t.Fatalf("failed to decode put payload: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `null`), nil
			}
			return datatypeJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
}

func reorderSortOrders(t *testing.T, body map[string]any) map[string]float64 {
	t.Helper()
	properties, ok := body["properties"].([]any)
	if !ok {
		t.Fatalf("missing properties in put payload: %+v", body)
	}
	orders := make(map[string]float64, len(properties))
	for _, item := range properties {
		entry := item.(map[string]any)
		alias, _ := entry["alias"].(string)
		value, _ := entry["sortOrder"].(float64)
		orders[alias] = value
	}
	return orders
}

func TestDoctypeReorderPropertiesAssignsPositionsAndKeepsRest(t *testing.T) {
	var observed map[string]any
	deps := reorderDoctypeDeps(t, &observed)

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "reorder-properties", "dt-1",
		"--aliases", "body,title",
	); err != nil {
		t.Fatalf("doctype reorder-properties failed: %v", err)
	}

	orders := reorderSortOrders(t, observed)
	if orders["body"] != 0 || orders["title"] != 1 {
		t.Fatalf("expected listed aliases to take positions 0,1, got %+v", orders)
	}
	if orders["subtitle"] != 2 {
		t.Fatalf("expected unlisted container property to follow the listed ones, got %+v", orders)
	}
	if orders["seoTitle"] != 0 {
		t.Fatalf("expected other-container property untouched, got %+v", orders)
	}
	if name := func() string {
		for _, item := range observed["properties"].([]any) {
			entry := item.(map[string]any)
			if entry["alias"] == "body" {
				s, _ := entry["name"].(string)
				return s
			}
		}
		return ""
	}(); name != "Body" {
		t.Fatalf("expected merge to preserve property fields, got name %q", name)
	}
}

func TestDoctypeReorderPropertiesSingleMoveSetsSortOrderVerbatim(t *testing.T) {
	var observed map[string]any
	deps := reorderDoctypeDeps(t, &observed)

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "reorder-properties", "dt-1",
		"--alias", "body",
		"--sort-order", "0",
	); err != nil {
		t.Fatalf("doctype reorder-properties single move failed: %v", err)
	}

	orders := reorderSortOrders(t, observed)
	if orders["body"] != 0 {
		t.Fatalf("expected body moved to 0, got %+v", orders)
	}
	if orders["title"] != 0 || orders["subtitle"] != 1 {
		t.Fatalf("expected other properties to keep their sortOrder, got %+v", orders)
	}
}

func TestDoctypeReorderPropertiesRejectsCrossContainerOrder(t *testing.T) {
	var observed map[string]any
	deps := reorderDoctypeDeps(t, &observed)

	_, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "reorder-properties", "dt-1",
		"--aliases", "title,seoTitle",
	)
	if err == nil || !strings.Contains(err.Error(), "different containers") {
		t.Fatalf("expected cross-container rejection, got %v", err)
	}
}

func TestDoctypeReorderPropertiesRejectsUnknownAliasAndModeMix(t *testing.T) {
	var observed map[string]any
	deps := reorderDoctypeDeps(t, &observed)

	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "reorder-properties", "dt-1", "--aliases", "missing"); err == nil || !strings.Contains(err.Error(), `no property with alias "missing"`) {
		t.Fatalf("expected unknown alias error, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "reorder-properties", "dt-1", "--aliases", ","); err == nil || !strings.Contains(err.Error(), "parsed to no property aliases") {
		t.Fatalf("expected empty alias list rejection, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "reorder-properties", "dt-1"); err == nil || !strings.Contains(err.Error(), "exactly one of") {
		t.Fatalf("expected mode requirement error, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "reorder-properties", "dt-1", "--alias", "body"); err == nil || !strings.Contains(err.Error(), "--sort-order") {
		t.Fatalf("expected sort-order requirement error, got %v", err)
	}
}

func TestDoctypeAddContainerReportsPrunedContainerHonestly(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodPut {
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			// The server accepted the PUT and pruned the empty container:
			// every read reports no containers.
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-1","alias":"partnerPage","name":"Partner Page","properties":[],"containers":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "doctype", "add-container", "dt-1", "--name", "Content", "--type", "Group")
	if err == nil || !strings.Contains(err.Error(), "pruned the empty container") || !strings.Contains(err.Error(), "--create-container") {
		t.Fatalf("expected honest prune error with the workflow hint, got %v", err)
	}
}

func TestDoctypeAddPropertyCreateContainerBuildsBothInOnePut(t *testing.T) {
	var observedPutBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document-type/dt-1":
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("decode put: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-1","alias":"partnerPage","name":"Partner Page","properties":[],"containers":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"doctype", "add-property", "dt-1",
		"--alias", "title", "--name", "Title", "--data-type", "dt-text",
		"--container", "Content", "--create-container", "--container-type", "tab",
	); err != nil {
		t.Fatalf("add-property --create-container failed: %v", err)
	}
	containers, _ := observedPutBody["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected the new container in the same PUT, got %+v", observedPutBody["containers"])
	}
	created := containers[0].(map[string]any)
	if created["name"] != "Content" || created["type"] != "Tab" {
		t.Fatalf("expected normalized container, got %+v", created)
	}
	properties, _ := observedPutBody["properties"].([]any)
	if len(properties) != 1 {
		t.Fatalf("expected the property in the same PUT, got %+v", observedPutBody["properties"])
	}
	property := properties[0].(map[string]any)
	reference, _ := property["container"].(map[string]any)
	if reference["id"] != created["id"] {
		t.Fatalf("expected the property to reference the created container id, got property=%v container=%v", reference, created["id"])
	}
}

func TestDoctypeAddPropertyMissingContainerHintsCreateFlag(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-1","alias":"x","name":"X","properties":[],"containers":[]}`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "doctype", "add-property", "dt-1", "--alias", "a", "--name", "A", "--data-type", "d", "--container", "Nope")
	if err == nil || !strings.Contains(err.Error(), "--create-container") {
		t.Fatalf("expected missing-container hint, got %v", err)
	}
	_, err = execute(buildRootWithCollections(t, deps), "doctype", "add-property", "dt-1", "--alias", "a", "--name", "A", "--data-type", "d", "--container", "Nope", "--container-type", "Tab")
	if err == nil || !strings.Contains(err.Error(), "--container-type only applies") {
		t.Fatalf("expected container-type guard, got %v", err)
	}
}

func TestDoctypeCreateFolderPostsAndReadsBack(t *testing.T) {
	var posted map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type/folder" && req.Method == http.MethodPost:
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &posted)
			return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document-type/folder/") && req.Method == http.MethodGet:
			id := strings.TrimPrefix(req.URL.Path, "/umbraco/management/api/v1/document-type/folder/")
			return datatypeJSONResponse(http.StatusOK, `{"id":"`+id+`","name":"Sectors","isTrashed":false}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	out, err := execute(buildRootWithCollections(t, deps), "doctype", "create-folder", "--name", "Sectors", "--parent", "16c53fcd-e119-4c96-a327-898adcbfac27")
	if err != nil {
		t.Fatalf("doctype create-folder failed: %v", err)
	}
	if posted["name"] != "Sectors" {
		t.Fatalf("expected the folder name in the POST body, got %+v", posted)
	}
	parent, _ := posted["parent"].(map[string]any)
	if parent["id"] != "16c53fcd-e119-4c96-a327-898adcbfac27" {
		t.Fatalf("expected parent reference in the POST body, got %+v", posted)
	}
	if id, _ := posted["id"].(string); !isUUIDLike(id) {
		t.Fatalf("expected a generated folder id, got %+v", posted)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["created"] != true || result["name"] != "Sectors" || result["id"] != posted["id"] {
		t.Fatalf("expected the read-back folder in the result, got %s", out)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "create-folder", "--name", "X", "--parent", "not-a-guid"); err == nil || !strings.Contains(err.Error(), "parent must be") {
		t.Fatalf("expected parent validation, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "create-folder"); err == nil || !strings.Contains(err.Error(), "requires a folder name") {
		t.Fatalf("expected the name to be required, got %v", err)
	}

	// --json is the primary path; flags fill only what the payload omits.
	posted = nil
	out, err = execute(buildRootWithCollections(t, deps), "doctype", "create-folder", "--json", `{"id":"bbbbbbbb-0000-4000-8000-000000000001","name":"From JSON"}`, "--name", "Ignored", "--parent", "16c53fcd-e119-4c96-a327-898adcbfac27")
	if err != nil {
		t.Fatalf("doctype create-folder --json failed: %v", err)
	}
	if posted["id"] != "bbbbbbbb-0000-4000-8000-000000000001" || posted["name"] != "From JSON" {
		t.Fatalf("expected the --json id and name to win, got %+v", posted)
	}
	if parent, _ := posted["parent"].(map[string]any); parent["id"] != "16c53fcd-e119-4c96-a327-898adcbfac27" {
		t.Fatalf("expected --parent to fill the omitted parent, got %+v", posted)
	}
	if !strings.Contains(out, `"bbbbbbbb-0000-4000-8000-000000000001"`) || !strings.Contains(out, `"created": true`) {
		t.Fatalf("expected the read-back folder, got %s", out)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "create-folder", "--json", `{"name":"X","parent":"16c53fcd-e119-4c96-a327-898adcbfac27"}`); err == nil || !strings.Contains(err.Error(), "parent must be") {
		t.Fatalf("expected a non-object parent to be rejected, got %v", err)
	}
}

func TestDoctypeDeleteFolderIsGatedAndHitsFolderRoute(t *testing.T) {
	var deleted string
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.Method == http.MethodDelete:
			deleted = req.URL.Path
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "delete-folder", "folder-1"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the force/dry-run gate, got %v", err)
	}
	out, err := execute(buildRootWithCollections(t, deps), "datatype", "delete-folder", "folder-1", "--force")
	if err != nil || deleted != "/umbraco/management/api/v1/data-type/folder/folder-1" || !strings.Contains(out, `"deleted": true`) {
		t.Fatalf("expected DELETE on the folder route, got err=%v path=%s out=%s", err, deleted, out)
	}
}

func TestDoctypeCreateNormalizesLegacyHistoryCleanup(t *testing.T) {
	var posted map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type" && req.Method == http.MethodPost:
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &posted)
			return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "create", "--json", `{"name":"A","alias":"a","historyCleanup":{"preventCleanup":true}}`); err != nil {
		t.Fatalf("doctype create failed: %v", err)
	}
	if _, legacy := posted["historyCleanup"]; legacy {
		t.Fatalf("expected historyCleanup renamed to cleanup, got %+v", posted)
	}
	if cleanup, _ := posted["cleanup"].(map[string]any); cleanup["preventCleanup"] != true {
		t.Fatalf("expected cleanup carried over, got %+v", posted)
	}
}

func TestDoctypeRemovePropertyWritesBackWithoutPropertyAndVerifies(t *testing.T) {
	const doctypeID = "aaaaaaaa-0000-4000-8000-0000000000aa"
	current := `{"id":"` + doctypeID + `","name":"Article","alias":"article","allowedAsRoot":true,"icon":"icon-document",
		"containers":[{"id":"tab-1","name":"Content","type":"Tab","parent":null,"sortOrder":0},{"id":"group-1","name":"Meta","type":"Group","parent":{"id":"tab-1"},"sortOrder":0},{"id":"group-2","name":"Body","type":"Group","parent":{"id":"tab-1"},"sortOrder":1}],
		"properties":[{"id":"p-1","alias":"title","name":"Title","container":{"id":"group-2"}},{"id":"p-2","alias":"seoTitle","name":"SEO title","container":{"id":"group-1"}}]}`
	var putBody map[string]any
	afterBody := current
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type/"+doctypeID && req.Method == http.MethodGet:
			if putBody != nil {
				return datatypeJSONResponse(http.StatusOK, afterBody), nil
			}
			return datatypeJSONResponse(http.StatusOK, current), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type/"+doctypeID && req.Method == http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &putBody)
			encoded, _ := json.Marshal(putBody)
			afterBody = string(encoded)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	dir := t.TempDir()
	backupPath := dir + "/article.backup.json"
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "seoTitle"); err == nil || !strings.Contains(err.Error(), "--force") || putBody != nil {
		t.Fatalf("expected the force/dry-run gate before any write, got err=%v put=%v", err, putBody != nil)
	}
	out, err := execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "seoTitle", "--backup="+backupPath, "--force")
	if err != nil {
		t.Fatalf("doctype remove-property failed: %v", err)
	}
	properties := putBody["properties"].([]any)
	if len(properties) != 1 || properties[0].(map[string]any)["alias"] != "title" {
		t.Fatalf("expected the PUT body to carry only the remaining property, got %+v", properties)
	}
	if putBody["allowedAsRoot"] != true || putBody["icon"] != "icon-document" || len(putBody["containers"].([]any)) != 3 {
		t.Fatalf("expected every other field written back unchanged, got %+v", putBody)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["verified"] != true || result["remainingProperties"] != float64(1) || result["backup"] != backupPath {
		t.Fatalf("unexpected result: %s", out)
	}
	removed := result["removed"].(map[string]any)
	if removed["alias"] != "seoTitle" || removed["id"] != "p-2" {
		t.Fatalf("expected the removed property described, got %+v", removed)
	}
	// Group "Meta" is left empty (the server will prune it); the tab stays
	// alive through the still-populated "Body" group.
	pruned, _ := result["prunedContainers"].([]any)
	if len(pruned) != 1 || pruned[0] != "Meta" {
		t.Fatalf("expected the emptied group flagged, got %+v", result["prunedContainers"])
	}
	envelope, err := readBackup(backupPath, "doctype")
	if err != nil || envelope.ID != doctypeID || len(envelope.Entity["properties"].([]any)) != 2 {
		t.Fatalf("expected the pre-change type in the backup, got err=%v envelope=%+v", err, envelope)
	}
}

func TestDoctypeRemovePropertyRejectsUnknownAliasAndFailsWhenNotRemoved(t *testing.T) {
	const doctypeID = "aaaaaaaa-0000-4000-8000-0000000000ab"
	current := `{"id":"` + doctypeID + `","name":"Article","alias":"article","containers":[],"properties":[{"id":"p-1","alias":"seoTitle","name":"SEO title","container":null}]}`
	puts := 0
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/document-type/"+doctypeID && req.Method == http.MethodGet:
			return datatypeJSONResponse(http.StatusOK, current), nil // never changes: the server "kept" the property
		case req.URL.Path == "/umbraco/management/api/v1/document-type/"+doctypeID && req.Method == http.MethodPut:
			puts++
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "seotitle", "--force")
	if err == nil || !strings.Contains(err.Error(), `did you mean "seoTitle"`) || puts != 0 {
		t.Fatalf("expected a case hint and no write, got err=%v puts=%d", err, puts)
	}
	_, err = execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "nothere", "--force")
	if err == nil || !strings.Contains(err.Error(), `has no property with alias "nothere"`) || puts != 0 {
		t.Fatalf("expected an unknown-alias error and no write, got err=%v puts=%d", err, puts)
	}
	_, err = execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "seoTitle", "--force")
	if err == nil || !strings.Contains(err.Error(), `still has property "seoTitle"`) || puts != 1 {
		t.Fatalf("expected the verify step to fail when the property survives, got err=%v puts=%d", err, puts)
	}
	out, err := execute(buildRootWithCollections(t, deps), "doctype", "remove-property", doctypeID, "--alias", "seoTitle", "--dry-run")
	if err != nil || !strings.Contains(out, `"dryRun": true`) || puts != 1 {
		t.Fatalf("expected dry-run to plan without writing, got err=%v out=%s puts=%d", err, out, puts)
	}
}

func TestSchemaTypeRemovePropertyStripsResponseOnlyFields(t *testing.T) {
	const typeID = "aaaaaaaa-0000-4000-8000-0000000000ac"
	var putBody map[string]any
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/media-type/"+typeID && req.Method == http.MethodGet:
			if putBody != nil {
				return endpointJSONResponse(http.StatusOK, `{"id":"`+typeID+`","alias":"image","properties":[]}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `{"id":"`+typeID+`","alias":"image","isDeletable":false,"aliasCanBeChanged":true,"containers":[],"properties":[{"id":"p-1","alias":"umbracoFile","name":"File"}]}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/media-type/"+typeID && req.Method == http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &putBody)
			return endpointJSONResponse(http.StatusOK, `{}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	if _, err := execute(buildSchemaTypeRoot(deps), "mediatype", "remove-property", typeID, "--alias", "umbracoFile", "--force"); err != nil {
		t.Fatalf("mediatype remove-property failed: %v", err)
	}
	for _, rejected := range []string{"id", "isDeletable", "aliasCanBeChanged"} {
		if _, present := putBody[rejected]; present {
			t.Fatalf("expected %s stripped from the PUT body, got %+v", rejected, putBody)
		}
	}
	if len(putBody["properties"].([]any)) != 0 {
		t.Fatalf("expected the property removed, got %+v", putBody)
	}
}
