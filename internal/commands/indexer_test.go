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

func buildIndexerRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterIndexer(root, deps)
	return root
}

func indexerDeps(handler func(req *http.Request) (*http.Response, error)) cmdkit.Dependencies {
	return cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestIndexerListPaginates(t *testing.T) {
	var requestedURI string
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"name":"ExternalIndex","healthStatus":{"status":"Healthy"},"documentCount":42}],"total":1}`), nil
	})

	out, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "list", "--take", "10")
	if err != nil {
		t.Fatalf("indexer list failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/umbraco/management/api/v1/indexer?") || !strings.Contains(requestedURI, "take=10") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
	if !strings.Contains(out, "ExternalIndex") {
		t.Fatalf("expected index in output, got %s", out)
	}
}

func TestIndexerGetEscapesName(t *testing.T) {
	var requestedURI string
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return cmdtest.JSONResponse(http.StatusOK, `{"name":"My Index","healthStatus":{"status":"Healthy"}}`), nil
	})

	if _, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "get", "My Index"); err != nil {
		t.Fatalf("indexer get failed: %v", err)
	}
	if requestedURI != "/umbraco/management/api/v1/indexer/My%20Index" {
		t.Fatalf("expected escaped index name, got %q", requestedURI)
	}
}

func TestIndexerRebuildIsGated(t *testing.T) {
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "rebuild", "ExternalIndex")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}
}

func TestIndexerRebuildRejectsDryRunWithWait(t *testing.T) {
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for flag validation error")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "rebuild", "ExternalIndex", "--dry-run", "--wait")
	if err == nil || !strings.Contains(err.Error(), "--wait has nothing to poll for") {
		t.Fatalf("expected dry-run/wait conflict error, got %v", err)
	}
}

func TestIndexerRebuildWithWaitPollsUntilHealthy(t *testing.T) {
	var rebuildRequests, statusRequests int
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/rebuild"):
			rebuildRequests++
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			statusRequests++
			if statusRequests == 1 {
				return cmdtest.JSONResponse(http.StatusOK, `{"name":"ExternalIndex","healthStatus":{"status":"Rebuilding"}}`), nil
			}
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"ExternalIndex","healthStatus":{"status":"Healthy"}}`), nil
		}
	})

	out, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "rebuild", "ExternalIndex", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "5s")
	if err != nil {
		t.Fatalf("rebuild --wait failed: %v", err)
	}
	if rebuildRequests != 1 || statusRequests != 2 {
		t.Fatalf("expected 1 rebuild + 2 polls, got %d + %d", rebuildRequests, statusRequests)
	}
	if !strings.Contains(out, `"rebuilt": true`) && !strings.Contains(out, `"rebuilt":true`) {
		t.Fatalf("expected rebuilt:true after polling, got %s", out)
	}
	if !strings.Contains(out, "Healthy") {
		t.Fatalf("expected final status in output, got %s", out)
	}
}

func TestIndexerRebuildWaitTimesOut(t *testing.T) {
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/rebuild") {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return cmdtest.JSONResponse(http.StatusOK, `{"name":"ExternalIndex","healthStatus":{"status":"Rebuilding"}}`), nil
	})

	_, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "rebuild", "ExternalIndex", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "10ms")
	if err == nil || !strings.Contains(err.Error(), "did not leave Rebuilding") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestIndexerRebuildWaitFailsOnCorruptTerminalState(t *testing.T) {
	statusRequests := 0
	deps := indexerDeps(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/rebuild") {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		statusRequests++
		if statusRequests == 1 {
			return cmdtest.JSONResponse(http.StatusOK, `{"name":"ExternalIndex","healthStatus":{"status":"Rebuilding"}}`), nil
		}
		return cmdtest.JSONResponse(http.StatusOK, `{"name":"ExternalIndex","healthStatus":{"status":"Corrupt"}}`), nil
	})

	_, err := cmdtest.Execute(buildIndexerRoot(deps), "indexer", "rebuild", "ExternalIndex", "--force", "--wait", "--poll-interval", "1ms", "--timeout", "5s")
	if err == nil || !strings.Contains(err.Error(), "status Corrupt") {
		t.Fatalf("expected corrupt terminal state to fail, got %v", err)
	}
}
