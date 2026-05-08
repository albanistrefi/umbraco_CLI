package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

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
	_ = fmt.Sprintf("%s", encoded)
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

func TestRequestSkipValidationAllowsMergedBodiesWithControlCharacters(t *testing.T) {
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

	body := map[string]any{
		"name": "Partner A",
		"values": []any{
			map[string]any{"alias": "bodyText", "value": "line1\nline2"},
			map[string]any{"alias": "skills", "value": []any{map[string]any{"type": "document", "unique": "62689bb1-3a4d-478f-a7b1-1c0e560d4748"}}},
		},
	}

	if _, err := client.Put(context.Background(), "/document/doc-1", body, RequestOptions{SkipValidation: true}); err != nil {
		t.Fatalf("expected skip validation request to succeed, got %v", err)
	}
}
