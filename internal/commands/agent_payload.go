package commands

import (
	"fmt"
	"strings"
)

type readTriageOptions struct {
	Summarize bool
	IDsOnly   bool
	FirstN    int
}

func ensurePayloadID(body map[string]any) (string, error) {
	if existing, ok := body["id"].(string); ok && strings.TrimSpace(existing) != "" {
		return existing, nil
	}
	id, err := newUUIDv4()
	if err != nil {
		return "", fmt.Errorf("failed to generate entity id: %w", err)
	}
	body["id"] = id
	return id, nil
}

func createResult(result any, body map[string]any, keys ...string) any {
	if resultMap, ok := result.(map[string]any); ok {
		if id, ok := resultMap["id"].(string); ok && strings.TrimSpace(id) != "" {
			return result
		}
		if success, ok := resultMap["success"].(bool); ok && !success {
			return result
		}
	}
	if result != nil {
		if resultMap, ok := result.(map[string]any); !ok || len(resultMap) != 1 || resultMap["success"] != true {
			return result
		}
	}

	minimal := map[string]any{}
	for _, key := range append([]string{"id", "name", "alias"}, keys...) {
		if value, ok := body[key]; ok && value != nil {
			minimal[key] = value
		}
	}
	if len(minimal) == 0 {
		return result
	}
	return minimal
}

func normalizeDoctypePayload(body map[string]any) {
	normalizeDoctypeProperties(body["properties"])
}

// normalizeDoctypePayloadHook adapts normalizeDoctypePayload to the
// error-returning Normalize contract used by update specs.
func normalizeDoctypePayloadHook(body map[string]any) error {
	normalizeDoctypePayload(body)
	return nil
}

func normalizeDoctypeProperties(raw any) {
	properties, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range properties {
		property, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := property["dataType"]; !exists {
			if dataTypeID, ok := property["dataTypeId"].(string); ok && strings.TrimSpace(dataTypeID) != "" {
				property["dataType"] = map[string]any{"id": dataTypeID}
				delete(property, "dataTypeId")
			}
		}
	}
}

// applyFieldsProjection trims each item in a collection (or the lone object) down to the
// comma-separated keys named in fields. Used to give --fields a guaranteed effect even on
// Management API endpoints that ignore the ?fields= query parameter.
func applyFieldsProjection(data any, fields string) any {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return data
	}
	keep := map[string]struct{}{}
	for _, raw := range strings.Split(fields, ",") {
		key := strings.TrimSpace(raw)
		if key != "" {
			keep[key] = struct{}{}
		}
	}
	if len(keep) == 0 {
		return data
	}

	if payload, ok := data.(map[string]any); ok {
		if items, ok := payload["items"].([]any); ok {
			next := cloneAnyMap(payload)
			projected := make([]any, 0, len(items))
			for _, item := range items {
				projected = append(projected, projectFieldsFromAny(item, keep))
			}
			next["items"] = projected
			return next
		}
		return projectFieldsFromAny(payload, keep)
	}
	if items, ok := data.([]any); ok {
		projected := make([]any, 0, len(items))
		for _, item := range items {
			projected = append(projected, projectFieldsFromAny(item, keep))
		}
		return projected
	}
	return data
}

func projectFieldsFromAny(value any, keep map[string]struct{}) any {
	entry, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(keep))
	for key := range keep {
		if v, ok := entry[key]; ok {
			out[key] = v
		}
	}
	return out
}

func applyReadTriage(data any, opts readTriageOptions) any {
	if !opts.Summarize && !opts.IDsOnly && opts.FirstN <= 0 {
		return data
	}

	if payload, ok := data.(map[string]any); ok {
		if items, ok := payload["items"].([]any); ok {
			next := cloneAnyMap(payload)
			next["items"] = triageItems(items, opts)
			if opts.FirstN > 0 {
				next["returned"] = len(next["items"].([]any))
			}
			return next
		}
	}
	if items, ok := data.([]any); ok {
		return triageItems(items, opts)
	}
	return data
}

func triageItems(items []any, opts readTriageOptions) []any {
	limit := len(items)
	if opts.FirstN > 0 && opts.FirstN < limit {
		limit = opts.FirstN
	}
	result := make([]any, 0, limit)
	for _, item := range items[:limit] {
		entry, ok := item.(map[string]any)
		if !ok {
			result = append(result, item)
			continue
		}
		if opts.IDsOnly {
			result = append(result, entry["id"])
			continue
		}
		if opts.Summarize {
			result = append(result, summarizeMap(entry))
			continue
		}
		result = append(result, item)
	}
	return result
}

func summarizeMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for _, key := range []string{"id", "name", "alias"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}

func cloneAnyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
