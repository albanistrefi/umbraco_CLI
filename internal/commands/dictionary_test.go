package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

type dictionaryRoundTripper func(*http.Request) (*http.Response, error)

func (fn dictionaryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func dictionaryJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func dictionaryDeps(handler dictionaryRoundTripper) cmdkit.Dependencies {
	cfg := config.Config{
		BaseURL:      "https://example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	httpClient := &http.Client{Transport: handler}
	output := "json"

	return cmdkit.Dependencies{
		Client:     api.NewClient(cfg, httpClient, auth.New(cfg, httpClient)),
		EnvOutput:  config.OutputJSON,
		OutputFlag: &output,
	}
}

func writeDictionaryImportFixture(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dictionary-import.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return path
}

func TestDictionaryImportDryRunPlansCreateUpdateAndSkip(t *testing.T) {
	file := writeDictionaryImportFixture(t, `[
  {"key":"Existing.NoChange","translations":{"en-US":"Same"}},
  {"key":"Existing.Update","translations":{"en-US":"Updated","da-DK":"Opdateret"}},
  {"key":"New.Key","translations":{"en-US":"Created"}}
]`)

	var mu sync.Mutex
	listRequests := 0
	getRequests := map[string]int{}
	postRequests := 0
	putRequests := 0

	deps := dictionaryDeps(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return dictionaryJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/dictionary":
			if req.Method != http.MethodGet {
				postRequests++
				return dictionaryJSONResponse(http.StatusCreated, `{}`), nil
			}
			listRequests++
			return dictionaryJSONResponse(http.StatusOK, `{
  "total": 2,
  "items": [
    {"id":"existing-1","name":"Existing.NoChange","parent":null,"translatedIsoCodes":["en-US"]},
    {"id":"existing-2","name":"Existing.Update","parent":null,"translatedIsoCodes":["en-US"]}
  ]
}`), nil
		case "/umbraco/management/api/v1/dictionary/existing-1":
			getRequests["existing-1"]++
			return dictionaryJSONResponse(http.StatusOK, `{
  "id":"existing-1",
  "name":"Existing.NoChange",
  "parent":null,
  "translations":[{"isoCode":"en-US","translation":"Same"}]
}`), nil
		case "/umbraco/management/api/v1/dictionary/existing-2":
			getRequests["existing-2"]++
			return dictionaryJSONResponse(http.StatusOK, `{
  "id":"existing-2",
  "name":"Existing.Update",
  "parent":null,
  "translations":[{"isoCode":"en-US","translation":"Old"}]
}`), nil
		case "/umbraco/management/api/v1/dictionary/import":
			return dictionaryJSONResponse(http.StatusNotFound, `{"error":"unexpected import endpoint"}`), nil
		case "/umbraco/management/api/v1/dictionary/new":
			return dictionaryJSONResponse(http.StatusNotFound, `{"error":"unexpected new endpoint"}`), nil
		default:
			if req.Method == http.MethodPut {
				putRequests++
			}
			if req.Method == http.MethodPost {
				postRequests++
			}
			return dictionaryJSONResponse(http.StatusNotFound, `{"error":"unexpected request"}`), nil
		}
	})

	output, err := cmdtest.Execute(
		buildRootWithCollections(t, deps),
		"dictionary", "import",
		"--file", file,
		"--update-existing",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("dictionary import dry-run failed: %v", err)
	}

	var result dictionaryImportResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse import result: %v", err)
	}

	if result.Created != 1 || result.Updated != 1 || result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dry-run summary: %+v", result)
	}
	if !result.DryRun {
		t.Fatalf("expected dryRun=true in result")
	}
	if listRequests != 1 {
		t.Fatalf("expected one list request, got %d", listRequests)
	}
	if getRequests["existing-1"] != 1 || getRequests["existing-2"] != 1 {
		t.Fatalf("expected one detail request per existing item, got %+v", getRequests)
	}
	if postRequests != 0 || putRequests != 0 {
		t.Fatalf("dry-run should not execute POST or PUT requests, got post=%d put=%d", postRequests, putRequests)
	}
}

