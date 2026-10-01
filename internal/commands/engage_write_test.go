package commands

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/api"
)

const engageTestGUID2 = "0b8f6c2d-7e1a-4c3b-8d9e-2f1a0b3c4d5e"

// engageBodies records the decoded JSON bodies sent to the Engage routes.
type engageBodies struct{ sent []any }

func (b *engageBodies) last(t *testing.T) map[string]any {
	t.Helper()
	if len(b.sent) == 0 {
		t.Fatalf("no body was sent")
	}
	object, ok := b.sent[len(b.sent)-1].(map[string]any)
	if !ok {
		t.Fatalf("expected an object body, got %T", b.sent[len(b.sent)-1])
	}
	return object
}

// engageByMethod dispatches one Engage path by HTTP method, recording any
// request body into bodies.
func engageByMethod(t *testing.T, bodies *engageBodies, handlers map[string]func(*http.Request) *http.Response) func(*http.Request) *http.Response {
	return func(req *http.Request) *http.Response {
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			if len(raw) > 0 {
				var decoded any
				if err := json.Unmarshal(raw, &decoded); err != nil {
					t.Fatalf("body is not JSON: %v\n%s", err, raw)
				}
				bodies.sent = append(bodies.sent, decoded)
			}
		}
		handler, ok := handlers[req.Method]
		if !ok {
			t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
		}
		return handler(req)
	}
}

func engageRequestsEqual(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestEngageCreateMergesScaffoldAndGeneratesGUIDs(t *testing.T) {
	bodies := &engageBodies{}
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/persona": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodPost: engageJSON(http.StatusOK, `{"persona":{"id":11,"unique":"`+engageTestGUID+`","title":"Group"},"validationResults":{"isValid":true,"warnings":[],"errors":[]}}`),
		}),
	})
	output, err := execute(buildRootWithCollections(t, deps), "engage", "persona", "create", "--json", `{"title":"Group","personas":[{"title":"Persona A"}]}`)
	if err != nil {
		t.Fatalf("persona create failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "POST "+engageAPIPrefix+"/persona")
	body := bodies.last(t)
	if body["id"] != float64(0) || body["title"] != "Group" || body["description"] != "" {
		t.Fatalf("expected id 0 and --json merged onto the scaffold, got %+v", body)
	}
	if unique, _ := body["unique"].(string); !isUUIDLike(unique) {
		t.Fatalf("expected a generated GUID `unique`, got %v", body["unique"])
	}
	personas, _ := body["personas"].([]any)
	if len(personas) != 1 {
		t.Fatalf("expected the one persona, got %+v", body["personas"])
	}
	if unique, _ := personas[0].(map[string]any)["unique"].(string); !isUUIDLike(unique) {
		t.Fatalf("expected a generated GUID on the nested persona, got %+v", personas[0])
	}
	if !strings.Contains(output, `"validationResults"`) {
		t.Fatalf("expected the save response printed, got %s", output)
	}
}

func TestEngageCreateRejectsExistingIDAndDryRunSendsNothing(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)
	_, err := execute(buildRootWithCollections(t, deps), "engage", "segment", "create", "--json", `{"id":7,"name":"Returning"}`)
	if err == nil || !strings.Contains(err.Error(), "engage segment update") {
		t.Fatalf("expected a non-zero id to be rejected with a pointer to update, got %v", err)
	}
	_, err = execute(buildRootWithCollections(t, deps), "engage", "segment", "create", "--json", `{"name":"Mobile","controlGroupSize":20}`)
	if err == nil || !strings.Contains(err.Error(), "fraction") {
		t.Fatalf("expected a percentage controlGroupSize to be rejected, got %v", err)
	}
	output, err := execute(buildRootWithCollections(t, deps), "engage", "segment", "create", "--json", `{"name":"Mobile","rules":[{"type":"Device","config":{}}]}`, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("create checks and --dry-run must send nothing, got %v", *requests)
	}
	var plan struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Body   map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(output), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, output)
	}
	if plan.Method != http.MethodPost || plan.Path != engageAPIPrefix+"/segments" || plan.Body["controlGroupSize"] != 0.2 || plan.Body["isTemporary"] != false {
		t.Fatalf("expected a POST /segments plan with the scaffold defaults, got %+v", plan)
	}
	rules, _ := plan.Body["rules"].([]any)
	if len(rules) != 1 || !isUUIDLike(rules[0].(map[string]any)["unique"].(string)) {
		t.Fatalf("expected the rule to get a GUID, got %+v", plan.Body["rules"])
	}
}

