package cmdkit

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"umbraco-cli/internal/api"
)

func TestGetCommandProjectsFieldsAndHonoursAPIPrefix(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET /umbraco/forms/management/api/v1/widget/w-1": fixed(200, `{"id":"w-1","name":"One","extra":true}`),
	})
	cmd := GetCommand(deps, GetSpec{
		Use:       "get <id>",
		Short:     "Get a widget",
		Path:      func(args []string) string { return api.JoinPath("/widget/%s", args[0]) },
		APIPrefix: "/umbraco/forms/management/api/v1",
	})
	out, _, err := runKit(t, cmd, "w-1", "--fields", "id,name")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := decodeObject(t, out)
	if !reflect.DeepEqual(got, map[string]any{"id": "w-1", "name": "One"}) {
		t.Fatalf("expected projected object, got %v", got)
	}
	if len(server.requests) != 1 {
		t.Fatalf("expected one request, got %+v", server.requests)
	}
	if _, _, err := runKit(t, GetCommand(deps, GetSpec{Use: "get <id>", Path: func(a []string) string { return "/x" }})); err == nil {
		t.Fatal("expected get without an id to fail argument validation")
	}
}

func TestCollectionCommandFallsBackPastNotFoundAndPassesPagination(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/legacy/children": fixed(200, `{"total":3,"items":[{"id":"a","name":"A","alias":"x","more":1},{"id":"b","name":"B"},{"id":"c","name":"C"}]}`),
	})
	cmd := CollectionCommand(deps, CollectionSpec{
		Use:   "children <id>",
		Short: "Children",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{
				{Path: "/tree/widget/children", Opts: api.RequestOptions{Params: WithParam(params, "parentId", args[0])}},
				{Path: "/legacy/children", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
	out, _, err := runKit(t, cmd, "p-1", "--skip", "0", "--take", "3", "--params", `{"foo":"bar"}`, "--first-n", "2", "--summarize")
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(server.requests) != 2 {
		t.Fatalf("expected the modern candidate then the fallback, got %+v", server.requests)
	}
	first, second := server.requests[0], server.requests[1]
	if first.Path != testMgmt+"/tree/widget/children" || first.Query.Get("parentId") != "p-1" {
		t.Fatalf("unexpected first candidate %+v", first)
	}
	if second.Query.Get("parentId") != "" {
		t.Fatalf("WithParam must not leak parentId into the shared params: %+v", second)
	}
	for _, req := range server.requests {
		if req.Query.Get("skip") != "0" || req.Query.Get("take") != "3" || req.Query.Get("foo") != "bar" {
			t.Fatalf("pagination/params not forwarded: %+v", req)
		}
	}
	got := decodeObject(t, out)
	items := got["items"].([]any)
	if len(items) != 2 || got["returned"] != float64(2) {
		t.Fatalf("expected --first-n 2 to trim, got %v", got)
	}
	if !reflect.DeepEqual(items[0], map[string]any{"id": "a", "name": "A", "alias": "x"}) {
		t.Fatalf("expected --summarize to keep id/name/alias, got %v", items[0])
	}
}

func TestCollectionCommandAllWalksPagesAndEnriches(t *testing.T) {
	pages := map[string]string{
		"0": `{"total":3,"items":[{"id":"a"},{"id":"b"}]}`,
		"2": `{"total":3,"items":[{"id":"c"}]}`,
	}
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/widget": func(req kitRequest) (int, string) {
			return 200, pages[req.Query.Get("skip")]
		},
	})
	enriched := false
	cmd := CollectionCommand(deps, CollectionSpec{
		Use: "list",
		Endpoints: func(args []string, params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{{Path: "/widget", Opts: api.RequestOptions{Params: params}}}
		},
		Enrich: func(ctx context.Context, result any) (any, error) {
			enriched = true
			return result, nil
		},
	})
	out, _, err := runKit(t, cmd, "--all", "--take", "2", "--ids-only")
	if err != nil {
		t.Fatalf("list --all: %v", err)
	}
	if len(server.requests) != 2 || !enriched {
		t.Fatalf("expected two pages and enrichment, got %d requests (enriched=%v)", len(server.requests), enriched)
	}
	got := decodeObject(t, out)
	if !reflect.DeepEqual(got["items"], []any{"a", "b", "c"}) || got["total"] != float64(3) {
		t.Fatalf("expected every id across pages, got %v", got)
	}

	failing := CollectionCommand(deps, CollectionSpec{
		Use: "list",
		Endpoints: func(args []string, params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{{Path: "/widget", Opts: api.RequestOptions{Params: params}}}
		},
		Enrich: func(ctx context.Context, result any) (any, error) { return nil, fmt.Errorf("enrich failed") },
	})
	_, _, err = runKit(t, failing)
	mustContain(t, err, "enrich failed")
	_, _, err = runKit(t, cmd, "--params", `[1]`)
	mustContain(t, err, "--params must be a JSON object")
}

func TestCollectionCommandDocumentOutputTrim(t *testing.T) {
	_, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/doc": fixed(200, `{"total":1,"items":[{"id":"d","name":"Doc","values":[]}]}`),
	})
	spec := CollectionSpec{
		Use:                "list",
		DocumentOutputTrim: true,
		Endpoints: func(args []string, params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{{Path: "/doc", Opts: api.RequestOptions{Params: params}}}
		},
	}
	out, _, err := runKit(t, CollectionCommand(deps, spec), "--fields", "id")
	if err != nil {
		t.Fatalf("list --fields: %v", err)
	}
	if items := decodeObject(t, out)["items"].([]any); !reflect.DeepEqual(items[0], map[string]any{"id": "d"}) {
		t.Fatalf("expected dotted projection to keep only id, got %v", items)
	}
	_, _, err = runKit(t, CollectionCommand(deps, spec), "--full", "--summary")
	mustContain(t, err, "--full")
}

