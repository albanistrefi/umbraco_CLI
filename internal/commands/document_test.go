package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

// endpointNoContent simulates a 204 No Content reply (the shape Umbraco
// returns for successful document update / publish PUTs). The HTTP client's
// parseResponse maps an empty body to nil, which is what reaches the
// command layer.
func endpointNoContent() *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func TestDocumentSearchUsesItemSearchEndpointAndFallsBack(t *testing.T) {
	var requests []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/document/search":
			requests = append(requests, req.URL.String())
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1","name":"Toxic"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "search", "--query", "Toxic", "--skip", "0", "--take", "25")
	if err != nil {
		t.Fatalf("document search failed: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 search attempts, got %+v", requests)
	}
	if !strings.Contains(requests[0], "/item/document/search") || !strings.Contains(requests[0], "query=Toxic") {
		t.Fatalf("unexpected primary document search request: %q", requests[0])
	}
	if !strings.Contains(requests[1], "/document/search") {
		t.Fatalf("unexpected fallback document search request: %q", requests[1])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode document search payload: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected document search payload, got %+v", payload)
	}
}

func TestDocumentGrepFindsBuriedSubstringThatSearchMisses(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1"},{"id":"doc-2"}],"total":2}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{
				"id":"doc-1",
				"name":"Audit Target",
				"documentType":{"alias":"article"},
				"values":[{"alias":"body","value":{"blocks":[{"content":{"url":"/compare_reports","label":"Deep"}}]}}]
			}`), nil
		case "/umbraco/management/api/v1/document/doc-2":
			return endpointJSONResponse(http.StatusOK, `{
				"id":"doc-2",
				"name":"No Match",
				"documentType":{"alias":"article"},
				"values":[{"alias":"body","value":"ordinary body"}]
			}`), nil
		case "/umbraco/management/api/v1/document/doc-1/published", "/umbraco/management/api/v1/document/doc-2/published":
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	searchOutput, err := execute(buildRootWithCollections(t, deps), "document", "search", "--query", "compare_reports")
	if err != nil {
		t.Fatalf("document search failed: %v", err)
	}
	var searchPayload map[string]any
	if err := json.Unmarshal([]byte(searchOutput), &searchPayload); err != nil {
		t.Fatalf("failed to decode search payload: %v", err)
	}
	if got := len(searchPayload["items"].([]any)); got != 0 {
		t.Fatalf("fixture search should miss buried content, got %d items", got)
	}

	grepOutput, err := execute(buildRootWithCollections(t, deps), "document", "grep", "compare_reports", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	var grepPayload documentGrepResult
	if err := json.Unmarshal([]byte(grepOutput), &grepPayload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if grepPayload.DocumentsWalked != 2 || grepPayload.DocumentsFetched != 2 || grepPayload.DocumentsMatched != 1 {
		t.Fatalf("unexpected grep counts: %+v", grepPayload)
	}
	if len(grepPayload.Hits) != 1 {
		t.Fatalf("expected one buried hit, got %+v", grepPayload.Hits)
	}
	hit := grepPayload.Hits[0]
	if hit.DocumentID != "doc-1" || hit.PropertyAlias != "body" || !strings.Contains(hit.Snippet, "compare_reports") {
		t.Fatalf("unexpected grep hit: %+v", hit)
	}
}

func TestDocumentGrepSupportsStartIDRegexIgnoreCaseAndFilters(t *testing.T) {
	var sawRoot bool
	var sawStartChildQuery bool
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			sawRoot = true
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			if req.URL.Query().Get("parentId") == "root-1" {
				sawStartChildQuery = true
				return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"child-1"}],"total":1}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/root-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"root-1","name":"Root","documentType":{"id":"dt-root"},"values":[{"alias":"body","value":"root has G2.COM but wrong doctype"}]}`), nil
		case "/umbraco/management/api/v1/document/child-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"child-1","variants":[{"name":"Child"}],"documentType":{"id":"dt-article"},"values":[{"alias":"body","value":"Visit G2.COM now"},{"alias":"summary","value":"G2.COM ignored by property filter"}]}`), nil
		case "/umbraco/management/api/v1/document-type/dt-root":
			return endpointJSONResponse(http.StatusOK, `{"id":"dt-root","alias":"landingPage"}`), nil
		case "/umbraco/management/api/v1/document-type/dt-article":
			return endpointJSONResponse(http.StatusOK, `{"id":"dt-article","alias":"article"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"document", "grep", `g2\.com`,
		"--regex",
		"--ignore-case",
		"--property", "body",
		"--doctype", "article",
		"--start-id", "root-1",
		"--draft",
		"--concurrency", "1",
	)
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	if sawRoot {
		t.Fatalf("--start-id should not fetch tree root")
	}
	if !sawStartChildQuery {
		t.Fatalf("--start-id should fetch children for the requested subtree root")
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if payload.Mode != "draft" || payload.StartID != "root-1" {
		t.Fatalf("unexpected mode/start metadata: %+v", payload)
	}
	if len(payload.Hits) != 1 {
		t.Fatalf("expected one filtered regex hit, got %+v", payload.Hits)
	}
	hit := payload.Hits[0]
	if hit.DocumentID != "child-1" || hit.DocumentName != "Child" || hit.DocumentTypeAlias != "article" || hit.PropertyAlias != "body" || hit.Match != "G2.COM" {
		t.Fatalf("unexpected filtered hit: %+v", hit)
	}
}

func TestDocumentGrepCaseSensitiveByDefault(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1"}],"total":1}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Case","values":[{"alias":"body","value":"G2.COM"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "grep", "g2.com", "--draft", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if len(payload.Hits) != 0 {
		t.Fatalf("case-sensitive search should not match uppercase text, got %+v", payload.Hits)
	}
}

func TestDocumentGrepIgnoreCaseKeepsUnicodeMatchIndexesOnOriginalText(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1"}],"total":1}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Unicode","values":[{"alias":"body","value":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA Kfoo suffix"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "grep", "kfoo", "--ignore-case", "--draft", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if len(payload.Hits) != 1 {
		t.Fatalf("expected one unicode case-folded hit, got %+v", payload.Hits)
	}
	hit := payload.Hits[0]
	if hit.Match != "Kfoo" {
		t.Fatalf("expected original unicode substring as match, got %q", hit.Match)
	}
	if !utf8.ValidString(hit.Snippet) {
		t.Fatalf("snippet should remain valid UTF-8, got %q", hit.Snippet)
	}
	if !strings.Contains(hit.Snippet, "Kfoo") {
		t.Fatalf("snippet should include original unicode match, got %q", hit.Snippet)
	}
}

func TestDocumentGrepPreservesExactWhitespaceNeedle(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1"}],"total":1}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Spacing","values":[{"alias":"body","value":"prefix compare_reports suffix"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "grep", " compare_reports ", "--draft", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if len(payload.Hits) != 1 || payload.Hits[0].Match != " compare_reports " {
		t.Fatalf("expected exact whitespace match, got %+v", payload.Hits)
	}
}

func TestDocumentGrepReportsSkippedFetchesAndKeepsProgressOnStderr(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-ok"},{"id":"doc-bad"}],"total":2}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-ok":
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-ok","name":"OK","values":[{"alias":"body","value":"compare_reports"}]}`), nil
		case "/umbraco/management/api/v1/document/doc-bad":
			return endpointJSONResponse(http.StatusInternalServerError, `{"title":"boom"}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	stdout, stderr, err := executeWithErr(buildRootWithCollections(t, deps), "document", "grep", "compare_reports", "--draft", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	if strings.Contains(stdout, "document grep:") {
		t.Fatalf("progress leaked into stdout JSON: %q", stdout)
	}
	if !strings.Contains(stderr, "document grep: walked=") {
		t.Fatalf("expected progress on stderr, got %q", stderr)
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout must remain parseable JSON: %v\n%s", err, stdout)
	}
	if len(payload.Hits) != 1 {
		t.Fatalf("expected successful doc hit, got %+v", payload.Hits)
	}
	if len(payload.Skipped) != 1 || payload.Skipped[0].ID != "doc-bad" || payload.Skipped[0].Stage != "draft" {
		t.Fatalf("expected skipped fetch for doc-bad, got %+v", payload.Skipped)
	}
}

func TestDocumentGrepPublishedScansPublishedEndpointOnly(t *testing.T) {
	var draftFetched bool
	var publishedFetched bool
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1"}],"total":1}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			draftFetched = true
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","values":[{"alias":"body","value":"draft-only"}]}`), nil
		case "/umbraco/management/api/v1/document/doc-1/published":
			publishedFetched = true
			return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","name":"Published","values":[{"alias":"body","value":"published-only"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "grep", "published-only", "--published", "--concurrency", "1")
	if err != nil {
		t.Fatalf("document grep failed: %v", err)
	}
	if draftFetched {
		t.Fatalf("--published should not fetch the draft endpoint")
	}
	if !publishedFetched {
		t.Fatalf("--published should fetch the published endpoint")
	}
	var payload documentGrepResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode grep payload: %v", err)
	}
	if payload.Mode != "published" || len(payload.Hits) != 1 || payload.Hits[0].State != "published" {
		t.Fatalf("unexpected published grep payload: %+v", payload)
	}
}

