package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestMediaUploadCreatesTemporaryFileThenMedia(t *testing.T) {
	var sawUpload bool
	var createPayload map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			if req.Method != http.MethodPost {
				return endpointJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
			if err := req.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("failed to parse multipart upload: %v", err)
			}
			if req.FormValue("id") == "" {
				t.Fatalf("expected temporary file id form field")
			}
			file, _, err := req.FormFile("file")
			if err != nil {
				t.Fatalf("expected file form field: %v", err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("failed to read uploaded file: %v", err)
			}
			if string(body) != "asset" {
				t.Fatalf("unexpected uploaded file body: %q", body)
			}
			sawUpload = true
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			if req.Method != http.MethodPost {
				return endpointJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&createPayload); err != nil {
				t.Fatalf("failed to decode media create payload: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "hero.svg")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	output, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "svg", "--name", "Hero")
	if err != nil {
		t.Fatalf("media upload failed: %v", err)
	}
	if !sawUpload {
		t.Fatalf("expected temporary-file upload request")
	}
	if createPayload["id"] == "" || createPayload["name"] != "Hero" {
		t.Fatalf("unexpected media create payload: %+v", createPayload)
	}
	mediaType := createPayload["mediaType"].(map[string]any)
	if mediaType["alias"] != "SVG" {
		t.Fatalf("expected built-in media type alias, got %+v", mediaType)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode upload result: %v", err)
	}
	if result["id"] != createPayload["id"] || result["name"] != "Hero" {
		t.Fatalf("expected upload result to expose created media id, got %+v", result)
	}
}
