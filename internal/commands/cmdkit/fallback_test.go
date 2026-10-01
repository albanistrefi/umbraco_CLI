package cmdkit

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"umbraco-cli/internal/api"
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
