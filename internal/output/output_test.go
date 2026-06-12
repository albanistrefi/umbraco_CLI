package output

import (
	"bytes"
	"encoding/json"
	"strings"
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

func TestTableOutputReturnsMarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{name: "array item", data: []any{func() {}}},
		{name: "map value", data: map[string]any{"bad": func() {}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Print(tt.data, "table", config.OutputJSON, &out)
			if err == nil {
				t.Fatal("expected table output to return json marshal error")
			}
			if !strings.Contains(err.Error(), "unsupported type: func") {
				t.Fatalf("expected unsupported type error, got %v", err)
			}
		})
	}
}

func TestTableOutputRendersPagedItemsAsColumns(t *testing.T) {
	payload := map[string]any{
		"total": float64(2),
		"items": []any{
			map[string]any{"id": "a1", "name": "Send Slack alert", "status": "Published", "zebra": true},
			map[string]any{"id": "b2", "name": "Sync products", "status": "Draft", "steps": []any{map[string]any{"alias": "http"}}},
		},
	}

	var out bytes.Buffer
	if err := Print(payload, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines: %q", len(lines), out.String())
	}
	header := strings.Fields(lines[0])
	want := []string{"id", "name", "status", "steps", "zebra"}
	if strings.Join(header, " ") != strings.Join(want, " ") {
		t.Fatalf("column order mismatch: got %v want %v", header, want)
	}
	if !strings.HasPrefix(lines[1], "a1") || !strings.Contains(lines[1], "Send Slack alert") {
		t.Fatalf("first row missing values: %q", lines[1])
	}
	if !strings.Contains(lines[2], `[{"alias":"http"}]`) {
		t.Fatalf("nested value should render as compact JSON: %q", lines[2])
	}
	if strings.Contains(out.String(), "(2 of 2)") {
		t.Fatalf("complete pages should not print a count footer: %q", out.String())
	}
}

func TestTableOutputPrintsPartialPageFooter(t *testing.T) {
	payload := map[string]any{
		"total": float64(41),
		"items": []any{map[string]any{"id": "a1"}},
	}

	var out bytes.Buffer
	if err := Print(payload, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}
	if !strings.Contains(out.String(), "(1 of 41)") {
		t.Fatalf("expected partial-page footer, got %q", out.String())
	}
}

func TestTableOutputRendersFirstNTriagedEnvelopes(t *testing.T) {
	payload := map[string]any{
		"total":    float64(41),
		"returned": float64(1),
		"items":    []any{map[string]any{"id": "a1", "name": "Send Slack alert"}},
	}

	var out bytes.Buffer
	if err := Print(payload, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[0], "id") {
		t.Fatalf("--first-n envelope should still render as a column table: %q", out.String())
	}
	if !strings.Contains(out.String(), "(1 of 41)") {
		t.Fatalf("expected partial-page footer, got %q", out.String())
	}
}

func TestTableOutputLeavesDetailMapsWithItemsFieldAlone(t *testing.T) {
	payload := map[string]any{
		"id":    "run-1",
		"items": []any{map[string]any{"alias": "step"}},
	}

	var out bytes.Buffer
	if err := Print(payload, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}
	if !strings.Contains(out.String(), "id") || !strings.Contains(out.String(), "run-1") {
		t.Fatalf("detail map with extra keys should render key/value rows: %q", out.String())
	}
}

func TestTableOutputSanitizesAndTruncatesCells(t *testing.T) {
	payload := []any{
		map[string]any{"id": "a1", "name": "line\none\ttab", "body": strings.Repeat("x", 200)},
	}

	var out bytes.Buffer
	if err := Print(payload, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}
	rendered := out.String()
	if strings.Contains(rendered, "line\none") {
		t.Fatalf("newline survived into a cell: %q", rendered)
	}
	if !strings.Contains(rendered, "line one tab") {
		t.Fatalf("control characters should become spaces: %q", rendered)
	}
	if !strings.Contains(rendered, "x…") {
		t.Fatalf("long cell should be truncated with ellipsis: %q", rendered)
	}
	if strings.Contains(rendered, strings.Repeat("x", 81)) {
		t.Fatalf("cell exceeds the list cap: %q", rendered)
	}
}

func TestTableOutputIndexesNonObjectArrays(t *testing.T) {
	var out bytes.Buffer
	if err := Print([]any{"alpha", float64(2)}, "table", config.OutputJSON, &out); err != nil {
		t.Fatalf("print table failed: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "0") || !strings.Contains(lines[1], "2") {
		t.Fatalf("expected indexed rows, got %q", out.String())
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
