package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Tests for the v0.4.0 gap areas: webhooks, languages, users, user groups,
// document versions, and document lifecycle. Each test pins the route and
// body shape the Management API expects.

func tokenOr404(t *testing.T, req *http.Request, handler func(req *http.Request) (*http.Response, error)) (*http.Response, error) {
	t.Helper()
	if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
		return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
	}
	return handler(req)
}

func TestWebhookLogsScopesToWebhookWhenIDGiven(t *testing.T) {
	var observedPath string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observedPath = req.URL.Path
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "webhook", "logs", "hook-1"); err != nil {
		t.Fatalf("webhook logs failed: %v", err)
	}
	if observedPath != "/umbraco/management/api/v1/webhook/hook-1/logs" {
		t.Fatalf("expected per-webhook logs route, got %q", observedPath)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "webhook", "logs"); err != nil {
		t.Fatalf("webhook logs (global) failed: %v", err)
	}
	if observedPath != "/umbraco/management/api/v1/webhook/logs" {
		t.Fatalf("expected global logs route, got %q", observedPath)
	}
}

func TestLanguageCreateBuildsBodyFromFlags(t *testing.T) {
	var observedBody map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/umbraco/management/api/v1/language" && req.Method == http.MethodPost {
				if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				return endpointJSONResponse(http.StatusCreated, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"language", "create", "--iso-code", "da-DK", "--name", "Danish", "--mandatory", "--fallback", "en-US")
	if err != nil {
		t.Fatalf("language create failed: %v", err)
	}
	if observedBody["isoCode"] != "da-DK" || observedBody["name"] != "Danish" {
		t.Fatalf("unexpected body: %+v", observedBody)
	}
	if observedBody["isMandatory"] != true || observedBody["isDefault"] != false {
		t.Fatalf("expected mandatory=true default=false, got %+v", observedBody)
	}
	if observedBody["fallbackIsoCode"] != "en-US" {
		t.Fatalf("expected fallback, got %+v", observedBody)
	}
	if !strings.Contains(output, `"created": true`) {
		t.Fatalf("expected created coalescing, got %s", output)
	}
}

func TestLanguageUpdateMergeStripsIsoCodeEchoedByFetch(t *testing.T) {
	var observedBody map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/management/api/v1/language/da-DK" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{"isoCode":"da-DK","name":"Danish","isDefault":false,"isMandatory":false}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"language", "update", "da-DK", "--merge-json", `{"isMandatory":true}`); err != nil {
		t.Fatalf("language update failed: %v", err)
	}
	if _, present := observedBody["isoCode"]; present {
		t.Fatalf("isoCode must be stripped from the update body (the model rejects it), got %+v", observedBody)
	}
	if observedBody["isMandatory"] != true || observedBody["name"] != "Danish" {
		t.Fatalf("expected merged body preserving name, got %+v", observedBody)
	}
}

func TestDocumentVersionCommandsHitVersionRoutes(t *testing.T) {
	var observedPath, observedQuery, observedMethod string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observedPath, observedQuery, observedMethod = req.URL.Path, req.URL.RawQuery, req.Method
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "version", "list", "doc-1", "--culture", "en-US"); err != nil {
		t.Fatalf("version list failed: %v", err)
	}
	if observedPath != "/umbraco/management/api/v1/document-version" || !strings.Contains(observedQuery, "documentId=doc-1") || !strings.Contains(observedQuery, "culture=en-US") {
		t.Fatalf("unexpected version list request: %s?%s", observedPath, observedQuery)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "document", "version", "rollback", "ver-1", "--culture", "da-DK"); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if observedMethod != http.MethodPost || observedPath != "/umbraco/management/api/v1/document-version/ver-1/rollback" || observedQuery != "culture=da-DK" {
		t.Fatalf("unexpected rollback request: %s %s?%s", observedMethod, observedPath, observedQuery)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "document", "version", "prevent-cleanup", "ver-1", "--disable"); err != nil {
		t.Fatalf("prevent-cleanup failed: %v", err)
	}
	if observedMethod != http.MethodPut || observedQuery != "preventCleanup=false" {
		t.Fatalf("unexpected prevent-cleanup request: %s %s?%s", observedMethod, observedPath, observedQuery)
	}
}

