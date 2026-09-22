package commands

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func buildRelationRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterRelation(root, deps)
	return root
}

func relationDeps(handler func(req *http.Request) (*http.Response, error)) Dependencies {
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestRelationTypeListPaginates(t *testing.T) {
	var requestedURI string
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"type-1","alias":"relateDocumentOnCopy"}],"total":1}`), nil
	})

	out, err := execute(buildRelationRoot(deps), "relation", "type", "list", "--take", "10")
	if err != nil {
		t.Fatalf("relation type list failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/umbraco/management/api/v1/relation-type?") || !strings.Contains(requestedURI, "take=10") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
	if !strings.Contains(out, "relateDocumentOnCopy") {
		t.Fatalf("expected relation type in output, got %s", out)
	}
}

func TestRelationTypeGetProjectsFields(t *testing.T) {
	var requestedURI string
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"id":"type-1","alias":"relateDocumentOnCopy","isBidirectional":true}`), nil
	})

	out, err := execute(buildRelationRoot(deps), "relation", "type", "get", "type-1", "--fields", "alias")
	if err != nil {
		t.Fatalf("relation type get failed: %v", err)
	}
	if requestedURI != "/umbraco/management/api/v1/relation-type/type-1?fields=alias" && !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/relation-type/type-1") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
	if strings.Contains(out, "isBidirectional") {
		t.Fatalf("expected fields projection to drop unlisted keys, got %s", out)
	}
}

func TestRelationTypeItemsSendsRepeatedIDs(t *testing.T) {
	var requestedURI string
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `[{"id":"type-1","name":"Relate Document On Copy"}]`), nil
	})

	if _, err := execute(buildRelationRoot(deps), "relation", "type", "items", "--ids", "type-1,type-2"); err != nil {
		t.Fatalf("relation type items failed: %v", err)
	}
	if !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/item/relation-type?") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
	if !strings.Contains(requestedURI, "id=type-1") || !strings.Contains(requestedURI, "id=type-2") {
		t.Fatalf("expected both ids as repeated params, got %q", requestedURI)
	}
}

func TestRelationTypeItemsRequiresIDs(t *testing.T) {
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --ids")
		return nil, nil
	})

	_, err := execute(buildRelationRoot(deps), "relation", "type", "items")
	if err == nil || !strings.Contains(err.Error(), "--ids") {
		t.Fatalf("expected missing ids error, got %v", err)
	}
}

func TestRelationListRequiresType(t *testing.T) {
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --type")
		return nil, nil
	})

	_, err := execute(buildRelationRoot(deps), "relation", "list")
	if err == nil || !strings.Contains(err.Error(), "--type") {
		t.Fatalf("expected missing type error, got %v", err)
	}
}

func TestRelationListUsesRelationTypeRoute(t *testing.T) {
	var requestedURI string
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"rel-1","parent":{"id":"p"},"child":{"id":"c"}}],"total":1}`), nil
	})

	out, err := execute(buildRelationRoot(deps), "relation", "list", "--type", "type 1", "--skip", "1", "--take", "2")
	if err != nil {
		t.Fatalf("relation list failed: %v", err)
	}
	if !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/relation/type/type%201?") {
		t.Fatalf("expected escaped relation type id in path, got %q", requestedURI)
	}
	if !strings.Contains(requestedURI, "skip=1") || !strings.Contains(requestedURI, "take=2") {
		t.Fatalf("expected pagination params, got %q", requestedURI)
	}
	if !strings.Contains(out, "rel-1") {
		t.Fatalf("expected relation row in output, got %s", out)
	}
}

func TestRelationListAllFollowsPages(t *testing.T) {
	requests := 0
	deps := relationDeps(func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.URL.Query().Get("skip") {
		case "", "0":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"rel-1"},{"id":"rel-2"}],"total":4}`), nil
		case "2":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"rel-3"},{"id":"rel-4"}],"total":4}`), nil
		default:
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":4}`), nil
		}
	})

	out, err := execute(buildRelationRoot(deps), "relation", "list", "--type", "type-1", "--take", "2", "--all")
	if err != nil {
		t.Fatalf("relation list --all failed: %v", err)
	}
	if requests < 2 {
		t.Fatalf("expected --all to fetch more than one page, got %d requests", requests)
	}
	for _, want := range []string{"rel-1", "rel-4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in aggregated output, got %s", want, out)
		}
	}
}