func TestDictionaryImportIsIdempotentWithSkipExisting(t *testing.T) {
	file := writeDictionaryImportFixture(t, `[
  {"key":"Alpha.One","translations":{"en-US":"One"}},
  {"key":"Alpha.Two","translations":{"en-US":"Two"}}
]`)

	type storedDictionaryItem struct {
		ID           string
		Name         string
		Translations []dictionaryTranslation
	}

	var mu sync.Mutex
	postRequests := 0
	items := map[string]storedDictionaryItem{}

	deps := dictionaryDeps(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return dictionaryJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/dictionary":
			switch req.Method {
			case http.MethodGet:
				overview := make([]dictionaryOverview, 0, len(items))
				for _, item := range items {
					overview = append(overview, dictionaryOverview{
						ID:                 item.ID,
						Name:               item.Name,
						Parent:             nil,
						TranslatedISOCodes: []string{"en-US"},
					})
				}
				sort.Slice(overview, func(i, j int) bool {
					return overview[i].Name < overview[j].Name
				})
				payload, err := json.Marshal(dictionaryListResponse{
					Total: len(overview),
					Items: overview,
				})
				if err != nil {
					t.Fatalf("failed to marshal overview payload: %v", err)
				}
				return dictionaryJSONResponse(http.StatusOK, string(payload)), nil
			case http.MethodPost:
				postRequests++
				var payload dictionaryCreateRequest
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("failed to decode create payload: %v", err)
				}
				items[payload.Name] = storedDictionaryItem{
					ID:           payload.ID,
					Name:         payload.Name,
					Translations: payload.Translations,
				}
				return dictionaryJSONResponse(http.StatusCreated, `{}`), nil
			default:
				return dictionaryJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
		default:
			return dictionaryJSONResponse(http.StatusNotFound, `{"error":"unexpected request"}`), nil
		}
	})

	firstRun, err := cmdtest.Execute(
		buildRootWithCollections(t, deps),
		"dictionary", "import",
		"--file", file,
	)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	secondRun, err := cmdtest.Execute(
		buildRootWithCollections(t, deps),
		"dictionary", "import",
		"--file", file,
	)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	var firstResult dictionaryImportResult
	if err := json.Unmarshal([]byte(firstRun), &firstResult); err != nil {
		t.Fatalf("failed to parse first import result: %v", err)
	}
	if firstResult.Created != 2 || firstResult.Skipped != 0 || firstResult.Failed != 0 {
		t.Fatalf("unexpected first import summary: %+v", firstResult)
	}

	var secondResult dictionaryImportResult
	if err := json.Unmarshal([]byte(secondRun), &secondResult); err != nil {
		t.Fatalf("failed to parse second import result: %v", err)
	}
	if secondResult.Created != 0 || secondResult.Skipped != 2 || secondResult.Failed != 0 {
		t.Fatalf("unexpected second import summary: %+v", secondResult)
	}
	if postRequests != 2 {
		t.Fatalf("expected two create requests across both imports, got %d", postRequests)
	}
}

func TestDictionaryImportMergesDuplicateKeysIntoSingleCreate(t *testing.T) {
	file := writeDictionaryImportFixture(t, `[
  {"key":"Pricing.AddonsSubtotal","translations":{"en-US":"Add-ons subtotal"}},
  {"key":"Pricing.AddonsSubtotal","translations":{"de-DE":"Zusatzkosten Zwischensumme"}},
  {"key":"Pricing.AddonsSubtotal","translations":{"es-ES":"Subtotal de complementos"}}
]`)

	var mu sync.Mutex
	postRequests := 0
	var createdPayload dictionaryCreateRequest

	deps := dictionaryDeps(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return dictionaryJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/dictionary":
			switch req.Method {
			case http.MethodGet:
				return dictionaryJSONResponse(http.StatusOK, `{"total":0,"items":[]}`), nil
			case http.MethodPost:
				postRequests++
				if err := json.NewDecoder(req.Body).Decode(&createdPayload); err != nil {
					t.Fatalf("failed to decode create payload: %v", err)
				}
				return dictionaryJSONResponse(http.StatusCreated, `{}`), nil
			default:
				return dictionaryJSONResponse(http.StatusMethodNotAllowed, `{"error":"method not allowed"}`), nil
			}
		default:
			return dictionaryJSONResponse(http.StatusNotFound, `{"error":"unexpected request"}`), nil
		}
	})

	output, err := cmdtest.Execute(
		buildRootWithCollections(t, deps),
		"dictionary", "import",
		"--file", file,
	)
	if err != nil {
		t.Fatalf("dictionary import failed: %v", err)
	}

	var result dictionaryImportResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse import result: %v", err)
	}

	if result.Created != 1 || result.Failed != 0 {
		t.Fatalf("unexpected import summary: %+v", result)
	}
	if postRequests != 1 {
		t.Fatalf("expected exactly one create request, got %d", postRequests)
	}
	if createdPayload.Name != "Pricing.AddonsSubtotal" {
		t.Fatalf("unexpected create payload name: %+v", createdPayload)
	}
	if len(createdPayload.Translations) != 3 {
		t.Fatalf("expected 3 translations in single create payload, got %+v", createdPayload.Translations)
	}
}
