package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUserDataListSendsRepeatedFilterValues(t *testing.T) {
	var observed string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/user-data":
			observed = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user-data", "list", "--groups", "a,b,a", "--identifiers", "one", "--take", "50"); err != nil {
		t.Fatalf("user-data list failed: %v", err)
	}
	if strings.Count(observed, "groups=") != 2 {
		t.Fatalf("expected two deduplicated repeated groups values, got %q", observed)
	}
	for _, expected := range []string{"groups=a", "groups=b", "identifiers=one", "take=50"} {
		if !strings.Contains(observed, expected) {
			t.Fatalf("expected %q in user-data list request, got %q", expected, observed)
		}
	}
}

func TestUserDataCreateBuildsKeyValueBody(t *testing.T) {
	var body map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/user-data" && req.Method == http.MethodPost:
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("failed to decode create body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"id":"key-1"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user-data", "create", "--group", "g1", "--identifier", "i1", "--value", "v1"); err != nil {
		t.Fatalf("user-data create failed: %v", err)
	}
	if body["group"] != "g1" || body["identifier"] != "i1" || body["value"] != "v1" {
		t.Fatalf("unexpected create body: %+v", body)
	}
	// The server assigns the key when it is omitted; the CLI must not
	// invent one the way entity creates do.
	if _, ok := body["key"]; ok {
		t.Fatalf("expected no key in the create body when --key is omitted: %+v", body)
	}
	if _, ok := body["id"]; ok {
		t.Fatalf("user data is keyed on key, not id: %+v", body)
	}
}

func TestUserDataCreateRequiresTheValueTriple(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user-data", "create", "--group", "g1"); err == nil {
		t.Fatalf("expected user-data create to require --identifier and --value")
	}
}

func TestUserDataUpdatePutsKeyInTheBody(t *testing.T) {
	var body map[string]any
	var observedPath string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.Method == http.MethodPut:
			observedPath = req.URL.Path
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("failed to decode update body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user-data", "update", "key-1", "--group", "g1", "--identifier", "i1", "--value", "v2"); err != nil {
		t.Fatalf("user-data update failed: %v", err)
	}
	// The PUT goes to the collection endpoint; the key travels in the body.
	if observedPath != "/umbraco/management/api/v1/user-data" {
		t.Fatalf("unexpected update path: %q", observedPath)
	}
	if body["key"] != "key-1" || body["value"] != "v2" {
		t.Fatalf("unexpected update body: %+v", body)
	}
}

func TestUserDataUpdateDryRunPrintsThePlannedRequest(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		t.Fatalf("dry run must not issue %s %s", req.Method, req.URL.Path)
		return nil, nil
	})

	output, err := execute(buildRootWithCollections(t, deps), "user-data", "update", "key-1", "--group", "g1", "--identifier", "i1", "--value", "v2", "--dry-run")
	if err != nil {
		t.Fatalf("user-data update --dry-run failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode dry-run payload: %v", err)
	}
	if payload["method"] != "PUT" || payload["dryRun"] != true {
		t.Fatalf("unexpected dry-run payload: %+v", payload)
	}
}
