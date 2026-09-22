package commands

import (
	"net/http"
	"strings"
	"testing"
)

func TestDoctypePropertyIsUsedResolvesAliasAndSendsQueryParams(t *testing.T) {
	var observed string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document-type/search":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"11111111-1111-1111-1111-111111111111"}]}`), nil
		case "/umbraco/management/api/v1/document-type/11111111-1111-1111-1111-111111111111":
			return endpointJSONResponse(http.StatusOK, `{"id":"11111111-1111-1111-1111-111111111111","alias":"blogPost"}`), nil
		case "/umbraco/management/api/v1/property-type/is-used":
			observed = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `true`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "doctype", "property-is-used", "blogPost", "--alias", "teaser")
	if err != nil {
		t.Fatalf("doctype property-is-used failed: %v", err)
	}
	if !strings.Contains(observed, "contentTypeId=11111111-1111-1111-1111-111111111111") || !strings.Contains(observed, "propertyAlias=teaser") {
		t.Fatalf("unexpected property-is-used request: %q", observed)
	}
	if strings.TrimSpace(output) != "true" {
		t.Fatalf("expected the bare boolean response to pass through, got %q", output)
	}
}

func TestDoctypePropertyIsUsedPassesGUIDsThroughAndRequiresAlias(t *testing.T) {
	var observed string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document-type/search":
			t.Fatalf("a GUID argument must not trigger an alias lookup")
			return nil, nil
		case "/umbraco/management/api/v1/property-type/is-used":
			observed = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `false`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "property-is-used", "22222222-2222-2222-2222-222222222222"); err == nil {
		t.Fatalf("expected doctype property-is-used to require --alias")
	}
	if _, err := execute(buildRootWithCollections(t, deps), "doctype", "property-is-used", "22222222-2222-2222-2222-222222222222", "--alias", "teaser"); err != nil {
		t.Fatalf("doctype property-is-used failed: %v", err)
	}
	if !strings.Contains(observed, "contentTypeId=22222222-2222-2222-2222-222222222222") {
		t.Fatalf("unexpected property-is-used request: %q", observed)
	}
}
