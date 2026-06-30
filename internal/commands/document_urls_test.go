package commands

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentURLsBatchesIDsAndPlainAbsoluteOutput(t *testing.T) {
	var observedIDs []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			observedIDs = req.URL.Query()["id"]
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[{"culture":"en-US","url":"/products/","provider":"umbDocumentUrlProvider","message":null}]},
				{"id":"doc-2","urlInfos":[{"culture":"da-DK","url":"/da/produkter/","provider":"umbDocumentUrlProvider","message":null}]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "doc-2", "--absolute", "-o", "plain")
	if err != nil {
		t.Fatalf("document urls failed: %v", err)
	}
	if !reflect.DeepEqual(observedIDs, []string{"doc-1", "doc-2"}) {
		t.Fatalf("expected repeated id query params, got %#v", observedIDs)
	}
	expected := "https://example.test/products/\nhttps://example.test/da/produkter/\n"
	if output != expected {
		t.Fatalf("unexpected plain output:\n%s", output)
	}
}

func TestDocumentURLsJSONPassthroughAndCultureFilter(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[
					{"culture":"en-US","url":"/products/","provider":"umbDocumentUrlProvider","message":null},
					{"culture":"da-DK","url":"/da/produkter/","provider":"umbDocumentUrlProvider","message":null}
				]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "--culture", "da-DK", "-o", "json")
	if err != nil {
		t.Fatalf("document urls failed: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	urlInfos := payload[0]["urlInfos"].([]any)
	if len(urlInfos) != 1 {
		t.Fatalf("expected one filtered URL info, got %+v", urlInfos)
	}
	info := urlInfos[0].(map[string]any)
	if info["culture"] != "da-DK" || info["url"] != "/da/produkter/" {
		t.Fatalf("unexpected filtered info: %+v", info)
	}
}

func TestDocumentURLsTableIncludesMessage(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[{"culture":"en-US","url":"","provider":"umbDocumentUrlProvider","message":"Document is not published"}]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "-o", "table")
	if !isDocumentURLsMissing(err) {
		t.Fatalf("expected missing URL error, got %v", err)
	}
	for _, expected := range []string{"id", "culture", "url", "provider", "message", "Document is not published"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected table output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestDocumentURLsAllowsNullURLAndPreservesMessage(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[{"culture":"en-US","url":null,"provider":"umbDocumentUrlProvider","message":"Document is not published"}]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "-o", "json")
	if !isDocumentURLsMissing(err) {
		t.Fatalf("expected missing URL error, got %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	info := payload[0]["urlInfos"].([]any)[0].(map[string]any)
	if info["url"] != nil {
		t.Fatalf("expected null url to be preserved, got %+v", info)
	}
	if info["message"] != "Document is not published" {
		t.Fatalf("expected message to be preserved, got %+v", info)
	}
}

func TestDocumentURLsMissingURLReturnsNonZeroWithPlainOutput(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[{"id":"doc-1","urlInfos":[]}]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "-o", "plain")
	if !isDocumentURLsMissing(err) {
		t.Fatalf("expected missing URL error, got %v", err)
	}
	if output != "" {
		t.Fatalf("expected no plain output for missing URL, got %q", output)
	}
}

func TestDocumentURLsMissingReturnedIDReturnsNonZero(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "document", "urls", "doc-1", "-o", "json")
	if !isDocumentURLsMissing(err) {
		t.Fatalf("expected missing URL error, got %v", err)
	}
}

func TestDocumentGetWithURLsAttachesBeforeFields(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Products","values":[]}`), nil
		case "/umbraco/management/api/v1/document/urls":
			if got := req.URL.Query()["id"]; !reflect.DeepEqual(got, []string{"doc-1"}) {
				t.Fatalf("unexpected url ids: %#v", got)
			}
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[{"culture":"en-US","url":"/products/","provider":"umbDocumentUrlProvider","message":null}]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-1", "--with-urls", "--fields", "id,urls", "-o", "json")
	if err != nil {
		t.Fatalf("document get --with-urls failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if _, ok := payload["name"]; ok {
		t.Fatalf("expected --fields to omit name, got %+v", payload)
	}
	urls := payload["urls"].([]any)
	if len(urls) != 1 || urls[0].(map[string]any)["url"] != "/products/" {
		t.Fatalf("expected attached urls, got %+v", payload)
	}
}

func TestDocumentGetWithURLsAllowsNullURL(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Products","values":[]}`), nil
		case "/umbraco/management/api/v1/document/urls":
			return endpointJSONResponse(http.StatusOK, `[
				{"id":"doc-1","urlInfos":[{"culture":"en-US","url":null,"provider":"umbDocumentUrlProvider","message":"Document is not published"}]}
			]`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-1", "--with-urls", "--fields", "id,urls", "-o", "json")
	if err != nil {
		t.Fatalf("document get --with-urls failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	info := payload["urls"].([]any)[0].(map[string]any)
	if info["url"] != nil || info["message"] != "Document is not published" {
		t.Fatalf("expected null URL and message to be attached, got %+v", info)
	}
}
