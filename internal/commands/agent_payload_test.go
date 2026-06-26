package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestDocumentOutputTrimProjectsDottedFieldsAndValueAliases(t *testing.T) {
	payload := map[string]any{
		"id":   "doc-1",
		"name": "Home",
		"documentType": map[string]any{
			"id":    "dt-1",
			"alias": "homePage",
		},
		"variants": []any{
			map[string]any{"name": "Home", "culture": "en-US"},
			map[string]any{"name": "Hem", "culture": "sv-SE"},
		},
		"values": []any{
			map[string]any{"alias": "bodyText", "value": "Welcome"},
			map[string]any{"alias": "empty", "value": ""},
		},
		"large": map[string]any{"nested": true},
	}

	out, err := applyDocumentOutputTrim(payload, outputTrimOptions{Fields: "id,documentType.alias,variants.name,values.bodyText"}, nil)
	if err != nil {
		t.Fatalf("trim failed: %v", err)
	}
	projected := out.(map[string]any)
	if len(projected) != 4 {
		t.Fatalf("expected four projected top-level keys, got %+v", projected)
	}
	if projected["id"] != "doc-1" {
		t.Fatalf("expected id projection, got %+v", projected)
	}
	if got := projected["documentType"].(map[string]any)["alias"]; got != "homePage" {
		t.Fatalf("expected documentType alias, got %v", got)
	}
	names := projected["variants"].(map[string]any)["name"].([]any)
	if len(names) != 2 || names[0] != "Home" || names[1] != "Hem" {
		t.Fatalf("expected variant names, got %+v", names)
	}
	if got := projected["values"].(map[string]any)["bodyText"]; got != "Welcome" {
		t.Fatalf("expected values alias projection, got %v", got)
	}
	if _, ok := projected["large"]; ok {
		t.Fatalf("unexpected unrequested field in projection: %+v", projected)
	}
}

func TestDocumentOutputTrimSummaryFieldsAndNoEmpty(t *testing.T) {
	payload := map[string]any{
		"id":   "doc-1",
		"name": "",
		"variants": []any{
			map[string]any{"name": "Home"},
		},
		"documentType": map[string]any{
			"id":    "dt-1",
			"alias": "",
			"icon":  nil,
		},
		"route":      map[string]any{"path": "/"},
		"updateDate": "",
		"values":     []any{map[string]any{"alias": "bodyText", "value": "Welcome"}},
	}

	out, err := applyDocumentOutputTrim(payload, outputTrimOptions{Summary: true, Fields: "values.bodyText", NoEmpty: true}, nil)
	if err != nil {
		t.Fatalf("trim failed: %v", err)
	}
	projected := out.(map[string]any)
	if projected["id"] != "doc-1" || projected["name"] != "Home" {
		t.Fatalf("expected summary id and variant-derived name, got %+v", projected)
	}
	docType := projected["documentType"].(map[string]any)
	if docType["id"] != "dt-1" {
		t.Fatalf("expected document type id, got %+v", docType)
	}
	if _, ok := docType["alias"]; ok {
		t.Fatalf("expected empty document type alias pruned, got %+v", docType)
	}
	if _, ok := projected["updateDate"]; ok {
		t.Fatalf("expected empty updateDate pruned, got %+v", projected)
	}
	if got := projected["values"].(map[string]any)["bodyText"]; got != "Welcome" {
		t.Fatalf("expected summary plus requested value alias, got %v", got)
	}
	if _, ok := projected["values"].(map[string]any)["empty"]; ok {
		t.Fatalf("unexpected empty value in projection: %+v", projected["values"])
	}
}

func TestDocumentOutputTrimPreservesEnvelopeAndWarnsUnknownFields(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{"id": "doc-1", "name": "Home", "extra": "drop"},
			map[string]any{"id": "doc-2", "name": "About", "extra": "drop"},
		},
		"total": 2,
	}
	var warnings bytes.Buffer

	out, err := applyDocumentOutputTrim(payload, outputTrimOptions{Fields: "id,missing"}, &warnings)
	if err != nil {
		t.Fatalf("trim failed: %v", err)
	}
	envelope := out.(map[string]any)
	if envelope["total"] != 2 {
		t.Fatalf("expected envelope metadata preserved, got %+v", envelope)
	}
	items := envelope["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected item count preserved, got %+v", items)
	}
	first := items[0].(map[string]any)
	if first["id"] != "doc-1" || len(first) != 1 {
		t.Fatalf("expected projected item, got %+v", first)
	}
	if got := warnings.String(); !strings.Contains(got, `warning: field "missing" not found in output`) {
		t.Fatalf("expected unknown field warning, got %q", got)
	}
}

func TestDocumentOutputTrimFullRejectsOtherTrimFlags(t *testing.T) {
	_, err := applyDocumentOutputTrim(map[string]any{"id": "doc-1"}, outputTrimOptions{Full: true, Fields: "id"}, nil)
	if err == nil {
		t.Fatalf("expected --full plus --fields to fail")
	}
}