func TestEngagePrintTemplateUsesEmptyRouteOrScaffold(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/customer-journey/empty": engageJSON(http.StatusOK, `{"id":0,"unique":"`+engageTestGUID+`","steps":[]}`),
	})
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "journey", "create", "--print-template"); err != nil {
		t.Fatalf("journey template failed: %v", err)
	}
	output, err := execute(buildRootWithCollections(t, deps), "engage", "campaign-group", "create", "--print-template")
	if err != nil {
		t.Fatalf("campaign-group template failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "GET "+engageAPIPrefix+"/customer-journey/empty")
	if !strings.Contains(output, `"campaigns"`) || !strings.Contains(output, `"personaScoring"`) {
		t.Fatalf("expected the local campaign-group scaffold, got %s", output)
	}
}

func TestEngageUpdateMergeFetchesThenPostsWithPinnedIdentity(t *testing.T) {
	bodies := &engageBodies{}
	current := `{"id":7,"unique":"` + engageTestGUID + `","name":"Returning","isTemporary":false,"sortOrder":2,"controlGroupSize":0.1,"rules":[{"id":3,"unique":"` + engageTestGUID2 + `","type":"Visits"}]}`
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/segments": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodGet:  engageJSON(http.StatusOK, current),
			http.MethodPost: engageJSON(http.StatusOK, current),
		}),
	})
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "segment", "update", engageTestGUID, "--merge-json", `{"name":"Returning visitors"}`); err != nil {
		t.Fatalf("segment update failed: %v", err)
	}
	engageRequestsEqual(t, *requests,
		"GET "+engageAPIPrefix+"/segments?id="+engageTestGUID,
		"POST "+engageAPIPrefix+"/segments",
	)
	body := bodies.last(t)
	if body["id"] != float64(7) || body["unique"] != engageTestGUID || body["name"] != "Returning visitors" || body["sortOrder"] != float64(2) {
		t.Fatalf("expected the fetched segment with the patch applied, got %+v", body)
	}
	if rules, _ := body["rules"].([]any); len(rules) != 1 {
		t.Fatalf("expected the unmentioned rules preserved, got %+v", body["rules"])
	}
}

func TestEngageUpdateJSONPinsServerIDAndRejectsMismatch(t *testing.T) {
	bodies := &engageBodies{}
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/traffic-filter": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodGet:  engageJSON(http.StatusOK, `{"id":4,"key":"`+engageTestGUID+`","name":"Office"}`),
			http.MethodPost: engageJSON(http.StatusOK, `"`+engageTestGUID+`"`),
		}),
	})
	output, err := execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "update", engageTestGUID, "--json", `{"name":"HQ","mode":"Filter","values":["10.0.0.0/8"]}`)
	if err != nil {
		t.Fatalf("traffic-filter update failed: %v", err)
	}
	body := bodies.last(t)
	if body["id"] != float64(4) || body["key"] != engageTestGUID || body["name"] != "HQ" {
		t.Fatalf("expected --json with the server id and key pinned, got %+v", body)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil || result["updated"] != true || result["key"] != engageTestGUID {
		t.Fatalf("expected the bare GUID answer as {updated,key}, got %s (%v)", output, err)
	}

	*requests = nil
	_, err = execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "update", engageTestGUID, "--json", `{"id":9,"name":"HQ"}`)
	if err == nil || !strings.Contains(err.Error(), "`id` 9") {
		t.Fatalf("expected a mismatched id to be rejected, got %v", err)
	}
	engageRequestsEqual(t, *requests, "GET "+engageAPIPrefix+"/traffic-filter?key="+engageTestGUID)
}

func TestEngageUpdateValidatesLocally(t *testing.T) {
	deps, requests := engageTestDeps(t, nil)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"engage", "persona", "update", "11", "--merge-json", `{}`}, "GUID `unique`"},
		{[]string{"engage", "persona", "update", engageTestGUID}, "exactly one of --json"},
		{[]string{"engage", "persona", "update", engageTestGUID, "--json", `{}`, "--merge-json", `{}`}, "exactly one of --json"},
		{[]string{"engage", "goal", "update", "5", "--merge-json", `{}`}, "GUID `key`"},
	} {
		_, err := execute(buildRootWithCollections(t, deps), tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: expected %q, got %v", tc.args, tc.want, err)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("local validation must send nothing, got %v", *requests)
	}
}

