package cmdkit

import (
	"reflect"
	"testing"
)

func TestResponseReaders(t *testing.T) {
	if ItemID(map[string]any{"id": " x "}) != "x" || ItemID("nope") != "" {
		t.Fatal("ItemID trims and tolerates non-objects")
	}
	names := TreeItemNames(map[string]any{"name": "Top", "variants": []any{map[string]any{"name": "Variant"}, "bad", map[string]any{"name": ""}}})
	if !reflect.DeepEqual(names, []string{"Top", "Variant"}) {
		t.Fatalf("TreeItemNames collects top-level and variant names, got %v", names)
	}
	if !reflect.DeepEqual(ResultItems(map[string]any{"items": []any{1}}), []any{1}) ||
		!reflect.DeepEqual(ResultItems([]any{2}), []any{2}) || ResultItems("x") != nil {
		t.Fatal("ResultItems reads envelopes and bare arrays")
	}
}
