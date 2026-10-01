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

func buildBinRoot(deps cmdkit.Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterDocument(root, deps)
	RegisterMedia(root, deps)
	return root
}

func binDeps(handler func(req *http.Request) (*http.Response, error)) cmdkit.Dependencies {
	return cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestBinListReadsRecycleBinRoot(t *testing.T) {
	for _, resource := range []string{"document", "media"} {
		var requestedURI string
		deps := binDeps(func(req *http.Request) (*http.Response, error) {
			requestedURI = req.URL.RequestURI()
			return cmdtest.JSONResponse(http.StatusOK, `{"items":[{"id":"trashed-1","name":"Old Page"}],"total":1}`), nil
		})

		out, err := cmdtest.Execute(buildBinRoot(deps), resource, "bin", "list", "--take", "10")
		if err != nil {
			t.Fatalf("%s bin list failed: %v", resource, err)
		}
		if !strings.Contains(requestedURI, "/umbraco/management/api/v1/recycle-bin/"+resource+"/root") || !strings.Contains(requestedURI, "take=10") {
			t.Fatalf("%s: unexpected request URI %q", resource, requestedURI)
		}
		if !strings.Contains(out, "Old Page") {
			t.Fatalf("%s: expected trashed item in output, got %s", resource, out)
		}
	}
}

func TestBinChildrenPassesParentID(t *testing.T) {
	var requestedURI string
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return cmdtest.JSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := cmdtest.Execute(buildBinRoot(deps), "document", "bin", "children", "trashed-1"); err != nil {
		t.Fatalf("bin children failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/recycle-bin/document/children") || !strings.Contains(requestedURI, "parentId=trashed-1") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
}

func TestBinOriginalParentReads(t *testing.T) {
	var requestedPath string
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		requestedPath = req.URL.Path
		return cmdtest.JSONResponse(http.StatusOK, `{"id":"parent-1"}`), nil
	})

	out, err := cmdtest.Execute(buildBinRoot(deps), "media", "bin", "original-parent", "trashed-1")
	if err != nil {
		t.Fatalf("bin original-parent failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/recycle-bin/media/trashed-1/original-parent" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(out, "parent-1") {
		t.Fatalf("expected parent in output, got %s", out)
	}
}

func TestBinDeleteIsGated(t *testing.T) {
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})

	_, err := cmdtest.Execute(buildBinRoot(deps), "document", "bin", "delete", "trashed-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}
}

func TestBinEmptyIsGatedAndDeletes(t *testing.T) {
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})
	_, err := cmdtest.Execute(buildBinRoot(deps), "document", "bin", "empty")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}

	var requestedPath, requestedMethod string
	deps = binDeps(func(req *http.Request) (*http.Response, error) {
		requestedPath = req.URL.Path
		requestedMethod = req.Method
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	out, err := cmdtest.Execute(buildBinRoot(deps), "document", "bin", "empty", "--force")
	if err != nil {
		t.Fatalf("bin empty failed: %v", err)
	}
	if requestedMethod != http.MethodDelete || requestedPath != "/umbraco/management/api/v1/recycle-bin/document" {
		t.Fatalf("unexpected request %s %s", requestedMethod, requestedPath)
	}
	if !strings.Contains(out, `"emptied": true`) && !strings.Contains(out, `"emptied":true`) {
		t.Fatalf("expected emptied:true, got %s", out)
	}
}

func TestBinEmptyDryRunSkipsRequest(t *testing.T) {
	requests := 0
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		requests++
		return cmdtest.JSONResponse(http.StatusOK, `{}`), nil
	})

	out, err := cmdtest.Execute(buildBinRoot(deps), "media", "bin", "empty", "--dry-run")
	if err != nil {
		t.Fatalf("bin empty dry-run failed: %v", err)
	}
	if requests != 0 {
		t.Fatalf("dry-run must not hit the API, saw %d requests", requests)
	}
	if !strings.Contains(out, `"dryRun": true`) && !strings.Contains(out, `"dryRun":true`) {
		t.Fatalf("expected dry-run preview, got %s", out)
	}
}

func TestBinEmptyRejectsStrayArguments(t *testing.T) {
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for a rejected invocation")
		return nil, nil
	})

	// A stray ID must fail loudly: silently ignoring it would turn an
	// intended single-item delete into a full-bin wipe.
	_, err := cmdtest.Execute(buildBinRoot(deps), "document", "bin", "empty", "trashed-1", "--force")
	if err == nil || !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("expected stray argument rejection, got %v", err)
	}
}

func TestMediaRestoreDefaultsToOriginalParent(t *testing.T) {
	var requests []string
	var putBody string
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.Path)
		switch req.URL.Path {
		case "/umbraco/management/api/v1/recycle-bin/media/trashed-1/original-parent":
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"folder-9"}`), nil
		case "/umbraco/management/api/v1/recycle-bin/media/trashed-1/restore":
			body, _ := io.ReadAll(req.Body)
			putBody = string(body)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return cmdtest.JSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	out, err := cmdtest.Execute(buildBinRoot(deps), "media", "restore", "trashed-1")
	if err != nil {
		t.Fatalf("media restore failed: %v", err)
	}
	if !strings.Contains(putBody, `"folder-9"`) {
		t.Fatalf("expected original parent as restore target, got %s", putBody)
	}
	if !strings.Contains(out, `"restored": true`) && !strings.Contains(out, `"restored":true`) {
		t.Fatalf("expected restored:true, got %s", out)
	}
	if requests[len(requests)-1] != "PUT /umbraco/management/api/v1/recycle-bin/media/trashed-1/restore" {
		t.Fatalf("expected modern restore route, got %v", requests)
	}
}

func TestMediaRestoreToRootSkipsParentLookup(t *testing.T) {
	var putBody string
	lookups := 0
	deps := binDeps(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/original-parent") {
			lookups++
			return cmdtest.JSONResponse(http.StatusOK, `{"id":"should-not-be-used"}`), nil
		}
		body, _ := io.ReadAll(req.Body)
		putBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	if _, err := cmdtest.Execute(buildBinRoot(deps), "media", "restore", "trashed-1", "--to", "root"); err != nil {
		t.Fatalf("media restore --to root failed: %v", err)
	}
	if lookups != 0 {
		t.Fatalf("expected no original-parent lookup with --to root")
	}
	if !strings.Contains(putBody, `"target":null`) {
		t.Fatalf("expected null target for root restore, got %s", putBody)
	}
}