func TestDocumentGetJSONOutputEscapesControlCharacters(t *testing.T) {
	controlString := allJSONControlCharacters() + `"\\emoji: 😀`
	apiPayload := map[string]any{
		"id":   "doc-control",
		"name": "Control Characters",
		"values": []any{
			map[string]any{
				"alias": "richText",
				"value": controlString,
			},
		},
	}
	apiBody, err := json.Marshal(apiPayload)
	if err != nil {
		t.Fatalf("failed to encode API fixture: %v", err)
	}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-control":
			return endpointJSONResponse(http.StatusOK, string(apiBody)), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-control", "-o", "json")
	if err != nil {
		t.Fatalf("document get failed: %v", err)
	}

	if !json.Valid([]byte(output)) {
		t.Fatalf("document get -o json emitted invalid JSON: %q", output)
	}
	assertNoRawControlCharactersInJSONStringTokens(t, output)
	assertStrictJSONParsersAccept(t, output)

	var fromCLI map[string]any
	if err := json.Unmarshal([]byte(output), &fromCLI); err != nil {
		t.Fatalf("failed to decode CLI output: %v", err)
	}
	var fromAPI map[string]any
	if err := json.Unmarshal(apiBody, &fromAPI); err != nil {
		t.Fatalf("failed to decode API fixture: %v", err)
	}
	if !reflect.DeepEqual(fromCLI, fromAPI) {
		t.Fatalf("CLI output changed API semantics:\nCLI: %+v\nAPI: %+v", fromCLI, fromAPI)
	}
}

