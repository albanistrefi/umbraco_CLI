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

func buildPublishedCacheRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterPublishedCache(root, deps)
	return root
}

func TestPublishedCacheStatusFallsBackToLegacyRoute(t *testing.T) {
	var requests []string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/published-cache/rebuild/status":
			requests = append(requests, req.URL.Path)
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		case "/umbraco/management/api/v1/published-cache/status":
			requests = append(requests, req.URL.Path)
			return cmdtest.JSONResponse(http.StatusOK, `"cache is ok"`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	out, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "status")
	if err != nil {
		t.Fatalf("published-cache status failed: %v", err)
	}
	if len(requests) != 2 || requests[0] != "/umbraco/management/api/v1/published-cache/rebuild/status" {
		t.Fatalf("expected modern-first fallback, got %v", requests)
	}
	if !strings.Contains(out, "cache is ok") {
		t.Fatalf("expected status payload, got %s", out)
	}
}

func TestPublishedCacheRebuildRequiresForceOrDryRun(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}
}

func TestPublishedCacheRebuildPostsWithForce(t *testing.T) {
	var requestedPath, requestedMethod string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			requestedMethod = req.Method
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
	})

	out, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--force")
	if err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}
	if requestedMethod != http.MethodPost || requestedPath != "/umbraco/management/api/v1/published-cache/rebuild" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(out, `"rebuilding": true`) && !strings.Contains(out, `"rebuilding":true`) {
		t.Fatalf("expected empty 200 reported as rebuilding:true, got %s", out)
	}
}

func TestPublishedCacheRebuildDryRunSkipsRequest(t *testing.T) {
	requests := 0
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		requests++
		return cmdtest.JSONResponse(http.StatusOK, `{}`), nil
	})

	out, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--dry-run")
	if err != nil {
		t.Fatalf("rebuild dry-run failed: %v", err)
	}
	if requests != 0 {
		t.Fatalf("dry-run must not hit the API, saw %d requests", requests)
	}
	if !strings.Contains(out, `"dryRun": true`) && !strings.Contains(out, `"dryRun":true`) {
		t.Fatalf("expected dry-run preview, got %s", out)
	}
}

func TestPublishedCacheReloadPosts(t *testing.T) {
	var requestedPath, requestedMethod string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			requestedPath = req.URL.Path
			requestedMethod = req.Method
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
	})

	out, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "reload")
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if requestedMethod != http.MethodPost || requestedPath != "/umbraco/management/api/v1/published-cache/reload" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(out, `"reloaded": true`) && !strings.Contains(out, `"reloaded":true`) {
		t.Fatalf("expected empty 200 reported as reloaded:true, got %s", out)
	}
}

func TestPublishedCacheRebuildWithWaitPollsUntilDone(t *testing.T) {
	var rebuilds, statusPolls int
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/published-cache/rebuild":
			rebuilds++
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "/umbraco/management/api/v1/published-cache/rebuild/status":
			statusPolls++
			if statusPolls == 1 {
				return cmdtest.JSONResponse(http.StatusOK, `{"isRebuilding":true}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"isRebuilding":false}`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	out, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "5s")
	if err != nil {
		t.Fatalf("rebuild --wait failed: %v", err)
	}
	if rebuilds != 1 || statusPolls != 2 {
		t.Fatalf("expected 1 rebuild + 2 status polls, got %d + %d", rebuilds, statusPolls)
	}
	if !strings.Contains(out, `"rebuilt": true`) && !strings.Contains(out, `"rebuilt":true`) {
		t.Fatalf("expected rebuilt:true after polling, got %s", out)
	}
}

func TestPublishedCacheRebuildWaitTimesOut(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/published-cache/rebuild":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return cmdtest.JSONResponse(http.StatusOK, `{"isRebuilding":true}`), nil
		}
	})

	_, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "10ms")
	if err == nil || !strings.Contains(err.Error(), "still rebuilding") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPublishedCacheRebuildRejectsDryRunWithWait(t *testing.T) {
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for flag validation error")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--dry-run", "--wait")
	if err == nil || !strings.Contains(err.Error(), "--wait has nothing to poll for") {
		t.Fatalf("expected dry-run/wait conflict error, got %v", err)
	}
}

func TestPublishedCacheRebuildWaitFallsBackToLegacyStatusRoute(t *testing.T) {
	var statusRequests []string
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/published-cache/rebuild":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "/umbraco/management/api/v1/published-cache/rebuild/status":
			statusRequests = append(statusRequests, req.URL.Path)
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		case "/umbraco/management/api/v1/published-cache/status":
			statusRequests = append(statusRequests, req.URL.Path)
			return cmdtest.JSONResponse(http.StatusOK, `"cache is ok"`), nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	_, err := cmdtest.Execute(buildPublishedCacheRoot(deps), "published-cache", "rebuild", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "5s")
	// The legacy payload has no isRebuilding flag: fail fast with a clear
	// message instead of burning the timeout, but only after the fallback
	// route was actually tried.
	if err == nil || !strings.Contains(err.Error(), "does not expose the isRebuilding flag") {
		t.Fatalf("expected clear wait-unsupported error, got %v", err)
	}
	if len(statusRequests) != 2 || statusRequests[1] != "/umbraco/management/api/v1/published-cache/status" {
		t.Fatalf("expected modern-then-legacy status polling, got %v", statusRequests)
	}
}
