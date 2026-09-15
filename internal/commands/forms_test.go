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

func TestFormsChildrenUsesTreeChildrenAndAnnotatesFolders(t *testing.T) {
	var observedPath string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/children/folder-1":
			observedPath = req.URL.String()
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"folder-2","name":"Nested","isFolder":true,"hasChildren":true},
				{"id":"f-2","name":"Event Form","isFolder":false,"entries":3}
			]}`), nil
		case "/umbraco/forms/management/api/v1/form":
			// The folderId filter is ignored by the server (returns every
			// form), so children must never fall back to it.
			t.Fatalf("forms children must not query /form?folderId")
			return nil, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "forms", "children", "folder-1", "--fields", "id,name,isFolder,type")
	if err != nil {
		t.Fatalf("forms children failed: %v", err)
	}
	if !strings.HasSuffix(observedPath, "/tree/form/children/folder-1") {
		t.Fatalf("expected the tree children route, got %q", observedPath)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode children payload: %v", err)
	}
	items := payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected two items, got %+v", items)
	}
	folder := items[0].(map[string]any)
	form := items[1].(map[string]any)
	if folder["isFolder"] != true || folder["type"] != "folder" {
		t.Fatalf("expected folder annotation, got %+v", folder)
	}
	if form["isFolder"] != false || form["type"] != "form" {
		t.Fatalf("expected form annotation, got %+v", form)
	}
}

func TestFormsChildrenExplainsNonFolderID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/children/f-1", "/umbraco/forms/management/api/v1/tree/form/children/empty-folder":
			// The tree answers an empty page for any id, folder or not.
			return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		case "/umbraco/forms/management/api/v1/folder/empty-folder":
			return datatypeJSONResponse(http.StatusOK, `{"id":"empty-folder","name":"Empty"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "forms", "children", "f-1")
	if err == nil || !strings.Contains(err.Error(), "is not a Forms folder id") || !strings.Contains(err.Error(), "forms get f-1") {
		t.Fatalf("expected a not-a-folder explanation, got %v", err)
	}
	// A probe failure other than 404 must surface as the API error (exit 4),
	// not as "not a folder".
	forbidden := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/children/f-1":
			return datatypeJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		case "/umbraco/forms/management/api/v1/folder/f-1":
			return datatypeJSONResponse(http.StatusForbidden, `{"title":"Forbidden","status":403}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err = execute(buildRootWithCollections(t, forbidden), "forms", "children", "f-1")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "is not a Forms folder id") {
		t.Fatalf("expected the probe's 403 to propagate, got %v", err)
	}
	out, err := execute(buildRootWithCollections(t, deps), "forms", "children", "empty-folder")
	if err != nil || !strings.Contains(out, `"total": 0`) {
		t.Fatalf("expected an actual empty folder to list as empty, got err=%v out=%s", err, out)
	}
}

func TestFormsListAnnotatesFormsWithoutFolderFlag(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/tree/form/root":
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"folder-1","name":"Contact sales","isFolder":true},
				{"id":"f-1","name":"Contact"}
			]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	output, err := execute(buildRootWithCollections(t, deps), "forms", "list")
	if err != nil {
		t.Fatalf("forms list failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode forms list payload: %v", err)
	}
	items := payload["items"].([]any)
	if items[0].(map[string]any)["type"] != "folder" || items[1].(map[string]any)["type"] != "form" || items[1].(map[string]any)["isFolder"] != false {
		t.Fatalf("expected isFolder/type on every item, got %+v", items)
	}
}

func TestFormsGetOnFolderExplains(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/folder-1":
			return datatypeJSONResponse(http.StatusNotFound, `{"title":"Not found","status":404}`), nil
		case "/umbraco/forms/management/api/v1/folder/folder-1":
			return datatypeJSONResponse(http.StatusOK, `{"id":"folder-1","name":"Contact sales","parentId":null}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "forms", "get", "folder-1")
	if err == nil || !strings.Contains(err.Error(), "is a Forms folder, not a form") || !strings.Contains(err.Error(), "forms children folder-1") {
		t.Fatalf("expected a folder explanation, got %v", err)
	}

	// An id that is neither keeps the real 404 (exit 4).
	_, err = execute(buildRootWithCollections(t, deps), "forms", "get", "missing")
	if err == nil || !strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "is a Forms folder") {
		t.Fatalf("expected the plain 404 for an unknown id, got %v", err)
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

func TestFormsRecordsAppliesDefaultTakeCapWhenNotSet(t *testing.T) {
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

	if _, err := execute(buildRootWithCollections(t, deps), "forms", "records", "f-1"); err != nil {
		t.Fatalf("forms records failed: %v", err)
	}
	if !strings.Contains(observedQuery, "take=100") {
		t.Fatalf("expected default take=100 to be applied, got query %q", observedQuery)
	}
}

func TestFormsRecordsExplicitTakeZeroDisablesDefaultCap(t *testing.T) {
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

	if _, err := execute(buildRootWithCollections(t, deps), "forms", "records", "f-1", "--take", "0"); err != nil {
		t.Fatalf("forms records --take 0 failed: %v", err)
	}
	if !strings.Contains(observedQuery, "take=0") {
		t.Fatalf("expected explicit --take=0 to pass through verbatim, got %q", observedQuery)
	}
	if strings.Contains(observedQuery, "take=100") {
		t.Fatalf("expected explicit --take=0 to override the default cap, got %q", observedQuery)
	}
}

func TestFormsRecordsPassesThroughDateFilters(t *testing.T) {
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

	_, err := execute(
		buildRootWithCollections(t, deps),
		"forms", "records", "f-1",
		"--from", "2026-01-01T00:00:00Z",
		"--to", "2026-06-30T23:59:59Z",
		"--skip", "10",
		"--take", "25",
	)
	if err != nil {
		t.Fatalf("forms records with date filters failed: %v", err)
	}

	for _, want := range []string{"from=2026-01-01", "to=2026-06-30", "skip=10", "take=25"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected query to contain %q, got %q", want, observedQuery)
		}
	}
}

