package schema

import "testing"

// TestBindingsResolveAgainstOpenAPIDocument is the drift guard between the
// CLI's endpoint bindings and the vendored Management API OpenAPI document:
// a binding pointing at an operation the spec does not declare is either a
// typo or an endpoint that needs a Manual entry.
func TestBindingsResolveAgainstOpenAPIDocument(t *testing.T) {
	for endpoint, binding := range endpointBindings {
		if binding.Manual != nil {
			continue
		}
		if _, ok := openAPIOperations[binding.Method+" "+binding.Path]; !ok {
			t.Errorf("binding %s points at %s %s, which the OpenAPI document does not declare; fix the binding or mark it Manual", endpoint, binding.Method, binding.Path)
		}
	}
}

func TestGeneratedOperationsCarryModelDetail(t *testing.T) {
	op, ok := openAPIOperations["PUT /document/{id}"]
	if !ok {
		t.Fatalf("expected PUT /document/{id} in generated operations")
	}
	if op.RequestBody == nil || len(op.RequestBody.Properties) == 0 {
		t.Fatalf("expected generated request body detail, got %+v", op.RequestBody)
	}
	if _, ok := op.RequestBody.Properties["values"]; !ok {
		t.Fatalf("expected 'values' property on the update model, got %+v", op.RequestBody.Properties)
	}
}

func TestSchemasComposeBindingOverlays(t *testing.T) {
	s, ok := Schemas["document.get"]
	if !ok {
		t.Fatalf("missing document.get schema")
	}
	if _, ok := s.QueryParams["fields"]; !ok {
		t.Fatalf("expected CLI fields overlay on document.get, got %+v", s.QueryParams)
	}
	for _, key := range []string{"summary", "no-empty", "full"} {
		if _, ok := s.QueryParams[key]; !ok {
			t.Fatalf("expected CLI trim overlay %q on document.get, got %+v", key, s.QueryParams)
		}
	}
	if s.PathParams["id"].Format != "uuid" {
		t.Fatalf("expected spec-derived id path param, got %+v", s.PathParams)
	}

	search, ok := Schemas["document.search"]
	if !ok {
		t.Fatalf("missing document.search schema")
	}
	for _, key := range []string{"fields", "summary", "no-empty", "full"} {
		if _, ok := search.QueryParams[key]; !ok {
			t.Fatalf("expected CLI trim overlay %q on document.search, got %+v", key, search.QueryParams)
		}
	}
}