func TestSearchCommandMergesFlagsUnderParams(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/item/widget/search": fixed(200, `{"total":0,"items":[]}`),
	})
	enriched := false
	spec := SearchSpec{
		Use:   "search",
		Extra: []ParamFlag{{Flag: "under", Param: "parentId", Usage: "Parent"}},
		Endpoints: func(params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{{Path: "/item/widget/search", Opts: api.RequestOptions{Params: params}}}
		},
		Enrich: func(ctx context.Context, result any) (any, error) {
			enriched = true
			return result, nil
		},
	}
	_, _, err := runKit(t, SearchCommand(deps, spec))
	mustContain(t, err, "requires either --params or --query")

	if _, _, err := runKit(t, SearchCommand(deps, spec), "--query", "flag", "--under", "p-1", "--skip", "1", "--take", "5", "--params", `{"query":"wins"}`); err != nil {
		t.Fatalf("search: %v", err)
	}
	req := server.requests[len(server.requests)-1]
	if req.Query.Get("query") != "wins" || req.Query.Get("parentId") != "p-1" || req.Query.Get("skip") != "1" || req.Query.Get("take") != "5" {
		t.Fatalf("expected --params to win and flags to fill gaps, got %v", req.Query)
	}
	if !enriched {
		t.Fatal("expected Enrich to run")
	}

	trimmed := spec
	trimmed.DocumentOutputTrim = true
	trimmed.Enrich = nil
	if _, _, err := runKit(t, SearchCommand(deps, trimmed), "--query", "x", "--fields", "id"); err != nil {
		t.Fatalf("search with document trim: %v", err)
	}
	_, _, err = runKit(t, SearchCommand(deps, trimmed), "--query", "x", "--full", "--no-empty")
	mustContain(t, err, "--full")
}

func TestReferencesAndAreReferencedCommands(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/widget/w-1/referenced-by": fixed(200, `{"total":1,"items":[{"id":"r"}]}`),
		"GET " + testMgmt + "/widget/are-referenced":    fixed(200, `{"total":0,"items":[]}`),
	})
	refs := ReferencesCommand(deps, ReferencesSpec{Use: "referenced-by <id>", Path: func(args []string) string {
		return api.JoinPath("/widget/%s/referenced-by", args[0])
	}})
	if _, _, err := runKit(t, refs, "w-1", "--take", "1"); err != nil {
		t.Fatalf("referenced-by: %v", err)
	}
	_, _, err := runKit(t, AreReferencedCommand(deps, "widget"))
	mustContain(t, err, "requires --ids")
	if _, _, err := runKit(t, AreReferencedCommand(deps, "widget"), "--ids", "a, b,a"); err != nil {
		t.Fatalf("are-referenced: %v", err)
	}
	req := server.requests[len(server.requests)-1]
	if !reflect.DeepEqual(req.Query["id"], []string{"a", "b"}) || req.Query.Get("take") != "2" || req.Query.Get("skip") != "0" {
		t.Fatalf("expected deduplicated ids with take=len(ids), got %v", req.Query)
	}
}

func TestObjectFromResultShapes(t *testing.T) {
	if got, err := ObjectFromResult("GET /x", `{"a":1}`); err != nil || got["a"] != float64(1) {
		t.Fatalf("expected a double-encoded object to unwrap, got %v %v", got, err)
	}
	cases := map[string]any{
		"returned a string":    "plain\x1btext",
		"returned an empty":    nil,
		"returned an array":    []any{1},
		"returned a number":    float64(2),
		"returned a boolean":   true,
		"returned int, not a ": 3,
	}
	for want, value := range cases {
		_, err := ObjectFromResult("GET /x", value)
		mustContain(t, err, want)
	}
	if _, err := ObjectFromResult("GET /x", strings.Repeat("y", 300)); !strings.Contains(fmt.Sprint(err), "…") {
		t.Fatalf("expected long bodies to be truncated, got %v", err)
	}
	if JSONShapeName(map[string]any{}) != "an object" {
		t.Fatal("expected map to read as an object")
	}
}

func TestFetchObjectReportsNonObjects(t *testing.T) {
	_, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/list": fixed(200, `[1,2]`),
	})
	_, err := FetchObject(context.Background(), deps.Client, "/list", api.RequestOptions{})
	mustContain(t, err, "GET /list returned an array")
	_, err = FetchObject(context.Background(), deps.Client, "/missing", api.RequestOptions{})
	if !api.IsStatus(err, 404) {
		t.Fatalf("expected a 404 APIError, got %v", err)
	}
}

func TestMergeParamsAndWithParam(t *testing.T) {
	if got := MergeParams(nil, nil); got != nil {
		t.Fatalf("expected nil params to stay nil, got %v", got)
	}
	got := MergeParams(nil, map[string]any{"a": 1})
	if !reflect.DeepEqual(got, map[string]any{"a": 1}) {
		t.Fatalf("expected flags to fill an empty map, got %v", got)
	}
	got = MergeParams(map[string]any{"a": "params"}, map[string]any{"a": "flag", "b": 2})
	if !reflect.DeepEqual(got, map[string]any{"a": "params", "b": 2}) {
		t.Fatalf("expected --params to win collisions, got %v", got)
	}
	base := map[string]any{"x": 1}
	next := WithParam(base, "y", 2)
	if len(base) != 1 || next["y"] != 2 || next["x"] != 1 {
		t.Fatalf("WithParam must clone, got base=%v next=%v", base, next)
	}
}