func TestFormsRecordFiltersListByUniqueIDAndNumericID(t *testing.T) {
	recordsPayload := `{
		"results": [
			{"id":16813,"uniqueId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","state":"Submitted","fields":[]},
			{"id":16814,"uniqueId":"917a242d-d48c-44ac-ad99-9dcfaf2d3e7f","state":"Approved","fields":[{"fieldId":"f-email","value":"alb@umbraco.dk"}]}
		],
		"schema": []
	}`

	var observedTake string
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record":
			observedTake = req.URL.Query().Get("take")
			return datatypeJSONResponse(http.StatusOK, recordsPayload), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	// Two-arg requirement holds.
	if _, err := execute(buildRootWithCollections(t, deps), "forms", "record", "f-1"); err == nil {
		t.Fatalf("expected forms record to require both formId and recordId")
	}

	// Lookup by uniqueId (GUID).
	output, err := execute(buildRootWithCollections(t, deps), "forms", "record", "f-1", "917a242d-d48c-44ac-ad99-9dcfaf2d3e7f")
	if err != nil {
		t.Fatalf("forms record by uniqueId failed: %v", err)
	}
	var byGUID map[string]any
	if err := json.Unmarshal([]byte(output), &byGUID); err != nil {
		t.Fatalf("failed to decode GUID lookup: %v", err)
	}
	if byGUID["uniqueId"] != "917a242d-d48c-44ac-ad99-9dcfaf2d3e7f" || byGUID["state"] != "Approved" {
		t.Fatalf("expected approved record back, got %+v", byGUID)
	}

	// Default scan should request 500 records.
	if observedTake != "500" {
		t.Fatalf("expected default scan=500, got take=%q", observedTake)
	}

	// Lookup by numeric id (stringified) hits the same record.
	output, err = execute(buildRootWithCollections(t, deps), "forms", "record", "f-1", "16814")
	if err != nil {
		t.Fatalf("forms record by numeric id failed: %v", err)
	}
	var byID map[string]any
	if err := json.Unmarshal([]byte(output), &byID); err != nil {
		t.Fatalf("failed to decode id lookup: %v", err)
	}
	if byID["uniqueId"] != "917a242d-d48c-44ac-ad99-9dcfaf2d3e7f" {
		t.Fatalf("expected numeric id 16814 to resolve to the same record, got %+v", byID)
	}
}

func TestFormsRecordNotFoundDistinguishesExhaustedFromDefinitive(t *testing.T) {
	// Scan window exhausted: API returned `scan` rows, more may exist.
	exhaustedDeps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record":
			// Return exactly --scan rows so the window is "full".
			rows := strings.Repeat(`{"id":1,"uniqueId":"aaa","state":"Submitted"},`, 3)
			rows = strings.TrimRight(rows, ",")
			return datatypeJSONResponse(http.StatusOK, `{"results":[`+rows+`],"schema":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, exhaustedDeps), "forms", "record", "f-1", "missing", "--scan", "3")
	if err == nil {
		t.Fatalf("expected not-found error when scan window is exhausted")
	}
	if !strings.Contains(err.Error(), "scan window exhausted") {
		t.Fatalf("expected 'scan window exhausted' wording, got: %v", err)
	}

	// Definitive miss: API returned fewer rows than --scan, so the record
	// genuinely isn't on the form.
	definitiveDeps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/forms/management/api/v1/form/f-1/record":
			return datatypeJSONResponse(http.StatusOK, `{"results":[{"id":1,"uniqueId":"aaa"}],"schema":[]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err = execute(buildRootWithCollections(t, definitiveDeps), "forms", "record", "f-1", "missing", "--scan", "500")
	if err == nil {
		t.Fatalf("expected not-found error when record is definitively absent")
	}
	if strings.Contains(err.Error(), "scan window exhausted") {
		t.Fatalf("did not expect exhaustion wording when only %d rows came back, got: %v", 1, err)
	}
	if !strings.Contains(err.Error(), "scanned all 1 records") {
		t.Fatalf("expected definitive-miss wording mentioning actual row count, got: %v", err)
	}
}

func TestFormsRecordRejectsNonPositiveScan(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	for _, v := range []string{"0", "-1"} {
		_, err := execute(buildRootWithCollections(t, deps), "forms", "record", "f-1", "x", "--scan", v)
		if err == nil {
			t.Fatalf("--scan=%s should be rejected", v)
		}
		if !strings.Contains(err.Error(), "must be a positive integer") {
			t.Fatalf("--scan=%s wrong error: %v", v, err)
		}
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
