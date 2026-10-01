package commands

import (
	"encoding/json"
	"fmt"
)

// buildUpdatePropertiesPatch normalizes the three accepted input shapes for
// "<resource> update-properties --json" into a {"values":[...]} envelope ready
// to merge into the current resource via cmdkit.MergeAliasPayload. Used by both
// document update-properties and member update-properties — any future
// resource with the same values[] shape can reuse it.
//
// Returning a structured error here is what prevents the v0.3.15 footgun
// where object-shape input landed at the resource root instead of inside
// values[] and silently no-op'd against the Management API.
func buildUpdatePropertiesPatch(raw string) (map[string]any, error) {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("invalid --json: %w", err)
	}

	switch payload := parsed.(type) {
	case []any:
		if err := validateValuesEntries(payload); err != nil {
			return nil, err
		}
		return map[string]any{"values": payload}, nil

	case map[string]any:
		if values, isEnvelope := payload["values"].([]any); isEnvelope && len(payload) == 1 {
			if err := validateValuesEntries(values); err != nil {
				return nil, err
			}
			return map[string]any{"values": values}, nil
		}
		values := make([]any, 0, len(payload))
		for alias, value := range payload {
			values = append(values, map[string]any{
				"alias":   alias,
				"value":   value,
				"culture": nil,
				"segment": nil,
			})
		}
		return map[string]any{"values": values}, nil

	default:
		return nil, fmt.Errorf("--json must be an object, an array of values entries, or an envelope {\"values\":[...]}; got %T", parsed)
	}
}

// validateValuesEntries enforces that every entry in a values[]-shaped array
// carries both 'alias' and 'value'. An explicit "value": null is fine (Umbraco
// treats it as "clear the value"), but a missing value key is rejected —
// otherwise the merge preserves the existing value and the PUT silently
// no-op's on that property, recreating the exact footgun this surface was
// built to prevent.
func validateValuesEntries(items []any) error {
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("values entry %d must be an object with 'alias' and 'value' keys", i)
		}
		alias, hasAlias := entry["alias"]
		if !hasAlias {
			return fmt.Errorf("values entry %d is missing required 'alias' key", i)
		}
		if _, hasValue := entry["value"]; !hasValue {
			return fmt.Errorf("values entry %d (alias %q) is missing required 'value' key; pass \"value\":null to clear", i, alias)
		}
	}
	return nil
}

// coalescePutResult returns true for a real (non-dry-run) PUT whose response
// body was empty (Umbraco answers 204 No Content for successful update /
// publish calls on documents, members, etc.). The previous behaviour of
// returning the raw nil here surfaced as {"updated":null} or
// {"published":null} in command output, which scripts could not distinguish
// from failure.
func coalescePutResult(result any, dryRun bool) any {
	if dryRun {
		return result
	}
	if result == nil {
		return true
	}
	return result
}
