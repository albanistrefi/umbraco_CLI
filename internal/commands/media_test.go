package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMediaSearchUsesItemSearchEndpointAndFallsBack(t *testing.T) {
	var requests []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/media/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/media/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"media-1","name":"Hero Image"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "media", "search", "--query", "Hero", "--skip", "0", "--take", "25")
	if err != nil {
		t.Fatalf("media search failed: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 media search attempts, got %+v", requests)
	}
	if !strings.Contains(requests[0], "/item/media/search") || !strings.Contains(requests[0], "query=Hero") {
		t.Fatalf("unexpected primary media search request: %q", requests[0])
	}
	if !strings.Contains(requests[1], "/media/search") {
		t.Fatalf("unexpected fallback media search request: %q", requests[1])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode media search payload: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected media search payload, got %+v", payload)
	}
}