func TestEngageUpdateUnknownGUIDNeverPosts(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/persona/details": engageJSON(http.StatusOK, `null`),
	})
	_, err := execute(buildRootWithCollections(t, deps), "engage", "persona", "update", engageTestGUID, "--merge-json", `{"title":"X"}`)
	if err == nil || !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("expected the missing entity to fail the update, got %v", err)
	}
	engageRequestsEqual(t, *requests, "GET "+engageAPIPrefix+"/persona/details?id="+engageTestGUID)
}

func TestEngageDeleteIsForceGated(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/traffic-filter": engageJSON(http.StatusOK, ``),
	})
	_, err := execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "delete", engageTestGUID)
	if err == nil || !strings.Contains(err.Error(), "pass --force to confirm or --dry-run to rehearse") {
		t.Fatalf("expected the force gate, got %v", err)
	}
	_, err = execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "delete", "4", "--force")
	if err == nil || !strings.Contains(err.Error(), "GUID `key`") {
		t.Fatalf("expected the wrong id kind rejected, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "delete", engageTestGUID, "--dry-run"); err != nil {
		t.Fatalf("dry-run delete failed: %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("gate, id check and dry-run must send nothing, got %v", *requests)
	}
	output, err := execute(buildRootWithCollections(t, deps), "engage", "traffic-filter", "delete", engageTestGUID, "--force")
	if err != nil {
		t.Fatalf("forced delete failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "DELETE "+engageAPIPrefix+"/traffic-filter?key="+engageTestGUID)
	if !strings.Contains(output, `"deleted": true`) {
		t.Fatalf("expected {deleted:true}, got %s", output)
	}
}

func TestEngageValidationFailureExitsFour(t *testing.T) {
	deps, _ := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/customer-journey": engageJSON(http.StatusOK, `{"isValid":false,"warnings":[],"errors":["Journey is used by a running A/B test"]}`),
	})
	output, err := execute(buildRootWithCollections(t, deps), "engage", "journey", "delete", engageTestGUID, "--force")
	var rejected engageRejectedError
	if !errors.As(err, &rejected) || rejected.ExitCode() != 4 || !strings.Contains(err.Error(), "running A/B test") {
		t.Fatalf("expected a rejected error with exit 4 and the server's reason, got %v", err)
	}
	if !strings.Contains(output, `"isValid": false`) {
		t.Fatalf("expected the validation result printed before the error, got %s", output)
	}
}

func TestEngageGoalCreateFillsScoreTypeAndHasNoDelete(t *testing.T) {
	bodies := &engageBodies{}
	deps, _ := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/goal": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodPost: engageJSON(http.StatusOK, `"`+engageTestGUID+`"`),
		}),
	})
	output, err := execute(buildRootWithCollections(t, deps), "engage", "goal", "create", "--json",
		`{"unique":"`+engageTestGUID+`","name":"Signup","goalTypeId":"`+engageTestGUID2+`","implicitPersonaScoring":[{"personaId":3,"score":10}]}`)
	if err != nil {
		t.Fatalf("goal create failed: %v", err)
	}
	body := bodies.last(t)
	scoring, _ := body["implicitPersonaScoring"].([]any)
	if len(scoring) != 1 || scoring[0].(map[string]any)["scoreType"] != "GoalCompletion" || body["unique"] != engageTestGUID {
		t.Fatalf("expected the given unique kept and scoreType filled, got %+v", body)
	}
	if steps, ok := body["implicitCustomerJourneyStepScoring"].([]any); !ok || len(steps) != 0 {
		t.Fatalf("expected the required journey scoring array, got %+v", body["implicitCustomerJourneyStepScoring"])
	}
	if !strings.Contains(output, `"created": true`) || !strings.Contains(output, engageTestGUID) {
		t.Fatalf("expected {created, unique}, got %s", output)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "goal", "delete", engageTestGUID, "--force"); err == nil {
		t.Fatalf("Engage exposes no goal delete route; the command must not exist")
	}
}

