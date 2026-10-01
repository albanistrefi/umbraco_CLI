package deploy

import (
	"reflect"
	"strings"
)

// udaComparer maps a Udi entity type to its Management API fetch path and a
// field comparer. Comparisons cover the fields the artifact carries;
// environment-only additions (e.g. migration markers in data-type values)
// are not drift.
func udaComparer(kind string) (string, func(artifact map[string]any, remote map[string]any) []string) {
	switch kind {
	case "data-type":
		return "/data-type/%s", compareDataTypeArtifact
	case "document-type":
		return "/document-type/%s", compareContentTypeArtifact
	case "media-type":
		return "/media-type/%s", compareContentTypeArtifact
	case "member-type":
		return "/member-type/%s", compareContentTypeArtifact
	case "template":
		return "/template/%s", compareTemplateArtifact
	case "document-type-container":
		return "/document-type/folder/%s", compareNameOnlyArtifact
	case "data-type-container":
		return "/data-type/folder/%s", compareNameOnlyArtifact
	case "media-type-container":
		return "/media-type/folder/%s", compareNameOnlyArtifact
	case "member-type-container":
		return "/member-type/folder/%s", compareNameOnlyArtifact
	case "member-group":
		return "/member-group/%s", compareNameOnlyArtifact
	case "relation-type":
		return "/relation-type/%s", compareRelationTypeArtifact
	case "language":
		return "/language/%s", compareLanguageArtifact
	}
	return "", nil
}

func compareNameOnlyArtifact(artifact map[string]any, remote map[string]any) []string {
	return diffFields(nil, fieldDiff("name", artifact["Name"], remote["name"]))
}

func compareDataTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("editorAlias", artifact["EditorAlias"], remote["editorAlias"]),
		fieldDiff("editorUiAlias", artifact["EditorUiAlias"], remote["editorUiAlias"]),
	)
	configuration, _ := artifact["Configuration"].(map[string]any)
	remoteValues := map[string]any{}
	if values, ok := remote["values"].([]any); ok {
		for _, item := range values {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := entry["alias"].(string); alias != "" {
				remoteValues[alias] = entry["value"]
			}
		}
	}
	for key, artifactValue := range configuration {
		remoteValue, exists := remoteValues[key]
		if !exists || !jsonValueEqual(artifactValue, remoteValue) {
			diffs = append(diffs, "configuration."+key)
		}
	}
	return diffs
}

func compareTemplateArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
	)
	if artifactContent, ok := artifact["Content"].(string); ok {
		remoteContent, _ := remote["content"].(string)
		if normalizeTemplateContent(artifactContent) != normalizeTemplateContent(remoteContent) {
			diffs = append(diffs, "content")
		}
	}
	return diffs
}

