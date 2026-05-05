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
    {"id":"c-1","alias":"content","name":"Content","type":"Tab"}
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
