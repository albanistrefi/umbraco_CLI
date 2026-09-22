package commands

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func buildSearcherRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterSearcher(root, deps)
	return root
}

func searcherDeps(handler func(req *http.Request) (*http.Response, error)) Dependencies {
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestSearcherListPaginates(t *testing.T) {
	var requestedURI string
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[{"name":"ExternalSearcher"}],"total":1}`), nil
	})

	out, err := execute(buildSearcherRoot(deps), "searcher", "list", "--skip", "5", "--take", "10")
	if err != nil {
		t.Fatalf("searcher list failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/umbraco/management/api/v1/searcher?") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
	if !strings.Contains(requestedURI, "skip=5") || !strings.Contains(requestedURI, "take=10") {
		t.Fatalf("expected pagination params, got %q", requestedURI)
	}
	if !strings.Contains(out, "ExternalSearcher") {
		t.Fatalf("expected searcher in output, got %s", out)
	}
}

func TestSearcherQuerySendsTermAndEscapesName(t *testing.T) {
	var requestedURI string
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"1","score":0.5,"fields":{"nodeName":"a"}}],"total":1}`), nil
	})

	if _, err := execute(buildSearcherRoot(deps), "searcher", "query", "My Searcher", "--term", "home page", "--skip", "2", "--take", "3"); err != nil {
		t.Fatalf("searcher query failed: %v", err)
	}
	if !strings.HasPrefix(requestedURI, "/umbraco/management/api/v1/searcher/My%20Searcher/query?") {
		t.Fatalf("expected escaped searcher name in path, got %q", requestedURI)
	}
	for _, want := range []string{"term=home+page", "skip=2", "take=3"} {
		if !strings.Contains(requestedURI, want) {
			t.Fatalf("expected %q in %q", want, requestedURI)
		}
	}
}

func TestSearcherQueryAcceptsQueryAsAliasOfTerm(t *testing.T) {
	var requestedURI string
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher", "--query", "home"); err != nil {
		t.Fatalf("searcher query --query failed: %v", err)
	}
	if !strings.Contains(requestedURI, "term=home") {
		t.Fatalf("expected --query to be sent as term, got %q", requestedURI)
	}
}

func TestSearcherQueryProjectsFields(t *testing.T) {
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"1","score":0.5,"fields":{"nodeName":"a"}}],"total":1}`), nil
	})

	out, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher", "--term", "home", "--fields", "id,score")
	if err != nil {
		t.Fatalf("searcher query --fields failed: %v", err)
	}
	if strings.Contains(out, "nodeName") {
		t.Fatalf("expected fields projection to drop unlisted keys, got %s", out)
	}
	if !strings.Contains(out, `"score"`) || !strings.Contains(out, `"id"`) {
		t.Fatalf("expected projected keys in output, got %s", out)
	}
}

func TestSearcherQueryParamsTermWinsOverFlag(t *testing.T) {
	var requestedURI string
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher", "--params", `{"term":"raw"}`, "--term", "x"); err != nil {
		t.Fatalf("searcher query with --params failed: %v", err)
	}
	if !strings.Contains(requestedURI, "term=raw") || strings.Contains(requestedURI, "term=x") {
		t.Fatalf("expected --params term to win over --term, got %q", requestedURI)
	}
}

func TestSearcherQueryAcceptsTermFromParamsAlone(t *testing.T) {
	var requestedURI string
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher", "--params", `{"term":"raw"}`); err != nil {
		t.Fatalf("searcher query with only a params term failed: %v", err)
	}
	if !strings.Contains(requestedURI, "term=raw") {
		t.Fatalf("expected params term to be sent, got %q", requestedURI)
	}
}

func TestSearcherQueryRequiresTerm(t *testing.T) {
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without a term")
		return nil, nil
	})

	_, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher")
	if err == nil || !strings.Contains(err.Error(), "--term") {
		t.Fatalf("expected missing term error, got %v", err)
	}
}

func TestSearcherQueryRejectsConflictingTermAndQuery(t *testing.T) {
	deps := searcherDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for conflicting aliases")
		return nil, nil
	})

	_, err := execute(buildSearcherRoot(deps), "searcher", "query", "ExternalSearcher", "--term", "a", "--query", "b")
	if err == nil || !strings.Contains(err.Error(), "aliases") {
		t.Fatalf("expected alias conflict error, got %v", err)
	}
}
