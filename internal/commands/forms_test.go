package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// All forms commands must hit the Forms Management API prefix
// (/umbraco/forms/management/api/v1), not the core CMS prefix. These tests
// pin that behavior end-to-end so a regression in RequestOptions.APIPrefix
// surfaces immediately.

func TestFormsListPrefersTreeRootUnderFormsPrefix(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/root":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"f-1","name":"Contact","alias":"contact"},
				{"id":"f-2","name":"Newsletter","alias":"newsletter"}
			]}`), nil
		case "/umbraco/forms/management/api/v1/form":
			t.Fatalf("forms list should prefer /tree/form/root over /form")
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "list", "--first-n", "1", "--fields", "id,name")
	if err != nil {
		t.Fatalf("forms list failed: %v", err)
	}
	if strings.Contains(observedPath, "fields=") {
		t.Fatalf("expected --fields to stay client-side, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode forms list payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected --first-n=1 to project a single item, got %+v", items)
	}
	item := items[0].(map[string]any)
	if len(item) != 2 || item["id"] != "f-1" || item["name"] != "Contact" {
		t.Fatalf("expected projected forms item, got %+v", item)
	}
}

func TestFormsListFallsBackToFlatEndpoint(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/root":
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/forms/management/api/v1/form":
			return datatypeJSONResponse(http.StatusOK, `{"items":[{"id":"f-9","name":"Survey"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "list")
	if err != nil {
		t.Fatalf("forms list fallback failed: %v", err)
	}
	if !strings.Contains(output, `"id": "f-9"`) {
		t.Fatalf("expected fallback to return /form payload, got %q", output)
	}
}

func TestFormsChildrenQueriesFormsByFolderId(t *testing.T) {
	var observedQuery string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form":
			observedQuery = req.URL.RawQuery
			return datatypeJSONResponse(http.StatusOK, `[
				{"id":"f-1","name":"Albans cool form","entries":0,"summary":"2 page form with 4 fields"},
				{"id":"f-2","name":"Event Form","entries":3,"summary":"1 page form with 16 fields"}
			]`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "children", "folder-1", "--fields", "id,name")
	if err != nil {
		t.Fatalf("forms children failed: %v", err)
	}
	if !strings.Contains(observedQuery, "folderId=folder-1") {
		t.Fatalf("expected folderId query param, got %q", observedQuery)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("failed to decode children payload: %v", err)
	}
	if len(items) != 2 || items[0]["id"] != "f-1" || items[0]["name"] != "Albans cool form" {
		t.Fatalf("unexpected projected items: %+v", items)
	}
	if _, hasSummary := items[0]["summary"]; hasSummary {
		t.Fatalf("expected --fields projection to drop summary, got %+v", items[0])
	}
}

func TestFormsGetHitsFormsPrefix(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1":
			observedPath = req.URL.Path
			return datatypeJSONResponse(http.StatusOK, `{"id":"f-1","name":"Contact","fields":[{"id":"field-guid-1","alias":"email"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "get", "f-1")
	if err != nil {
		t.Fatalf("forms get failed: %v", err)
	}
	if observedPath != "/umbraco/forms/management/api/v1/form/f-1" {
		t.Fatalf("expected forms get to hit Forms prefix, got %q", observedPath)
	}
	if !strings.Contains(output, "field-guid-1") {
		t.Fatalf("expected field GUID in response, got %q", output)
	}
}

func TestFormsRecordsPassesThroughFiltersWithParamsPrecedence(t *testing.T) {
	var observedQuery string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record":
			observedQuery = req.URL.RawQuery
			return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	// --params.state should win over --state on key collision.
	_, err := execute(
		buildRootWithCollections(t, deps),
		"forms", "records", "f-1",
		"--state", "submitted",
		"--take", "5",
		"--params", `{"state":"approved"}`,
	)
	if err != nil {
		t.Fatalf("forms records failed: %v", err)
	}

	if !strings.Contains(observedQuery, "state=approved") {
		t.Fatalf("expected --params.state to override --state, got query %q", observedQuery)
	}
	if strings.Contains(observedQuery, "state=submitted") {
		t.Fatalf("expected --params to override --state, but --state value leaked: %q", observedQuery)
	}
	if !strings.Contains(observedQuery, "take=5") {
		t.Fatalf("expected --take to be passed through as query param, got %q", observedQuery)
	}
}

func TestFormsRecordRequiresFormIdAndRecordId(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record/r-7":
			return datatypeJSONResponse(http.StatusOK, `{"id":"r-7","state":"approved"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "forms", "record", "r-7"); err == nil {
		t.Fatalf("expected forms record to require both formId and recordId, got nil error")
	}

	output, err := execute(buildRootWithCollections(t, deps), "forms", "record", "f-1", "r-7")
	if err != nil {
		t.Fatalf("forms record failed: %v", err)
	}
	if !strings.Contains(output, `"id": "r-7"`) {
		t.Fatalf("expected record payload to round-trip, got %q", output)
	}
}

func TestFormsRecordWorkflowLogHitsAuditTrail(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record/r-7/workflow-audit-trail":
			observedPath = req.URL.Path
			return datatypeJSONResponse(http.StatusOK, `[{"workflowId":"wf-1","status":"completed"}]`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "record-workflow-log", "f-1", "r-7")
	if err != nil {
		t.Fatalf("forms record-workflow-log failed: %v", err)
	}
	if observedPath != "/umbraco/forms/management/api/v1/form/f-1/record/r-7/workflow-audit-trail" {
		t.Fatalf("expected workflow audit trail path, got %q", observedPath)
	}
	if !strings.Contains(output, "wf-1") {
		t.Fatalf("expected workflow id in response, got %q", output)
	}
}