func TestEngagePersonalizationEmptiesTheUnusedTargets(t *testing.T) {
	deps, _ := engageTestDeps(t, nil)
	output, err := execute(buildRootWithCollections(t, deps), "engage", "personalization", "create", "--dry-run", "--json",
		`{"name":"Mobile hero","segmentId":7,"type":"ContentType","pages":[{"nodeId":1}],"contentTypes":[{"contentTypeId":2}]}`)
	if err != nil {
		t.Fatalf("personalization create failed: %v", err)
	}
	var plan struct {
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(output), &plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pages, _ := plan.Body["pages"].([]any)
	types, _ := plan.Body["contentTypes"].([]any)
	if len(pages) != 0 || len(types) != 1 || !isUUIDLike(types[0].(map[string]any)["key"].(string)) {
		t.Fatalf("expected pages emptied and the content type keyed, got %+v", plan.Body)
	}
}

func TestEngageSegmentUpdatePriority(t *testing.T) {
	bodies := &engageBodies{}
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/segments/update-priority": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodPost: engageJSON(http.StatusOK, ``),
		}),
	})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--order", engageTestGUID}, "numeric segment `id`s"},
		{[]string{"--order", "7,7"}, "twice"},
		{[]string{"--order", "7", "--json", `[]`}, "exactly one of"},
		{[]string{"--json", `[{"id":"7","sortOrder":0}]`}, "numeric segment `id`"},
	} {
		_, err := execute(buildRootWithCollections(t, deps), append([]string{"engage", "segment", "update-priority"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: expected %q, got %v", tc.args, tc.want, err)
		}
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "segment", "update-priority", "--order", "7, 3,9"); err != nil {
		t.Fatalf("update-priority failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "POST "+engageAPIPrefix+"/segments/update-priority")
	encoded, _ := json.Marshal(bodies.sent[0])
	if string(encoded) != `[{"id":7,"sortOrder":0},{"id":3,"sortOrder":1},{"id":9,"sortOrder":2}]` {
		t.Fatalf("unexpected priority body %s", encoded)
	}
}

func TestEngageMainSwitchIsForceGatedAndReadsBack(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/main-switch/turn-off": engageJSON(http.StatusOK, ``),
		"/main-switch":          engageJSON(http.StatusOK, `{"on":false}`),
	})
	for _, use := range []string{"on", "off"} {
		_, err := execute(buildRootWithCollections(t, deps), "engage", "main-switch", use)
		if err == nil || !strings.Contains(err.Error(), "for the whole site; pass --force") {
			t.Fatalf("main-switch %s: expected the force gate, got %v", use, err)
		}
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "main-switch", "off", "--dry-run"); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("gate and dry-run must send nothing, got %v", *requests)
	}
	output, err := execute(buildRootWithCollections(t, deps), "engage", "main-switch", "off", "--force")
	if err != nil {
		t.Fatalf("main-switch off failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "POST "+engageAPIPrefix+"/main-switch/turn-off", "GET "+engageAPIPrefix+"/main-switch")
	if !strings.Contains(output, `"on": false`) {
		t.Fatalf("expected the switch state read back, got %s", output)
	}
}

func TestEngageReportingGenerateGateAndConflictHint(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/reporting/generation/start": engageJSON(http.StatusConflict, `{"title":"Reporting generation is already running","status":409}`),
	})
	_, err := execute(buildRootWithCollections(t, deps), "engage", "reporting", "generate")
	if err == nil || !strings.Contains(err.Error(), "pass --force") || len(*requests) != 0 {
		t.Fatalf("expected the force gate before any request, got %v (%v)", err, *requests)
	}
	_, err = execute(buildRootWithCollections(t, deps), "engage", "reporting", "generate", "--force")
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "already running") || strings.Contains(err.Error(), "schema alignment") {
		t.Fatalf("expected the already-running hint on a non-unavailable 409, got %v", err)
	}

	deps, _ = engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/reporting/generation/start": engageJSON(http.StatusConflict, engageUnavailable),
	})
	_, err = execute(buildRootWithCollections(t, deps), "engage", "reporting", "generate", "--force")
	if err == nil || !strings.Contains(err.Error(), "schema alignment") {
		t.Fatalf("expected the unavailable hint kept, got %v", err)
	}
}

