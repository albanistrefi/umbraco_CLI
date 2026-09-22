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

// Regression: variant objects carry no alias, so the merge used to treat
// variants[] as unidentifiable and replace the whole array — renaming one
// culture dropped every other culture from the payload.
func TestMergeAliasObjectArraysKeysAliaslessVariantsByCultureAndSegment(t *testing.T) {
	current := []any{
		map[string]any{"culture": "en-US", "segment": nil, "name": "English name", "state": "Draft"},
		map[string]any{"culture": "da-DK", "segment": nil, "name": "Danish name", "state": "Draft"},
	}
	patch := []any{
		map[string]any{"culture": "da-DK", "name": "New Danish name"},
	}

	merged := mergeAliasObjectArrays(current, patch)
	if len(merged) != 2 {
		t.Fatalf("expected both variants to survive, got %d: %+v", len(merged), merged)
	}

	byCulture := map[string]map[string]any{}
	for _, entry := range merged {
		object, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected object entries, got %+v", entry)
		}
		culture, _ := object["culture"].(string)
		byCulture[culture] = object
	}
	if byCulture["da-DK"]["name"] != "New Danish name" {
		t.Fatalf("expected the patched culture to be renamed, got %+v", byCulture["da-DK"])
	}
	if byCulture["en-US"]["name"] != "English name" {
		t.Fatalf("expected the untouched culture to keep its name, got %+v", byCulture["en-US"])
	}
	// The patch names one field of one variant; everything else on that
	// variant is preserved, exactly as it is for an alias-keyed entry.
	if byCulture["da-DK"]["state"] != "Draft" {
		t.Fatalf("expected unmentioned fields of the patched variant to survive, got %+v", byCulture["da-DK"])
	}
}

// Segments are the other half of the variant key: two entries for the same
// culture but different segments are distinct rows.
func TestMergeAliasObjectArraysKeysAliaslessEntriesBySegment(t *testing.T) {
	current := []any{
		map[string]any{"culture": "en-US", "segment": nil, "name": "Default"},
		map[string]any{"culture": "en-US", "segment": "mobile", "name": "Mobile"},
	}
	patch := []any{
		map[string]any{"culture": "en-US", "segment": "mobile", "name": "Mobile renamed"},
	}

	merged := mergeAliasObjectArrays(current, patch)
	if len(merged) != 2 {
		t.Fatalf("expected both segments to survive, got %d: %+v", len(merged), merged)
	}
	first, _ := merged[0].(map[string]any)
	second, _ := merged[1].(map[string]any)
	if first["name"] != "Default" || second["name"] != "Mobile renamed" {
		t.Fatalf("expected only the mobile segment to change, got %+v", merged)
	}
}

// An alias-less patch entry that names neither culture nor segment is not
// identifiable, so the array keeps the wholesale-replacement contract it
// had before. This is what holds a doctype's containers[] — keyed by an id
// the caller does not restate — to its existing behaviour.
func TestMergeAliasObjectArraysReplacesUnidentifiableArrays(t *testing.T) {
	current := map[string]any{
		"containers": []any{
			map[string]any{"id": "c1", "name": "Content", "type": "Tab", "sortOrder": float64(0)},
			map[string]any{"id": "c2", "name": "Settings", "type": "Tab", "sortOrder": float64(1)},
		},
	}
	patch := map[string]any{
		"containers": []any{
			map[string]any{"id": "c1", "name": "Content", "type": "Tab", "sortOrder": float64(0)},
		},
	}

	merged := mergeAliasPayload(current, patch)
	containers, _ := merged["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected containers to be replaced wholesale, got %d: %+v", len(containers), containers)
	}
}
