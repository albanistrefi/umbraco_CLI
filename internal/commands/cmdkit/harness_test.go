package cmdkit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

// The builders are exercised end to end by every command package; these
// helpers let cmdkit pin their shared contract on its own, against a fake
// Management API, without depending on any command group.

const (
	testTokenPath = "/umbraco/management/api/v1/security/back-office/token"
	testMgmt      = "/umbraco/management/api/v1"
)

type kitRoundTripper func(*http.Request) (*http.Response, error)

func (fn kitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type kitRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// kitServer answers "METHOD /path" routes and records every non-token
// request. Unrouted requests answer 404.
type kitServer struct {
	mu       sync.Mutex
	routes   map[string]func(req kitRequest) (int, string)
	requests []kitRequest
}

func (s *kitServer) handle(req *http.Request) (*http.Response, error) {
	if req.URL.Path == testTokenPath {
		return kitResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	}
	rec := kitRequest{Method: req.Method, Path: req.URL.Path, Query: req.URL.Query()}
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.Body)
		}
	}
	s.mu.Lock()
	s.requests = append(s.requests, rec)
	s.mu.Unlock()
	if route, ok := s.routes[req.Method+" "+req.URL.Path]; ok {
		status, body := route(rec)
		return kitResponse(status, body), nil
	}
	return kitResponse(http.StatusNotFound, `{"title":"Not Found","status":404}`), nil
}

func kitResponse(status int, body string) *http.Response {
	header := http.Header{}
	if body != "" {
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func fixed(status int, body string) func(kitRequest) (int, string) {
	return func(kitRequest) (int, string) { return status, body }
}

func newKitServer(routes map[string]func(kitRequest) (int, string)) (*kitServer, Dependencies) {
	server := &kitServer{routes: routes}
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	httpClient := &http.Client{Transport: kitRoundTripper(server.handle)}
	output := "json"
	return server, Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		Config:     cfg,
		HTTPClient: httpClient,
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

// runKit mounts cmd under "umbraco widget" and executes it with args.
func runKit(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	group := &cobra.Command{Use: "widget"}
	group.AddCommand(cmd)
	root.AddCommand(group)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"widget", cmd.Name()}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func decodeObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, raw)
	}
	return out
}

func mustContain(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %v", want, err)
	}
}
