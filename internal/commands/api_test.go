package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIPassthroughGETPreservesRepeatedQueryParams(t *testing.T) {
	var observedIDs []string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/ancestors":
			observedIDs = req.URL.Query()["id"]
			return datatypeJSONResponse(http.StatusOK, `{"total":2,"items":[{"id":"doc-1"},{"id":"doc-2"}]}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "api", "GET", "/item/document/ancestors?id=doc-1&id=doc-2")
	if err != nil {
		t.Fatalf("api GET failed: %v", err)
	}
	if len(observedIDs) != 2 || observedIDs[0] != "doc-1" || observedIDs[1] != "doc-2" {
		t.Fatalf("expected repeated id params to be preserved, got %#v", observedIDs)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode api output: %v", err)
	}
	if payload["statusCode"] != float64(http.StatusOK) || payload["ok"] != true {
		t.Fatalf("unexpected api output: %+v", payload)
	}
	body := payload["body"].(map[string]any)
	if body["total"] != float64(2) {
		t.Fatalf("expected response body to be preserved, got %+v", payload)
	}
}

func TestAPIPassthroughPOSTReadsBodyFile(t *testing.T) {
	bodyPath := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(bodyPath, []byte(`{"name":"Example","enabled":true}`), 0o644); err != nil {
		t.Fatalf("failed to write payload file: %v", err)
	}
	var observedBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/some/endpoint":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			return datatypeJSONResponse(http.StatusCreated, `{"created":true}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/umbraco/management/api/v1/some/endpoint", "--body", "@"+bodyPath)
	if err != nil {
		t.Fatalf("api POST failed: %v", err)
	}
	if observedBody["name"] != "Example" || observedBody["enabled"] != true {
		t.Fatalf("unexpected request body: %+v", observedBody)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode api output: %v", err)
	}
	if payload["statusCode"] != float64(http.StatusCreated) || payload["path"] != "/some/endpoint" {
		t.Fatalf("unexpected api output: %+v", payload)
	}
}

func TestAPIPassthroughPrintsErrorStatusAndBody(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/missing":
			return datatypeJSONResponse(http.StatusNotFound, `{"title":"missing"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "api", "GET", "/missing")
	if err != nil {
		t.Fatalf("api GET should print non-2xx API responses without failing command execution: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode api output: %v", err)
	}
	if payload["ok"] != false || payload["statusCode"] != float64(http.StatusNotFound) {
		t.Fatalf("unexpected non-2xx api output: %+v", payload)
	}
	body := payload["body"].(map[string]any)
	if body["title"] != "missing" {
		t.Fatalf("expected error response body to be preserved, got %+v", payload)
	}
}

func TestAPIPassthroughFormSendsMultipartAndRawPathSkipsPrefix(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(filePath, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	var observedPath, observedID, observedFile, observedHeader string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/custom/upload":
			observedPath = req.URL.Path
			observedHeader = req.Header.Get("X-Custom")
			if err := req.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("multipart parse failed: %v", err)
			}
			observedID = req.FormValue("id")
			file, _, err := req.FormFile("file")
			if err != nil {
				t.Fatalf("missing file part: %v", err)
			}
			defer func() { _ = file.Close() }()
			content, _ := io.ReadAll(file)
			observedFile = string(content)
			return datatypeJSONResponse(http.StatusCreated, `{"id":"tmp-1"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/umbraco/custom/upload", "--raw-path",
		"--form", "id=tmp-1", "--form", "file=@"+filePath, "--header", "X-Custom: yes")
	if err != nil {
		t.Fatalf("api POST --form failed: %v", err)
	}
	if observedPath != "/umbraco/custom/upload" || observedID != "tmp-1" || observedFile != "<svg/>" || observedHeader != "yes" {
		t.Fatalf("unexpected request: path=%q id=%q file=%q header=%q", observedPath, observedID, observedFile, observedHeader)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode api output: %v", err)
	}
	if payload["statusCode"] != float64(http.StatusCreated) || payload["ok"] != true {
		t.Fatalf("unexpected api output: %+v", payload)
	}
}

func TestAPIPassthroughRejectsFormWithBodyAndBadHeader(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/temporary-file", "--form", "id=1", "--body", `{}`); err == nil {
		t.Fatalf("expected --form with --body to be rejected")
	}
	if _, err := execute(buildRootWithCollections(t, deps), "api", "GET", "/temporary-file", "--form", "id=1"); err == nil {
		t.Fatalf("expected --form with GET to be rejected")
	}
	if _, err := execute(buildRootWithCollections(t, deps), "api", "GET", "/server/status", "--header", "novalue"); err == nil {
		t.Fatalf("expected malformed --header to be rejected")
	}
}

func TestAPIPassthroughCanonicalizesHeadersLastWins(t *testing.T) {
	headers, err := parseAPIHeaders([]string{"X-Trace: default", "x-trace: override"})
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 1 || headers["X-Trace"] != "override" {
		t.Fatalf("expected canonicalized last-wins header, got %#v", headers)
	}
}

func TestAPIPassthroughOutWritesBinaryBodyVerbatim(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "asset", "logo.png")
	binary := []byte{0x89, 'P', 'N', 'G', 0xff, 0xfe, 0x00, 0x01}
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/media/abc/logo.png":
			return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(binary))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	output, err := execute(buildRootWithCollections(t, deps), "api", "GET", "/media/abc/logo.png", "--raw-path", "--out", outPath)
	if err != nil {
		t.Fatalf("api --out failed: %v", err)
	}
	saved, err := os.ReadFile(outPath)
	if err != nil || !bytes.Equal(saved, binary) {
		t.Fatalf("expected verbatim bytes on disk, got %v (%v)", saved, err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["bytes"] != float64(len(binary)) || payload["contentType"] != "image/png" || payload["ok"] != true || payload["statusCode"] != float64(http.StatusPartialContent) {
		t.Fatalf("unexpected summary (status must be the server's, not a constant 200): %s", output)
	}
	if entries, _ := os.ReadDir(filepath.Dir(outPath)); len(entries) != 1 {
		t.Fatalf("expected no leftover .part file, got %v", entries)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/x", "--out", outPath); err == nil {
		t.Fatalf("expected --out with POST to be rejected")
	}
}

func TestAPIDryRunPreviewIncludesHeaders(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "x.svg")
	_ = os.WriteFile(filePath, []byte("<svg/>"), 0o644)
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})
	output, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/temporary-file", "--dry-run", "--form", "file=@"+filePath, "--header", "X-Test: 1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"X-Test": "1"`) {
		t.Fatalf("expected headers in the dry-run preview, got %s", output)
	}
	output, err = execute(buildRootWithCollections(t, deps), "api", "GET", "/server/status", "--dry-run", "--header", "X-Test: 2")
	if err != nil || !strings.Contains(output, `"X-Test": "2"`) {
		t.Fatalf("expected headers in the JSON dry-run preview, got err=%v out=%s", err, output)
	}
}

func TestAPIDryRunPreviewIncludesImplicitHeaders(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})
	output, err := execute(buildRootWithCollections(t, deps), "api", "POST", "/server/status", "--dry-run", "--body", `{}`, "--header", "x-test: 1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Content-Type": "application/json"`, `"Authorization": "Bearer <redacted>"`, `"User-Agent": "umbraco-cli/`, `"X-Test": "1"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %s in preview headers, got %s", want, output)
		}
	}
	if strings.Contains(output, "token-123") {
		t.Fatalf("dry-run must not leak the token")
	}
	output, _ = execute(buildRootWithCollections(t, deps), "api", "GET", "/server/status", "--dry-run")
	if strings.Contains(output, "Content-Type") {
		t.Fatalf("a bodiless GET must not claim a Content-Type: %s", output)
	}
}
