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
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"mt-image","alias":"umbracoMediaImage","name":"Image"},
				{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"SVG"}
			]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-svg":
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"SVG","variesByCulture":false}`), nil
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
	if createPayload["id"] == "" {
		t.Fatalf("unexpected media create payload: %+v", createPayload)
	}
	if _, hasTopName := createPayload["name"]; hasTopName {
		t.Fatalf("expected variants envelope; top-level name should be absent: %+v", createPayload)
	}
	variants := createPayload["variants"].([]any)
	variant := variants[0].(map[string]any)
	if variant["name"] != "Hero" {
		t.Fatalf("expected variant name=Hero, got %+v", variant)
	}
	if variant["culture"] != nil {
		t.Fatalf("expected variant culture to be null for non-varying media type, got %+v", variant)
	}
	mediaType := createPayload["mediaType"].(map[string]any)
	if mediaType["id"] != "mt-svg" {
		t.Fatalf("expected resolved media type id, got %+v", mediaType)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode upload result: %v", err)
	}
	if result["id"] != createPayload["id"] || result["name"] != "Hero" {
		t.Fatalf("expected upload result to expose created media id, got %+v", result)
	}
}

func TestMediaUploadCultureVaryingMediaTypeUsesVariantPayload(t *testing.T) {
	var createPayload map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"mt-image","alias":"umbracoMediaImage","name":"Image"}]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-image":
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-image","alias":"umbracoMediaImage","name":"Image","variesByCulture":true}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			if err := json.NewDecoder(req.Body).Decode(&createPayload); err != nil {
				t.Fatalf("failed to decode media create payload: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "Image", "--name", "Hero", "--culture", "en-US"); err != nil {
		t.Fatalf("media upload failed: %v", err)
	}

	if _, exists := createPayload["name"]; exists {
		t.Fatalf("expected culture-varying payload to omit top-level name, got %+v", createPayload)
	}
	variants := createPayload["variants"].([]any)
	variant := variants[0].(map[string]any)
	if variant["name"] != "Hero" || variant["culture"] != "en-US" {
		t.Fatalf("unexpected variants payload: %+v", createPayload)
	}
	values := createPayload["values"].([]any)
	value := values[0].(map[string]any)
	if value["culture"] != "en-US" {
		t.Fatalf("expected value culture for culture-varying media, got %+v", value)
	}
}

func TestMediaUploadResolvesFriendlySVGToCanonicalAlias(t *testing.T) {
	var detailFetched string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			// Real Umbraco names the SVG type "Vector Graphics (SVG)"; alias is umbracoMediaVectorGraphics.
			return endpointJSONResponse(http.StatusOK, `{"total":2,"items":[
				{"id":"mt-image","alias":"Image","name":"Image"},
				{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"Vector Graphics (SVG)"}
			]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-svg":
			detailFetched = "mt-svg"
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"Vector Graphics (SVG)","variesByCulture":false}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "hero.svg")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "SVG", "--name", "Hero"); err != nil {
		t.Fatalf("media upload --type SVG failed: %v", err)
	}
	if detailFetched != "mt-svg" {
		t.Fatalf("expected friendly SVG to resolve via alias map, got detailFetched=%q", detailFetched)
	}
}

func TestMediaUploadResolvesCanonicalAliasDirectly(t *testing.T) {
	var detailFetched string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[
				{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"Vector Graphics (SVG)"}
			]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-svg":
			detailFetched = "mt-svg"
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"Vector Graphics (SVG)","variesByCulture":false}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "umbracoMediaVectorGraphics", "--name", "Logo"); err != nil {
		t.Fatalf("media upload --type <canonical alias> failed: %v", err)
	}
	if detailFetched != "mt-svg" {
		t.Fatalf("expected canonical alias to resolve via alias match, got detailFetched=%q", detailFetched)
	}
}