func TestDocumentGetTrimsFieldsSummaryNoEmptyAndWarnsUnknownFields(t *testing.T) {
	var observedPath string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			observedPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{
				"id":"doc-1",
				"name":"",
				"variants":[{"name":"Home"}],
				"documentType":{"id":"dt-1","alias":"","icon":null},
				"route":{"path":"/"},
				"updateDate":"",
				"values":[
					{"alias":"bodyText","value":"Welcome"},
					{"alias":"empty","value":""}
				],
				"big":{"nested":true}
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	stdout, stderr, err := executeWithErr(buildRootWithCollections(t, deps),
		"document", "get", "doc-1",
		"--summary",
		"--fields", "values.bodyText,missing",
		"--no-empty",
		"-o", "json",
	)
	if err != nil {
		t.Fatalf("document get trim failed: %v", err)
	}
	if strings.Contains(observedPath, "fields=") {
		t.Fatalf("expected --fields to stay client-side, got %q", observedPath)
	}
	if !strings.Contains(stderr, `warning: field "missing" not found in output`) {
		t.Fatalf("expected missing-field warning on stderr, got %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("document get trim emitted invalid JSON: %v\n%s", err, stdout)
	}
	if payload["id"] != "doc-1" || payload["name"] != "Home" {
		t.Fatalf("expected compact summary id/name, got %+v", payload)
	}
	if _, ok := payload["big"]; ok {
		t.Fatalf("expected unrequested large field to be dropped, got %+v", payload)
	}
	docType := payload["documentType"].(map[string]any)
	if docType["id"] != "dt-1" {
		t.Fatalf("expected documentType id, got %+v", docType)
	}
	if _, ok := docType["alias"]; ok {
		t.Fatalf("expected empty documentType alias pruned, got %+v", docType)
	}
	if got := payload["values"].(map[string]any)["bodyText"]; got != "Welcome" {
		t.Fatalf("expected values.bodyText projection, got %v", got)
	}
}

func TestDocumentGetSummaryIsSmallerThanFullPayload(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{
				"id":"doc-1",
				"name":"Home",
				"documentType":{"id":"dt-1","alias":"homePage"},
				"route":{"path":"/"},
				"values":[
					{"alias":"bodyText","value":"large body text that a summary should omit"},
					{"alias":"seoDescription","value":"large seo text that a summary should omit"}
				],
				"permissions":["A","B","C"],
				"cultures":{"en-US":{"name":"Home","url":"/"}}
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	full, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-1", "-o", "json")
	if err != nil {
		t.Fatalf("document get full failed: %v", err)
	}
	summary, err := execute(buildRootWithCollections(t, deps), "document", "get", "doc-1", "--summary", "-o", "json")
	if err != nil {
		t.Fatalf("document get summary failed: %v", err)
	}
	if len(summary) >= len(full) {
		t.Fatalf("expected summary output (%d bytes) to be smaller than full output (%d bytes)\nsummary=%s\nfull=%s", len(summary), len(full), summary, full)
	}
}

func TestDocumentCollectionFieldsPreserveEnvelopeAndDoNotChangeQuery(t *testing.T) {
	var observedPath string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			observedPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{
				"items":[
					{"id":"doc-1","name":"Home","documentType":{"id":"dt-1"},"extra":"drop"},
					{"id":"doc-2","name":"About","documentType":{"id":"dt-2"},"extra":"drop"}
				],
				"total":2
			}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "children", "parent-1", "--fields", "id,name", "--skip", "10", "--take", "5", "-o", "json")
	if err != nil {
		t.Fatalf("document children fields failed: %v", err)
	}
	if strings.Contains(observedPath, "fields=") || !strings.Contains(observedPath, "parentId=parent-1") || !strings.Contains(observedPath, "skip=10") || !strings.Contains(observedPath, "take=5") {
		t.Fatalf("unexpected document children request path: %q", observedPath)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode children payload: %v", err)
	}
	if payload["total"] != float64(2) {
		t.Fatalf("expected total preserved, got %+v", payload)
	}
	items := payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected item count preserved, got %+v", items)
	}
	first := items[0].(map[string]any)
	if first["id"] != "doc-1" || first["name"] != "Home" || len(first) != 2 {
		t.Fatalf("expected projected child item, got %+v", first)
	}
}

func TestDocumentRootAndSearchSupportSummaryOutput(t *testing.T) {
	var sawRoot bool
	var sawSearch bool
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			sawRoot = true
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"root-1","variants":[{"name":"Home"}],"documentType":{"id":"dt-1","alias":"homePage"},"extra":"drop"}],"total":1}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			sawSearch = true
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"search-1","name":"Result","documentType":{"id":"dt-2","alias":"article"},"route":{"path":"/result"},"extra":"drop"}],"total":1}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	rootOutput, err := execute(buildRootWithCollections(t, deps), "document", "root", "--summary", "-o", "json")
	if err != nil {
		t.Fatalf("document root summary failed: %v", err)
	}
	var rootPayload map[string]any
	if err := json.Unmarshal([]byte(rootOutput), &rootPayload); err != nil {
		t.Fatalf("failed to decode root payload: %v", err)
	}
	rootItem := rootPayload["items"].([]any)[0].(map[string]any)
	if rootItem["id"] != "root-1" || rootItem["name"] != "Home" {
		t.Fatalf("expected compact root summary, got %+v", rootItem)
	}
	if _, ok := rootItem["extra"]; ok {
		t.Fatalf("expected root summary to drop extra fields, got %+v", rootItem)
	}

	searchOutput, err := execute(buildRootWithCollections(t, deps), "document", "search", "--query", "Result", "--summary", "-o", "json")
	if err != nil {
		t.Fatalf("document search summary failed: %v", err)
	}
	var searchPayload map[string]any
	if err := json.Unmarshal([]byte(searchOutput), &searchPayload); err != nil {
		t.Fatalf("failed to decode search payload: %v", err)
	}
	searchItem := searchPayload["items"].([]any)[0].(map[string]any)
	if searchItem["id"] != "search-1" || searchItem["route"].(map[string]any)["path"] != "/result" {
		t.Fatalf("expected compact search summary, got %+v", searchItem)
	}
	if _, ok := searchItem["extra"]; ok {
		t.Fatalf("expected search summary to drop extra fields, got %+v", searchItem)
	}
	if !sawRoot || !sawSearch {
		t.Fatalf("expected root and search endpoints to be called")
	}
}

