package cmdkit

import (
	"encoding/json"
	"fmt"
	"strings"

	"umbraco-cli/internal/uuid"
)

// ParseJSONObject parses raw as a JSON object; label names the flag in errors.
func ParseJSONObject(raw string, label string) (map[string]any, error) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", label, err)
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return obj, nil
}

// ParsePayload parses the --json request body.
func ParsePayload(raw string) (map[string]any, error) {
	return ParseJSONObject(raw, "--json")
}

// OptionalBody parses an optional --json body; blank means an empty object.
func OptionalBody(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	return ParsePayload(raw)
}

// EnsurePayloadID keeps the body's non-blank "id" or sets a generated GUID,
// and returns the id.
func EnsurePayloadID(body map[string]any) (string, error) {
	if existing, ok := body["id"].(string); ok && strings.TrimSpace(existing) != "" {
		return existing, nil
	}
	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("failed to generate entity id: %w", err)
	}
	body["id"] = id
	return id, nil
}

// CreateResult is what a create prints: the server's answer when it carries
// an id (or reports a failure), otherwise the identity fields (id, name,
// alias, plus keys) echoed from the request body.
func CreateResult(result any, body map[string]any, keys ...string) any {
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
