package cmdkit

// documentVariantsKey is the one payload field whose array is keyed by
// culture and segment rather than by alias. See mergeVariantArrays.
const documentVariantsKey = "variants"

// MergeAliasPayload deep-merges a partial patch into a current Management API payload, preserving
// fields the patch does not mention and merging alias-keyed arrays (`properties`, `containers`,
// `values`, …) entry-wise. The payload's own `variants` array is the one exception, keyed by
// culture and segment instead — see mergeVariantArrays.
// Used by the merge-json flows for documents, doctypes, and datatypes.
func MergeAliasPayload(current map[string]any, patch map[string]any) map[string]any {
	return mergePayloadObjects(current, patch, true)
}

// mergePayloadObjects carries the one piece of context the merge needs:
// whether it is looking at the payload's own fields (atRoot) or at
// something nested inside one of them. Only the payload's own `variants`
// field gets the culture/segment identity; everything reached by recursion
// — a property value that happens to be an array of culture-tagged objects,
// a data type setting, a nested block payload — keeps the alias-or-nothing
// rule it had before, so a value like [{"culture":"en-US",…}] is still
// replaced wholesale rather than merged entry-wise.
func mergePayloadObjects(current map[string]any, patch map[string]any, atRoot bool) map[string]any {
	merged := CloneObject(current)
	for key, value := range patch {
		existing, exists := merged[key]
		if !exists {
			merged[key] = CloneAliasValue(value)
			continue
		}

		if atRoot && key == documentVariantsKey {
			if variants, ok := mergeVariantArrays(existing, value); ok {
				merged[key] = variants
				continue
			}
		}

		merged[key] = MergeAliasValue(existing, value)
	}

	return merged
}

func MergeAliasValue(current any, patch any) any {
	currentMap, currentIsMap := current.(map[string]any)
	patchMap, patchIsMap := patch.(map[string]any)
	if currentIsMap && patchIsMap {
		return mergePayloadObjects(currentMap, patchMap, false)
	}

	currentArray, currentIsArray := current.([]any)
	patchArray, patchIsArray := patch.([]any)
	if currentIsArray && patchIsArray && isAliasObjectArray(currentArray) && isAliasObjectArray(patchArray) {
		return MergeObjectArrays(currentArray, patchArray, AliasMergeKey)
	}

	return CloneAliasValue(patch)
}

// mergeVariantArrays merges a document's variants[] by culture and segment,
// reporting false when either side is not an array of culture- or
// segment-tagged objects (in which case the caller falls back to the
// ordinary alias rule, which for variants means wholesale replacement).
//
// Variants carry no alias, so without an identity of their own a patch
// naming one culture replaced the whole array and silently dropped every
// culture it did not mention. On 'document create --from-blueprint --json'
// that produced a document missing its default culture entirely.
//
// A patch entry that names neither culture nor segment is not identifiable,
// so such a patch still replaces the array wholesale, as it did before.
func mergeVariantArrays(current any, patch any) ([]any, bool) {
	currentArray, currentIsArray := current.([]any)
	patchArray, patchIsArray := patch.([]any)
	if !currentIsArray || !patchIsArray {
		return nil, false
	}
	if !isVariantObjectArray(currentArray) || !isVariantObjectArray(patchArray) {
		return nil, false
	}
	return MergeObjectArrays(currentArray, patchArray, variantMergeKey), true
}

// MergeObjectArrays merges two object arrays entry-wise under the supplied
// identity. Current entries keep their order, patched entries are merged in
// place, and patch entries with no counterpart are appended.
func MergeObjectArrays(current []any, patch []any, identity func(any) (string, map[string]any, bool)) []any {
	merged := make([]any, 0, len(current)+len(patch))
	patchByKey := make(map[string]map[string]any, len(patch))
	for _, item := range patch {
		key, itemMap, ok := identity(item)
		if !ok {
			continue
		}
		patchByKey[key] = itemMap
	}

	seen := make(map[string]struct{}, len(patchByKey))
	for _, item := range current {
		key, itemMap, ok := identity(item)
		if !ok {
			merged = append(merged, CloneAliasValue(item))
			continue
		}

		patchItem, hasPatch := patchByKey[key]
		if !hasPatch {
			merged = append(merged, CloneAliasValue(itemMap))
			continue
		}

		// Entries are ordinary nested objects: whatever they contain is
		// below the root, so no nested "variants" field of theirs is
		// treated as a document's variants array.
		merged = append(merged, mergePayloadObjects(itemMap, patchItem, false))
		seen[key] = struct{}{}
	}

	for _, item := range patch {
		key, itemMap, ok := identity(item)
		if !ok {
			merged = append(merged, CloneAliasValue(item))
			continue
		}
		if _, alreadyMerged := seen[key]; alreadyMerged {
			continue
		}
		merged = append(merged, CloneAliasValue(itemMap))
	}

	return merged
}

// AliasMergeKey returns the compound key used to match patch entries against
// current entries inside an alias-keyed object array (e.g. a document's
// values[]).
//
// For Umbraco "values entry" shapes the legitimate identity is the triple
// (alias, culture, segment) — the same alias appears once per culture on a
// variant property, so keying on alias alone collapses culture-specific
// entries into one another. For shapes that don't carry culture/segment
// (e.g. doctype properties, where alias alone is the identity), both fields
// default to the empty string and the behaviour is identical to the old
// alias-only key.
func AliasMergeKey(item any) (string, map[string]any, bool) {
	alias, itemMap, ok := AliasObject(item)
	if !ok {
		return "", nil, false
	}
	culture, _ := itemMap["culture"].(string)
	segment, _ := itemMap["segment"].(string)
	return alias + "\x00" + culture + "\x00" + segment, itemMap, true
}

// variantMergeKey is the identity of a document variant: culture plus
// segment, with a null or absent part keying as the empty string so the
// invariant variant matches itself.
func variantMergeKey(item any) (string, map[string]any, bool) {
	itemMap, ok := variantObject(item)
	if !ok {
		return "", nil, false
	}
	culture, _ := itemMap["culture"].(string)
	segment, _ := itemMap["segment"].(string)
	return culture + "\x00" + segment, itemMap, true
}

func isAliasObjectArray(items []any) bool {
	for _, item := range items {
		if _, _, ok := AliasObject(item); !ok {
			return false
		}
	}
	return true
}

func isVariantObjectArray(items []any) bool {
	for _, item := range items {
		if _, ok := variantObject(item); !ok {
			return false
		}
	}
	return true
}

func variantObject(item any) (map[string]any, bool) {
	itemMap, ok := item.(map[string]any)
	if !ok {
		return nil, false
	}
	_, hasCulture := itemMap["culture"]
	_, hasSegment := itemMap["segment"]
	if !hasCulture && !hasSegment {
		return nil, false
	}
	return itemMap, true
}

func AliasObject(item any) (string, map[string]any, bool) {
	itemMap, ok := item.(map[string]any)
	if !ok {
		return "", nil, false
	}
	alias, ok := itemMap["alias"].(string)
	if !ok || alias == "" {
		return "", nil, false
	}
	return alias, itemMap, true
}

func CloneObject(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = CloneAliasValue(value)
	}
	return cloned
}

func CloneAliasValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return CloneObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = CloneAliasValue(item)
		}
		return cloned
	default:
		return typed
	}
}
