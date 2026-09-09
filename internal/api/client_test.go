package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestHTTPClient(handler roundTripFunc) *http.Client {
	return &http.Client{Transport: handler}
}

func jsonResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	for key, value := range headers {
		header.Set(key, value)
	}

	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDryRunReturnsPreview(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test"}
	client := NewClient(cfg, http.DefaultClient, nil)

	result, err := client.Post(context.Background(), "/document/abc-123/publish", map[string]any{"cultures": []any{"en-US"}}, RequestOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run should not fail: %v", err)
	}

	dryRun, ok := result.(DryRunResult)
	if !ok {
		t.Fatalf("expected DryRunResult, got %T", result)
	}

	if !dryRun.DryRun || !dryRun.Valid || dryRun.Method != http.MethodPost {
		t.Fatalf("unexpected dry-run metadata: %+v", dryRun)
	}
	if dryRun.Path != "/umbraco/management/api/v1/document/abc-123/publish" {
		t.Fatalf("unexpected dry-run path: %s", dryRun.Path)
	}
}

func TestRequestBuildsURLAndUsesToken(t *testing.T) {
	var observedRequestPath string
	var observedAuth string

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/root":
			observedRequestPath = r.URL.String()
			observedAuth = r.Header.Get("Authorization")
			return jsonResponse(http.StatusOK, `{"items":[{"id":"root"}]}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	tokenProvider := auth.New(cfg, httpClient)
	client := NewClient(cfg, httpClient, tokenProvider)

	result, err := client.Get(context.Background(), "/document/root", RequestOptions{Fields: "id,name", Params: map[string]any{"skip": 0, "take": 10, "culture": "en-US"}})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if strings.Contains(observedRequestPath, "fields=") || !strings.Contains(observedRequestPath, "skip=0") || !strings.Contains(observedRequestPath, "take=10") {
		t.Fatalf("unexpected query string: %s", observedRequestPath)
	}
	if observedAuth != "Bearer token-123" {
		t.Fatalf("unexpected auth header: %s", observedAuth)
	}

	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", result)
	}
	if _, ok := payload["items"]; !ok {
		t.Fatalf("expected items in response")
	}
}

func TestRequestReturnsAPIErrorBody(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/root":
			return jsonResponse(http.StatusBadRequest, `{"error":"invalid request"}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	_, err := client.Get(context.Background(), "/document/root", RequestOptions{})
	if err == nil {
		t.Fatalf("expected API error")
	}
	if !strings.Contains(err.Error(), "API 400") {
		t.Fatalf("expected status in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "GET /umbraco/management/api/v1/document/root") {
		t.Fatalf("expected method and path in API error, got: %v", err)
	}
}

func TestRequestReturnsIDFromLocationHeaderWhenBodyIsEmpty(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/source-1/copy":
			return jsonResponse(http.StatusCreated, ``, map[string]string{"Location": "https://example.test/umbraco/management/api/v1/document/copy-1"}), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	result, err := client.Post(context.Background(), "/document/source-1/copy", map[string]any{"target": map[string]any{"id": "parent-1"}}, RequestOptions{})
	if err != nil {
		t.Fatalf("copy request failed: %v", err)
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["id"] != "copy-1" {
		t.Fatalf("expected id from Location header, got %+v", result)
	}
}

func TestRequestMergesIDFromLocationHeaderIntoSuccessBody(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/data-type":
			return jsonResponse(http.StatusCreated, `{"success":true}`, map[string]string{"Location": "/umbraco/management/api/v1/data-type/dt-1"}), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	result, err := client.Post(context.Background(), "/data-type", map[string]any{"name": "Text"}, RequestOptions{})
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["id"] != "dt-1" || payload["success"] != true {
		t.Fatalf("expected merged id from Location header, got %+v", result)
	}
}

func TestRequestAddsNotFoundHintWithResolvedPath(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/item/data-type/search":
			return jsonResponse(http.StatusNotFound, `null`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	_, err := client.Get(context.Background(), "/item/data-type/search", RequestOptions{Params: map[string]any{"query": "google"}})
	if err == nil {
		t.Fatalf("expected API error")
	}
	if !strings.Contains(err.Error(), "GET /umbraco/management/api/v1/item/data-type/search?query=google") {
		t.Fatalf("expected resolved request path in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "may not be supported in your Umbraco version") {
		t.Fatalf("expected version hint in error, got: %v", err)
	}
}

func TestDryRunBodySerializesConsistently(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test"}
	client := NewClient(cfg, http.DefaultClient, nil)

	result, err := client.Post(context.Background(), "/document/abc-123/publish", map[string]any{"cultures": []any{"da-DK"}}, RequestOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(encoded), `"dryRun":true`) {
		t.Fatalf("unexpected dry-run JSON: %s", string(encoded))
	}
	if !strings.Contains(string(encoded), `"da-DK"`) {
		t.Fatalf("expected body culture in JSON: %s", string(encoded))
	}
	_ = string(encoded)
}

func TestRequestRetriesOn429(t *testing.T) {
	requests := 0

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/root":
			requests++
			if requests == 1 {
				return jsonResponse(http.StatusTooManyRequests, `{"error":"slow down"}`, map[string]string{"Retry-After": "0"}), nil
			}
			return jsonResponse(http.StatusOK, `{"items":[{"id":"root"}]}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	result, err := client.Get(context.Background(), "/document/root", RequestOptions{})
	if err != nil {
		t.Fatalf("request should succeed after retry: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 document requests, got %d", requests)
	}

	payload, ok := result.(map[string]any)
	if !ok || payload["items"] == nil {
		t.Fatalf("expected retried response payload, got %+v", result)
	}
}

func TestRequestRefreshesTokenAfter401(t *testing.T) {
	tokenRequests := 0
	documentRequests := 0
	var observedAuth string

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			tokenRequests++
			return jsonResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"token-%d","expires_in":3600}`, tokenRequests), nil), nil
		case "/umbraco/management/api/v1/document/root":
			documentRequests++
			observedAuth = r.Header.Get("Authorization")
			if documentRequests == 1 {
				return jsonResponse(http.StatusUnauthorized, `{"error":"expired token"}`, nil), nil
			}
			return jsonResponse(http.StatusOK, `{"items":[{"id":"root"}]}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	_, err := client.Get(context.Background(), "/document/root", RequestOptions{})
	if err != nil {
		t.Fatalf("request should succeed after token refresh: %v", err)
	}
	if tokenRequests != 2 {
		t.Fatalf("expected 2 token requests, got %d", tokenRequests)
	}
	if documentRequests != 2 {
		t.Fatalf("expected 2 document requests, got %d", documentRequests)
	}
	if observedAuth != "Bearer token-2" {
		t.Fatalf("expected refreshed token on second request, got %s", observedAuth)
	}
}

func TestRequestAllowsArbitraryContentInBodies(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return jsonResponse(http.StatusOK, `{"ok":true}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	// Real CMS payloads contain multiline text, encoded URLs, and property
	// aliases like "video"/"width" that earlier heuristics misread as IDs.
	body := map[string]any{
		"name": "Partner A",
		"values": []any{
			map[string]any{"alias": "bodyText", "value": "line1\nline2\tindented"},
			map[string]any{"alias": "video", "value": "https://youtube.com/watch?v=abc#t=10"},
			map[string]any{"alias": "width", "value": "50% off %20"},
			map[string]any{"alias": "skills", "value": []any{map[string]any{"type": "document", "unique": "62689bb1-3a4d-478f-a7b1-1c0e560d4748"}}},
		},
	}

	if _, err := client.Put(context.Background(), "/document/doc-1", body, RequestOptions{}); err != nil {
		t.Fatalf("expected request with real-world content to succeed, got %v", err)
	}
}

func TestJoinPathEscapesArguments(t *testing.T) {
	cases := []struct {
		format string
		args   []string
		want   string
	}{
		{"/document/%s", []string{"abc-123"}, "/document/abc-123"},
		{"/document/%s/children", []string{".."}, "/document/%2E%2E/children"},
		{"/document/%s", []string{"."}, "/document/%2E"},
		{"/document/%s/copy", []string{"../server/status"}, "/document/..%2Fserver%2Fstatus/copy"},
		{"/document/%s", []string{"id?x=1#y"}, "/document/id%3Fx=1%23y"},
		{"/health-check-group/%s", []string{"Data Integrity"}, "/health-check-group/Data%20Integrity"},
	}
	for _, tc := range cases {
		if got := JoinPath(tc.format, tc.args...); got != tc.want {
			t.Fatalf("JoinPath(%q, %v) = %q, want %q", tc.format, tc.args, got, tc.want)
		}
	}
}

func TestRequestPreservesEscapedPathSegments(t *testing.T) {
	var observedURI string
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		default:
			observedURI = r.URL.RequestURI()
			return jsonResponse(http.StatusOK, `{"ok":true}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	// A traversal attempt escaped by JoinPath must reach the server as one
	// literal segment, not rewrite the route.
	if _, err := client.Get(context.Background(), JoinPath("/document/%s", "../user-group/admin"), RequestOptions{}); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if observedURI != "/umbraco/management/api/v1/document/..%2Fuser-group%2Fadmin" {
		t.Fatalf("expected escaped segment to survive into the request URI, got %q", observedURI)
	}
}

func TestMultipartPostRetriesOn429(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(filePath, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	uploads := 0
	var retriedBody string

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/temporary-file":
			uploads++
			if uploads == 1 {
				return jsonResponse(http.StatusTooManyRequests, `{"error":"slow down"}`, map[string]string{"Retry-After": "0"}), nil
			}
			body, _ := io.ReadAll(r.Body)
			retriedBody = string(body)
			return jsonResponse(http.StatusCreated, `{"id":"tmp-1"}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	result, err := client.MultipartPost(context.Background(), "/temporary-file", map[string]string{"Id": "tmp-1"}, "File", filePath, RequestOptions{})
	if err != nil {
		t.Fatalf("upload should succeed after retry: %v", err)
	}
	if uploads != 2 {
		t.Fatalf("expected 2 upload requests, got %d", uploads)
	}
	if !strings.Contains(retriedBody, "png-bytes") {
		t.Fatalf("retried request should replay the full multipart body, got: %q", retriedBody)
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["id"] != "tmp-1" {
		t.Fatalf("unexpected upload result: %+v", result)
	}
}

func TestMultipartPostRefreshesTokenAfter401(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(filePath, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tokenRequests := 0
	uploads := 0
	var observedAuth string

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			tokenRequests++
			return jsonResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"token-%d","expires_in":3600}`, tokenRequests), nil), nil
		case "/umbraco/management/api/v1/temporary-file":
			uploads++
			observedAuth = r.Header.Get("Authorization")
			if uploads == 1 {
				return jsonResponse(http.StatusUnauthorized, `{"error":"expired token"}`, nil), nil
			}
			return jsonResponse(http.StatusCreated, `{"id":"tmp-1"}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	if _, err := client.MultipartPost(context.Background(), "/temporary-file", nil, "File", filePath, RequestOptions{}); err != nil {
		t.Fatalf("upload should succeed after token refresh: %v", err)
	}
	if tokenRequests != 2 {
		t.Fatalf("expected 2 token requests, got %d", tokenRequests)
	}
	if uploads != 2 {
		t.Fatalf("expected 2 upload requests, got %d", uploads)
	}
	if observedAuth != "Bearer token-2" {
		t.Fatalf("expected refreshed token on second upload, got %s", observedAuth)
	}
}

func TestAPIErrorTruncatesLargePayload(t *testing.T) {
	err := &APIError{
		StatusCode: 400,
		Method:     http.MethodPost,
		Path:       "/umbraco/management/api/v1/document",
		Payload:    map[string]any{"detail": strings.Repeat("é", 10_000)},
	}

	message := err.Error()
	if len(message) > maxErrorPayloadBytes+200 {
		t.Fatalf("expected truncated error message, got %d bytes", len(message))
	}
	if !strings.Contains(message, "…(truncated)") {
		t.Fatalf("expected truncation marker in message: %s", message[:100])
	}
	if !utf8.ValidString(message) {
		t.Fatalf("truncation must not split a UTF-8 rune")
	}

	small := &APIError{StatusCode: 404, Payload: map[string]any{"error": "not found"}}
	if strings.Contains(small.Error(), "truncated") {
		t.Fatalf("small payloads must not be truncated: %s", small.Error())
	}
}

func TestRetryAfterDelayJitterWithinBounds(t *testing.T) {
	for attempt, base := range map[int]time.Duration{0: 200 * time.Millisecond, 2: 800 * time.Millisecond} {
		for i := 0; i < 50; i++ {
			delay := retryAfterDelay("", attempt)
			if delay < base || delay >= base+base/2 {
				t.Fatalf("attempt %d: delay %v outside [%v, %v)", attempt, delay, base, base+base/2)
			}
		}
	}

	// Server-provided Retry-After values are honored verbatim, no jitter.
	if delay := retryAfterDelay("2", 0); delay != 2*time.Second {
		t.Fatalf("expected exact Retry-After honor, got %v", delay)
	}
}

func TestRequestSetsUserAgentAndCustomHeadersOnTokenAndAPICalls(t *testing.T) {
	observed := map[string]string{}

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			observed["token-ua"] = r.Header.Get("User-Agent")
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/server/status":
			observed["api-ua"] = r.Header.Get("User-Agent")
			observed["custom"] = r.Header.Get("X-Trace")
			return jsonResponse(http.StatusOK, `{"serverStatus":"Run"}`, nil), nil
		default:
			return jsonResponse(http.StatusNotFound, `null`, nil), nil
		}
	})

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))
	if _, err := client.Get(context.Background(), "/server/status", RequestOptions{Headers: map[string]string{"X-Trace": "abc"}}); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	for _, key := range []string{"token-ua", "api-ua"} {
		if !strings.HasPrefix(observed[key], "umbraco-cli/") {
			t.Fatalf("expected %s to carry umbraco-cli User-Agent, got %q", key, observed[key])
		}
	}
	if observed["custom"] != "abc" {
		t.Fatalf("expected custom header to be forwarded, got %q", observed["custom"])
	}
}

func TestRawPathSkipsManagementAPIPrefix(t *testing.T) {
	var observedPath string
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		default:
			observedPath = r.URL.Path
			return jsonResponse(http.StatusOK, `{"ok":true}`, nil), nil
		}
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))
	if _, err := client.Get(context.Background(), "/umbraco/automate/management/api/v1/automations", RequestOptions{RawPath: true}); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if observedPath != "/umbraco/automate/management/api/v1/automations" {
		t.Fatalf("expected raw path to be sent verbatim, got %q", observedPath)
	}
}

