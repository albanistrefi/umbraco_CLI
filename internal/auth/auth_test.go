package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"umbraco-cli/internal/config"
)

type authRoundTripper func(*http.Request) (*http.Response, error)

func (fn authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func authJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAccessTokenIncludesResolvedBaseURLOnTransportFailure(t *testing.T) {
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp [::1]:44391: connect: connection refused")
	})}

	cfg := config.Config{
		BaseURL:      "https://localhost:44391",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}

	_, err := New(cfg, httpClient).AccessToken(context.Background())
	if err == nil {
		t.Fatalf("expected auth transport error")
	}
	if !strings.Contains(err.Error(), "resolved base URL https://localhost:44391") {
		t.Fatalf("expected base URL in auth error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "/umbraco/management/api/v1/security/back-office/token") {
		t.Fatalf("expected token endpoint in auth error, got %q", err.Error())
	}
}

func TestAccessTokenIncludesResolvedBaseURLOnHTTPFailure(t *testing.T) {
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		return authJSONResponse(http.StatusUnauthorized, `{"error":"bad client"}`), nil
	})}

	cfg := config.Config{
		BaseURL:      "https://localhost:44314",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}

	_, err := New(cfg, httpClient).AccessToken(context.Background())
	if err == nil {
		t.Fatalf("expected auth HTTP error")
	}
	if !strings.Contains(err.Error(), "resolved base URL https://localhost:44314") {
		t.Fatalf("expected base URL in auth error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected status code in auth error, got %q", err.Error())
	}
}

func TestAccessTokenCachesUntilExpiry(t *testing.T) {
	tokenRequests := 0
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		tokenRequests++
		return authJSONResponse(http.StatusOK, `{"access_token":"token-1","expires_in":3600}`), nil
	})}

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	provider := New(cfg, httpClient)

	for i := 0; i < 3; i++ {
		token, err := provider.AccessToken(context.Background())
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if token != "token-1" {
			t.Fatalf("call %d: unexpected token %q", i, token)
		}
	}
	if tokenRequests != 1 {
		t.Fatalf("expected 1 token request for cached calls, got %d", tokenRequests)
	}
}

func TestAccessTokenInvalidateForcesRefetch(t *testing.T) {
	tokenRequests := 0
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		tokenRequests++
		return authJSONResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"token-%d","expires_in":3600}`, tokenRequests)), nil
	})}

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	provider := New(cfg, httpClient)

	if _, err := provider.AccessToken(context.Background()); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	provider.Invalidate()
	token, err := provider.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("post-invalidate call failed: %v", err)
	}
	if token != "token-2" || tokenRequests != 2 {
		t.Fatalf("expected refetched token-2 after Invalidate, got %q after %d requests", token, tokenRequests)
	}
}

func TestAccessTokenExpiryMarginPreventsCachingShortLivedTokens(t *testing.T) {
	// The provider refreshes one minute before expiry, so a token whose
	// lifetime is inside that margin must not be served from cache.
	tokenRequests := 0
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		tokenRequests++
		return authJSONResponse(http.StatusOK, `{"access_token":"short-lived","expires_in":30}`), nil
	})}

	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	provider := New(cfg, httpClient)

	for i := 0; i < 2; i++ {
		if _, err := provider.AccessToken(context.Background()); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	if tokenRequests != 2 {
		t.Fatalf("expected short-lived token to bypass the cache, got %d token requests", tokenRequests)
	}
}

func TestAccessTokenRejectsIncompleteTokenResponse(t *testing.T) {
	cases := map[string]string{
		"missing token":       `{"expires_in":3600}`,
		"missing expiry":      `{"access_token":"token-1"}`,
		"non-positive expiry": `{"access_token":"token-1","expires_in":0}`,
	}
	for name, body := range cases {
		httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
			return authJSONResponse(http.StatusOK, body), nil
		})}
		cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}

		_, err := New(cfg, httpClient).AccessToken(context.Background())
		if err == nil || !strings.Contains(err.Error(), "missing required fields") {
			t.Fatalf("%s: expected missing-fields error, got %v", name, err)
		}
	}
}

func TestAccessTokenRejectsMalformedTokenResponse(t *testing.T) {
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		return authJSONResponse(http.StatusOK, `not-json`), nil
	})}
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}

	if _, err := New(cfg, httpClient).AccessToken(context.Background()); err == nil {
		t.Fatalf("expected decode error for malformed token response")
	}
}

func TestAccessTokenRequiresCredentials(t *testing.T) {
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected when credentials are missing")
		return nil, nil
	})}
	cfg := config.Config{BaseURL: "https://example.test"}

	_, err := New(cfg, httpClient).AccessToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "UMBRACO_CLIENT_ID") {
		t.Fatalf("expected credential validation error, got %v", err)
	}
}

func TestAccessTokenSendsClientCredentialsForm(t *testing.T) {
	var observedContentType, observedBody string
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		observedContentType = req.Header.Get("Content-Type")
		body, _ := io.ReadAll(req.Body)
		observedBody = string(body)
		return authJSONResponse(http.StatusOK, `{"access_token":"token-1","expires_in":3600}`), nil
	})}
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}

	if _, err := New(cfg, httpClient).AccessToken(context.Background()); err != nil {
		t.Fatalf("token request failed: %v", err)
	}
	if observedContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected content type %q", observedContentType)
	}
	for _, expected := range []string{"grant_type=client_credentials", "client_id=client-id", "client_secret=client-secret"} {
		if !strings.Contains(observedBody, expected) {
			t.Fatalf("expected %q in form body, got %q", expected, observedBody)
		}
	}
}

func TestAccessTokenFetchOutlivesAnImpatientCaller(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret"}
	var requests atomic.Int32
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		time.Sleep(200 * time.Millisecond)
		return authJSONResponse(http.StatusOK, `{"access_token":"abc","expires_in":3600}`), nil
	})}
	provider := New(cfg, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := provider.AccessToken(ctx)
	var authError *Error
	if !errors.As(err, &authError) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an impatient caller gets an auth error wrapping its deadline, got %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	token, err := provider.AccessToken(context.Background())
	if err != nil || token != "abc" {
		t.Fatalf("the abandoned request should have been cached, got %q %v", token, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one token request, got %d", got)
	}
}

func TestAccessTokenConcurrentCallersShareOneRequest(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret"}
	var requests atomic.Int32
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		time.Sleep(50 * time.Millisecond)
		return authJSONResponse(http.StatusOK, `{"access_token":"abc","expires_in":3600}`), nil
	})}
	provider := New(cfg, httpClient)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if token, err := provider.AccessToken(context.Background()); err != nil || token != "abc" {
				t.Errorf("got %q %v", token, err)
			}
		}()
	}
	wg.Wait()
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected the callers to share one token request, got %d", got)
	}
}

func TestInvalidateIfCurrentKeepsANewerToken(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret"}
	var requests atomic.Int32
	httpClient := &http.Client{Transport: authRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return authJSONResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"t%d","expires_in":3600}`, requests.Load())), nil
	})}
	provider := New(cfg, httpClient)
	current, _ := provider.AccessToken(context.Background())
	provider.InvalidateIfCurrent("an-older-token")
	if token, _ := provider.AccessToken(context.Background()); token != current || requests.Load() != 1 {
		t.Fatalf("a stale invalidation discarded the current token: %q after %d requests", token, requests.Load())
	}
	provider.InvalidateIfCurrent(current)
	if token, _ := provider.AccessToken(context.Background()); token == current || requests.Load() != 2 {
		t.Fatalf("invalidating the current token should fetch a new one: %q after %d requests", token, requests.Load())
	}
}
