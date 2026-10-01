// Package cmdtest is the shared test harness for command packages: fake
// Management API transports, canned responses, Dependencies wired to them,
// and helpers that build and execute a command root. It is imported only by
// _test.go files.
package cmdtest

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
)

// TokenPath is the back-office token route every authenticated fake answers.
const TokenPath = "/umbraco/management/api/v1/security/back-office/token"

// RoundTripper adapts a function to http.RoundTripper so a test can answer
// requests inline.
type RoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip calls fn.
func (fn RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// JSONResponse builds a JSON reply with the given status.
func JSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// NoContent simulates a 204 No Content reply (the shape Umbraco
// returns for successful document update / publish PUTs). The HTTP client's
// parseResponse maps an empty body to nil, which is what reaches the
// command layer.
func NoContent() *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// TokenOr404 answers the token request and delegates everything else.
func TokenOr404(t *testing.T, req *http.Request, handler func(req *http.Request) (*http.Response, error)) (*http.Response, error) {
	t.Helper()
	if req.URL.Path == TokenPath {
		return JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	}
	return handler(req)
}

// Deps wires an authenticated client, the config and the HTTP client to
// handler, with JSON output.
func Deps(handler RoundTripper) cmdkit.Dependencies {
	cfg := config.Config{
		BaseURL:      "https://example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	httpClient := &http.Client{Transport: handler}
	output := "json"

	return cmdkit.Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		Config:     cfg,
		HTTPClient: httpClient,
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

// ClientDeps is Deps with only the API client set (no Config or
// HTTPClient), for commands that must work from the client alone.
func ClientDeps(handler RoundTripper) cmdkit.Dependencies {
	cfg := config.Config{
		BaseURL:      "https://example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	httpClient := &http.Client{Transport: handler}
	output := "json"

	return cmdkit.Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

// MakeDeps returns unauthenticated Dependencies on the default HTTP client,
// for tests that only build or inspect the command tree.
func MakeDeps() cmdkit.Dependencies {
	cfg := config.Config{BaseURL: "https://example.test"}
	client := api.NewClient(cfg, http.DefaultClient, nil)
	output := "json"
	return cmdkit.Dependencies{Client: client, Config: cfg, HTTPClient: http.DefaultClient, EnvOutput: config.OutputJSON, OutputFlag: &output}
}