func TestMultipartResultSendsFieldsAndFiles(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(filePath, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	var observedID, observedFile, observedName string
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/temporary-file":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("multipart parse failed: %v", err)
			}
			observedID = r.FormValue("id")
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("missing file part: %v", err)
			}
			defer func() { _ = file.Close() }()
			content, _ := io.ReadAll(file)
			observedFile = string(content)
			observedName = header.Filename
			return jsonResponse(http.StatusCreated, ``, map[string]string{"Location": "/umbraco/management/api/v1/temporary-file/tmp-1"}), nil
		default:
			return jsonResponse(http.StatusNotFound, `null`, nil), nil
		}
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))
	result, err := client.MultipartResult(context.Background(), http.MethodPost, "/temporary-file", map[string]string{"id": "tmp-1"}, map[string]string{"file": filePath}, RequestOptions{})
	if err != nil {
		t.Fatalf("multipart failed: %v", err)
	}
	if result.StatusCode != http.StatusCreated || observedID != "tmp-1" || observedFile != "<svg/>" || observedName != "logo.svg" {
		t.Fatalf("unexpected multipart observation: status=%d id=%q file=%q name=%q", result.StatusCode, observedID, observedFile, observedName)
	}
}

func TestNotFoundHintDistinguishesMissingEntityFromMissingRoute(t *testing.T) {
	entity := buildAPIErrorHint(http.StatusNotFound, http.MethodGet, "/umbraco/management/api/v1/document-type/x", map[string]any{"operationStatus": "NotFound", "detail": "The specified document type was not found"})
	if !strings.Contains(entity, "The specified document type was not found") || strings.Contains(entity, "may not be supported") {
		t.Fatalf("expected entity-missing hint, got %q", entity)
	}
	route := buildAPIErrorHint(http.StatusNotFound, http.MethodGet, "/umbraco/management/api/v1/health-check-group/x/run", nil)
	if !strings.Contains(route, "may not be supported in your Umbraco version") {
		t.Fatalf("expected route-missing hint, got %q", route)
	}
}

