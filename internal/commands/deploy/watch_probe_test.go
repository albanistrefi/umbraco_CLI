package deploy

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

// protectedSite mimics a Cloud non-live environment: without the shared
// secret every public path redirects to the basic-auth login form, which
// itself answers 200. "/" otherwise redirects to "/en/" (a normal hop).
func protectedSite(seen map[string]string) *http.Client {
	return &http.Client{Transport: cmdtest.RoundTripper(func(r *http.Request) (*http.Response, error) {
		seen[r.URL.Host+r.URL.Path] = r.Header.Get(config.DefaultBasicAuthSharedSecretHeader)
		if r.URL.Path == "/umbraco/basic-auth/login" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<form>login</form>"))}, nil
		}
		if r.URL.Host == "dev.example.test" && r.Header.Get(config.DefaultBasicAuthSharedSecretHeader) != "shh" {
			location := "https://dev.example.test/umbraco/basic-auth/login?returnPath=" + url.QueryEscape(r.URL.Path)
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if r.URL.Path == "/" {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"/en/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("home"))}, nil
	})}
}

func TestWatchHealthProbeDoesNotCountTheBasicAuthLoginPageAsServing(t *testing.T) {
	seen := map[string]string{}
	probes := &watchProbes{
		httpClient:  protectedSite(seen),
		timeout:     time.Second,
		cfg:         config.Config{BaseURL: "https://dev.example.test"},
		publicURL:   "https://dev.example.test",
		healthPaths: []string{"/"},
	}
	if health := probes.probeHealth(context.Background()); health["/"] {
		t.Fatalf("a redirect to the basic-auth login was counted as healthy")
	}
	if _, followed := seen["dev.example.test/umbraco/basic-auth/login"]; followed {
		t.Fatalf("the login redirect should not be followed")
	}
}

func TestWatchHealthProbeSendsSharedSecretToTheEnvironmentOnly(t *testing.T) {
	seen := map[string]string{}
	probes := &watchProbes{
		httpClient:  protectedSite(seen),
		timeout:     time.Second,
		cfg:         config.Config{BaseURL: "https://dev.example.test", BasicAuthSharedSecret: "shh"},
		publicURL:   "https://dev.example.test",
		healthPaths: []string{"/"},
	}
	if health := probes.probeHealth(context.Background()); !health["/"] {
		t.Fatalf("with the shared secret the site should probe healthy (following the normal / -> /en/ hop); seen %v", seen)
	}
	if seen["dev.example.test/en/"] != "shh" {
		t.Fatalf("same-host hop should keep the secret; seen %v", seen)
	}

	seen = map[string]string{}
	probes.publicURL = "https://www.example.test"
	if health := probes.probeHealth(context.Background()); !health["/"] {
		t.Fatalf("public host probe failed; seen %v", seen)
	}
	for target, secret := range seen {
		if secret != "" {
			t.Fatalf("the shared secret went to %s, which is not the configured base URL's host", target)
		}
	}
}
