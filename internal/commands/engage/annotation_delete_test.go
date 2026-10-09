package engage

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

// fakeAnnotationStore mimics Engage 18.1.0's annotation routes: the list
// returns stored rows that are not invalid plus generated A/B test entries
// with id 0, answers 500 without both ends of the range, and DELETE marks a
// row invalid and answers 200 whether or not the id exists.
type fakeAnnotationStore struct {
	t *testing.T
	// invalid maps each stored annotation id to its invalid flag.
	invalid map[int64]bool
	// ignoreDelete makes DELETE answer 200 without changing anything.
	ignoreDelete bool
}

func (s *fakeAnnotationStore) routes() map[string]func(*http.Request) *http.Response {
	return map[string]func(*http.Request) *http.Response{
		"/annotations/all": func(req *http.Request) *http.Response {
			query := req.URL.Query()
			if query.Get("from") != engageAnnotationEarliest || query.Get("to") != engageAnnotationLatest {
				s.t.Fatalf("lookup must list the widest range Engage accepts, got %s", req.URL.RawQuery)
			}
			entries := []map[string]any{
				{"id": 0, "description": `A/B Test "CTA" started`, "visibility": "AbTestStart", "invalid": false, "timestamp": "2026-04-24T05:53:49.88Z"},
			}
			for id, invalid := range s.invalid {
				if !invalid {
					entries = append(entries, map[string]any{"id": id, "description": "Campaign launch", "visibility": "Always", "invalid": false, "timestamp": "2026-09-01T00:00:00Z"})
				}
			}
			raw, _ := json.Marshal(entries)
			return cmdtest.JSONResponse(http.StatusOK, string(raw))
		},
		"/annotations": func(req *http.Request) *http.Response {
			if req.Method != http.MethodDelete {
				s.t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
			}
			if !s.ignoreDelete {
				for id := range s.invalid {
					if req.URL.Query().Get("id") == jsonNumber(id) {
						s.invalid[id] = true
					}
				}
			}
			return cmdtest.JSONResponse(http.StatusOK, ``)
		},
	}
}

func jsonNumber(id int64) string {
	raw, _ := json.Marshal(id)
	return string(raw)
}

const engageAnnotationLookup = "GET " + engageAPIPrefix + "/annotations/all?from=1753-01-02T00%3A00%3A00Z&to=9999-12-30T23%3A59%3A59Z"

func exitCodeOf(err error) int {
	if coder, ok := err.(interface{ ExitCode() int }); ok {
		return coder.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

func TestEngageAnnotationDeleteRefusesGeneratedIDsWithoutRequest(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false}}
	deps, requests := engageTestDeps(t, store.routes())
	for _, args := range [][]string{
		{"0", "--force"},
		{"--force", "--", "-3"},
		{"0", "--dry-run"},
	} {
		_, err := cmdtest.Execute(buildEngageRoot(t, deps), append([]string{"engage", "annotation", "delete"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "is not a stored annotation") || !strings.Contains(err.Error(), "list with `id` 0 and cannot be deleted") {
			t.Fatalf("%v: expected the generated-annotation refusal, got %v", args, err)
		}
		if code := exitCodeOf(err); code != 1 {
			t.Fatalf("%v: a refused id is a usage error (exit 1), got exit %d", args, code)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("a refused id must not reach the API, got %v", *requests)
	}
}

func TestEngageAnnotationDeleteUnknownIDSendsNoDelete(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false, 22: true}}
	deps, requests := engageTestDeps(t, store.routes())
	for _, args := range [][]string{{"99", "--force"}, {"22", "--force"}, {"99", "--dry-run"}} {
		_, err := cmdtest.Execute(buildEngageRoot(t, deps), append([]string{"engage", "annotation", "delete"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "no stored annotation with id "+args[0]) {
			t.Fatalf("%v: expected the not-found error, got %v", args, err)
		}
		if code := exitCodeOf(err); code != 4 {
			t.Fatalf("%v: expected exit 4, got %d", args, code)
		}
	}
	engageRequestsEqual(t, *requests, engageAnnotationLookup, engageAnnotationLookup, engageAnnotationLookup)
	if store.invalid[21] {
		t.Fatalf("annotation 21 must be untouched")
	}
}

func TestEngageAnnotationDeleteHidesAndConfirms(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false, 30: false}}
	deps, requests := engageTestDeps(t, store.routes())
	output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "annotation", "delete", "21", "--force")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	engageRequestsEqual(t, *requests, engageAnnotationLookup, "DELETE "+engageAPIPrefix+"/annotations?id=21", engageAnnotationLookup)
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil || result["deleted"] != true || result["id"] != float64(21) {
		t.Fatalf("expected {deleted: true, id: 21}, got %s (%v)", output, err)
	}
	if !store.invalid[21] || store.invalid[30] {
		t.Fatalf("expected only 21 hidden, got %v", store.invalid)
	}
}

