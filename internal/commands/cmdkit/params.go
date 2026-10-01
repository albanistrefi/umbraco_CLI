package cmdkit

import (
	"strings"
)

// ParseParams parses the --params JSON object; blank means no params.
func ParseParams(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return ParseJSONObject(raw, "--params")
}

// MergeParams folds convenience-flag values into a --params map. The
// documented precedence on every command that accepts both: --params wins
// on key collisions, flags fill the gaps.
func MergeParams(params map[string]any, flagValues map[string]any) map[string]any {
	if len(flagValues) == 0 {
		return params
	}
	if params == nil {
		params = map[string]any{}
	}
	for key, value := range flagValues {
		if _, exists := params[key]; !exists {
			params[key] = value
		}
	}
	return params
}

// WithParam clones a params map and sets one extra key, for fallback
// candidates whose endpoints take an ID as a query parameter.
func WithParam(params map[string]any, key string, value any) map[string]any {
	next := make(map[string]any, len(params)+1)
	for k, v := range params {
		next[k] = v
	}
	next[key] = value
	return next
}

// ApplyPaginationParams folds skip/take into an existing params map, leaving
// it nil-safe so callers don't need to pre-allocate when no other params
// are present.
func ApplyPaginationParams(params map[string]any, skip int, take int) map[string]any {
	if skip < 0 && take < 0 {
		return params
	}
	if params == nil {
		params = map[string]any{}
	}
	if skip >= 0 {
		params["skip"] = skip
	}
	if take >= 0 {
		params["take"] = take
	}
	return params
}

// StringsToAny converts a string list to a query-parameter value, which the
// API client sends as one repeated key (?id=a&id=b).
func StringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
