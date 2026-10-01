package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

const fakeSharedSecret = "s3cret-value"

// basicAuthGate mimics the CMS BasicAuthenticationMiddleware with
// RedirectToLoginPage, as Umbraco Cloud Public Access configures it: core
// Management API routes are exempt, everything else needs the shared-secret
// header or is redirected to /umbraco/basic-auth/login.
func basicAuthGate(t *testing.T, header string, seen *[]*http.Request) *http.Client {
	t.Helper()
	return newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		*seen = append(*seen, r)
		if r.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		}
		if !strings.HasPrefix(r.URL.Path, "/umbraco/management/") && r.Header.Get(header) != fakeSharedSecret {
			location := "https://example.test/umbraco/basic-auth/login?returnPath=" + url.QueryEscape(r.URL.Path)
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if r.Header.Get("Authorization") != "Bearer token-123" {
			return jsonResponse(http.StatusUnauthorized, `{}`, nil), nil
		}
		return jsonResponse(http.StatusOK, `{"ok":true}`, nil), nil
	})
}

func deployGet(client *Client) (any, error) {
	return client.Get(context.Background(), "/configuration/client", RequestOptions{APIPrefix: "/umbraco/deploy/management/api/v1"})
}

func TestBasicAuthRedirectWithoutSecretNamesTheFix(t *testing.T) {
	var seen []*http.Request
	httpClient := basicAuthGate(t, config.DefaultBasicAuthSharedSecretHeader, &seen)
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	_, err := deployGet(client)
	var authErr *AuthRedirectError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthRedirectError, got %v", err)
	}
	if authErr.ExitCode() != 3 || !authErr.BasicAuth() || authErr.SecretSent {
		t.Fatalf("unexpected error fields: %+v", authErr)
	}
	for _, want := range []string{"blocked by basic authentication: GET /umbraco/deploy/management/api/v1/configuration/client redirected (302)", "Set basicAuthSharedSecret", "allow-list"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
	for _, r := range seen {
		if r.Header.Get(config.DefaultBasicAuthSharedSecretHeader) != "" {
			t.Fatalf("no secret is configured, yet %s carried the header", r.URL)
		}
	}
}

func TestBasicAuthSharedSecretGetsAddOnRequestsThrough(t *testing.T) {
	for _, tc := range []struct {
		name       string
		header     string
		configured string
	}{
		{name: "default header", header: config.DefaultBasicAuthSharedSecretHeader},
		{name: "custom header", header: "X-Cloud-Secret", configured: "X-Cloud-Secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen []*http.Request
			httpClient := basicAuthGate(t, tc.header, &seen)
			cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret", BasicAuthSharedSecret: fakeSharedSecret, BasicAuthSharedSecretHeader: tc.configured}
			client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

			result, err := deployGet(client)
			if err != nil {
				t.Fatalf("expected the shared secret to pass the gate, got %v", err)
			}
			if body, _ := json.Marshal(result); string(body) != `{"ok":true}` {
				t.Fatalf("unexpected body %s", body)
			}
			last := seen[len(seen)-1]
			if last.Header.Get("Authorization") != "Bearer token-123" || last.Header.Get(tc.header) != fakeSharedSecret {
				t.Fatalf("expected bearer token and shared secret side by side, got %v", last.Header)
			}
		})
	}
}

func TestBasicAuthWrongSecretSaysItWasRefused(t *testing.T) {
	var seen []*http.Request
	httpClient := basicAuthGate(t, config.DefaultBasicAuthSharedSecretHeader, &seen)
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret", BasicAuthSharedSecret: "wrong-value"}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	_, err := deployGet(client)
	var authErr *AuthRedirectError
	if !errors.As(err, &authErr) || !authErr.SecretSent || !strings.Contains(err.Error(), "shared secret was sent and not accepted") {
		t.Fatalf("expected the refused-secret error, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong-value") {
		t.Fatalf("the error leaks the secret: %v", err)
	}
}

func TestBasicAuthSecretNeverLeavesTheConfiguredHost(t *testing.T) {
	var seen []*http.Request
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r)
		switch r.URL.Host + r.URL.Path {
		case "example.test/umbraco/management/api/v1/security/back-office/token":
			return jsonResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`, nil), nil
		case "example.test/media/abc/logo.png":
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://cdn.example.test/blob/logo.png"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "cdn.example.test/blob/logo.png":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("PNG"))}, nil
		}
		return jsonResponse(http.StatusNotFound, `{}`, nil), nil
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "id", ClientSecret: "secret", BasicAuthSharedSecret: fakeSharedSecret}
	client := NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	var out strings.Builder
	if _, err := client.GetStream(context.Background(), "/media/abc/logo.png", &out, RequestOptions{RawPath: true}); err != nil || out.String() != "PNG" {
		t.Fatalf("download failed: err=%v body=%q", err, out.String())
	}
	var origin, cdn *http.Request
	for _, r := range seen {
		switch r.URL.Host {
		case "cdn.example.test":
			cdn = r
		case "example.test":
			if r.URL.Path == "/media/abc/logo.png" {
				origin = r
			}
		}
	}
	if origin == nil || origin.Header.Get(config.DefaultBasicAuthSharedSecretHeader) != fakeSharedSecret {
		t.Fatalf("the configured host should get the secret")
	}
	if cdn == nil || cdn.Header.Get(config.DefaultBasicAuthSharedSecretHeader) != "" {
		t.Fatalf("the secret followed a redirect to another host")
	}

	req, _ := http.NewRequest(http.MethodGet, "https://elsewhere.test/x", nil)
	if _, sent := SetBasicAuthSecret(req, cfg); sent || req.Header.Get(config.DefaultBasicAuthSharedSecretHeader) != "" {
		t.Fatalf("SetBasicAuthSecret must not add the secret for another host")
	}
}

func TestDryRunPreviewRedactsBasicAuthSecret(t *testing.T) {
	cfg := config.Config{BaseURL: "https://example.test", BasicAuthSharedSecret: fakeSharedSecret}
	client := NewClient(cfg, http.DefaultClient, nil)
	result, err := client.Get(context.Background(), "/queue", RequestOptions{DryRun: true, APIPrefix: "/umbraco/deploy/management/api/v1"})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	preview := result.(DryRunResult)
	if preview.Headers[config.DefaultBasicAuthSharedSecretHeader] != "***" {
		t.Fatalf("expected the secret header redacted in the preview, got %v", preview.Headers)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), fakeSharedSecret) {
		t.Fatalf("dry-run preview leaks the secret: %s", encoded)
	}
}
