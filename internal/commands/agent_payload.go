package commands

import (
	"fmt"
	"io"
	"strings"
)

type readTriageOptions struct {
	Summarize bool
	IDsOnly   bool
	FirstN    int
}

type outputTrimOptions struct {
	Fields  string
	Summary bool
	NoEmpty bool
	Full    bool
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

func applyDocumentOutputTrim(data any, opts outputTrimOptions, warnings io.Writer) (any, error) {
	if opts.Full {
		if strings.TrimSpace(opts.Fields) != "" || opts.Summary || opts.NoEmpty {
			return nil, fmt.Errorf("--full cannot be combined with --fields, --summary, or --no-empty")
		}
		return data, nil
	}
	paths := parseFieldPaths(opts.Fields)
	if !opts.Summary && len(paths) == 0 && !opts.NoEmpty {
		return data, nil
	}

	var out any
	if opts.Summary || len(paths) > 0 {
		found := make([]bool, len(paths))
		out = projectDocumentPayload(data, opts.Summary, paths, found)
		for i, ok := range found {
			if !ok && warnings != nil {
				fmt.Fprintf(warnings, "warning: field %q not found in output\n", strings.Join(paths[i], "."))
			}
		}
	} else {
		out = data
	}
	if opts.NoEmpty {
		out = pruneEmptyValues(out)
	}
	return out, nil
}

func parseFieldPaths(fields string) [][]string {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return nil
	}
	paths := make([][]string, 0)
	for _, raw := range strings.Split(fields, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		segments := make([]string, 0)
		for _, segment := range strings.Split(raw, ".") {
			segment = strings.TrimSpace(segment)
			if segment != "" {
				segments = append(segments, segment)
			}
		}
		if len(segments) > 0 {
			paths = append(paths, segments)
		}
	}
	return paths
}

func projectDocumentPayload(data any, summary bool, paths [][]string, found []bool) any {
	if payload, ok := data.(map[string]any); ok {
		if items, ok := payload["items"].([]any); ok {
			next := cloneAnyMap(payload)
			projected := make([]any, 0, len(items))
			for _, item := range items {
				projected = append(projected, projectDocumentItem(item, summary, paths, found))
			}
			next["items"] = projected
			return next
		}
		return projectDocumentItem(payload, summary, paths, found)
	}
	if items, ok := data.([]any); ok {
		projected := make([]any, 0, len(items))
		for _, item := range items {
			projected = append(projected, projectDocumentItem(item, summary, paths, found))
		}
		return projected
	}
	return data
}

func projectDocumentItem(item any, summary bool, paths [][]string, found []bool) any {
	entry, ok := item.(map[string]any)
	if !ok {
		return item
	}
	out := map[string]any{}
	if summary {
		out = summarizeDocumentMap(entry)
	}
	for i, path := range paths {
		if value, ok := extractDottedValue(entry, path); ok {
			found[i] = true
			insertDottedValue(out, path, value)
		}
	}
	return out
}

func summarizeDocumentMap(input map[string]any) map[string]any {
	output := map[string]any{}
	copyIfPresent(output, input, "id")
	if name, _ := input["name"].(string); name != "" {
		output["name"] = name
	} else if names := treeItemNames(input); len(names) > 0 {
		output["name"] = names[0]
	}
	if docType, ok := input["documentType"].(map[string]any); ok {
		summary := map[string]any{}
		copyIfPresent(summary, docType, "id")
		copyIfPresent(summary, docType, "alias")
		copyIfPresent(summary, docType, "icon")
		if len(summary) > 0 {
			output["documentType"] = summary
		}
	}
	for _, key := range []string{"url", "urls", "route", "path", "published", "isPublished", "publishedState", "createDate", "updateDate", "publishDate"} {
		copyIfPresent(output, input, key)
	}
	return output
}

func copyIfPresent(target map[string]any, source map[string]any, key string) {
	if value, ok := source[key]; ok {
		target[key] = value
	}
}

func extractDottedValue(value any, path []string) (any, bool) {
	if len(path) == 0 {
		return value, true
	}
	switch typed := value.(type) {
	case map[string]any:
		next, ok := typed[path[0]]
		if !ok {
			return nil, false
		}
		return extractDottedValue(next, path[1:])
	case []any:
		aliasMatches := make([]any, 0)
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			alias, _ := entry["alias"].(string)
			if alias != path[0] {
				continue
			}
			var extracted any
			var found bool
			if len(path) == 1 {
				extracted, found = entry["value"]
			} else {
				extracted, found = extractDottedValue(entry["value"], path[1:])
			}
			if found {
				aliasMatches = append(aliasMatches, extracted)
			}
		}
		if len(aliasMatches) == 1 {
			return aliasMatches[0], true
		}
		if len(aliasMatches) > 1 {
			return aliasMatches, true
		}

		matches := make([]any, 0)
		for _, item := range typed {
			if extracted, ok := extractDottedValue(item, path); ok {
				matches = append(matches, extracted)
			}
		}
		if len(matches) == 1 {
			return matches[0], true
		}
		if len(matches) > 1 {
			return matches, true
		}
	}
	return nil, false
}

func insertDottedValue(target map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		target[path[0]] = value
		return
	}
	next, _ := target[path[0]].(map[string]any)
	if next == nil {
		next = map[string]any{}
		target[path[0]] = next
	}
	insertDottedValue(next, path[1:], value)
}

func pruneEmptyValues(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, raw := range typed {
			pruned := pruneEmptyValues(raw)
			if !isEmptyOutputValue(pruned) {
				out[key] = pruned
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, raw := range typed {
			pruned := pruneEmptyValues(raw)
			if !isEmptyOutputValue(pruned) {
				out = append(out, pruned)
			}
		}
		return out
	default:
		return value
	}
}

func isEmptyOutputValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
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