// normalizeTemplateContent normalizes line endings only: leading and
// trailing whitespace in a Razor template can be rendered output, so it is
// significant and compared verbatim.
func normalizeTemplateContent(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func compareContentTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
		fieldDiff("icon", artifact["Icon"], remote["icon"]),
	)
	if permissions, ok := artifact["Permissions"].(map[string]any); ok {
		if isElement, ok := permissions["IsElementType"].(bool); ok {
			remoteElement, _ := remote["isElement"].(bool)
			if isElement != remoteElement {
				diffs = append(diffs, "isElement")
			}
		}
		if allowedAtRoot, ok := permissions["AllowedAtRoot"].(bool); ok {
			remoteRoot, _ := remote["allowedAsRoot"].(bool)
			if allowedAtRoot != remoteRoot {
				diffs = append(diffs, "allowedAsRoot")
			}
		}
		if allowedChildren, ok := permissions["AllowedChildContentTypes"].([]any); ok {
			remoteChildren := referencedGUIDSet(remote["allowedDocumentTypes"], "documentType")
			if remoteChildren == nil {
				remoteChildren = referencedGUIDSet(remote["allowedMediaTypes"], "mediaType")
			}
			if remoteChildren != nil && !udiSetMatches(allowedChildren, remoteChildren) {
				diffs = append(diffs, "allowedChildContentTypes")
			}
		}
	}
	if description, ok := artifact["Description"].(string); ok {
		if normalizeNullableString(description) != normalizeNullableString(udaStringField(remote, "description")) {
			diffs = append(diffs, "description")
		}
	}
	if compositions, ok := artifact["CompositionContentTypes"].([]any); ok {
		remoteCompositions := referencedGUIDSet(remote["compositions"], "documentType")
		if remoteCompositions == nil {
			remoteCompositions = referencedGUIDSet(remote["compositions"], "mediaType")
		}
		if remoteCompositions == nil {
			remoteCompositions = referencedGUIDSet(remote["compositions"], "memberType")
		}
		if remoteCompositions != nil && !udiSetMatches(compositions, remoteCompositions) {
			diffs = append(diffs, "compositions")
		}
	}

	if templates, ok := artifact["AllowedTemplates"].([]any); ok {
		if remoteTemplates := referencedGUIDSetFlat(remote["allowedTemplates"]); remoteTemplates != nil && !udiSetMatches(templates, remoteTemplates) {
			diffs = append(diffs, "allowedTemplates")
		}
	}
	if defaultTemplate, ok := artifact["DefaultTemplate"].(string); ok {
		_, want := parseUdi(defaultTemplate)
		got := ""
		if ref, ok := remote["defaultTemplate"].(map[string]any); ok {
			got, _ = ref["id"].(string)
		}
		if _, hasKey := remote["defaultTemplate"]; hasKey && !guidLikeEqual(want, got) {
			diffs = append(diffs, "defaultTemplate")
		}
	}
	// Deploy writes ListView as a bare GUID, not a Udi; guidLikeEqual
	// accepts either, so the value is compared as written.
	if want, ok := artifact["ListView"].(string); ok {
		got := ""
		if ref, ok := remote["collection"].(map[string]any); ok {
			got, _ = ref["id"].(string)
		}
		if _, hasKey := remote["collection"]; hasKey && !guidLikeEqual(want, got) {
			diffs = append(diffs, "collection")
		}
	}
	if history, ok := artifact["HistoryCleanup"].(map[string]any); ok {
		if cleanup, ok := remote["cleanup"].(map[string]any); ok {
			diffs = diffFields(diffs,
				fieldDiff("cleanup.preventCleanup", udaBool(history, "PreventCleanup"), udaBool(cleanup, "preventCleanup")),
				fieldDiff("cleanup.keepAllVersionsNewerThanDays", history["KeepAllVersionsNewerThanDays"], cleanup["keepAllVersionsNewerThanDays"]),
				fieldDiff("cleanup.keepLatestVersionPerDayForDays", history["KeepLatestVersionPerDayForDays"], cleanup["keepLatestVersionPerDayForDays"]),
			)
		}
	}
	if variations, ok := artifact["Variations"]; ok {
		wantCulture, wantSegment := parseVariations(variations)
		if got, isBool := remote["variesByCulture"].(bool); isBool && got != wantCulture {
			diffs = append(diffs, "variesByCulture")
		}
		if got, isBool := remote["variesBySegment"].(bool); isBool && got != wantSegment {
			diffs = append(diffs, "variesBySegment")
		}
	}
	if groups, ok := artifact["PropertyGroups"].([]any); ok {
		if remoteContainers, ok := remote["containers"].([]any); ok {
			diffs = append(diffs, compareContainers(groups, remoteContainers)...)
		}
	}

	artifactProperties := artifactPropertyIndex(artifact)
	remoteProperties := map[string]map[string]any{}
	if properties, ok := remote["properties"].([]any); ok {
		for _, item := range properties {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := entry["alias"].(string); alias != "" {
				remoteProperties[alias] = entry
			}
		}
	}
	for alias, artifactProperty := range artifactProperties {
		remoteProperty, exists := remoteProperties[alias]
		if !exists {
			diffs = append(diffs, "property "+alias+" (missing remotely)")
			continue
		}
		if name, _ := artifactProperty["Name"].(string); name != udaStringField(remoteProperty, "name") {
			diffs = append(diffs, "property "+alias+".name")
		}
		if _, dataTypeGUID := parseUdi(udaStringField(artifactProperty, "DataType")); dataTypeGUID != "" {
			remoteDataType := ""
			if reference, ok := remoteProperty["dataType"].(map[string]any); ok {
				remoteDataType, _ = reference["id"].(string)
			}
			if !strings.EqualFold(dataTypeGUID, remoteDataType) {
				diffs = append(diffs, "property "+alias+".dataType")
			}
		}
		if sortOrder, ok := artifactProperty["SortOrder"].(float64); ok {
			if remoteSort, ok := remoteProperty["sortOrder"].(float64); ok && sortOrder != remoteSort {
				diffs = append(diffs, "property "+alias+".sortOrder")
			}
		}
		if description, ok := artifactProperty["Description"].(string); ok {
			if normalizeNullableString(description) != normalizeNullableString(udaStringField(remoteProperty, "description")) {
				diffs = append(diffs, "property "+alias+".description")
			}
		}
		if mandatory, ok := artifactProperty["Mandatory"].(bool); ok {
			remoteMandatory := false
			if validation, ok := remoteProperty["validation"].(map[string]any); ok {
				remoteMandatory, _ = validation["mandatory"].(bool)
			}
			if mandatory != remoteMandatory {
				diffs = append(diffs, "property "+alias+".mandatory")
			}
		}
		if varies, ok := artifactProperty["VariesByCulture"].(bool); ok {
			if remoteVaries, isBool := remoteProperty["variesByCulture"].(bool); isBool && varies != remoteVaries {
				diffs = append(diffs, "property "+alias+".variesByCulture")
			}
		}
	}
	for alias := range remoteProperties {
		if _, exists := artifactProperties[alias]; !exists {
			diffs = append(diffs, "property "+alias+" (missing in artifact)")
		}
	}
	return diffs
}

