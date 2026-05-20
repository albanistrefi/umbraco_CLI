package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"umbraco-cli/internal/config"
)

func TestJSONOutputEscapesControlCharactersInStrings(t *testing.T) {
	controlString := allControlCharacters() + `"\\emoji: 😀`
	payload := map[string]any{
		"id": "doc-1",
		"values": []any{
			map[string]any{
				"alias": "body",
				"value": controlString,
			},
		},
	}

	var out bytes.Buffer
	if err := Print(payload, "json", config.OutputJSON, &out); err != nil {
		t.Fatalf("print json failed: %v", err)
	}

	if !json.Valid(out.Bytes()) {
		t.Fatalf("json output is not parseable: %q", out.String())
	}
	assertNoRawControlCharactersInJSONStrings(t, out.String())

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output did not unmarshal: %v", err)
	}
	values := decoded["values"].([]any)
	item := values[0].(map[string]any)
	if item["value"] != controlString {
		t.Fatalf("round-trip value mismatch: got %q", item["value"])
	}
}

func allControlCharacters() string {
	runes := make([]rune, 0, 32)
	for value := rune(0); value <= 0x1f; value++ {
		runes = append(runes, value)
	}
	return string(runes)
}

func assertNoRawControlCharactersInJSONStrings(t *testing.T, raw string) {
	t.Helper()

	inString := false
	escaped := false
	for index, r := range raw {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString && r <= 0x1f:
			t.Fatalf("raw control character U+%04X found inside JSON string at byte %d", r, index)
		}
	}
	if inString {
		t.Fatal("unterminated JSON string")
	}
}
