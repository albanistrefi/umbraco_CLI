package commands

import (
	"encoding/json"
	"net/http"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

func TestTreeWalkResolvesNestedPath(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"home","name":"Home"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			switch req.URL.Query().Get("parentId") {
			case "home":
				return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"partners","name":"Partners"}]}`), nil
			case "partners":
				return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"partner-list","name":"Partner List"}]}`), nil
			default:
				return cmdtest.JSONResponse(http.StatusOK, `{"items":[]}`), nil
			}
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "tree", "walk", "Home/Partners/Partner List")
	if err != nil {
		t.Fatalf("tree walk failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode tree walk payload: %v", err)
	}
	if payload["id"] != "partner-list" || payload["path"] != "Home/Partners/Partner List" {
		t.Fatalf("unexpected tree walk payload: %+v", payload)
	}
}

func TestTreeWalkFailsWhenSegmentIsMissing(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"home","name":"Home"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := cmdtest.Execute(buildRootWithCollections(t, deps), "tree", "walk", "Home/Missing")
	if err == nil {
		t.Fatalf("expected tree walk missing segment to fail")
	}
}
