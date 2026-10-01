package cmdkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/jsonvalue"
)

// FetchObject retrieves a resource as a generic object, for merge flows
// that need the current server-side state.
func FetchObject(ctx context.Context, client *api.Client, path string, opts api.RequestOptions) (map[string]any, error) {
	result, err := client.Get(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	return ObjectFromResult("GET "+path, result)
}

// ObjectFromResult turns a decoded response into an object. A JSON string
// that itself holds an object is unwrapped (some endpoints double-encode
// their payload — seen on Umbraco Cloud for Deploy's client configuration);
// anything else is reported with the request and the shape received, so a
// surprising body reads as "GET /x returned an array" rather than a bare
// json.Unmarshal error.
func ObjectFromResult(request string, result any) (map[string]any, error) {
	switch value := result.(type) {
	case map[string]any:
		return value, nil
	case string:
		var nested any
		if json.Unmarshal([]byte(value), &nested) == nil {
			if object, ok := nested.(map[string]any); ok {
				return object, nil
			}
		}
		return nil, fmt.Errorf("%s returned a string, not a JSON object: %s", request, truncateForError(value, 200))
	case nil:
		return nil, fmt.Errorf("%s returned an empty body where a JSON object was expected", request)
	default:
		encoded, _ := json.Marshal(value)
		return nil, fmt.Errorf("%s returned %s, not a JSON object: %s", request, jsonvalue.ShapeName(value), truncateForError(string(encoded), 200))
	}
}

// truncateForError bounds and sanitizes server-provided text before it is
// interpolated into an error: control characters and Unicode format
// characters are stripped (as API error text already is) so a response
// cannot steer the terminal, and the value is quoted.
func truncateForError(text string, limit int) string {
	text = api.SanitizeTerminalText(strings.TrimSpace(text))
	if len(text) > limit {
		text = text[:limit] + "…"
	}
	return strconv.Quote(text)
}

// ResultItems returns the items of an {items} envelope or a bare array, or
// nil for any other shape.
func ResultItems(result any) []any {
	if payload, ok := result.(map[string]any); ok {
		if items, ok := payload["items"].([]any); ok {
			return items
		}
	}
	if items, ok := result.([]any); ok {
		return items
	}
	return nil
}

// ItemID returns an item's trimmed string "id", or "".
func ItemID(item any) string {
	entry, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := entry["id"].(string)
	return strings.TrimSpace(id)
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