func TestDocumentSortBuildsSortingFromOrderedIDs(t *testing.T) {
	var observedBody map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/umbraco/management/api/v1/document/sort" && req.Method == http.MethodPut {
				if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"document", "sort", "--parent", "parent-1", "--ids", "b-2,a-1"); err != nil {
		t.Fatalf("document sort failed: %v", err)
	}
	sorting, ok := observedBody["sorting"].([]any)
	if !ok || len(sorting) != 2 {
		t.Fatalf("expected two sorting entries, got %+v", observedBody)
	}
	first := sorting[0].(map[string]any)
	if first["id"] != "b-2" || first["sortOrder"] != float64(0) {
		t.Fatalf("expected positional sortOrder, got %+v", first)
	}
	parent, ok := observedBody["parent"].(map[string]any)
	if !ok || parent["id"] != "parent-1" {
		t.Fatalf("expected parent ref, got %+v", observedBody["parent"])
	}
}

func TestDocumentPublicAccessSetCreatesWhenAbsentAndReplacesWhenPresent(t *testing.T) {
	var observedMethod string
	exists := false
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/management/api/v1/document/doc-1/public-access" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			if req.Method == http.MethodGet {
				if exists {
					return endpointJSONResponse(http.StatusOK, `{"memberGroupNames":["Members"]}`), nil
				}
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			observedMethod = req.Method
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	payload := `{"loginDocument":{"id":"l"},"errorDocument":{"id":"e"},"memberGroupNames":["Members"],"memberUserNames":[]}`

	if _, err := execute(buildRootWithCollections(t, deps), "document", "public-access", "set", "doc-1", "--json", payload); err != nil {
		t.Fatalf("public-access set (create) failed: %v", err)
	}
	if observedMethod != http.MethodPost {
		t.Fatalf("expected POST when no rules exist, got %s", observedMethod)
	}

	exists = true
	if _, err := execute(buildRootWithCollections(t, deps), "document", "public-access", "set", "doc-1", "--json", payload); err != nil {
		t.Fatalf("public-access set (replace) failed: %v", err)
	}
	if observedMethod != http.MethodPut {
		t.Fatalf("expected PUT when rules exist, got %s", observedMethod)
	}
}

func TestUserEnableSendsReferenceShapedIDs(t *testing.T) {
	var observedBody map[string]any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/umbraco/management/api/v1/user/enable" && req.Method == http.MethodPost {
				if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user", "enable", "--ids", "u-1,u-2"); err != nil {
		t.Fatalf("user enable failed: %v", err)
	}
	userIDs, ok := observedBody["userIds"].([]any)
	if !ok || len(userIDs) != 2 {
		t.Fatalf("expected two userIds refs, got %+v", observedBody)
	}
	if ref := userIDs[0].(map[string]any); ref["id"] != "u-1" {
		t.Fatalf("expected {id} reference shape, got %+v", userIDs[0])
	}
}

func TestUserGroupMembershipSendsArrayBody(t *testing.T) {
	var observedMethod string
	var observedBody []any
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/umbraco/management/api/v1/user-group/g-1/users" {
				observedMethod = req.Method
				if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				return endpointJSONResponse(http.StatusOK, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user-group", "add-users", "g-1", "--ids", "u-1"); err != nil {
		t.Fatalf("add-users failed: %v", err)
	}
	if observedMethod != http.MethodPost || len(observedBody) != 1 {
		t.Fatalf("unexpected add-users request: %s %+v", observedMethod, observedBody)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "user-group", "remove-users", "g-1", "--ids", "u-1"); err != nil {
		t.Fatalf("remove-users failed: %v", err)
	}
	if observedMethod != http.MethodDelete {
		t.Fatalf("expected DELETE for remove-users, got %s", observedMethod)
	}
	if ref := observedBody[0].(map[string]any); ref["id"] != "u-1" {
		t.Fatalf("expected {id} reference shape, got %+v", observedBody[0])
	}
}