func TestDocumentCopyPublishPublishesCopiedDocument(t *testing.T) {
	var publishPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/source-1/copy":
			return endpointJSONResponse(http.StatusOK, `{"id":"copy-1","name":"Copied"}`), nil
		case "/umbraco/management/api/v1/document/copy-1/publish":
			publishPath = req.URL.Path
			return endpointJSONResponse(http.StatusOK, `{"published":true}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "copy", "source-1", "--to", "parent-1", "--publish")
	if err != nil {
		t.Fatalf("document copy --publish failed: %v", err)
	}
	if publishPath != "/umbraco/management/api/v1/document/copy-1/publish" {
		t.Fatalf("expected copied document to be published, got %q", publishPath)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode copy publish result: %v", err)
	}
	if result["copied"] == nil || result["published"] == nil {
		t.Fatalf("unexpected copy publish result: %+v", result)
	}
}

func TestDocumentCopyPublishDryRunPlansBothRequests(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("dry-run must not reach the server, got %s %s", req.Method, req.URL.Path)
		return nil, nil
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "copy", "source-1", "--to", "parent-1", "--publish", "--dry-run")
	if err != nil {
		t.Fatalf("document copy --publish --dry-run failed: %v", err)
	}

	var result struct {
		Copied    map[string]any `json:"copied"`
		Published map[string]any `json:"published"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to decode dry-run result: %v", err)
	}
	if result.Copied["path"] != "/umbraco/management/api/v1/document/source-1/copy" {
		t.Fatalf("unexpected copy plan: %+v", result.Copied)
	}
	if result.Published["path"] != "/umbraco/management/api/v1/document/copied-document-id/publish" {
		t.Fatalf("unexpected publish plan: %+v", result.Published)
	}
}

func allJSONControlCharacters() string {
	runes := make([]rune, 0, 32)
	for value := rune(0); value <= 0x1f; value++ {
		runes = append(runes, value)
	}
	return string(runes)
}

func assertNoRawControlCharactersInJSONStringTokens(t *testing.T, raw string) {
	t.Helper()

	inString := false
	escaped := false
	for index, r := range raw {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString && r <= 0x1f:
			t.Fatalf("raw control character U+%04X found inside JSON string at byte %d", r, index)
		}
	}
	if inString {
		t.Fatal("unterminated JSON string")
	}
}

func assertStrictJSONParsersAccept(t *testing.T, raw string) {
	t.Helper()

	parsers := []struct {
		name string
		args []string
	}{
		{name: "python3", args: []string{"-c", "import json,sys; json.load(sys.stdin)"}},
		{name: "node", args: []string{"-e", "JSON.parse(require('fs').readFileSync(0, 'utf8'))"}},
		{name: "jq", args: []string{"."}},
	}
	for _, parser := range parsers {
		if _, err := exec.LookPath(parser.name); err != nil {
			t.Logf("%s not found; skipping external JSON parser check", parser.name)
			continue
		}
		cmd := exec.Command(parser.name, parser.args...)
		cmd.Stdin = strings.NewReader(raw)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s rejected CLI JSON output: %v: %s", parser.name, err, stderr.String())
		}
	}
}

func TestDocumentSearchSupportsUnderShortcut(t *testing.T) {
	var observedPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/item/document/search":
			observedPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"doc-1","name":"Toxic"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"document", "search",
		"--query", "Toxic",
		"--under", "partners-root",
		"--skip", "0",
		"--take", "25",
	)
	if err != nil {
		t.Fatalf("document search --under failed: %v", err)
	}

	if !strings.Contains(observedPath, "parentId=partners-root") {
		t.Fatalf("expected --under to map to parentId, got %q", observedPath)
	}
}

func TestDocumentTreeCommandsPreferTreeEndpoints(t *testing.T) {
	var rootPath string
	var childrenPath string
	var ancestorsPath string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			rootPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"root-1"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			childrenPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"child-1"}]}`), nil
		case "/umbraco/management/api/v1/tree/document/ancestors":
			ancestorsPath = req.URL.String()
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"ancestor-1"}]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "root", "--fields", "id,name"); err != nil {
		t.Fatalf("document root failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "document", "children", "parent-1", "--fields", "id,name"); err != nil {
		t.Fatalf("document children failed: %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "document", "ancestors", "doc-1"); err != nil {
		t.Fatalf("document ancestors failed: %v", err)
	}

	if !strings.Contains(rootPath, "/tree/document/root") || strings.Contains(rootPath, "fields=") {
		t.Fatalf("unexpected document root path: %q", rootPath)
	}
	if !strings.Contains(childrenPath, "/tree/document/children") || !strings.Contains(childrenPath, "parentId=parent-1") {
		t.Fatalf("unexpected document children path: %q", childrenPath)
	}
	if !strings.Contains(ancestorsPath, "/tree/document/ancestors") || !strings.Contains(ancestorsPath, "descendantId=doc-1") {
		t.Fatalf("unexpected document ancestors path: %q", ancestorsPath)
	}
}

func TestDocumentUpdateMergeJSONFetchesAndMergesCurrentDocument(t *testing.T) {
	var observedPutBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Toxic",
  "documentType":{"id":"type-1"},
  "values":[
    {"alias":"title","value":"Old title"},
    {"alias":"summary","value":"Keep me"}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode merged document payload: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "update", "doc-1", "--merge-json", `{"values":[{"alias":"title","value":"New title"}]}`)
	if err != nil {
		t.Fatalf("document update --merge-json failed: %v", err)
	}

	if observedPutBody["name"] != "Toxic" {
		t.Fatalf("expected merged document to preserve root fields, got %+v", observedPutBody)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("expected merged values payload, got %+v", observedPutBody["values"])
	}

	firstValue, _ := values[0].(map[string]any)
	secondValue, _ := values[1].(map[string]any)
	if firstValue["value"] != "New title" || secondValue["value"] != "Keep me" {
		t.Fatalf("expected alias-based merge to preserve untouched values, got %+v", observedPutBody["values"])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode document update result: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected document update result: %+v", payload)
	}
}

func TestDocumentUpdateMergeJSONAllowsExistingControlCharactersInFetchedDocument(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[
    {"alias":"bodyText","value":"line1\nline2"},
    {"alias":"title","value":"Old title"}
  ]
}`), nil
			}
			if req.Method == http.MethodPut {
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--merge-json", `{"values":[{"alias":"skills","value":[{"type":"document","unique":"62689bb1-3a4d-478f-a7b1-1c0e560d4748"}]}]}`,
	)
	if err != nil {
		t.Fatalf("document update --merge-json with existing control characters failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode merge-json regression payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected merge-json regression payload: %+v", payload)
	}
}

