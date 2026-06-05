package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const memberID = "mem-1"
const memberAPIPath = "/umbraco/management/api/v1/member/mem-1"
const memberFilterAPIPath = "/umbraco/management/api/v1/filter/member"

// currentMemberPayload is the GET fixture used across member-mutation tests.
// Includes a custom values[] entry plus two existing groups so tests can
// verify the merge preserves untouched data.
func currentMemberPayload() string {
	return `{
		"id":"mem-1",
		"username":"test-member@example.invalid",
		"email":"test-member@example.invalid",
		"memberType":{"id":"mt-1"},
		"isApproved":false,
		"isLockedOut":true,
		"failedPasswordAttempts":5,
		"groups":["grp-a","grp-b"],
		"values":[{"alias":"company","value":"Acme","culture":null,"segment":null}]
	}`
}

func mockMemberMutations(t *testing.T) (deps Dependencies, captured *map[string]any) {
	t.Helper()
	put := map[string]any{}
	deps = endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case memberAPIPath:
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentMemberPayload()), nil
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

func TestMemberListPassesFilterAndPaginationToFilterEndpoint(t *testing.T) {
	var observedQuery string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case memberFilterAPIPath:
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"mem-1","username":"test-member@example.invalid","email":"test-member@example.invalid","isApproved":true}],"total":1}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "member", "list", "--filter", "testq", "--take", "5"); err != nil {
		t.Fatalf("member list failed: %v", err)
	}
	for _, want := range []string{"filter=testq", "take=5"} {
		if !strings.Contains(observedQuery, want) {
			t.Fatalf("expected query to contain %q, got %q", want, observedQuery)
		}
	}
}

func TestMemberSearchUsesFilterAsPositionalArg(t *testing.T) {
	var observedQuery string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case memberFilterAPIPath:
			observedQuery = req.URL.RawQuery
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	if _, err := execute(buildRootWithCollections(t, deps), "member", "search", "testq"); err != nil {
		t.Fatalf("member search failed: %v", err)
	}
	if !strings.Contains(observedQuery, "filter=testq") {
		t.Fatalf("expected positional arg to map to filter=, got %q", observedQuery)
	}
}

func TestMemberSetGroupsRequiresExactlyOneMode(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(buildRootWithCollections(t, deps), "member", "set-groups", memberID, "--groups", "a", "--add-groups", "b")
	if err == nil {
		t.Fatalf("expected rejection when multiple mode flags supplied")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemberSetGroupsReplaceProducesNewArray(t *testing.T) {
	deps, captured := mockMemberMutations(t)
	if _, err := execute(buildRootWithCollections(t, deps), "member", "set-groups", memberID, "--groups", "grp-x,grp-y"); err != nil {
		t.Fatalf("set-groups --groups failed: %v", err)
	}
	groups, _ := (*captured)["groups"].([]any)
	if len(groups) != 2 || groups[0] != "grp-x" || groups[1] != "grp-y" {
		t.Fatalf("expected groups replaced with [grp-x,grp-y], got %+v", groups)
	}
}

func TestMemberSetGroupsAddIsIdempotent(t *testing.T) {
	var putCount int32
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case memberAPIPath:
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, currentMemberPayload()), nil
			}
			if req.Method == http.MethodPut {
				atomic.AddInt32(&putCount, 1)
				return endpointNoContent(), nil
			}
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	// Adding a group already on the member should be a no-op.
	output, err := execute(buildRootWithCollections(t, deps), "member", "set-groups", memberID, "--add-groups", "grp-a")
	if err != nil {
		t.Fatalf("idempotent set-groups failed: %v", err)
	}
	if atomic.LoadInt32(&putCount) != 0 {
		t.Fatalf("expected zero PUTs for no-op, got %d", putCount)
	}
	var summary map[string]any
	_ = json.Unmarshal([]byte(output), &summary)
	if summary["changed"] != false {
		t.Fatalf("expected changed=false, got %+v", summary)
	}
}

func TestMemberSetGroupsRemoveSubtracts(t *testing.T) {
	deps, captured := mockMemberMutations(t)
	if _, err := execute(buildRootWithCollections(t, deps), "member", "set-groups", memberID, "--remove-groups", "grp-a"); err != nil {
		t.Fatalf("set-groups --remove-groups failed: %v", err)
	}
	groups, _ := (*captured)["groups"].([]any)
	if len(groups) != 1 || groups[0] != "grp-b" {
		t.Fatalf("expected only grp-b to remain, got %+v", groups)
	}
}

