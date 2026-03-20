package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

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
