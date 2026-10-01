package commands

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

func buildRedirectRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterRedirect(root, deps)
	return root
}

func redirectDeps(handler func(req *http.Request) (*http.Response, error)) cmdkit.Dependencies {
	return cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestRedirectListForwardsFilterAndPagination(t *testing.T) {
	var requestedURI string
	deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"originalUrl":"/old-page/","destinationUrl":"/new-page/"}],"total":1}`), nil
	})

	out, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", "list", "--filter", "old-page", "--skip", "0", "--take", "20")
	if err != nil {
		t.Fatalf("redirect list failed: %v", err)
	}
	for _, expected := range []string{"/umbraco/management/api/v1/redirect-management?", "filter=old-page", "skip=0", "take=20"} {
		if !strings.Contains(requestedURI, expected) {
			t.Fatalf("expected %q in request URI %q", expected, requestedURI)
		}
	}
	if !strings.Contains(out, "/old-page/") {
		t.Fatalf("expected redirect item in output, got %s", out)
	}
}

func TestRedirectGetPaginatesPerDocument(t *testing.T) {
	var requestedURI string
	deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", "get", "doc-1", "--take", "5"); err != nil {
		t.Fatalf("redirect get failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/umbraco/management/api/v1/redirect-management/doc-1") || !strings.Contains(requestedURI, "take=5") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
}

func TestRedirectDeleteIsGated(t *testing.T) {
	deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", "delete", "r-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}
}

func TestRedirectDeleteWithForce(t *testing.T) {
	var requestedPath, requestedMethod string
	deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
		requestedPath = req.URL.Path
		requestedMethod = req.Method
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	out, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", "delete", "r-1", "--force")
	if err != nil {
		t.Fatalf("redirect delete failed: %v", err)
	}
	if requestedMethod != http.MethodDelete || requestedPath != "/umbraco/management/api/v1/redirect-management/r-1" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(out, `"deleted": true`) && !strings.Contains(out, `"deleted":true`) {
		t.Fatalf("expected deleted:true, got %s", out)
	}
}

func TestRedirectEnableDisablePostStatus(t *testing.T) {
	for _, tc := range []struct {
		command string
		status  string
		verb    string
	}{
		{command: "enable", status: "status=Enabled", verb: `"enabled"`},
		{command: "disable", status: "status=Disabled", verb: `"disabled"`},
	} {
		var requestedURI, requestedMethod string
		deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
			requestedURI = req.URL.RequestURI()
			requestedMethod = req.Method
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})

		out, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", tc.command)
		if err != nil {
			t.Fatalf("redirect %s failed: %v", tc.command, err)
		}
		if requestedMethod != http.MethodPost || !strings.Contains(requestedURI, "/redirect-management/status") || !strings.Contains(requestedURI, tc.status) {
			t.Fatalf("unexpected request %s %s", requestedMethod, requestedURI)
		}
		if !strings.Contains(out, tc.verb) {
			t.Fatalf("expected %s verb in output, got %s", tc.verb, out)
		}
	}
}

func TestRedirectStatusReads(t *testing.T) {
	deps := redirectDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/umbraco/management/api/v1/redirect-management/status" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		return cmdtest.JSONResponse(http.StatusOK, `{"status":"Enabled","userIsAdmin":true}`), nil
	})

	out, err := cmdtest.Execute(buildRedirectRoot(deps), "redirect", "status")
	if err != nil {
		t.Fatalf("redirect status failed: %v", err)
	}
	if !strings.Contains(out, "Enabled") {
		t.Fatalf("expected status payload, got %s", out)
	}
}
