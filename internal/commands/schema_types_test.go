package commands

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func buildSchemaTypeRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterMediaType(root, deps)
	RegisterMemberType(root, deps)
	return root
}

func schemaTypeDeps(handler func(req *http.Request) (*http.Response, error)) Dependencies {
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(req)
	})
}

func TestSchemaTypeListRecursiveFlattensFoldersForBothGroups(t *testing.T) {
	for _, tc := range []struct {
		group    string
		resource string
	}{
		{group: "mediatype", resource: "media-type"},
		{group: "membertype", resource: "member-type"},
	} {
		deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Path == "/umbraco/management/api/v1/tree/"+tc.resource+"/root":
				return endpointJSONResponse(http.StatusOK, `{"items":[
					{"id":"folder-1","name":"Compositions","isFolder":true},
					{"id":"type-1","name":"Image","alias":"image","isFolder":false}
				],"total":2}`), nil
			case req.URL.Path == "/umbraco/management/api/v1/tree/"+tc.resource+"/children" && req.URL.Query().Get("parentId") == "folder-1":
				return endpointJSONResponse(http.StatusOK, `{"items":[
					{"id":"type-2","name":"Nested Type","alias":"nestedType","isFolder":false}
				],"total":1}`), nil
			default:
				return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		})

		out, err := execute(buildSchemaTypeRoot(deps), tc.group, "list", "--recursive", "--types-only")
		if err != nil {
			t.Fatalf("%s list --recursive failed: %v", tc.group, err)
		}
		if !strings.Contains(out, "nestedType") || !strings.Contains(out, `"image"`) {
			t.Fatalf("%s: expected nested and root types in output, got %s", tc.group, out)
		}
		if strings.Contains(out, "Compositions") {
			t.Fatalf("%s: expected folders excluded with --types-only, got %s", tc.group, out)
		}
	}
}

func TestSchemaTypeGetExplainsFolderIDs(t *testing.T) {
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/media-type/f0f0f0f0-0000-4000-8000-000000000001", "/umbraco/management/api/v1/media-type/f0f0f0f0-0000-4000-8000-000000000002":
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		case "/umbraco/management/api/v1/media-type/folder/f0f0f0f0-0000-4000-8000-000000000001":
			return endpointJSONResponse(http.StatusOK, `{"id":"f0f0f0f0-0000-4000-8000-000000000001","name":"Icons"}`), nil
		case "/umbraco/management/api/v1/tree/media-type/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	_, err := execute(buildSchemaTypeRoot(deps), "mediatype", "get", "f0f0f0f0-0000-4000-8000-000000000001")
	if err == nil || !strings.Contains(err.Error(), "is a folder, not a media type") {
		t.Fatalf("expected folder explanation, got %v", err)
	}

	_, err = execute(buildSchemaTypeRoot(deps), "mediatype", "get", "f0f0f0f0-0000-4000-8000-000000000002")
	if err == nil || strings.Contains(err.Error(), "is a folder") {
		t.Fatalf("expected the real API error for a missing media type, got %v", err)
	}
}

func TestSchemaTypeCreateAndUpdateHitResourceEndpoints(t *testing.T) {
	var requests []string
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.Path)
		return endpointJSONResponse(http.StatusOK, `{"id":"mt-1","name":"Docs"}`), nil
	})

	root := buildSchemaTypeRoot(deps)
	if _, err := execute(root, "membertype", "create", "--json", `{"name":"Docs","alias":"docs"}`); err != nil {
		t.Fatalf("membertype create failed: %v", err)
	}
	if _, err := execute(root, "membertype", "update", "mt-1", "--json", `{"name":"Docs2","alias":"docs"}`); err != nil {
		t.Fatalf("membertype update failed: %v", err)
	}
	expected := []string{
		"POST /umbraco/management/api/v1/member-type",
		"PUT /umbraco/management/api/v1/member-type/mt-1",
	}
	for _, want := range expected {
		found := false
		for _, got := range requests {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected request %q, got %v", want, requests)
		}
	}
}

func TestSchemaTypeDeleteIsGated(t *testing.T) {
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected without --force or --dry-run")
		return nil, nil
	})

	_, err := execute(buildSchemaTypeRoot(deps), "mediatype", "delete", "mt-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force/dry-run gate, got %v", err)
	}
}

func TestSchemaTypeExportReadsUDT(t *testing.T) {
	var requestedPath string
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		requestedPath = req.URL.Path
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       io.NopCloser(strings.NewReader(`<umbPackage><MediaTypes/></umbPackage>`)),
		}, nil
	})

	out, err := execute(buildSchemaTypeRoot(deps), "mediatype", "export", "mt-1")
	if err != nil {
		t.Fatalf("mediatype export failed: %v", err)
	}
	if requestedPath != "/umbraco/management/api/v1/media-type/mt-1/export" {
		t.Fatalf("unexpected path %q", requestedPath)
	}
	if !strings.Contains(out, "umbPackage") {
		t.Fatalf("expected exported document in output, got %s", out)
	}
}

func TestSchemaTypeSearchUsesItemEndpoint(t *testing.T) {
	var requestedURI string
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
	})

	if _, err := execute(buildSchemaTypeRoot(deps), "mediatype", "search", "--query", "image"); err != nil {
		t.Fatalf("mediatype search failed: %v", err)
	}
	if !strings.Contains(requestedURI, "/item/media-type/search") || !strings.Contains(requestedURI, "query=image") {
		t.Fatalf("unexpected request URI %q", requestedURI)
	}
}