func TestDocumentUpdatePropertyTargetsPropertiesEndpoint(t *testing.T) {
	var observedGetCount int
	var observedPutPath string
	var observedPutBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				observedGetCount++
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			if req.Method == http.MethodPut {
				observedPutPath = req.URL.Path
				if err := json.NewDecoder(req.Body).Decode(&observedPutBody); err != nil {
					t.Fatalf("failed to decode merged document payload: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
	)
	if err != nil {
		t.Fatalf("document property update failed: %v", err)
	}

	if observedGetCount != 1 {
		t.Fatalf("expected one GET before property merge update, got %d", observedGetCount)
	}
	if observedPutPath != "/umbraco/management/api/v1/document/doc-1" {
		t.Fatalf("unexpected merged document update path: %q", observedPutPath)
	}

	values, ok := observedPutBody["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("unexpected properties payload: %+v", observedPutBody)
	}
	var foundSkills bool
	for _, item := range values {
		valueEntry, _ := item.(map[string]any)
		if valueEntry["alias"] == "skills" && valueEntry["value"] == "C#;Go" {
			foundSkills = true
		}
	}
	if !foundSkills {
		t.Fatalf("expected merged property value entry, got %+v", observedPutBody)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode property update output: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected property update result: %+v", payload)
	}
}

func TestDocumentUpdateSaveAndPublishDryRunReturnsBothSteps(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
		"--save-and-publish",
		"--culture", "en-US",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document save-and-publish dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode save-and-publish output: %v", err)
	}
	if payload["saveAndPublish"] != true {
		t.Fatalf("expected saveAndPublish marker, got %+v", payload)
	}

	updated, ok := payload["updated"].(map[string]any)
	if !ok {
		t.Fatalf("missing updated dry-run payload: %+v", payload)
	}
	published, ok := payload["published"].(map[string]any)
	if !ok {
		t.Fatalf("missing published dry-run payload: %+v", payload)
	}

	if updated["path"] != "/umbraco/management/api/v1/document/doc-1" {
		t.Fatalf("unexpected update dry-run path: %+v", updated)
	}
	if published["path"] != "/umbraco/management/api/v1/document/doc-1/publish" {
		t.Fatalf("unexpected publish dry-run path: %+v", published)
	}
	body, _ := published["body"].(map[string]any)
	cultures, _ := body["cultures"].([]any)
	if len(cultures) != 1 || cultures[0] != "en-US" {
		t.Fatalf("expected publish culture in dry-run body, got %+v", body)
	}
}

func TestDocumentPublishDryRunDefaultsToInvariantPublishSchedule(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "publish", "doc-1",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document publish dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode publish dry-run payload: %v", err)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing publish body in dry-run payload: %+v", payload)
	}
	publishSchedules, ok := body["publishSchedules"].([]any)
	if !ok || len(publishSchedules) != 1 {
		t.Fatalf("expected invariant publishSchedules payload, got %+v", body)
	}
	entry, ok := publishSchedules[0].(map[string]any)
	if !ok || entry["culture"] != nil {
		t.Fatalf("expected publishSchedules culture=null, got %+v", publishSchedules[0])
	}
}

func TestDocumentUpdateSaveAndPublishDryRunDefaultsToInvariantPublishSchedule(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
			}
			return endpointJSONResponse(http.StatusMethodNotAllowed, `null`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--property", "skills",
		"--value", "C#;Go",
		"--save-and-publish",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document save-and-publish invariant dry-run failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode invariant save-and-publish output: %v", err)
	}
	published, ok := payload["published"].(map[string]any)
	if !ok {
		t.Fatalf("missing published dry-run payload: %+v", payload)
	}
	body, ok := published["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing publish body: %+v", published)
	}
	publishSchedules, ok := body["publishSchedules"].([]any)
	if !ok || len(publishSchedules) != 1 {
		t.Fatalf("expected invariant publishSchedules payload, got %+v", body)
	}
	entry, ok := publishSchedules[0].(map[string]any)
	if !ok || entry["culture"] != nil {
		t.Fatalf("expected publishSchedules culture=null, got %+v", publishSchedules[0])
	}
}

func TestDocumentBulkUpdateDryRunUsesExplicitIDsAndSkipsNoOps(t *testing.T) {
	var putRequests int

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
		case "/umbraco/management/api/v1/document/doc-2":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"doc-2",
  "name":"Partner B",
  "values":[{"alias":"title","value":"New title"}]
}`), nil
		default:
			if req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				putRequests++
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "bulk-update",
		"--id", "doc-1",
		"--id", "doc-2",
		"--merge-json", `{"values":[{"alias":"title","value":"New title"}]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document bulk-update failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run bulk update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentBulkUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode bulk update result: %v", err)
	}
	if payload.Total != 2 || payload.Updated != 1 || payload.Skipped != 1 || payload.Failed != 0 {
		t.Fatalf("unexpected bulk update summary: %+v", payload)
	}
}