func TestNotFoundHintTruncatesLongServerDetail(t *testing.T) {
	long := strings.Repeat("x", 5000)
	hint := buildAPIErrorHint(http.StatusNotFound, http.MethodGet, "/umbraco/management/api/v1/document-type/x", map[string]any{"operationStatus": "NotFound", "detail": long})
	if len(hint) > 600 {
		t.Fatalf("expected hint to be capped, got %d bytes", len(hint))
	}
}

func TestNotFoundHintStripsControlCharacters(t *testing.T) {
	hint := buildAPIErrorHint(http.StatusNotFound, http.MethodGet, "/umbraco/management/api/v1/document-type/x", map[string]any{"operationStatus": "NotFound", "detail": "gone\x1b[2J\x1b]52;c;evil\x07 really"})
	if strings.ContainsAny(hint, "\x1b\x07\r\n") || !strings.Contains(hint, "gone") {
		t.Fatalf("expected control characters stripped, got %q", hint)
	}
	bidi := buildAPIErrorHint(http.StatusNotFound, http.MethodGet, "/umbraco/management/api/v1/document-type/x", map[string]any{"operationStatus": "NotFound", "detail": "ok\u202etxt.exe\u2066hidden\u2069 æøå"})
	if strings.ContainsAny(bidi, "\u202e\u2066\u2069") || !strings.Contains(bidi, "æøå") {
		t.Fatalf("expected Unicode format controls stripped and printable text kept, got %q", bidi)
	}
}
