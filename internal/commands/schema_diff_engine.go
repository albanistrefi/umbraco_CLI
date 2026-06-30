package commands

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type schemaDiffEntityKind string

const (
	schemaDiffDoctype  schemaDiffEntityKind = "doctype"
	schemaDiffDatatype schemaDiffEntityKind = "datatype"
)

type schemaDiffOptions struct {
	Entities []schemaDiffEntityKind
	Include  []string
	Exclude  []string
}

type schemaDiffEntity struct {
	Kind       schemaDiffEntityKind
	Alias      string
	Name       string
	Raw        map[string]any
	Normalized any
}

type schemaDiffReport struct {
	EnvA        string                            `json:"envA"`
	EnvB        string                            `json:"envB"`
	Entities    []schemaDiffEntityKind            `json:"entities"`
	Filters     schemaDiffFilters                 `json:"filters"`
	Counts      schemaDiffCounts                  `json:"counts"`
	Differences map[schemaDiffEntityKind]kindDiff `json:"differences"`
	Equal       bool                              `json:"equal"`
}

type schemaDiffFilters struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type schemaDiffCounts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

type kindDiff struct {
	Added   []schemaEntitySummary `json:"added"`
	Removed []schemaEntitySummary `json:"removed"`
	Changed []schemaEntityChange  `json:"changed"`
}

type schemaEntitySummary struct {
	Alias string `json:"alias"`
	Name  string `json:"name,omitempty"`
}

type schemaEntityChange struct {
	Alias  string             `json:"alias"`
	Name   string             `json:"name,omitempty"`
	Fields []schemaFieldDelta `json:"fields"`
}

type schemaFieldDelta struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

func defaultSchemaDiffEntities() []schemaDiffEntityKind {
	return []schemaDiffEntityKind{schemaDiffDoctype, schemaDiffDatatype}
}

func computeSchemaDiff(envA string, envB string, left []schemaDiffEntity, right []schemaDiffEntity, opts schemaDiffOptions) schemaDiffReport {
	entities := opts.Entities
	if len(entities) == 0 {
		entities = defaultSchemaDiffEntities()
	}
	include := normalizeStringSet(opts.Include)
	exclude := normalizeStringSet(opts.Exclude)

	report := schemaDiffReport{
		EnvA:     envA,
		EnvB:     envB,
		Entities: append([]schemaDiffEntityKind(nil), entities...),
		Filters: schemaDiffFilters{
			Include: sortedSetKeys(include),
			Exclude: sortedSetKeys(exclude),
		},
		Differences: map[schemaDiffEntityKind]kindDiff{},
		Equal:       true,
	}

	for _, kind := range entities {
		leftByAlias := entitiesByAlias(left, kind, include, exclude)
		rightByAlias := entitiesByAlias(right, kind, include, exclude)
		diff := diffEntityKind(leftByAlias, rightByAlias)
		report.Differences[kind] = diff
		report.Counts.Added += len(diff.Added)
		report.Counts.Removed += len(diff.Removed)
		report.Counts.Changed += len(diff.Changed)
	}
	report.Equal = report.Counts.Added == 0 && report.Counts.Removed == 0 && report.Counts.Changed == 0
	return report
}

func entitiesByAlias(entities []schemaDiffEntity, kind schemaDiffEntityKind, include map[string]struct{}, exclude map[string]struct{}) map[string]schemaDiffEntity {
	out := map[string]schemaDiffEntity{}
	for _, entity := range entities {
		if entity.Kind != kind {
			continue
		}
		alias := strings.TrimSpace(entity.Alias)
		if alias == "" {
			continue
		}
		filterKey := strings.ToLower(alias)
		if len(include) > 0 {
			if _, ok := include[filterKey]; !ok {
				continue
			}
		}
		if _, ok := exclude[filterKey]; ok {
			continue
		}
		out[alias] = entity
	}
	return out
}

func diffEntityKind(left map[string]schemaDiffEntity, right map[string]schemaDiffEntity) kindDiff {
	diff := kindDiff{}
	aliases := unionSortedEntityAliases(left, right)
	for _, alias := range aliases {
		leftEntity, leftOK := left[alias]
		rightEntity, rightOK := right[alias]
		switch {
		case !leftOK && rightOK:
			diff.Added = append(diff.Added, summarizeSchemaEntity(rightEntity))
		case leftOK && !rightOK:
			diff.Removed = append(diff.Removed, summarizeSchemaEntity(leftEntity))
		case leftOK && rightOK:
			if !reflect.DeepEqual(leftEntity.Normalized, rightEntity.Normalized) {
				deltas := diffNormalizedValue("", leftEntity.Normalized, rightEntity.Normalized)
				diff.Changed = append(diff.Changed, schemaEntityChange{
					Alias:  alias,
					Name:   firstNonEmpty(leftEntity.Name, rightEntity.Name),
					Fields: deltas,
				})
			}
		}
	}
	return diff
}

func normalizeSchemaEntity(kind schemaDiffEntityKind, raw map[string]any, refs schemaDiffReferences) schemaDiffEntity {
	alias := schemaEntityAlias(kind, raw)
	return schemaDiffEntity{
		Kind:       kind,
		Alias:      alias,
		Name:       schemaEntityName(raw),
		Raw:        raw,
		Normalized: normalizeSchemaValue(raw, refs),
	}
}

type schemaDiffReferences struct {
	DataTypes     map[string]string
	DocumentTypes map[string]string
}

func schemaEntityAlias(kind schemaDiffEntityKind, raw map[string]any) string {
	if alias, ok := stringField(raw, "alias"); ok {
		return alias
	}
	if kind == schemaDiffDatatype {
		if name, ok := stringField(raw, "name"); ok {
			return name
		}
	}
	if name, ok := stringField(raw, "name"); ok {
		return name
	}
	return ""
}