func TestMediaUploadResolvesAliasWhenLightweightEndpointsOmitIt(t *testing.T) {
	// Simulate the realistic Umbraco shape: tree/item endpoints return id+name only (no alias),
	// and only GET /media-type/{id} carries the alias. The CLI must fetch each candidate's
	// detail to make alias-based --type values resolve.
	var detailHits []string
	var sawSearch, sawTree bool

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/media-type/search":
			sawSearch = true
			// Search returns just id+name+icon (no alias) per MediaTypeItemResponseModel.
			return endpointJSONResponse(http.StatusOK, `{"items":[]}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			sawTree = true
			// Tree-root returns id+name only (no alias) per MediaTypeTreeItemResponseModel.
			return endpointJSONResponse(http.StatusOK, `{"items":[
				{"id":"mt-image","name":"Image"},
				{"id":"mt-svg","name":"Vector Graphics (SVG)"}
			]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-image":
			detailHits = append(detailHits, "mt-image")
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-image","alias":"Image","name":"Image","variesByCulture":false}`), nil
		case "/umbraco/management/api/v1/media-type/mt-svg":
			detailHits = append(detailHits, "mt-svg")
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-svg","alias":"umbracoMediaVectorGraphics","name":"Vector Graphics (SVG)","variesByCulture":false}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "umbracoMediaVectorGraphics", "--name", "Logo"); err != nil {
		t.Fatalf("alias-only resolution failed: %v", err)
	}
	if !sawSearch {
		t.Fatalf("expected search candidate phase to run")
	}
	if !sawTree {
		t.Fatalf("expected tree-root candidate phase to run when search yields no hit")
	}
	// Should have fetched detail for at least mt-svg to discover its alias.
	matched := false
	for _, id := range detailHits {
		if id == "mt-svg" {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected /media-type/mt-svg detail fetch to verify alias, got %v", detailHits)
	}
}

func TestMediaUploadExplicitCultureForcesVariantPayloadOnNonVaryingType(t *testing.T) {
	var createPayload map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"mt-image","alias":"Image","name":"Image"}]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-image":
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-image","alias":"Image","name":"Image","variesByCulture":false}`), nil
		case "/umbraco/management/api/v1/temporary-file":
			return endpointJSONResponse(http.StatusCreated, `{"success":true}`), nil
		case "/umbraco/management/api/v1/media":
			if err := json.NewDecoder(req.Body).Decode(&createPayload); err != nil {
				t.Fatalf("failed to decode media create payload: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "Image", "--name", "Hero", "--culture", "en-US"); err != nil {
		t.Fatalf("media upload with explicit culture failed: %v", err)
	}

	if _, exists := createPayload["name"]; exists {
		t.Fatalf("expected variants envelope to omit top-level name, got %+v", createPayload)
	}
	variants, ok := createPayload["variants"].([]any)
	if !ok || len(variants) == 0 {
		t.Fatalf("expected variants[] payload, got %+v", createPayload)
	}
	variant := variants[0].(map[string]any)
	if variant["name"] != "Hero" {
		t.Fatalf("expected variant name=Hero, got %+v", variant)
	}
	// --culture on a non-varying media type is ignored (the API only accepts
	// culture: null for invariant types). The CLI warns, but emits null.
	if variant["culture"] != nil {
		t.Fatalf("expected variant culture to be null when media type does not vary, got %+v", variant)
	}
	values := createPayload["values"].([]any)
	value := values[0].(map[string]any)
	if value["culture"] != nil {
		t.Fatalf("expected value culture to be null on non-varying media type, got %+v", value)
	}
}

func TestMediaUploadCultureVaryingMediaTypeRequiresCultureWhenDefaultCannotResolve(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"mt-image","alias":"umbracoMediaImage","name":"Image"}]}`), nil
		case "/umbraco/management/api/v1/media-type/mt-image":
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-image","alias":"umbracoMediaImage","name":"Image","variesByCulture":true}`), nil
		case "/umbraco/management/api/v1/server/configuration":
			return endpointJSONResponse(http.StatusOK, `{}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	filePath := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("failed to write upload fixture: %v", err)
	}

	_, err := execute(buildRootWithCollections(t, deps), "media", "upload", filePath, "--type", "Image", "--name", "Hero")
	if err == nil || !strings.Contains(err.Error(), "varies by culture") {
		t.Fatalf("expected culture requirement error, got %v", err)
	}
}
