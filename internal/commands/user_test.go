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

func buildUserRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterUser(root, deps)
	return root
}

func TestUserGetSingleIDUsesUserPath(t *testing.T) {
	var requestedURI string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"user-1"}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildUserRoot(deps), "user", "get", "user-1"); err != nil {
		t.Fatalf("user get failed: %v", err)
	}
	if requestedURI != "/umbraco/management/api/v1/user/user-1" {
		t.Fatalf("unexpected URI %q", requestedURI)
	}
}

func TestUserGetMultipleIDsUseBatchEndpoint(t *testing.T) {
	var requestedURI string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `[{"id":"user-1"},{"id":"user-2"}]`), nil
		}
	})

	if _, err := cmdtest.Execute(buildUserRoot(deps), "user", "get", "user-1", "user-2"); err != nil {
		t.Fatalf("user get batch failed: %v", err)
	}
	if !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/user/batch?") {
		t.Fatalf("expected batch endpoint, got %q", requestedURI)
	}
	if !strings.Contains(requestedURI, "id=user-1") || !strings.Contains(requestedURI, "id=user-2") {
		t.Fatalf("expected repeated id params, got %q", requestedURI)
	}
}

func TestUserSetLanguagePutsProfile(t *testing.T) {
	var requestedPath, requestedMethod, requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			requestedMethod = req.Method
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `null`), nil
		}
	})

	out, err := cmdtest.Execute(buildUserRoot(deps), "user", "set-language", "da-DK")
	if err != nil {
		t.Fatalf("user set-language failed: %v", err)
	}
	if requestedMethod != http.MethodPut || requestedPath != "/umbraco/management/api/v1/user/current/profile" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(requestedBody, `"languageIsoCode":"da-DK"`) {
		t.Fatalf("expected languageIsoCode in body, got %q", requestedBody)
	}
	if !strings.Contains(out, `"updated": true`) {
		t.Fatalf("expected empty-success envelope, got %s", out)
	}
}