func TestSchemaTypeMergeUpdateStripsReadOnlyFields(t *testing.T) {
	var putBody string
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return endpointJSONResponse(http.StatusOK, `{"id":"mt-1","name":"Image","alias":"image","isDeletable":false,"aliasCanBeChanged":true,"allowedAsRoot":true}`), nil
		case http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			putBody = string(body)
			return endpointJSONResponse(http.StatusOK, `{}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})

	if _, err := execute(buildSchemaTypeRoot(deps), "mediatype", "update", "mt-1", "--merge-json", `{"name":"Image v2"}`); err != nil {
		t.Fatalf("mediatype merge update failed: %v", err)
	}
	// The update request model rejects response-only fields
	// (additionalProperties: false), so the merged PUT must not echo them.
	for _, rejected := range []string{`"id"`, `"isDeletable"`, `"aliasCanBeChanged"`} {
		if strings.Contains(putBody, rejected) {
			t.Fatalf("expected %s stripped from merged PUT body, got %s", rejected, putBody)
		}
	}
	if !strings.Contains(putBody, `"Image v2"`) || !strings.Contains(putBody, `"allowedAsRoot":true`) {
		t.Fatalf("expected merged fields preserved, got %s", putBody)
	}
}

func TestSchemaTypeGetResolvesAliasOrExplains(t *testing.T) {
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/item/media-type/search":
			if strings.EqualFold(req.URL.Query().Get("query"), "hero") {
				return endpointJSONResponse(http.StatusOK, `{"total":2,"items":[{"id":"aaaaaaaa-0000-4000-8000-000000000001","name":"Hero Image"},{"id":"aaaaaaaa-0000-4000-8000-000000000002","name":"Hero Image Legacy"}]}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
		case "/umbraco/management/api/v1/media-type/aaaaaaaa-0000-4000-8000-000000000001":
			return endpointJSONResponse(http.StatusOK, `{"id":"aaaaaaaa-0000-4000-8000-000000000001","alias":"heroImage","name":"Hero Image"}`), nil
		case "/umbraco/management/api/v1/media-type/aaaaaaaa-0000-4000-8000-000000000002":
			return endpointJSONResponse(http.StatusOK, `{"id":"aaaaaaaa-0000-4000-8000-000000000002","alias":"heroImageLegacy","name":"Hero Image Legacy"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	out, err := execute(buildSchemaTypeRoot(deps), "mediatype", "get", "HeroImage")
	if err != nil || !strings.Contains(out, `"alias": "heroImage"`) {
		t.Fatalf("expected alias resolution (case-insensitive, exact), got err=%v out=%s", err, out)
	}
	_, err = execute(buildSchemaTypeRoot(deps), "mediatype", "get", "someAliasThatDoesNotExist123")
	if err == nil || !strings.Contains(err.Error(), "not a GUID and no media type has that alias") || strings.Contains(err.Error(), "Umbraco version") {
		t.Fatalf("expected a clear alias-not-found error, got %v", err)
	}
}

func TestSchemaTypeAliasFallsBackToTreeAndUsesBatchAndPropagatesErrors(t *testing.T) {
	var batchIDs []string
	deps := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/item/media-type/search":
			return endpointJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil // renamed type: name no longer matches the alias
		case "/umbraco/management/api/v1/tree/media-type/root":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"aaaaaaaa-0000-4000-8000-000000000009","name":"News Article","alias":"x","isFolder":false}]}`), nil
		case "/umbraco/management/api/v1/media-type/batch":
			batchIDs = req.URL.Query()["id"]
			return endpointJSONResponse(http.StatusOK, `[{"id":"aaaaaaaa-0000-4000-8000-000000000009","alias":"blogPost","name":"News Article"}]`), nil
		case "/umbraco/management/api/v1/media-type/aaaaaaaa-0000-4000-8000-000000000009":
			return endpointJSONResponse(http.StatusOK, `{"id":"aaaaaaaa-0000-4000-8000-000000000009","alias":"blogPost","name":"News Article"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	out, err := execute(buildSchemaTypeRoot(deps), "mediatype", "get", "blogPost")
	if err != nil || !strings.Contains(out, `"alias": "blogPost"`) {
		t.Fatalf("expected tree fallback to resolve a renamed type, got err=%v out=%s", err, out)
	}
	if len(batchIDs) != 1 || batchIDs[0] != "aaaaaaaa-0000-4000-8000-000000000009" {
		t.Fatalf("expected batch ids as repeated query values, got %v", batchIDs)
	}

	failing := schemaTypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/item/media-type/search":
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"aaaaaaaa-0000-4000-8000-000000000009"}]}`), nil
		case "/umbraco/management/api/v1/media-type/batch":
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/media-type/aaaaaaaa-0000-4000-8000-000000000009":
			return endpointJSONResponse(http.StatusForbidden, `{"title":"Forbidden"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	_, err = execute(buildSchemaTypeRoot(failing), "mediatype", "get", "blogPost")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "no media type has that alias") {
		t.Fatalf("expected the API failure to propagate rather than a not-found, got %v", err)
	}
}