func TestDocumentBulkUpdateLoadsIDsFromFile(t *testing.T) {
	idFile := filepath.Join(t.TempDir(), "ids.txt")
	if err := os.WriteFile(idFile, []byte("doc-1\n\ndoc-2\n"), 0o644); err != nil {
		t.Fatalf("failed to write id file: %v", err)
	}

	var putRequests int
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		default:
			if req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				putRequests++
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `{"id":"doc","name":"Doc","values":[]}`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "bulk-update",
		"--id-file", idFile,
		"--json", `{"values":[]}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document bulk-update with id file failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run id-file bulk update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentBulkUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode bulk update id-file result: %v", err)
	}
	if payload.Total != 2 || payload.Updated != 2 {
		t.Fatalf("unexpected id-file bulk update summary: %+v", payload)
	}
}

func TestDocumentCSVUpdateDryRunUsesMappedProperties(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "partners.csv")
	if err := os.WriteFile(csvPath, []byte("id,skills\npartner-1,C#;Go\npartner-2,\n"), 0o644); err != nil {
		t.Fatalf("failed to write CSV fixture: %v", err)
	}

	var putRequests int
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/partner-1":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"partner-1",
  "name":"Partner A",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
		case "/umbraco/management/api/v1/document/partner-2":
			return endpointJSONResponse(http.StatusOK, `{
  "id":"partner-2",
  "name":"Partner B",
  "values":[{"alias":"title","value":"Old title"}]
}`), nil
		default:
			if req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				putRequests++
				return endpointJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "csv-update",
		"--file", csvPath,
		"--property", "skills",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document csv-update failed: %v", err)
	}

	if putRequests != 0 {
		t.Fatalf("expected dry-run CSV update to avoid real PUT requests, got %d", putRequests)
	}

	var payload documentCSVUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode CSV update result: %v", err)
	}
	if payload.TotalRows != 2 || payload.Updated != 1 || payload.Skipped != 1 || payload.Failed != 0 {
		t.Fatalf("unexpected CSV update summary: %+v", payload)
	}
}

func TestDocumentCSVUpdateRejectsDuplicateIDs(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "partners.csv")
	if err := os.WriteFile(csvPath, []byte("id,skills\npartner-1,C#\npartner-1,Go\n"), 0o644); err != nil {
		t.Fatalf("failed to write CSV fixture: %v", err)
	}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/partner-1":
			return endpointJSONResponse(http.StatusOK, `{"id":"partner-1","name":"Partner A","values":[]}`), nil
		default:
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "csv-update",
		"--file", csvPath,
		"--property", "skills",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document csv-update duplicate case failed: %v", err)
	}

	var payload documentCSVUpdateResult
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode CSV duplicate result: %v", err)
	}
	if payload.Failed != 1 {
		t.Fatalf("expected one failed duplicate row, got %+v", payload)
	}
}

// --- Regression coverage for the four document command bugs reported
//     against v0.3.15. See commit message for the full background. ---

// currentDocPayload is the GET response used across the update-properties
// regressions. Includes one pre-existing values entry so tests can assert
// that the merge preserves untouched properties.
func currentDocPayload() string {
	return `{
		"id":"doc-1",
		"name":"Test Doc",
		"documentType":{"id":"type-1"},
		"values":[{"alias":"existingProp","value":"keep me","culture":null,"segment":null}]
	}`
}

func mockUpdatePropertiesPut(t *testing.T) (deps Dependencies, captured *map[string]any) {
	t.Helper()
	put := map[string]any{}
	deps = endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentDocPayload()), nil
			}
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&put)
				return endpointNoContent(), nil
			}
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	return deps, &put
}

// Bug #1: --json object form used to land at the document root and silently
// no-op. It must now merge into values[] as alias entries.
func TestDocumentUpdatePropertiesObjectFormMergesIntoValuesArray(t *testing.T) {
	deps, captured := mockUpdatePropertiesPut(t)

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update-properties", "doc-1",
		"--json", `{"isMigrationCaseStudy": true, "products": ["Umbraco CMS","Forms"]}`,
	)
	if err != nil {
		t.Fatalf("object-form update-properties failed: %v", err)
	}

	put := *captured
	// The pre-bug regression: object keys must NOT appear at the document root.
	for _, leakedKey := range []string{"isMigrationCaseStudy", "products"} {
		if _, leaked := put[leakedKey]; leaked {
			t.Fatalf("property %q leaked to top-level body — silent no-op regression: %+v", leakedKey, put)
		}
	}

	values, ok := put["values"].([]any)
	if !ok {
		t.Fatalf("expected values array in PUT, got %+v", put["values"])
	}
	got := map[string]any{}
	for _, v := range values {
		entry := v.(map[string]any)
		got[entry["alias"].(string)] = entry
	}
	for _, alias := range []string{"isMigrationCaseStudy", "products", "existingProp"} {
		if _, present := got[alias]; !present {
			t.Fatalf("expected alias %q in merged values[], got %+v", alias, values)
		}
	}
	if got["existingProp"].(map[string]any)["value"] != "keep me" {
		t.Fatalf("untouched property must be preserved by the merge, got %+v", got["existingProp"])
	}
	if got["isMigrationCaseStudy"].(map[string]any)["value"] != true {
		t.Fatalf("new bool value missing, got %+v", got["isMigrationCaseStudy"])
	}

	// 204 No Content from the server must surface as {"updated": true}, not nil.
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["updated"] != true {
		t.Fatalf("expected {\"updated\":true} for empty 204 response, got %+v", payload)
	}
}

// Bug #2: array form used to be rejected up front.
func TestDocumentUpdatePropertiesAcceptsArrayForm(t *testing.T) {
	deps, captured := mockUpdatePropertiesPut(t)

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update-properties", "doc-1",
		"--json", `[{"alias":"isMigrationCaseStudy","value":true,"culture":null,"segment":null}]`,
	); err != nil {
		t.Fatalf("array-form update-properties failed: %v", err)
	}

	values := (*captured)["values"].([]any)
	var found bool
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] == "isMigrationCaseStudy" && entry["value"] == true {
			found = true
		}
	}
	if !found {
		t.Fatalf("array-form entry didn't land in values[]: %+v", values)
	}
}

// Envelope form continues to work after the parser refactor.
func TestDocumentUpdatePropertiesAcceptsEnvelopeForm(t *testing.T) {
	deps, captured := mockUpdatePropertiesPut(t)

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update-properties", "doc-1",
		"--json", `{"values":[{"alias":"isMigrationCaseStudy","value":true,"culture":null,"segment":null}]}`,
	); err != nil {
		t.Fatalf("envelope-form update-properties failed: %v", err)
	}
	values := (*captured)["values"].([]any)
	if len(values) < 2 { // existing + new
		t.Fatalf("expected merged values[], got %+v", values)
	}
}

// Malformed inputs are rejected loudly so agents don't get a silent no-op.
func TestDocumentUpdatePropertiesRejectsMalformedPayloads(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	for label, json := range map[string]string{
		"array entry missing alias":    `[{"value":"x"}]`,
		"array entry missing value":    `[{"alias":"x"}]`,
		"envelope entry missing value": `{"values":[{"alias":"x"}]}`,
		"envelope entry missing alias": `{"values":[{"value":"x"}]}`,
		"non-object array entry":       `["string-not-object"]`,
		"top-level string":             `"just a string"`,
		"top-level number":             `42`,
	} {
		_, err := execute(buildRootWithCollections(t, deps), "document", "update-properties", "doc-1", "--json", json)
		if err == nil {
			t.Fatalf("%s: expected rejection, got nil", label)
		}
	}
}

// Explicit "value":null must be accepted — it's how callers clear a property
// value. Distinguishing "key absent" from "value:null" is the whole reason we
// validate key presence rather than just nil-ness.
func TestDocumentUpdatePropertiesAcceptsExplicitNullValue(t *testing.T) {
	var captured map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{"id":"doc-1","values":[{"alias":"existingProp","value":"keep me"}]}`), nil
			}
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&captured)
				return endpointNoContent(), nil
			}
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update-properties", "doc-1",
		"--json", `[{"alias":"products","value":null,"culture":null,"segment":null}]`,
	); err != nil {
		t.Fatalf("explicit value:null should be accepted, got: %v", err)
	}
	values := captured["values"].([]any)
	var found bool
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] == "products" {
			if entry["value"] != nil {
				t.Fatalf("explicit null lost: %+v", entry)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("products entry never landed in PUT body: %+v", values)
	}
}

