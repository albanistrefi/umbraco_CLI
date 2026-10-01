package cmdkit

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/uuid"
)

func TestGetAllPagesWithFallbackStopsOnLimitAndSticksToTheWinner(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/modern": func(req kitRequest) (int, string) {
			skip, _ := strconv.Atoi(req.Query.Get("skip"))
			return 200, `{"total":100,"items":[{"id":"` + strconv.Itoa(skip) + `"},{"id":"` + strconv.Itoa(skip+1) + `"}]}`
		},
	})
	candidates := []GetRequestCandidate{
		{Path: "/legacy", Opts: api.RequestOptions{Params: map[string]any{"keep": "me"}}},
		{Path: "/modern", Opts: api.RequestOptions{Params: map[string]any{"keep": "me"}}},
	}
	result, err := GetAllPagesWithFallback(context.Background(), deps.Client, 2, -5, 3, candidates...)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	got := result.(map[string]any)
	if len(got["items"].([]any)) != 3 || got["total"] != float64(100) {
		t.Fatalf("expected the walk to stop at the limit, got %v", got)
	}
	paths := []string{}
	for _, req := range server.requests {
		paths = append(paths, req.Path+"?skip="+req.Query.Get("skip")+"&keep="+req.Query.Get("keep"))
	}
	want := []string{
		testMgmt + "/legacy?skip=0&keep=me",
		testMgmt + "/modern?skip=0&keep=me",
		testMgmt + "/modern?skip=2&keep=me",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("expected the 404ing candidate to be probed once, got %v", paths)
	}
	if candidates[0].Opts.Params["skip"] != nil {
		t.Fatal("paging must not mutate the caller's params")
	}
}

func TestGetAllPagesWithFallbackCeilingAndNonEnvelope(t *testing.T) {
	_, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/endless": fixed(200, `{"total":1000000,"items":[{"id":"x"}]}`),
		"GET " + testMgmt + "/array":   fixed(200, `[1,2]`),
		"GET " + testMgmt + "/broken":  fixed(500, `{"title":"Boom","status":500}`),
	})
	_, err := GetAllPagesWithFallback(context.Background(), deps.Client, 1, 0, 0, GetRequestCandidate{Path: "/endless"})
	mustContain(t, err, "--all hit the safety ceiling of 200 pages × 1 items = 200 after 200 items collected")
	mustContain(t, err, "Use --skip 200 to resume")

	result, err := GetAllPagesWithFallback(context.Background(), deps.Client, 0, 0, 0, GetRequestCandidate{Path: "/array"})
	if err != nil || !reflect.DeepEqual(result, []any{float64(1), float64(2)}) {
		t.Fatalf("expected a non-envelope page verbatim, got %v %v", result, err)
	}
	_, err = GetAllPagesWithFallback(context.Background(), deps.Client, 0, 0, 0, GetRequestCandidate{Path: "/broken"})
	if !api.IsStatus(err, 500) {
		t.Fatalf("expected the 500 to surface, got %v", err)
	}
}

func TestGetWithFallbackErrors(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/forbidden": fixed(403, `{"title":"Forbidden","status":403}`),
	})
	_, err := GetWithFallback(context.Background(), deps.Client)
	mustContain(t, err, "no endpoint candidates were configured")

	_, err = GetWithFallback(context.Background(), deps.Client, GetRequestCandidate{Path: "/a"}, GetRequestCandidate{Path: "/b"})
	if !api.IsStatus(err, 404) || len(server.requests) != 2 {
		t.Fatalf("expected the last 404 after trying both, got %v", err)
	}
	_, err = GetWithFallback(context.Background(), deps.Client, GetRequestCandidate{Path: "/forbidden"}, GetRequestCandidate{Path: "/never"})
	if !api.IsStatus(err, 403) || len(server.requests) != 3 {
		t.Fatalf("expected a non-404 to stop the fallback, got %v", err)
	}
}

func TestPayloadParsingHelpers(t *testing.T) {
	if params, err := ParseParams("  "); err != nil || params != nil {
		t.Fatalf("expected blank --params to be nil, got %v %v", params, err)
	}
	if _, err := ParseParams(`{"a":`); err == nil || !strings.Contains(err.Error(), "invalid --params JSON") {
		t.Fatalf("expected a labelled parse error, got %v", err)
	}
	if body, err := OptionalBody(""); err != nil || len(body) != 0 || body == nil {
		t.Fatalf("expected an empty, non-nil body, got %v %v", body, err)
	}
	if body, err := OptionalBody(`{"a":1}`); err != nil || body["a"] != float64(1) {
		t.Fatalf("expected the parsed body, got %v %v", body, err)
	}
	mustContain(t, RequireValue("--name", " "), "missing required option: --name")
	if RequireValue("--name", "x") != nil {
		t.Fatal("expected a present value to pass")
	}
	if got := ApplyPaginationParams(nil, -1, -1); got != nil {
		t.Fatalf("expected sentinels to leave params untouched, got %v", got)
	}
	if got := ApplyPaginationParams(nil, 0, 5); !reflect.DeepEqual(got, map[string]any{"skip": 0, "take": 5}) {
		t.Fatalf("expected skip/take, got %v", got)
	}
	if got := ApplyPaginationParams(map[string]any{"a": 1}, -1, 3); !reflect.DeepEqual(got, map[string]any{"a": 1, "take": 3}) {
		t.Fatalf("expected take only, got %v", got)
	}
	if body, err := TargetActionBody("", "p"); err != nil || !reflect.DeepEqual(body, map[string]any{"target": map[string]any{"id": "p"}}) {
		t.Fatalf("expected the --to envelope, got %v %v", body, err)
	}
	if err := StripFields("a", "b")(map[string]any{"a": 1}); err != nil {
		t.Fatalf("strip: %v", err)
	}
}