func TestEngageAnnotationCreateAndDelete(t *testing.T) {
	bodies := &engageBodies{}
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/annotations": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodPost:   engageJSON(http.StatusOK, `{"id":21,"description":"Campaign launch"}`),
			http.MethodDelete: engageJSON(http.StatusOK, ``),
		}),
	})
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "create", "--json", `{"id":5}`); err == nil || !strings.Contains(err.Error(), "`id` 5") {
		t.Fatalf("expected a non-zero id rejected, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "delete", engageTestGUID, "--force"); err == nil || !strings.Contains(err.Error(), "numeric `id`") {
		t.Fatalf("expected a GUID rejected for an annotation, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "create", "--json", `{"timestamp":"2026-09-01T00:00:00Z","description":"Campaign launch","visibility":"Always"}`); err != nil {
		t.Fatalf("annotation create failed: %v", err)
	}
	body := bodies.last(t)
	if body["id"] != float64(0) || body["visibility"] != "Always" {
		t.Fatalf("expected id 0 and the payload, got %+v", body)
	}
	if variants, ok := body["pageVariants"].([]any); !ok || len(variants) != 0 {
		t.Fatalf("expected the required pageVariants array, got %+v", body["pageVariants"])
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "delete", "21", "--force"); err != nil {
		t.Fatalf("annotation delete failed: %v", err)
	}
	engageRequestsEqual(t, *requests, "POST "+engageAPIPrefix+"/annotations", "DELETE "+engageAPIPrefix+"/annotations?id=21")
}

func TestEngageRejectsNonStringGUIDsInsteadOfGeneratingOne(t *testing.T) {
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/persona/details": engageJSON(http.StatusOK, `{"id":11,"unique":"`+engageTestGUID+`","title":"Group"}`),
	})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"persona", "create", "--json", `{"title":"Group","unique":123}`}, "`unique` must be a GUID string, got a number"},
		{[]string{"persona", "create", "--json", `{"title":"Group","personas":[{"title":"A","unique":{}}]}`}, "personas[]: `unique` must be a GUID string, got an object"},
		{[]string{"persona", "update", engageTestGUID, "--merge-json", `{"unique":5}`}, "`unique` must be a GUID string, got a number"},
	} {
		_, err := execute(buildRootWithCollections(t, deps), append([]string{"engage"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: expected error containing %q, got %v", tc.args, tc.want, err)
		}
	}
	for _, request := range *requests {
		if strings.HasPrefix(request, "POST ") {
			t.Fatalf("a malformed GUID must never reach a save, got %v", *requests)
		}
	}
}

func TestEngageAnnotationCreateValidatesBeforePosting(t *testing.T) {
	bodies := &engageBodies{}
	deps, requests := engageTestDeps(t, map[string]func(*http.Request) *http.Response{
		"/annotations": engageByMethod(t, bodies, map[string]func(*http.Request) *http.Response{
			http.MethodPost: engageJSON(http.StatusOK, `{"id":22}`),
		}),
	})
	for _, tc := range []struct {
		json string
		want string
	}{
		{`{"timestamp":"2026-09-01T00:00:00Z","visibility":"Always"}`, "non-empty string `description`"},
		{`{"timestamp":"2026-09-01","description":"Launch","visibility":"Always"}`, "RFC 3339 date-time"},
		{`{"timestamp":"2026-09-01T00:00:00Z","description":"Launch","visibility":"Sometimes"}`, "is not one of Always"},
		{`{"timestamp":"2026-09-01T00:00:00Z","description":"Launch","visibility":"Node"}`, "needs at least one"},
		{`{"timestamp":"2026-09-01T00:00:00Z","description":"Launch","visibility":"Node","pageVariants":[{"unique":"not-a-guid"}]}`, "pageVariants[0].unique must be a document GUID"},
		{`{"timestamp":"2026-09-01T00:00:00Z","description":"Launch","visibility":"Always","pageVariants":"x"}`, "`pageVariants` must be an array"},
	} {
		_, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "create", "--json", tc.json, "--dry-run")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.json, tc.want, err)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("invalid annotations must fail before any request, got %v", *requests)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "engage", "annotation", "create", "--json",
		`{"timestamp":"2026-09-01T00:00:00Z","description":"Launch","visibility":"node","pageVariants":[{"unique":"`+engageTestGUID+`","culture":""}]}`); err != nil {
		t.Fatalf("valid page annotation failed: %v", err)
	}
	if body := bodies.last(t); body["visibility"] != "Node" {
		t.Fatalf("expected the visibility canonicalised to Node, got %+v", body["visibility"])
	}
}
