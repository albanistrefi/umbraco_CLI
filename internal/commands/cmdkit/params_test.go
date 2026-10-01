package cmdkit

import (
	"reflect"
	"strings"
	"testing"
)

func TestParamsHelpers(t *testing.T) {
	if params, err := ParseParams("  "); err != nil || params != nil {
		t.Fatalf("expected blank --params to be nil, got %v %v", params, err)
	}
	if _, err := ParseParams(`{"a":`); err == nil || !strings.Contains(err.Error(), "invalid --params JSON") {
		t.Fatalf("expected a labelled parse error, got %v", err)
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
	if !reflect.DeepEqual(StringsToAny([]string{"a"}), []any{"a"}) {
		t.Fatal("StringsToAny converts")
	}
}
