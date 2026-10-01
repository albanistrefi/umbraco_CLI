package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

func TestTagListSendsFilterAndPaginationParams(t *testing.T) {
	var observed string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tag":
			observed = req.URL.String()
			return cmdtest.JSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"tag-1","text":"alpha","group":"default","nodeCount":2}]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "tag", "list", "--query", "alp", "--group", "default", "--culture", "en-US", "--skip", "5", "--take", "10")
	if err != nil {
		t.Fatalf("tag list failed: %v", err)
	}
	for _, expected := range []string{"query=alp", "tagGroup=default", "culture=en-US", "skip=5", "take=10"} {
		if !strings.Contains(observed, expected) {
			t.Fatalf("expected %q in tag list request, got %q", expected, observed)
		}
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode tag list payload: %v", err)
	}
	if payload["total"] != float64(1) {
		t.Fatalf("expected the paged envelope to survive, got %+v", payload)
	}
}

func TestTagListParamsWinOverConvenienceFlags(t *testing.T) {
	var observed string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tag":
			observed = req.URL.String()
			return cmdtest.JSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), "tag", "list", "--group", "flag-group", "--params", `{"tagGroup":"params-group"}`); err != nil {
		t.Fatalf("tag list failed: %v", err)
	}
	if !strings.Contains(observed, "tagGroup=params-group") || strings.Contains(observed, "flag-group") {
		t.Fatalf("expected --params to win on tagGroup, got %q", observed)
	}
}

func TestTagListAutoPaginatesWithAll(t *testing.T) {
	var requests []string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tag":
			requests = append(requests, req.URL.String())
			if strings.Contains(req.URL.RawQuery, "skip=0") {
				return cmdtest.JSONResponse(http.StatusOK, `{"total":3,"items":[{"id":"tag-1"},{"id":"tag-2"}]}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"total":3,"items":[{"id":"tag-3"}]}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "tag", "list", "--take", "2", "--all")
	if err != nil {
		t.Fatalf("tag list --all failed: %v", err)
	}
	if len(requests) < 2 {
		t.Fatalf("expected --all to page more than once, got %+v", requests)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode tag list payload: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("expected all 3 tags to be collected, got %+v", payload)
	}
}