// referencedGUIDSetFlat reads [{id}] reference lists (allowedTemplates).
func referencedGUIDSetFlat(value any) map[string]struct{} {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range items {
		if ref, ok := item.(map[string]any); ok {
			if id, _ := ref["id"].(string); id != "" {
				set[strings.ToLower(id)] = struct{}{}
			}
		}
	}
	return set
}

// compareContainers matches artifact PropertyGroups to remote containers by
// key and compares name, type, and sort order; unmatched entries on either
// side are diffs.
func compareContainers(groups []any, remoteContainers []any) []string {
	diffs := []string{}
	remoteByID := map[string]map[string]any{}
	for _, item := range remoteContainers {
		if container, ok := item.(map[string]any); ok {
			if id, _ := container["id"].(string); id != "" {
				remoteByID[strings.ToLower(id)] = container
			}
		}
	}
	seen := map[string]bool{}
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := normalizeGUID(udaStringField(group, "Key"))
		name := udaStringField(group, "Name")
		remote, exists := remoteByID[key]
		if !exists {
			diffs = append(diffs, "container "+name+" (missing remotely)")
			continue
		}
		seen[key] = true
		if name != udaStringField(remote, "name") {
			diffs = append(diffs, "container "+name+".name")
		}
		if containerTypeName(group["Type"]) != udaStringField(remote, "type") {
			diffs = append(diffs, "container "+name+".type")
		}
		if sortOrder, ok := group["SortOrder"].(float64); ok {
			if remoteSort, ok := remote["sortOrder"].(float64); ok && sortOrder != remoteSort {
				diffs = append(diffs, "container "+name+".sortOrder")
			}
		}
	}
	for id, remote := range remoteByID {
		if !seen[id] {
			diffs = append(diffs, "container "+udaStringField(remote, "name")+" (missing in artifact)")
		}
	}
	return diffs
}

// artifactPropertyIndex reads BOTH property collections a content-type
// artifact carries: PropertyGroups[].PropertyTypes (grouped, the normal
// case) and the top-level PropertyTypes[] (ungrouped). Reading only the
// grouped collection silently ignores ungrouped properties.
func artifactPropertyIndex(artifact map[string]any) map[string]map[string]any {
	index := map[string]map[string]any{}
	collect := func(items []any) {
		for _, item := range items {
			property, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := property["Alias"].(string); alias != "" {
				index[alias] = property
			}
		}
	}
	if groups, ok := artifact["PropertyGroups"].([]any); ok {
		for _, item := range groups {
			group, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if properties, ok := group["PropertyTypes"].([]any); ok {
				collect(properties)
			}
		}
	}
	if properties, ok := artifact["PropertyTypes"].([]any); ok {
		collect(properties)
	}
	return index
}

func udaStringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func fieldDiff(field string, artifactValue any, remoteValue any) string {
	if artifactValue == nil {
		return ""
	}
	if jsonValueEqual(artifactValue, remoteValue) {
		return ""
	}
	return field
}

func diffFields(diffs []string, candidates ...string) []string {
	for _, candidate := range candidates {
		if candidate != "" {
			diffs = append(diffs, candidate)
		}
	}
	return diffs
}

func jsonValueEqual(a any, b any) bool {
	return reflect.DeepEqual(a, b)
}

// compareRelationTypeArtifact compares the behavioral relation-type fields
// the artifact carries: directionality, dependency behavior, and the
// parent/child object types — a shared name alone says nothing.
func compareRelationTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
	)
	if bidirectional, ok := artifact["IsBidirectional"].(bool); ok {
		if remoteValue, isBool := remote["isBidirectional"].(bool); isBool && bidirectional != remoteValue {
			diffs = append(diffs, "isBidirectional")
		}
	}
	if dependency, ok := artifact["IsDependency"].(bool); ok {
		if remoteValue, isBool := remote["isDependency"].(bool); isBool && dependency != remoteValue {
			diffs = append(diffs, "isDependency")
		}
	}
	// The Management API returns the object types as parentObject/childObject
	// {id, name}; the flat parentObjectType/childObjectType are read as a
	// fallback. The diff names are kept as they were reported before.
	for artifactKey, remoteKey := range map[string]string{"ParentObjectType": "parentObject", "ChildObjectType": "childObject"} {
		if value, ok := artifact[artifactKey].(string); ok && value != "" {
			remoteValue := udaStringField(remote, remoteKey+"Type")
			if object, ok := remote[remoteKey].(map[string]any); ok {
				remoteValue = udaStringField(object, "id")
			}
			if !guidLikeEqual(value, remoteValue) {
				diffs = append(diffs, remoteKey+"Type")
			}
		}
	}
	return diffs
}

// referencedGUIDSet extracts the lowercase GUID set from response arrays
// shaped [{"<refKey>": {"id": ...}, ...}]; nil when the field is absent or
// not that shape, so callers skip rather than false-drift.
func referencedGUIDSet(value any, refKey string) map[string]struct{} {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		reference, ok := entry[refKey].(map[string]any)
		if !ok {
			return nil
		}
		if id, _ := reference["id"].(string); id != "" {
			set[strings.ToLower(id)] = struct{}{}
		}
	}
	return set
}

// udiSetMatches compares an artifact's udi list against a remote GUID set.
func udiSetMatches(udis []any, remote map[string]struct{}) bool {
	local := map[string]struct{}{}
	for _, item := range udis {
		udi, ok := item.(string)
		if !ok {
			return false
		}
		if _, guid := parseUdi(udi); guid != "" {
			local[strings.ToLower(guid)] = struct{}{}
		}
	}
	if len(local) != len(remote) {
		return false
	}
	for guid := range local {
		if _, ok := remote[guid]; !ok {
			return false
		}
	}
	return true
}

// guidLikeEqual compares two identifiers that may each be a bare GUID or a
// udi, case-insensitively.
func guidLikeEqual(a string, b string) bool {
	normalize := func(value string) string {
		if _, guid := parseUdi(value); guid != "" {
			return strings.ToLower(guid)
		}
		return strings.ToLower(strings.TrimSpace(value))
	}
	return normalize(a) == normalize(b)
}

func normalizeNullableString(value string) string {
	return strings.TrimSpace(value)
}

// compareLanguageArtifact compares the language fields the artifact
// carries. Languages are keyed by ISO code, not GUID.
func compareLanguageArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("isoCode", artifact["IsoCode"], remote["isoCode"]),
	)
	for artifactKey, remoteKey := range map[string]string{"IsDefault": "isDefault", "IsMandatory": "isMandatory"} {
		if value, ok := artifact[artifactKey].(bool); ok {
			if remoteValue, isBool := remote[remoteKey].(bool); isBool && value != remoteValue {
				diffs = append(diffs, remoteKey)
			}
		}
	}
	if fallback, ok := artifact["FallbackIsoCode"].(string); ok && fallback != "" {
		if fallback != udaStringField(remote, "fallbackIsoCode") {
			diffs = append(diffs, "fallbackIsoCode")
		}
	}
	return diffs
}
