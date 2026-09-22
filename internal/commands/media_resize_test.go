package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMediaResizeURLsSendsRepeatedIDsAndDimensions(t *testing.T) {
	var observed string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/imaging/resize/urls":
			observed = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `[{"id":"m-1","urlInfos":[{"culture":null,"url":"/media/a.jpg?width=300"}]}]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "media", "resize-urls", "--ids", "m-1,m-2,m-1", "--width", "300", "--height", "150", "--mode", "Crop")
	if err != nil {
		t.Fatalf("media resize-urls failed: %v", err)
	}
	if strings.Count(observed, "id=") != 2 {
		t.Fatalf("expected two deduplicated repeated id values, got %q", observed)
	}
	for _, expected := range []string{"id=m-1", "id=m-2", "width=300", "height=150", "mode=Crop"} {
		if !strings.Contains(observed, expected) {
			t.Fatalf("expected %q in resize-urls request, got %q", expected, observed)
		}
	}
	var payload []any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode resize-urls payload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected the array response to pass through, got %+v", payload)
	}
}

func TestMediaResizeURLsOmitsUnsetDimensionsAndRequiresIDs(t *testing.T) {
	var observed string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/imaging/resize/urls":
			observed = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `[]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "media", "resize-urls"); err == nil {
		t.Fatalf("expected media resize-urls to require --ids")
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "resize-urls", "--ids", "m-1"); err != nil {
		t.Fatalf("media resize-urls failed: %v", err)
	}
	// Unset dimensions must not be sent as 0; the server defaults them.
	if strings.Contains(observed, "width=") || strings.Contains(observed, "height=") || strings.Contains(observed, "mode=") {
		t.Fatalf("expected no dimension parameters when the flags are unset, got %q", observed)
	}
}

func TestMediaResizeURLsRejectsNonPositiveDimensions(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		t.Fatalf("a rejected dimension must not reach %s %s", req.Method, req.URL.Path)
		return nil, nil
	})

	// An explicit 0 or negative size is a mistake, not a request for the
	// server default, so it must not be silently dropped.
	for _, args := range [][]string{
		{"media", "resize-urls", "--ids", "m-1", "--width", "0"},
		{"media", "resize-urls", "--ids", "m-1", "--height", "-10"},
	} {
		output, err := execute(buildRootWithCollections(t, deps), args...)
		if err == nil {
			t.Fatalf("expected %v to be rejected, got output %q", args, output)
		}
		if !strings.Contains(err.Error(), "positive pixel count") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}