func TestEngageAnnotationDeleteErrorsWhenStillListed(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false}, ignoreDelete: true}
	deps, requests := engageTestDeps(t, store.routes())
	output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "annotation", "delete", "21", "--force")
	if err == nil || !strings.Contains(err.Error(), "Engage answered the DELETE for annotation 21, but it is still listed") {
		t.Fatalf("expected the still-listed error, got %v", err)
	}
	if code := exitCodeOf(err); code != 4 {
		t.Fatalf("expected exit 4, got %d", code)
	}
	if strings.Contains(output, `"deleted"`) {
		t.Fatalf("nothing may report success, got %s", output)
	}
	engageRequestsEqual(t, *requests, engageAnnotationLookup, "DELETE "+engageAPIPrefix+"/annotations?id=21", engageAnnotationLookup)
}

func TestEngageAnnotationDeleteDryRunLooksUpAndPlans(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false}}
	deps, requests := engageTestDeps(t, store.routes())
	output, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "annotation", "delete", "21", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	engageRequestsEqual(t, *requests, engageAnnotationLookup)
	if !strings.Contains(output, `"dryRun": true`) || !strings.Contains(output, `"method": "DELETE"`) || !strings.Contains(output, engageAPIPrefix+"/annotations?id=21") {
		t.Fatalf("expected the planned DELETE, got %s", output)
	}
	if store.invalid[21] {
		t.Fatalf("dry-run must not hide the annotation")
	}
}

func TestEngageAnnotationDeleteNeedsForceBeforeAnyRequest(t *testing.T) {
	store := &fakeAnnotationStore{t: t, invalid: map[int64]bool{21: false}}
	deps, requests := engageTestDeps(t, store.routes())
	_, err := cmdtest.Execute(buildEngageRoot(t, deps), "engage", "annotation", "delete", "21")
	if err == nil || !strings.Contains(err.Error(), "hides (soft-deletes) the annotation; pass --force to confirm or --dry-run to rehearse") {
		t.Fatalf("expected the force gate, got %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("the force gate must come before the lookup, got %v", *requests)
	}
}

func TestEngageAnnotationHelpDescribesSoftDeleteAndGeneratedIDs(t *testing.T) {
	help := func(args ...string) string {
		t.Helper()
		output, err := cmdtest.Execute(buildEngageRoot(t, cmdtest.MakeDeps()), append(args, "--help")...)
		if err != nil {
			t.Fatalf("%v --help failed: %v", args, err)
		}
		return strings.Join(strings.Fields(output), " ")
	}
	remove := help("engage", "annotation", "delete")
	if group := help("engage", "annotation"); !strings.Contains(group, "Hide (soft-delete) a stored annotation by its numeric `id`") {
		t.Fatalf("annotation group help lacks the delete summary:\n%s", group)
	}
	for _, want := range []string{
		"marks the annotation invalid, so it is no longer listed, but keeps its row",
		"list with `id` 0 and cannot be deleted",
		"reports success only when the annotation is gone",
		"--dry-run runs the lookup and prints the planned DELETE",
	} {
		if !strings.Contains(remove, want) {
			t.Fatalf("annotation delete help lacks %q:\n%s", want, remove)
		}
	}
	if strings.Contains(remove, "Permanently") || strings.Contains(remove, "permanent") {
		t.Fatalf("annotation delete is a soft delete, not permanent:\n%s", remove)
	}
	if list := help("engage", "annotation", "list"); !strings.Contains(list, "those list with `id` 0 and cannot be deleted") {
		t.Fatalf("annotation list help lacks the id 0 note:\n%s", list)
	}
}
