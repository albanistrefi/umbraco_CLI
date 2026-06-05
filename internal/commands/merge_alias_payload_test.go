package commands

import (
	"reflect"
	"testing"
)

// Regression: previously mergeAliasObjectArrays keyed values[] entries by
// alias only, so when a document had a culture-variant property (same alias,
// different culture) and a patch supplied entries for two cultures, both
// patched entries got collapsed onto whichever current entry the merge
// matched first — duplicating one culture and losing the other.
func TestMergeAliasObjectArraysKeysByAliasCultureAndSegment(t *testing.T) {
	current := []any{
		map[string]any{"alias": "title", "value": "Old en", "culture": "en-US", "segment": nil},
		map[string]any{"alias": "title", "value": "Old da", "culture": "da-DK", "segment": nil},
		map[string]any{"alias": "summary", "value": "Keep me", "culture": nil, "segment": nil},
	}
	patch := []any{
		map[string]any{"alias": "title", "value": "New en", "culture": "en-US", "segment": nil},
		map[string]any{"alias": "title", "value": "New da", "culture": "da-DK", "segment": nil},
	}

	merged := mergeAliasObjectArrays(current, patch)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged entries, got %d: %+v", len(merged), merged)
	}

	byKey := map[string]map[string]any{}
	for _, entry := range merged {
		m := entry.(map[string]any)
		culture, _ := m["culture"].(string)
		byKey[m["alias"].(string)+"|"+culture] = m
	}
	if byKey["title|en-US"]["value"] != "New en" {
		t.Fatalf("en-US title not updated: %+v", byKey["title|en-US"])
	}
	if byKey["title|da-DK"]["value"] != "New da" {
		t.Fatalf("da-DK title not updated (the regression case — used to overwrite/dup): %+v", byKey["title|da-DK"])
	}
	if byKey["summary|"]["value"] != "Keep me" {
		t.Fatalf("untouched invariant summary lost: %+v", byKey["summary|"])
	}
}

// Doctype properties only carry alias (no culture/segment). The compound-key
// scheme must still behave correctly: alias alone identifies the entry, and
// updates land on the right property.
func TestMergeAliasObjectArraysAliasOnlyShapesUnchanged(t *testing.T) {
	current := []any{
		map[string]any{"alias": "title", "name": "Title"},
		map[string]any{"alias": "body", "name": "Body"},
	}
	patch := []any{
		map[string]any{"alias": "title", "name": "Headline"},
	}

	merged := mergeAliasObjectArrays(current, patch)
	if len(merged) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(merged))
	}

	want := []any{
		map[string]any{"alias": "title", "name": "Headline"},
		map[string]any{"alias": "body", "name": "Body"},
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("alias-only merge changed shape:\n got %+v\nwant %+v", merged, want)
	}
}

// A patch entry whose culture is omitted (i.e. invariant) must NOT match
// an existing variant entry for the same alias — they're distinct values.
func TestMergeAliasObjectArraysInvariantPatchDoesNotMatchVariantCurrent(t *testing.T) {
	current := []any{
		map[string]any{"alias": "title", "value": "en value", "culture": "en-US"},
	}
	patch := []any{
		map[string]any{"alias": "title", "value": "invariant value", "culture": nil},
	}

	merged := mergeAliasObjectArrays(current, patch)
	if len(merged) != 2 {
		t.Fatalf("invariant and variant entries with the same alias must coexist after merge, got %d: %+v", len(merged), merged)
	}
}
