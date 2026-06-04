package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const mbTokenPath = "/umbraco/management/api/v1/security/back-office/token"

func TestModelsBuilderDashboardHitsCoreAPI(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"SourceCodeManual","outOfDateModels":true,"canGenerate":true}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "models-builder", "dashboard")
	if err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["mode"] != "SourceCodeManual" || payload["outOfDateModels"] != true {
		t.Fatalf("unexpected dashboard payload: %+v", payload)
	}
}

func TestModelsBuilderBuildRejectsNonSourceMode(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"InMemoryAuto","canGenerate":true}`), nil
		case "/umbraco/management/api/v1/models-builder/build":
			t.Fatalf("build POST must NOT fire when mode is not SourceCode*")
			return datatypeJSONResponse(http.StatusOK, `null`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "models-builder", "build")
	if err == nil {
		t.Fatalf("expected build to refuse non-source mode")
	}
	if !strings.Contains(err.Error(), "InMemoryAuto") || !strings.Contains(err.Error(), "SourceCode") {
		t.Fatalf("error should name the current mode and the required mode family, got: %v", err)
	}
}

func TestModelsBuilderBuildSurfacesCanGenerateFalseWithLastError(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"SourceCodeManual","canGenerate":false,"lastError":"compilation failed: x"}`), nil
		case "/umbraco/management/api/v1/models-builder/build":
			t.Fatalf("build POST must NOT fire when canGenerate=false")
			return datatypeJSONResponse(http.StatusOK, `null`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "models-builder", "build")
	if err == nil {
		t.Fatalf("expected canGenerate=false to abort the build")
	}
	if !strings.Contains(err.Error(), "compilation failed: x") {
		t.Fatalf("error should quote lastError, got: %v", err)
	}
}

func TestModelsBuilderBuildPostsWhenSourceModeAndCanGenerate(t *testing.T) {
	var postCount int32
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"SourceCodeManual","canGenerate":true}`), nil
		case "/umbraco/management/api/v1/models-builder/build":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			atomic.AddInt32(&postCount, 1)
			return datatypeJSONResponse(http.StatusOK, `{"triggered":true}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "models-builder", "build")
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if atomic.LoadInt32(&postCount) != 1 {
		t.Fatalf("expected exactly one build POST, got %d", postCount)
	}
	if !strings.Contains(output, "triggered") {
		t.Fatalf("expected server response in output, got %q", output)
	}
}

func TestModelsBuilderBuildWaitPollsStatusUntilCurrent(t *testing.T) {
	var pollCount int32
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"SourceCodeManual","canGenerate":true}`), nil
		case "/umbraco/management/api/v1/models-builder/build":
			return datatypeJSONResponse(http.StatusOK, `{"triggered":true}`), nil
		case "/umbraco/management/api/v1/models-builder/status":
			// First two polls return OutOfDate, third returns Current.
			n := atomic.AddInt32(&pollCount, 1)
			if n < 3 {
				return datatypeJSONResponse(http.StatusOK, `{"status":"OutOfDate"}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"status":"Current"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"models-builder", "build",
		"--wait",
		"--timeout", "5s",
		"--poll-interval", "10ms",
	)
	if err != nil {
		t.Fatalf("build --wait failed: %v", err)
	}
	if atomic.LoadInt32(&pollCount) < 3 {
		t.Fatalf("expected at least 3 polls before reaching Current, got %d", pollCount)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode wait result: %v", err)
	}
	if payload["status"] != "Current" {
		t.Fatalf("expected final status=Current, got %+v", payload)
	}
}

func TestModelsBuilderBuildWaitTimesOutWithLastStatus(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case mbTokenPath:
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/models-builder/dashboard":
			return datatypeJSONResponse(http.StatusOK, `{"mode":"SourceCodeAuto","canGenerate":true}`), nil
		case "/umbraco/management/api/v1/models-builder/build":
			return datatypeJSONResponse(http.StatusOK, `{"triggered":true}`), nil
		case "/umbraco/management/api/v1/models-builder/status":
			return datatypeJSONResponse(http.StatusOK, `{"status":"OutOfDate"}`), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"models-builder", "build",
		"--wait",
		"--timeout", "30ms",
		"--poll-interval", "10ms",
	)
	if err == nil {
		t.Fatalf("expected --wait to timeout when status never reaches Current")
	}
	if !strings.Contains(err.Error(), "did not reach Current") || !strings.Contains(err.Error(), "OutOfDate") {
		t.Fatalf("timeout error should quote the last status seen, got: %v", err)
	}
}
