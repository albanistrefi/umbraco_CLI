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

const mediaInspectTestImage = `{"id":"m-2","isTrashed":false,"mediaType":{"id":"mt-img"},"variants":[{"culture":null,"segment":null,"name":"Hero","createDate":"2026-01-01T00:00:00Z","updateDate":"2026-01-02T00:00:00Z"}],"values":[` +
	`{"alias":"umbracoFile","culture":null,"segment":null,"editorAlias":"Umbraco.ImageCropper","value":{"src":"/media/xyz/hero.png","crops":[],"focalPoint":{"left":0.5,"top":0.5}}},` +
	`{"alias":"umbracoWidth","value":"207"},{"alias":"umbracoHeight","value":"45"},{"alias":"umbracoBytes","value":"1234"},{"alias":"umbracoExtension","value":"png"},{"alias":"altText","value":"Hero image"}]}`

func mediaInspectDeps(t *testing.T, svgBody string) Dependencies {
	t.Helper()
	return datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/media/m-2":
			return datatypeJSONResponse(http.StatusOK, mediaInspectTestImage), nil
		case "/umbraco/management/api/v1/media/m-1":
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
		case "/umbraco/management/api/v1/media/urls":
			id := req.URL.Query().Get("id")
			return datatypeJSONResponse(http.StatusOK, `[{"id":"`+id+`","urlInfos":[{"culture":null,"url":"https://example.test/media/xyz/file"}]}]`), nil
		case "/media/abc/old.svg":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/svg+xml"}}, Body: io.NopCloser(strings.NewReader(svgBody))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
}

func TestMediaInspectFlattensRasterDimensionsAndURLs(t *testing.T) {
	output, err := execute(buildRootWithCollections(t, mediaInspectDeps(t, "")), "media", "inspect", "m-2")
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	file := payload["file"].(map[string]any)
	if file["src"] != "/media/xyz/hero.png" || file["width"] != float64(207) || file["height"] != float64(45) || file["bytes"] != float64(1234) || file["extension"] != "png" {
		t.Fatalf("unexpected file summary: %#v", file)
	}
	if file["url"] != "https://example.test/media/xyz/file" || payload["name"] != "Hero" {
		t.Fatalf("expected url and name, got %s", output)
	}
	if payload["otherValues"].(map[string]any)["altText"] != "Hero image" {
		t.Fatalf("expected non-file values surfaced, got %s", output)
	}
	if _, present := payload["otherValues"].(map[string]any)["umbracoWidth"]; present {
		t.Fatalf("derived values should be folded into file, got %s", output)
	}
}

func TestMediaInspectReadsSVGViewBoxAndAliasesWork(t *testing.T) {
	deps := mediaInspectDeps(t, `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="20px" viewBox="0 0 207 45"><rect/></svg>`)
	output, err := execute(buildRootWithCollections(t, deps), "media", "info", "m-1")
	if err != nil {
		t.Fatalf("inspect via alias failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	file := payload["file"].(map[string]any)
	if file["viewBox"] != "0 0 207 45" || file["svgWidth"] != "20px" || file["extension"] != "svg" {
		t.Fatalf("unexpected svg summary: %#v", file)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "find-references", "m-1"); err == nil {
		// referenced-by is not mocked here; the alias must still resolve to the command (404 → API error, not "unknown command").
		t.Fatalf("expected API error from unmocked referenced-by, got success")
	} else if strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("alias find-references did not resolve: %v", err)
	}
}

func TestMediaDownloadWritesAssetIntoDirectory(t *testing.T) {
	dir := t.TempDir()
	deps := mediaInspectDeps(t, "<svg>old</svg>")
	output, err := execute(buildRootWithCollections(t, deps), "media", "download", "m-1", dir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "old.svg"))
	if err != nil || string(saved) != "<svg>old</svg>" {
		t.Fatalf("expected asset written under its server name, got %q (%v)", saved, err)
	}
	if !strings.Contains(output, `"bytes": 14`) || !strings.Contains(output, `"contentType": "image/svg+xml"`) {
		t.Fatalf("unexpected output: %s", output)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "download", "m-1", filepath.Join(dir, "x.svg"), "--property", "nope"); err == nil {
		t.Fatalf("expected missing property to fail")
	}
}

func TestMediaInspectFailsWhenURLLookupFailsAndDownloadDryRun(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/media/m-1":
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
		default:
			return datatypeJSONResponse(http.StatusForbidden, `{"title":"Forbidden"}`), nil
		}
	})
	if _, err := execute(buildRootWithCollections(t, deps), "media", "inspect", "m-1"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected the URL lookup failure to propagate, got %v", err)
	}

	dir := t.TempDir()
	output, err := execute(buildRootWithCollections(t, deps), "media", "download", "m-1", dir, "--dry-run")
	if err != nil {
		t.Fatalf("download --dry-run failed: %v", err)
	}
	if !strings.Contains(output, `"dryRun": true`) || !strings.Contains(output, filepath.Join(dir, "old.svg")) {
		t.Fatalf("unexpected dry-run output: %s", output)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("dry-run must not write, found %d entries", len(entries))
	}
}

func TestMediaInspectRequiresCultureForVariants(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/media/m-3":
			return datatypeJSONResponse(http.StatusOK, mediaFileTestVariantItem), nil
		case "/umbraco/management/api/v1/media/urls":
			return datatypeJSONResponse(http.StatusOK, `[]`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	if _, err := execute(buildRootWithCollections(t, deps), "media", "inspect", "m-3"); err == nil || !strings.Contains(err.Error(), "--culture") {
		t.Fatalf("expected culture requirement, got %v", err)
	}
	output, err := execute(buildRootWithCollections(t, deps), "media", "inspect", "m-3", "--culture", "da-DK", "--no-fetch")
	if err != nil {
		t.Fatalf("inspect --culture failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	file := payload["file"].(map[string]any)
	if file["src"] != "/media/abc/da.svg" || file["culture"] != "da-DK" {
		t.Fatalf("expected da-DK variant flattened, got %#v", file)
	}
	if others, _ := payload["otherVariants"].([]any); len(others) != 1 {
		t.Fatalf("expected the en-US variant listed under otherVariants, got %s", output)
	}
}
