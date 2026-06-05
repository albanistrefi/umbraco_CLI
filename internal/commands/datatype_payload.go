package commands

import (
	"context"
	"fmt"
	"slices"

	"umbraco-cli/internal/api"
)

type datatypeMutationSummary struct {
	Action  string `json:"action"`
	Alias   string `json:"alias"`
	Value   string `json:"value"`
	Changed bool   `json:"changed"`
	Message string `json:"message,omitempty"`
}

func fetchDatatypeObject(ctx context.Context, client *api.Client, id string) (map[string]any, error) {
	result, err := client.Get(ctx, fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, id), api.RequestOptions{})
	if err != nil {
		return nil, err
	}

	return decodeResult[map[string]any](result)
}

// mergeAliasPayload deep-merges a partial patch into a current Management API payload, preserving
// fields the patch does not mention and merging alias-keyed arrays (`properties`, `containers`,
// `values`, …) entry-wise. Used by the merge-json flows for documents, doctypes, and datatypes.
func mergeAliasPayload(current map[string]any, patch map[string]any) map[string]any {
	merged := cloneObject(current)
	for key, value := range patch {
		if existing, exists := merged[key]; exists {
			merged[key] = mergeAliasValue(existing, value)
			continue
		}

		merged[key] = cloneAliasValue(value)
	}

	return merged
}

func mergeAliasValue(current any, patch any) any {
	currentMap, currentIsMap := current.(map[string]any)
	patchMap, patchIsMap := patch.(map[string]any)
	if currentIsMap && patchIsMap {
		return mergeAliasPayload(currentMap, patchMap)
	}

	currentArray, currentIsArray := current.([]any)
	patchArray, patchIsArray := patch.([]any)
	if currentIsArray && patchIsArray && isAliasObjectArray(currentArray) && isAliasObjectArray(patchArray) {
		return mergeAliasObjectArrays(currentArray, patchArray)
	}

	return cloneAliasValue(patch)
}

func mergeAliasObjectArrays(current []any, patch []any) []any {
	merged := make([]any, 0, len(current)+len(patch))
	patchByKey := make(map[string]map[string]any, len(patch))
	for _, item := range patch {
		key, itemMap, ok := aliasMergeKey(item)
		if !ok {
			continue
		}
		patchByKey[key] = itemMap
	}

	seen := make(map[string]struct{}, len(patchByKey))
	for _, item := range current {
		key, itemMap, ok := aliasMergeKey(item)
		if !ok {
			merged = append(merged, cloneAliasValue(item))
			continue
		}

		patchItem, hasPatch := patchByKey[key]
		if !hasPatch {
			merged = append(merged, cloneAliasValue(itemMap))
			continue
		}

		merged = append(merged, mergeAliasPayload(itemMap, patchItem))
		seen[key] = struct{}{}
	}

	for _, item := range patch {
		key, itemMap, ok := aliasMergeKey(item)
		if !ok {
			merged = append(merged, cloneAliasValue(item))
			continue
		}
		if _, alreadyMerged := seen[key]; alreadyMerged {
			continue
		}
		merged = append(merged, cloneAliasValue(itemMap))
	}

	return merged
}

// aliasMergeKey returns the compound key used to match patch entries against
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
func aliasMergeKey(item any) (string, map[string]any, bool) {
	alias, itemMap, ok := aliasObject(item)
	if !ok {
		return "", nil, false
	}
	culture, _ := itemMap["culture"].(string)
	segment, _ := itemMap["segment"].(string)
	return alias + "\x00" + culture + "\x00" + segment, itemMap, true
}

func isAliasObjectArray(items []any) bool {
	for _, item := range items {
		if _, _, ok := aliasObject(item); !ok {
			return false
		}
	}
	return true
}

func aliasObject(item any) (string, map[string]any, bool) {
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

func cloneObject(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneAliasValue(value)
	}
	return cloned
}

func cloneAliasValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneAliasValue(item)
		}
		return cloned
	default:
		return typed
	}
}

func datatypeStringArrayValue(payload map[string]any, alias string) []string {
	for _, candidate := range []func(map[string]any, string) ([]string, bool){
		datatypeStringArrayFromTopLevel,
		datatypeStringArrayFromConfiguration,
		datatypeStringArrayFromValues,
	} {
		if values, ok := candidate(payload, alias); ok {
			return values
		}
	}

	return []string{}
}

