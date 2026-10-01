package cmdkit

import (
	"sort"
	"strings"
)

func ItemID(item any) string {
	entry, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := entry["id"].(string)
	return strings.TrimSpace(id)
}

func SortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func UniqueCSV(s string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range strings.Split(s, ",") {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func StringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// TreeItemNames returns every name a tree item is known by. Older
// Management APIs put a top-level name on tree items; modern ones carry
// per-culture names inside variants[] with no top-level field, which made
// matching on item["name"] silently find nothing.
func TreeItemNames(item map[string]any) []string {
	names := make([]string, 0, 2)
	if name, ok := item["name"].(string); ok && name != "" {
		names = append(names, name)
	}
	variants, _ := item["variants"].([]any)
	for _, raw := range variants {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := variant["name"].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}