func TestMemberUpdatePropertiesReusesValuesParser(t *testing.T) {
	// Smoke-test that member update-properties picks up the same three-shape
	// parser as document update-properties. Object form → values[] entries.
	deps, captured := mockMemberMutations(t)
	if _, err := execute(
		buildRootWithCollections(t, deps),
		"member", "update-properties", memberID,
		"--json", `{"phone":"+1-555","industry":"saas"}`,
	); err != nil {
		t.Fatalf("update-properties failed: %v", err)
	}
	put := *captured
	for _, leakedKey := range []string{"phone", "industry"} {
		if _, leaked := put[leakedKey]; leaked {
			t.Fatalf("property %q leaked to top-level — same regression as document update-properties", leakedKey)
		}
	}
	values, _ := put["values"].([]any)
	got := map[string]any{}
	for _, v := range values {
		entry := v.(map[string]any)
		got[entry["alias"].(string)] = entry["value"]
	}
	if got["phone"] != "+1-555" || got["industry"] != "saas" || got["company"] != "Acme" {
		t.Fatalf("expected merged values[] with all three entries, got %+v", got)
	}
}

func TestMemberUpdateRefusesAmbiguousFlags(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(buildRootWithCollections(t, deps), "member", "update", memberID, "--json", `{}`, "--merge-json", `{}`)
	if err == nil {
		t.Fatalf("expected error when both --json and --merge-json passed")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemberGroupListFallsBackToTreeRoot(t *testing.T) {
	var observedPaths []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/member-group":
			observedPaths = append(observedPaths, req.URL.Path)
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		case "/umbraco/management/api/v1/tree/member-group/root":
			observedPaths = append(observedPaths, req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"items":[{"id":"grp-a","name":"Gold"}]}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), "member-group", "list"); err != nil {
		t.Fatalf("member-group list failed: %v", err)
	}
	if len(observedPaths) != 2 {
		t.Fatalf("expected fallback to /tree/member-group/root after /member-group 404, got %v", observedPaths)
	}
}

// Codex re-review caught that 'member update' would still produce
// {"updated":true} for a patch containing isApproved / isLockedOut /
// failedPasswordAttempts / isTwoFactorEnabled — the Management API
// accepts the PUT (204) but doesn't change the field, so the CLI's
// help text was the only safeguard. Now reject those keys up front.
func TestMemberUpdateRejectsReadOnlyFields(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case memberAPIPath:
			t.Fatalf("read-only-field patch must be rejected before any HTTP call")
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	for _, field := range []string{"isApproved", "isLockedOut", "failedPasswordAttempts", "isTwoFactorEnabled"} {
		_, err := execute(buildRootWithCollections(t, deps), "member", "update", memberID, "--merge-json", `{"`+field+`":true}`)
		if err == nil {
			t.Fatalf("expected --merge-json with %q to be rejected", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("error must name the offending field %q, got: %v", field, err)
		}
	}
}

// Patches without any read-only field still work — sanity check that the
// rejection isn't catching everything.
func TestMemberUpdateAllowsLegitimateFields(t *testing.T) {
	deps, _ := mockMemberMutations(t)
	if _, err := execute(buildRootWithCollections(t, deps), "member", "update", memberID, "--merge-json", `{"email":"new@example.invalid"}`); err != nil {
		t.Fatalf("legitimate update must pass the read-only gate: %v", err)
	}
}

// Symmetric guard: 'member create --json' must reject the same read-only
// fields that 'member update' does. Otherwise an agent that passes
// isApproved=true at create time gets a successful create with the
// server-side default (false), which is the same false-positive shape
// the update-side gate was added to prevent.
func TestMemberCreateRejectsReadOnlyFields(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/umbraco/management/api/v1/member":
			t.Fatalf("read-only-field create payload must be rejected before any HTTP call")
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	for _, field := range []string{"isApproved", "isLockedOut", "failedPasswordAttempts", "isTwoFactorEnabled"} {
		_, err := execute(
			buildRootWithCollections(t, deps),
			"member", "create",
			"--json", `{"email":"x@example.invalid","username":"x","password":"P@ss!123","memberType":{"id":"mt-1"},"`+field+`":true}`,
		)
		if err == nil {
			t.Fatalf("expected create with %q to be rejected", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("error must name the offending field %q, got: %v", field, err)
		}
	}
}
