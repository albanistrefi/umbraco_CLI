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

func buildSortChildrenRoot(deps cmdkit.Dependencies, resource string, withCulture bool) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	group := &cobra.Command{Use: resource}
	group.AddCommand(sortChildrenCommand(deps, resource, withCulture))
	root.AddCommand(group)
	return root
}

func TestSortChildrenTargetsRootRouteWithoutParent(t *testing.T) {
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

	out, err := cmdtest.Execute(buildSortChildrenRoot(deps, "document", true), "document", "sort-children", "--field", "name")
	if err != nil {
		t.Fatalf("sort-children failed: %v", err)
	}
	if requestedMethod != http.MethodPut || requestedPath != "/umbraco/management/api/v1/document/root/sort-children" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(requestedBody, `"field":"Name"`) {
		t.Fatalf("expected canonical field casing, got %q", requestedBody)
	}
	if !strings.Contains(requestedBody, `"direction":"Ascending"`) {
		t.Fatalf("expected default Ascending direction, got %q", requestedBody)
	}
	if !strings.Contains(out, `"sorted": true`) {
		t.Fatalf("expected empty-success envelope, got %s", out)
	}
}

func TestSortChildrenTargetsParentRouteAndNormalizesDesc(t *testing.T) {
	var requestedPath, requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `null`), nil
		}
	})

	if _, err := cmdtest.Execute(
		buildSortChildrenRoot(deps, "media", false),
		"media", "sort-children", "parent-1",
		"--field", "createdate",
		"--direction", "desc",
	); err != nil {
		t.Fatalf("sort-children with parent failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/media/parent-1/sort-children" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(requestedBody, `"field":"CreateDate"`) || !strings.Contains(requestedBody, `"direction":"Descending"`) {
		t.Fatalf("expected normalized enum casing, got %q", requestedBody)
	}
}

func TestSortChildrenCulturePassesThroughForDocuments(t *testing.T) {
	var requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusOK, `null`), nil
		}
	})

	if _, err := cmdtest.Execute(
		buildSortChildrenRoot(deps, "document", true),
		"document", "sort-children",
		"--field", "Name",
		"--culture", "da-DK",
	); err != nil {
		t.Fatalf("sort-children with culture failed: %v", err)
	}
	if !strings.Contains(requestedBody, `"culture":"da-DK"`) {
		t.Fatalf("expected culture in body, got %q", requestedBody)
	}
}

func TestSortChildrenRequiresField(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	_, err := cmdtest.Execute(buildSortChildrenRoot(deps, "document", true), "document", "sort-children")
	if err == nil || !strings.Contains(err.Error(), "--field is required") {
		t.Fatalf("expected required --field error, got %v", err)
	}
}

func TestSortChildrenRejectsUnknownField(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	_, err := cmdtest.Execute(buildSortChildrenRoot(deps, "document", true), "document", "sort-children", "--field", "SortOrder")
	if err == nil || !strings.Contains(err.Error(), "--field must be one of") {
		t.Fatalf("expected enum rejection, got %v", err)
	}
}

func TestSortChildrenMediaHasNoCultureFlag(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	_, err := cmdtest.Execute(buildSortChildrenRoot(deps, "media", false), "media", "sort-children", "--field", "Name", "--culture", "da-DK")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error for media --culture, got %v", err)
	}
}