func schemaEntityName(raw map[string]any) string {
	if name, ok := stringField(raw, "name"); ok {
		return name
	}
	return ""
}

func normalizeSchemaValue(value any, refs schemaDiffReferences) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeSchemaMap(typed, refs)
	case []any:
		return normalizeSchemaSlice(typed, refs)
	default:
		return typed
	}
}

func normalizeSchemaMap(input map[string]any, refs schemaDiffReferences) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		normalizedKey := normalizeSchemaKey(key)
		if isVolatileSchemaKey(normalizedKey) {
			continue
		}
		switch normalizedKey {
		case "dataTypeId":
			if alias := refs.DataTypes[schemaDiffStringValue(value)]; alias != "" {
				out["dataTypeAlias"] = alias
				continue
			}
		case "documentTypeId":
			if alias := refs.DocumentTypes[schemaDiffStringValue(value)]; alias != "" {
				out["documentTypeAlias"] = alias
				continue
			}
		case "dataType":
			if mapped, ok := normalizeReferenceMap(value, refs.DataTypes); ok {
				out[normalizedKey] = mapped
				continue
			}
		case "documentType":
			if mapped, ok := normalizeReferenceMap(value, refs.DocumentTypes); ok {
				out[normalizedKey] = mapped
				continue
			}
		}
		out[normalizedKey] = normalizeSchemaValue(value, refs)
	}
	return out
}

func normalizeReferenceMap(value any, refs map[string]string) (any, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	next := map[string]any{}
	for key, nested := range obj {
		normalizedKey := normalizeSchemaKey(key)
		if normalizedKey == "id" {
			if alias := refs[schemaDiffStringValue(nested)]; alias != "" {
				next["alias"] = alias
			}
			continue
		}
		if isVolatileSchemaKey(normalizedKey) {
			continue
		}
		next[normalizedKey] = normalizeSchemaValue(nested, schemaDiffReferences{})
	}
	return next, true
}

func normalizeSchemaSlice(input []any, refs schemaDiffReferences) []any {
	out := make([]any, 0, len(input))
	for _, item := range input {
		out = append(out, normalizeSchemaValue(item, refs))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return stableJSON(out[i]) < stableJSON(out[j])
	})
	return out
}

func normalizeSchemaKey(key string) string {
	return strings.TrimSpace(key)
}

func isVolatileSchemaKey(key string) bool {
	switch strings.ToLower(key) {
	case "id", "key", "udi", "createDate", "createdate", "updateDate", "updatedate", "deleteDate", "deletedate", "sortOrder", "sortorder":
		return true
	default:
		return false
	}
}

func diffNormalizedValue(path string, before any, after any) []schemaFieldDelta {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	beforeMap, beforeMapOK := before.(map[string]any)
	afterMap, afterMapOK := after.(map[string]any)
	if beforeMapOK && afterMapOK {
		return diffNormalizedMaps(path, beforeMap, afterMap)
	}
	return []schemaFieldDelta{{Path: pathOrRoot(path), Before: before, After: after}}
}

func diffNormalizedMaps(path string, before map[string]any, after map[string]any) []schemaFieldDelta {
	keys := make([]string, 0, len(before)+len(after))
	seen := map[string]struct{}{}
	for key := range before {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range after {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	deltas := make([]schemaFieldDelta, 0)
	for _, key := range keys {
		nextPath := key
		if path != "" {
			nextPath = path + "." + key
		}
		beforeValue, beforeOK := before[key]
		afterValue, afterOK := after[key]
		switch {
		case !beforeOK:
			deltas = append(deltas, schemaFieldDelta{Path: nextPath, After: afterValue})
		case !afterOK:
			deltas = append(deltas, schemaFieldDelta{Path: nextPath, Before: beforeValue})
		default:
			deltas = append(deltas, diffNormalizedValue(nextPath, beforeValue, afterValue)...)
		}
	}
	return deltas
}

func parseSchemaDiffEntities(raw string) ([]schemaDiffEntityKind, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultSchemaDiffEntities(), nil
	}
	var entities []schemaDiffEntityKind
	seen := map[schemaDiffEntityKind]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.ToLower(strings.TrimSpace(part))
		var kind schemaDiffEntityKind
		switch value {
		case "doctype", "documenttype", "document-type", "document_type":
			kind = schemaDiffDoctype
		case "datatype", "data-type", "data_type":
			kind = schemaDiffDatatype
		case "":
			continue
		default:
			return nil, fmt.Errorf("unknown schema diff entity %q; use doctype, datatype", strings.TrimSpace(part))
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		entities = append(entities, kind)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("--entity must include doctype or datatype")
	}
	return entities, nil
}

func splitSchemaDiffFilters(values []string) []string {
	var out []string
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func normalizeStringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range splitSchemaDiffFilters(values) {
		out[strings.ToLower(value)] = struct{}{}
	}
	return out
}

func sortedSetKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func summarizeSchemaEntity(entity schemaDiffEntity) schemaEntitySummary {
	return schemaEntitySummary{Alias: entity.Alias, Name: entity.Name}
}

func unionSortedEntityAliases(left map[string]schemaDiffEntity, right map[string]schemaDiffEntity) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0, len(left)+len(right))
	for alias := range left {
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
	}
	for alias := range right {
		if _, ok := seen[alias]; ok {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func stringField(raw map[string]any, key string) (string, bool) {
	value, ok := raw[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func schemaDiffStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pathOrRoot(path string) string {
	if path == "" {
		return "$"
	}
	return path
}

func stableJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}