func TestDocumentTrashPrefersModernMethodAndFallsBack(t *testing.T) {
	var methods []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/umbraco/management/api/v1/document/doc-1/move-to-recycle-bin" {
				methods = append(methods, req.Method)
				if req.Method == http.MethodPut {
					return endpointJSONResponse(http.StatusNotFound, `null`), nil
				}
				return endpointJSONResponse(http.StatusOK, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "trash", "doc-1")
	if err != nil {
		t.Fatalf("document trash failed: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodPost {
		t.Fatalf("expected PUT-then-POST fallback, got %v", methods)
	}
	if !strings.Contains(output, `"trashed": true`) {
		t.Fatalf("expected trashed coalescing, got %s", output)
	}
}

func TestDocumentRestoreUsesRecycleBinRouteFirst(t *testing.T) {
	var observed []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "document", "restore", "doc-1"); err != nil {
		t.Fatalf("document restore failed: %v", err)
	}
	want := []string{
		"GET /umbraco/management/api/v1/recycle-bin/document/doc-1/original-parent",
		"PUT /umbraco/management/api/v1/recycle-bin/document/doc-1/restore",
	}
	if len(observed) != 2 || observed[0] != want[0] || observed[1] != want[1] {
		t.Fatalf("expected original-parent lookup then modern restore route, got %v", observed)
	}
}

func TestDocumentRestoreFallsBackToLegacyWhenRecycleBinAPIAbsent(t *testing.T) {
	// Older servers have neither the original-parent lookup nor the modern
	// restore route; the command must still reach the legacy POST.
	var observed []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			if req.URL.Path == "/umbraco/management/api/v1/document/doc-1/restore" && req.Method == http.MethodPost {
				return endpointJSONResponse(http.StatusOK, ``), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps), "document", "restore", "doc-1")
	if err != nil {
		t.Fatalf("document restore failed on legacy server: %v", err)
	}
	want := []string{
		"GET /umbraco/management/api/v1/recycle-bin/document/doc-1/original-parent",
		"PUT /umbraco/management/api/v1/recycle-bin/document/doc-1/restore",
		"POST /umbraco/management/api/v1/document/doc-1/restore",
	}
	if len(observed) != 3 || observed[0] != want[0] || observed[1] != want[1] || observed[2] != want[2] {
		t.Fatalf("expected lookup then modern-then-legacy restore, got %v", observed)
	}
	if !strings.Contains(output, `"restored": true`) {
		t.Fatalf("expected restored coalescing, got %s", output)
	}
}

func TestDocumentUpdateSurfacesFetchErrorsUnmasked(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			return endpointJSONResponse(http.StatusInternalServerError, `{"title":"boom"}`), nil
		})
	})

	_, err := execute(buildRootWithCollections(t, deps), "document", "update", "doc-1", "--merge-json", `{"values":[]}`)
	if err == nil {
		t.Fatalf("expected fetch failure to propagate")
	}
	if strings.Contains(err.Error(), "requires exactly one of") {
		t.Fatalf("fetch failure must not be masked as a flag-validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "API 500") {
		t.Fatalf("expected the real API error, got: %v", err)
	}
}

func TestUserPermissionsSelectsSurfaceByType(t *testing.T) {
	var observedPath string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observedPath = req.URL.Path
			return endpointJSONResponse(http.StatusOK, `{"permissions":[]}`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps), "user", "permissions", "--ids", "n-1", "--type", "document"); err != nil {
		t.Fatalf("user permissions failed: %v", err)
	}
	if observedPath != "/umbraco/management/api/v1/user/current/permissions/document" {
		t.Fatalf("expected document permission surface, got %q", observedPath)
	}

	if _, err := execute(buildRootWithCollections(t, deps), "user", "permissions", "--ids", "n-1", "--type", "bogus"); err == nil {
		t.Fatalf("expected invalid --type to fail")
	}
}