// Bug #3: --save-and-publish previously returned {"updated":null,"published":null}
// because Umbraco answers 204 No Content. Both flags must be true booleans on
// success.
func TestDocumentUpdateSaveAndPublishReturnsTrueBooleansOn204(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentDocPayload()), nil
			}
			if req.Method == http.MethodPut {
				return endpointNoContent(), nil
			}
		case "/umbraco/management/api/v1/document/doc-1/publish":
			return endpointNoContent(), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--merge-json", `{"values":[{"alias":"existingProp","value":"new","culture":null,"segment":null}]}`,
		"--save-and-publish",
	)
	if err != nil {
		t.Fatalf("save-and-publish failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["updated"] != true || payload["published"] != true || payload["saveAndPublish"] != true {
		t.Fatalf("expected all three flags true on 204+204 success, got %+v", payload)
	}
}

// Bug #4: the spurious 400 "culture for an [invariant content]" race that
// surfaces under rapid back-to-back save-and-publish loops should be retried
// transparently. The exact same publish request succeeds on retry per the
// bug report.
func TestDocumentSaveAndPublishRetriesInvariantContentRace(t *testing.T) {
	var publishAttempts int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentDocPayload()), nil
			}
			if req.Method == http.MethodPut {
				return endpointNoContent(), nil
			}
		case "/umbraco/management/api/v1/document/doc-1/publish":
			n := atomic.AddInt32(&publishAttempts, 1)
			// First two publish attempts hit the race; third succeeds.
			if n < 3 {
				return endpointJSONResponse(http.StatusBadRequest, `{"detail":"One or more property values specify a culture for an [invariant content]"}`), nil
			}
			return endpointNoContent(), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--merge-json", `{"values":[{"alias":"existingProp","value":"v","culture":null,"segment":null}]}`,
		"--save-and-publish",
	)
	if err != nil {
		t.Fatalf("save-and-publish under race should retry to success, got: %v", err)
	}
	if got := atomic.LoadInt32(&publishAttempts); got != 3 {
		t.Fatalf("expected exactly 3 publish attempts (2 races + 1 success), got %d", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["published"] != true {
		t.Fatalf("expected published=true after retry, got %+v", payload)
	}
}

// Unrelated 400s must NOT be retried — only the specific invariant-content race.
func TestDocumentSaveAndPublishDoesNotRetryUnrelated400s(t *testing.T) {
	var publishAttempts int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1":
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentDocPayload()), nil
			}
			if req.Method == http.MethodPut {
				return endpointNoContent(), nil
			}
		case "/umbraco/management/api/v1/document/doc-1/publish":
			atomic.AddInt32(&publishAttempts, 1)
			return endpointJSONResponse(http.StatusBadRequest, `{"detail":"Validation failed: country is required"}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "doc-1",
		"--merge-json", `{"values":[{"alias":"existingProp","value":"v","culture":null,"segment":null}]}`,
		"--save-and-publish",
	)
	if err == nil {
		t.Fatalf("expected the unrelated 400 to surface, not be retried")
	}
	if got := atomic.LoadInt32(&publishAttempts); got != 1 {
		t.Fatalf("expected exactly one publish attempt for non-race 400, got %d", got)
	}
}

// Regression: document children was capped at the server's default page (~100)
// because --skip/--take weren't exposed. The flags now pass through verbatim
// and the request URL carries them — so '--first-n N' is a client-side cap
// on a per-page response, while --skip lets you walk past page 1.
func TestDocumentChildrenPassesSkipAndTakeAsQueryParams(t *testing.T) {
	var observedQuery string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "children", "doc-1", "--skip", "100", "--take", "100"); err != nil {
		t.Fatalf("children failed: %v", err)
	}
	for _, want := range []string{"parentId=doc-1", "skip=100", "take=100"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected query to contain %q, got %q", want, observedQuery)
		}
	}
}

func TestDocumentRootPassesSkipAndTakeAsQueryParams(t *testing.T) {
	var observedQuery string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/root":
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "root", "--skip", "50", "--take", "25"); err != nil {
		t.Fatalf("root failed: %v", err)
	}
	for _, want := range []string{"skip=50", "take=25"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected query to contain %q, got %q", want, observedQuery)
		}
	}
}

// --all walks pages until the server returns a short page or items run out.
// 'short page' is len(items) < take — what the server signals at the
// boundary of the collection. Three pages of 100 + a final 7-item page
// should be merged into one envelope of 307 items, with the loop stopping
// because the last page is shorter than the page size.
func TestDocumentChildrenAllAutoPaginatesAcrossPages(t *testing.T) {
	var pages int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			skip := req.URL.Query().Get("skip")
			n := atomic.AddInt32(&pages, 1)
			items := make([]string, 0, 100)
			pageSize := 100
			if n == 4 { // last partial page
				pageSize = 7
			}
			if n > 4 {
				return endpointJSONResponse(http.StatusOK, `{"items":[],"total":307}`), nil
			}
			for i := 0; i < pageSize; i++ {
				items = append(items, `{"id":"x"}`)
			}
			return endpointJSONResponse(http.StatusOK, `{"items":[`+strings.Join(items, ",")+`],"total":307,"_observed_skip":"`+skip+`"}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	// --take pins the page size so the mock's 100-item pages match what
	// the helper requests; loop should walk 3 full pages + 1 short.
	output, err := execute(buildRootWithCollections(t, deps), "document", "children", "doc-1", "--all", "--take", "100")
	if err != nil {
		t.Fatalf("--all failed: %v", err)
	}
	if got := atomic.LoadInt32(&pages); got != 4 {
		t.Fatalf("expected exactly 4 pages walked (3 full + 1 short), got %d", got)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(output), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items := env["items"].([]any)
	if len(items) != 307 {
		t.Fatalf("expected 307 merged items, got %d", len(items))
	}
}

// --first-n should short-circuit --all so the loop doesn't pull pages whose
// items would be thrown away.
func TestDocumentChildrenAllRespectsFirstNAsEarlyStop(t *testing.T) {
	var pages int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			atomic.AddInt32(&pages, 1)
			// 500 items per page (default --all page size); plenty.
			items := make([]string, 500)
			for i := range items {
				items[i] = `{"id":"x"}`
			}
			return endpointJSONResponse(http.StatusOK, `{"items":[`+strings.Join(items, ",")+`],"total":99999}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "children", "doc-1", "--all", "--first-n", "150"); err != nil {
		t.Fatalf("--all --first-n failed: %v", err)
	}
	if got := atomic.LoadInt32(&pages); got != 1 {
		t.Fatalf("expected --first-n 150 to stop after the first 500-item page, got %d pages", got)
	}
}

// 'document references' wraps /document/{id}/referenced-by and shares the
// pagination plumbing with children/root, so the same skip/take/all flags
// pass through to the URL.
func TestDocumentReferencesPassesPaginationToReferencedByEndpoint(t *testing.T) {
	var observedQuery string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1/referenced-by":
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"ref-1"},{"id":"ref-2"}],"total":2}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "references", "doc-1", "--skip", "10", "--take", "50")
	if err != nil {
		t.Fatalf("references failed: %v", err)
	}
	for _, want := range []string{"skip=10", "take=50"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected %q in query, got %q", want, observedQuery)
		}
	}
	if !strings.Contains(output, "ref-1") {
		t.Fatalf("expected response body to surface, got %q", output)
	}
}

func TestDocumentReferencedDescendantsHitsDescendantsEndpoint(t *testing.T) {
	var hit bool
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/doc-1/referenced-descendants":
			hit = true
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "document", "referenced-descendants", "doc-1"); err != nil {
		t.Fatalf("referenced-descendants failed: %v", err)
	}
	if !hit {
		t.Fatalf("expected the descendants endpoint to be called")
	}
}

func TestDocumentAreReferencedRequiresIDsAndRepeatsQueryParam(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "document", "are-referenced"); err == nil {
		t.Fatalf("expected error when --ids is missing")
	}

	var observedQuery string
	deps = endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/document/are-referenced":
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":["doc-1"],"total":1}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "document", "are-referenced", "--ids", "doc-1,doc-2,doc-3"); err != nil {
		t.Fatalf("are-referenced failed: %v", err)
	}
	// Each id must be its own ?id=...&id=... entry.
	for _, want := range []string{"id=doc-1", "id=doc-2", "id=doc-3"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected %q in query, got %q", want, observedQuery)
		}
	}
}

// Media references symmetry — should hit the /media/{id}/referenced-by path
// exactly like document references hits /document/.../referenced-by.
func TestMediaReferencesHitsMediaReferencedByEndpoint(t *testing.T) {
	var hit bool
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/media/m-1/referenced-by":
			hit = true
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "media", "references", "m-1"); err != nil {
		t.Fatalf("media references failed: %v", err)
	}
	if !hit {
		t.Fatalf("expected the media referenced-by endpoint to be called")
	}
}

// Regression for the silent-truncation bug surfaced by Codex review.
// When --all hits the safety ceiling (200 pages × 500 items = 100k items)
// without the server ever returning a short page, the helper must error
// out — silently returning the first 100k items would let callers mistake
// a cap hit for a complete walk.
func TestDocumentChildrenAllErrorsOnSafetyCeiling(t *testing.T) {
	var pages int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/tree/document/children":
			atomic.AddInt32(&pages, 1)
			// Always return a full page so the loop never sees a short page
			// and runs straight into the cap. Use a small page size (--take
			// 5) so the cap is hit in 200 quick iterations rather than
			// 200 × default 500 = 100k network round-trips in the test.
			items := make([]string, 5)
			for i := range items {
				items[i] = `{"id":"x"}`
			}
			return endpointJSONResponse(http.StatusOK, `{"items":[`+strings.Join(items, ",")+`],"total":999999}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := execute(buildRootWithCollections(t, deps), "document", "children", "doc-1", "--all", "--take", "5")
	if err == nil {
		t.Fatalf("expected --all to error on the safety ceiling, got success")
	}
	if !strings.Contains(err.Error(), "safety ceiling") {
		t.Fatalf("error should mention 'safety ceiling', got: %v", err)
	}
	// Sanity: the loop did walk the full cap before giving up.
	if got := atomic.LoadInt32(&pages); got != 200 {
		t.Fatalf("expected 200 pages walked before bailing, got %d", got)
	}
	// The resume offset must point at the first UNREAD item, not one page
	// past it. With --take 5 and 200 pages walked, collected offsets are
	// 0..995 → resume must be --skip 1000, NOT 1005.
	if !strings.Contains(err.Error(), "--skip 1000") {
		t.Fatalf("error must point caller at the exact next-unread offset (--skip 1000), got: %v", err)
	}
}
