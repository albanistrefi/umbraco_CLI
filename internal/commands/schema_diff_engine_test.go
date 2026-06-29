package commands

import "testing"

func TestSchemaDiffIdenticalIgnoresVolatileFieldsAndOrdering(t *testing.T) {
	refsA := schemaDiffReferences{DataTypes: map[string]string{"dt-a": "Textstring"}}
	refsB := schemaDiffReferences{DataTypes: map[string]string{"dt-b": "Textstring"}}
	left := []schemaDiffEntity{normalizeSchemaEntity(schemaDiffDoctype, map[string]any{
		"id":         "doctype-a",
		"alias":      "home",
		"name":       "Home",
		"updateDate": "2026-06-01T10:00:00Z",
		"properties": []any{
			map[string]any{"id": "prop-a", "alias": "title", "dataType": map[string]any{"id": "dt-a"}},
			map[string]any{"id": "prop-b", "alias": "body", "dataType": map[string]any{"id": "dt-a"}},
		},
	}, refsA)}
	right := []schemaDiffEntity{normalizeSchemaEntity(schemaDiffDoctype, map[string]any{
		"id":         "doctype-b",
		"alias":      "home",
		"name":       "Home",
		"updateDate": "2026-06-02T10:00:00Z",
		"properties": []any{
			map[string]any{"id": "prop-y", "alias": "body", "dataType": map[string]any{"id": "dt-b"}},
			map[string]any{"id": "prop-x", "alias": "title", "dataType": map[string]any{"id": "dt-b"}},
		},
	}, refsB)}

	report := computeSchemaDiff("dev", "live", left, right, schemaDiffOptions{Entities: []schemaDiffEntityKind{schemaDiffDoctype}})
	if !report.Equal {
		t.Fatalf("expected identical normalized schema, got %+v", report)
	}
}

func TestSchemaDiffClassifiesAddedRemovedAndChanged(t *testing.T) {
	left := []schemaDiffEntity{
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "home", "name": "Home", "allowedAtRoot": true}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "old", "name": "Old"}, schemaDiffReferences{}),
	}
	right := []schemaDiffEntity{
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "home", "name": "Home", "allowedAtRoot": false}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "new", "name": "New"}, schemaDiffReferences{}),
	}

	report := computeSchemaDiff("dev", "live", left, right, schemaDiffOptions{Entities: []schemaDiffEntityKind{schemaDiffDoctype}})
	diff := report.Differences[schemaDiffDoctype]
	if report.Equal {
		t.Fatalf("expected differences")
	}
	if len(diff.Added) != 1 || diff.Added[0].Alias != "new" {
		t.Fatalf("expected added new doctype, got %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Alias != "old" {
		t.Fatalf("expected removed old doctype, got %+v", diff.Removed)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Alias != "home" {
		t.Fatalf("expected changed home doctype, got %+v", diff.Changed)
	}
	if len(diff.Changed[0].Fields) != 1 || diff.Changed[0].Fields[0].Path != "allowedAtRoot" {
		t.Fatalf("expected field-level allowedAtRoot delta, got %+v", diff.Changed[0].Fields)
	}
}

func TestSchemaDiffFiltersAndEntityScope(t *testing.T) {
	left := []schemaDiffEntity{
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "home", "name": "Home"}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "blog", "name": "Blog"}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDatatype, map[string]any{"name": "Textstring", "editorAlias": "Umbraco.TextBox"}, schemaDiffReferences{}),
	}
	right := []schemaDiffEntity{
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "home", "name": "Homepage"}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDoctype, map[string]any{"alias": "blog", "name": "Blog"}, schemaDiffReferences{}),
		normalizeSchemaEntity(schemaDiffDatatype, map[string]any{"name": "Textstring", "editorAlias": "Umbraco.TextArea"}, schemaDiffReferences{}),
	}

	report := computeSchemaDiff("dev", "live", left, right, schemaDiffOptions{
		Entities: []schemaDiffEntityKind{schemaDiffDoctype},
		Include:  []string{"home,Textstring"},
		Exclude:  []string{"blog"},
	})
	if report.Equal {
		t.Fatalf("expected included doctype difference")
	}
	if _, ok := report.Differences[schemaDiffDatatype]; ok {
		t.Fatalf("did not expect datatype diff when scoped to doctype")
	}
	diff := report.Differences[schemaDiffDoctype]
	if len(diff.Changed) != 1 || diff.Changed[0].Alias != "home" {
		t.Fatalf("expected only home doctype change, got %+v", diff)
	}
}

func TestParseSchemaDiffEntitiesRejectsUnknownEntity(t *testing.T) {
	if _, err := parseSchemaDiffEntities("doctype,banana"); err == nil {
		t.Fatalf("expected unknown entity error")
	}
}
