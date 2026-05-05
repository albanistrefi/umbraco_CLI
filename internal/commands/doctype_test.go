package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
	if !strings.Contains(err.Error(), "exactly one of --json or --merge-json") {
		t.Fatalf("unexpected merge-json validation error: %v", err)
	}
}