func TestDependenciesProviders(t *testing.T) {
	var deps Dependencies
	if deps.RequestedOutput() != "" || deps.CurrentEnvOutput() != "" || deps.CurrentConfig().BaseURL != "" {
		t.Fatal("expected zero Dependencies to read as empty")
	}
	if !reflect.DeepEqual(deps.ConfigOptions(), config.LoadOptions{}) {
		t.Fatal("expected zero config options")
	}
	output := "table"
	deps = Dependencies{
		OutputFlag:            &output,
		EnvOutput:             config.OutputPlain,
		Config:                config.Config{BaseURL: "https://static.test"},
		EnvOutputProvider:     func() config.OutputFormat { return config.OutputJSON },
		ConfigProvider:        func() config.Config { return config.Config{BaseURL: "https://runtime.test"} },
		ConfigOptionsProvider: func() config.LoadOptions { return config.LoadOptions{Profile: "p"} },
	}
	if deps.RequestedOutput() != "table" || deps.CurrentEnvOutput() != config.OutputJSON ||
		deps.CurrentConfig().BaseURL != "https://runtime.test" || deps.ConfigOptions().Profile != "p" {
		t.Fatalf("expected providers to win over static values, got %+v", deps)
	}
	deps.EnvOutputProvider = nil
	deps.ConfigProvider = nil
	if deps.CurrentEnvOutput() != config.OutputPlain || deps.CurrentConfig().BaseURL != "https://static.test" {
		t.Fatal("expected static values without providers")
	}
}

func TestPrintMutationResultAndFlagHelpers(t *testing.T) {
	_, deps := newKitServer(nil)
	cmd := &cobra.Command{Use: "noop", RunE: func(cmd *cobra.Command, args []string) error {
		return PrintMutationResult(cmd, deps, "touched", nil, false)
	}}
	var skip, take int
	var all bool
	var triage ReadTriageOptions
	AddPaginationFlags(cmd, &skip, &take)
	AddAutoPaginationFlag(cmd, &all)
	AddReadTriageFlags(cmd, &triage)
	for _, name := range []string{"skip", "take", "all", "summarize", "ids-only", "first-n"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected --%s", name)
		}
	}
	if cmd.Flags().Lookup("skip").DefValue != "-1" || cmd.Flags().Lookup("take").DefValue != "-1" {
		t.Fatal("expected the -1 pagination sentinel")
	}
	out, _, err := runKit(t, cmd)
	if err != nil || !reflect.DeepEqual(decodeObject(t, out), map[string]any{"touched": true}) {
		t.Fatalf("expected {touched:true}, got %s (%v)", out, err)
	}
}

func TestValueHelpers(t *testing.T) {
	if AsString(nil) != "" || AsString("s") != "s" || AsString(float64(3)) != "3" || AsString(1.5) != "1.5" || AsString(true) != "true" {
		t.Fatal("AsString renders JSON scalars")
	}
	if CultureValue(nil) != "" || CultureValue("en") != "en" || CultureValue(2) != "2" {
		t.Fatal("CultureValue renders scalars")
	}
	if StringValue(nil) != "" || StringValue("x") != "x" || StringValue(4) != "4" {
		t.Fatal("StringValue renders scalars")
	}
	if !reflect.DeepEqual(UniqueCSV(" a,b,,a , c"), []string{"a", "b", "c"}) {
		t.Fatal("UniqueCSV trims and deduplicates in order")
	}
	if !reflect.DeepEqual(StringsToAny([]string{"a"}), []any{"a"}) {
		t.Fatal("StringsToAny converts")
	}
	if !reflect.DeepEqual(SortedKeys(map[string]struct{}{"b": {}, "a": {}}), []string{"a", "b"}) {
		t.Fatal("SortedKeys sorts")
	}
	if ItemID(map[string]any{"id": " x "}) != "x" || ItemID("nope") != "" {
		t.Fatal("ItemID trims and tolerates non-objects")
	}
	names := TreeItemNames(map[string]any{"name": "Top", "variants": []any{map[string]any{"name": "Variant"}, "bad", map[string]any{"name": ""}}})
	if !reflect.DeepEqual(names, []string{"Top", "Variant"}) {
		t.Fatalf("TreeItemNames collects top-level and variant names, got %v", names)
	}
}

func TestEnsurePayloadIDKeepsOrGeneratesAnID(t *testing.T) {
	body := map[string]any{"id": "keep"}
	if id, err := EnsurePayloadID(body); err != nil || id != "keep" {
		t.Fatalf("expected an existing id to be kept, got %q %v", id, err)
	}
	generated := map[string]any{}
	if id, err := EnsurePayloadID(generated); err != nil || !uuid.Valid(id) || generated["id"] != id {
		t.Fatalf("expected a generated id set on the body, got %q %v (%v)", id, err, generated)
	}
}
