package cmdkit

import (
	"reflect"
	"testing"

	"umbraco-cli/internal/uuid"
)

func TestPayloadParsingHelpers(t *testing.T) {
	if body, err := OptionalBody(""); err != nil || len(body) != 0 || body == nil {
		t.Fatalf("expected an empty, non-nil body, got %v %v", body, err)
	}
	if body, err := OptionalBody(`{"a":1}`); err != nil || body["a"] != float64(1) {
		t.Fatalf("expected the parsed body, got %v %v", body, err)
	}
	if body, err := TargetActionBody("", "p"); err != nil || !reflect.DeepEqual(body, map[string]any{"target": map[string]any{"id": "p"}}) {
		t.Fatalf("expected the --to envelope, got %v %v", body, err)
	}
	if err := StripFields("a", "b")(map[string]any{"a": 1}); err != nil {
		t.Fatalf("strip: %v", err)
	}
}

func TestEnsurePayloadIDKeepsOrGeneratesAnID(t *testing.T) {
	body := map[string]any{"id": "keep"}
	if id, err := EnsurePayloadID(body); err != nil || id != "keep" {
		t.Fatalf("expected an existing id to be kept, got %q %v", id, err)
	}
	generated := map[string]any{}
	if id, err := EnsurePayloadID(generated); err != nil || !uuid.Valid(id) || generated["id"] != id {
		t.Fatalf("expected a generated id set on the body, got %q %v (%v)", id, err, generated)
	}
}