func datatypeStringArrayFromTopLevel(payload map[string]any, alias string) ([]string, bool) {
	value, ok := payload[alias]
	if !ok {
		return nil, false
	}
	values, ok := stringArray(value)
	return values, ok
}

func datatypeStringArrayFromConfiguration(payload map[string]any, alias string) ([]string, bool) {
	configuration, ok := payload["configuration"].(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := configuration[alias]
	if !ok {
		return nil, false
	}
	values, ok := stringArray(value)
	return values, ok
}

func datatypeStringArrayFromValues(payload map[string]any, alias string) ([]string, bool) {
	items, ok := payload["values"].([]any)
	if !ok {
		return nil, false
	}

	for _, item := range items {
		itemAlias, itemMap, ok := aliasObject(item)
		if !ok || itemAlias != alias {
			continue
		}

		values, ok := stringArray(itemMap["value"])
		return values, ok
	}

	return nil, false
}

func stringArray(value any) ([]string, bool) {
	rawItems, ok := value.([]any)
	if !ok {
		return nil, false
	}

	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		items = append(items, text)
	}
	return items, true
}

func datatypeAddStringArrayValue(payload map[string]any, alias string, value string) map[string]any {
	values := datatypeStringArrayValue(payload, alias)
	if !slices.Contains(values, value) {
		values = append(values, value)
	}
	return datatypeSetStringArrayValue(payload, alias, values)
}

func datatypeRemoveStringArrayValue(payload map[string]any, alias string, value string) map[string]any {
	current := datatypeStringArrayValue(payload, alias)
	next := make([]string, 0, len(current))
	for _, item := range current {
		if item != value {
			next = append(next, item)
		}
	}
	return datatypeSetStringArrayValue(payload, alias, next)
}

func datatypeSetStringArrayValue(payload map[string]any, alias string, values []string) map[string]any {
	merged := cloneObject(payload)
	encoded := make([]any, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, value)
	}

	if configuration, ok := merged["configuration"].(map[string]any); ok {
		if _, exists := configuration[alias]; exists {
			nextConfiguration := cloneObject(configuration)
			nextConfiguration[alias] = encoded
			merged["configuration"] = nextConfiguration
			return merged
		}
	}

	if topLevelValue, exists := merged[alias]; exists {
		if _, ok := stringArray(topLevelValue); ok {
			merged[alias] = encoded
			return merged
		}
	}

	if rawValues, ok := merged["values"].([]any); ok {
		nextValues := make([]any, 0, len(rawValues)+1)
		replaced := false
		for _, item := range rawValues {
			itemAlias, itemMap, ok := aliasObject(item)
			if !ok {
				nextValues = append(nextValues, cloneAliasValue(item))
				continue
			}
			if itemAlias != alias {
				nextValues = append(nextValues, cloneObject(itemMap))
				continue
			}

			nextItem := cloneObject(itemMap)
			nextItem["value"] = encoded
			nextValues = append(nextValues, nextItem)
			replaced = true
		}

		if !replaced {
			nextValues = append(nextValues, map[string]any{
				"alias": alias,
				"value": encoded,
			})
		}

		merged["values"] = nextValues
		return merged
	}

	merged[alias] = encoded
	return merged
}

func mutateDatatypeStringArray(ctx context.Context, client *api.Client, id string, alias string, value string, dryRun bool, action string) (any, error) {
	payload, err := fetchDatatypeObject(ctx, client, id)
	if err != nil {
		return nil, err
	}

	currentValues := datatypeStringArrayValue(payload, alias)
	switch action {
	case "add":
		if slices.Contains(currentValues, value) {
			return datatypeMutationSummary{
				Action:  action,
				Alias:   alias,
				Value:   value,
				Changed: false,
				Message: "value already present",
			}, nil
		}

		next := datatypeAddStringArrayValue(payload, alias, value)
		return client.Put(ctx, fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, id), next, api.RequestOptions{DryRun: dryRun, SkipValidation: true})
	case "remove":
		if !slices.Contains(currentValues, value) {
			return datatypeMutationSummary{
				Action:  action,
				Alias:   alias,
				Value:   value,
				Changed: false,
				Message: "value not present",
			}, nil
		}

		next := datatypeRemoveStringArrayValue(payload, alias, value)
		return client.Put(ctx, fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, id), next, api.RequestOptions{DryRun: dryRun, SkipValidation: true})
	default:
		return nil, fmt.Errorf("unsupported datatype mutation action: %s", action)
	}
}
