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

func buildElementRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterElement(root, deps)
	return root
}

func TestElementListHitsTreeRoot(t *testing.T) {
	var requestedPath string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildElementRoot(deps), "element", "list"); err != nil {
		t.Fatalf("element list failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/tree/element/root" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
}

func TestElementCreatePublishTargetsCombinedEndpoint(t *testing.T) {
	var requestedPath, requestedBody string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			body, _ := io.ReadAll(req.Body)
			requestedBody = string(body)
			return cmdtest.JSONResponse(http.StatusCreated, `null`), nil
		}
	})

	if _, err := cmdtest.Execute(
		buildElementRoot(deps),
		"element", "create",
		"--json", `{"documentType":{"id":"et-1"},"variants":[{"name":"Test Element"}],"values":[]}`,
		"--publish",
	); err != nil {
		t.Fatalf("element create --publish failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/element/create-and-publish" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(requestedBody, `"culturesToPublish":[]`) {
		t.Fatalf("expected empty culturesToPublish for invariant content, got %q", requestedBody)
	}
}

func TestElementUpdateSaveAndPublishTargetsAtomicRoute(t *testing.T) {
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

	out, err := cmdtest.Execute(
		buildElementRoot(deps),
		"element", "update", "el-1",
		"--json", `{"values":[],"variants":[]}`,
		"--save-and-publish",
	)
	if err != nil {
		t.Fatalf("element update --save-and-publish failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/element/el-1/update-and-publish" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(requestedBody, `"culturesToPublish":[]`) {
		t.Fatalf("expected empty culturesToPublish, got %q", requestedBody)
	}
	if !strings.Contains(out, `"saveAndPublish": true`) {
		t.Fatalf("expected saveAndPublish marker, got %s", out)
	}
}

func TestElementPublishDefaultsToInvariantSchedule(t *testing.T) {
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

	if _, err := cmdtest.Execute(buildElementRoot(deps), "element", "publish", "el-1"); err != nil {
		t.Fatalf("element publish failed: %v", err)
	}
	if !strings.Contains(requestedBody, `"publishSchedules":[{"culture":null}]`) {
		t.Fatalf("expected invariant publish schedule, got %q", requestedBody)
	}
}

func TestElementVersionListPassesElementID(t *testing.T) {
	var requestedURI string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildElementRoot(deps), "element", "version", "list", "el-1"); err != nil {
		t.Fatalf("element version list failed: %v", err)
	}
	if !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/element-version?") || !strings.Contains(requestedURI, "elementId=el-1") {
		t.Fatalf("unexpected URI %q", requestedURI)
	}
}

func TestElementPublishCultureBuildsPublishSchedule(t *testing.T) {
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

	if _, err := cmdtest.Execute(buildElementRoot(deps), "element", "publish", "el-1", "--culture", "en-US"); err != nil {
		t.Fatalf("element publish --culture failed: %v", err)
	}
	if !strings.Contains(requestedBody, `"publishSchedules":[{"culture":"en-US"}]`) {
		t.Fatalf("expected publishSchedules entry for the culture, got %q", requestedBody)
	}
	if strings.Contains(requestedBody, `"cultures"`) {
		t.Fatalf("cultures belongs to the unpublish model, got %q", requestedBody)
	}
}

func TestElementUpdateCultureRequiresSaveAndPublish(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	_, err := cmdtest.Execute(buildElementRoot(deps), "element", "update", "el-1", "--json", `{"values":[],"variants":[]}`, "--culture", "en-US")
	if err == nil || !strings.Contains(err.Error(), "--culture requires --save-and-publish") {
		t.Fatalf("expected culture guard, got %v", err)
	}
}

func TestElementAreReferencedRequestsAllIDs(t *testing.T) {
	var requestedURI string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
	})

	if _, err := cmdtest.Execute(buildElementRoot(deps), "element", "are-referenced", "--ids", "a,b,c"); err != nil {
		t.Fatalf("element are-referenced failed: %v", err)
	}
	if !strings.Contains(requestedURI, "take=3") {
		t.Fatalf("expected take sized to the id count, got %q", requestedURI)
	}
}

func TestElementBinDeleteIsForceGated(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	})

	_, err := cmdtest.Execute(buildElementRoot(deps), "element", "bin", "delete", "el-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force gate on bin delete, got %v", err)
	}
}
