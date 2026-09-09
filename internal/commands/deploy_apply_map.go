package commands

import (
	"fmt"
	"sort"
	"strings"

	"umbraco-cli/internal/api"
)

// udaWriteSpec is how one artifact kind is written through the Management
// API: the collection endpoint for creates, the item endpoint for updates,
// and a mapper turning the Deploy artifact into request bodies. Kinds
// without a spec (Automate, relation types — read-only in the Management
// API) are reported as unsupported, never guessed.
type udaWriteSpec struct {
	CreatePath string
	UpdatePath string // JoinPath format with one %s (GUID or ISO code)
	// Map returns the create body and the update body. Both are full
	// replacements: Deploy's schema pass has replace semantics, so apply
	// sends everything the artifact carries.
	Map func(artifact map[string]any) (create map[string]any, update map[string]any, err error)
}

func udaWriter(kind string) (udaWriteSpec, bool) {
	switch kind {
	case "language":
		return udaWriteSpec{CreatePath: "/language", UpdatePath: "/language/%s", Map: mapLanguageArtifact}, true
	case "data-type":
		return udaWriteSpec{CreatePath: "/data-type", UpdatePath: "/data-type/%s", Map: mapDataTypeArtifact}, true
	case "data-type-container":
		return udaWriteSpec{CreatePath: "/data-type/folder", UpdatePath: "/data-type/folder/%s", Map: mapContainerArtifact}, true
	case "document-type-container":
		return udaWriteSpec{CreatePath: "/document-type/folder", UpdatePath: "/document-type/folder/%s", Map: mapContainerArtifact}, true
	case "media-type-container":
		return udaWriteSpec{CreatePath: "/media-type/folder", UpdatePath: "/media-type/folder/%s", Map: mapContainerArtifact}, true
	case "member-type-container":
		return udaWriteSpec{CreatePath: "/member-type/folder", UpdatePath: "/member-type/folder/%s", Map: mapContainerArtifact}, true
	case "template":
		return udaWriteSpec{CreatePath: "/template", UpdatePath: "/template/%s", Map: mapTemplateArtifact}, true
	case "member-group":
		return udaWriteSpec{CreatePath: "/member-group", UpdatePath: "/member-group/%s", Map: mapMemberGroupArtifact}, true
	case "document-type":
		return udaWriteSpec{CreatePath: "/document-type", UpdatePath: "/document-type/%s", Map: contentTypeMapper("document")}, true
	case "media-type":
		return udaWriteSpec{CreatePath: "/media-type", UpdatePath: "/media-type/%s", Map: contentTypeMapper("media")}, true
	case "member-type":
		return udaWriteSpec{CreatePath: "/member-type", UpdatePath: "/member-type/%s", Map: contentTypeMapper("member")}, true
	}
	return udaWriteSpec{}, false
}

// --- shared helpers -------------------------------------------------------

func udaGUIDRef(udi any) map[string]any {
	value, _ := udi.(string)
	if _, guid := parseUdi(value); guid != "" {
		return map[string]any{"id": guid}
	}
	return nil
}

func udaBool(object map[string]any, key string) bool {
	value, _ := object[key].(bool)
	return value
}

func udaNullableString(object map[string]any, key string) any {
	value, ok := object[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func udaNumber(object map[string]any, key string) float64 {
	switch value := object[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	}
	return 0
}

func withoutKey(body map[string]any, keys ...string) map[string]any {
	copied := make(map[string]any, len(body))
	for key, value := range body {
		copied[key] = value
	}
	for _, key := range keys {
		delete(copied, key)
	}
	return copied
}

// --- mappers --------------------------------------------------------------

func mapLanguageArtifact(artifact map[string]any) (map[string]any, map[string]any, error) {
	iso, _ := artifact["IsoCode"].(string)
	if iso == "" {
		return nil, nil, fmt.Errorf("language artifact has no IsoCode")
	}
	name, _ := artifact["CultureName"].(string)
	if name == "" {
		name, _ = artifact["Name"].(string)
	}
	body := map[string]any{
		"isoCode":     iso,
		"name":        name,
		"isDefault":   udaBool(artifact, "IsDefault"),
		"isMandatory": udaBool(artifact, "IsMandatory"),
	}
	if fallback, _ := artifact["FallbackLanguage"].(string); fallback != "" {
		if _, code := parseUdi(fallback); code != "" {
			body["fallbackIsoCode"] = code
		}
	} else if fallback, _ := artifact["FallbackIsoCode"].(string); fallback != "" {
		body["fallbackIsoCode"] = fallback
	}
	return body, withoutKey(body, "isoCode"), nil
}

func mapDataTypeArtifact(artifact map[string]any) (map[string]any, map[string]any, error) {
	_, guid := parseUdi(udaStringField(artifact, "Udi"))
	name, _ := artifact["Name"].(string)
	editorAlias, _ := artifact["EditorAlias"].(string)
	if name == "" || editorAlias == "" {
		return nil, nil, fmt.Errorf("data-type artifact needs Name and EditorAlias")
	}
	values := []any{}
	configuration, _ := artifact["Configuration"].(map[string]any)
	keys := make([]string, 0, len(configuration))
	for key := range configuration {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values = append(values, map[string]any{"alias": key, "value": configuration[key]})
	}
	body := map[string]any{
		"id":            guid,
		"name":          name,
		"editorAlias":   editorAlias,
		"editorUiAlias": artifact["EditorUiAlias"],
		"values":        values,
	}
	if parent := udaGUIDRef(artifact["Parent"]); parent != nil {
		body["parent"] = parent
	}
	return body, withoutKey(body, "id", "parent"), nil
}

func mapContainerArtifact(artifact map[string]any) (map[string]any, map[string]any, error) {
	_, guid := parseUdi(udaStringField(artifact, "Udi"))
	name, _ := artifact["Name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("container artifact has no Name")
	}
	body := map[string]any{"id": guid, "name": name}
	if parent := udaGUIDRef(artifact["Parent"]); parent != nil {
		body["parent"] = parent
	}
	return body, map[string]any{"name": name}, nil
}

func mapTemplateArtifact(artifact map[string]any) (map[string]any, map[string]any, error) {
	_, guid := parseUdi(udaStringField(artifact, "Udi"))
	name, _ := artifact["Name"].(string)
	alias, _ := artifact["Alias"].(string)
	if name == "" || alias == "" {
		return nil, nil, fmt.Errorf("template artifact needs Name and Alias")
	}
	content, _ := artifact["Content"].(string)
	body := map[string]any{"id": guid, "name": name, "alias": alias, "content": content}
	return body, withoutKey(body, "id"), nil
}

func mapMemberGroupArtifact(artifact map[string]any) (map[string]any, map[string]any, error) {
	_, guid := parseUdi(udaStringField(artifact, "Udi"))
	name, _ := artifact["Name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("member-group artifact has no Name")
	}
	return map[string]any{"id": guid, "name": name}, map[string]any{"name": name}, nil
}

// contentTypeMapper builds the document/media/member-type request models.
// family selects the reference key names the Management API uses
// (allowedDocumentTypes vs allowedMediaTypes, compositions[].documentType…).
func contentTypeMapper(family string) func(map[string]any) (map[string]any, map[string]any, error) {
	return func(artifact map[string]any) (map[string]any, map[string]any, error) {
		_, guid := parseUdi(udaStringField(artifact, "Udi"))
		name, _ := artifact["Name"].(string)
		alias, _ := artifact["Alias"].(string)
		if name == "" || alias == "" {
			return nil, nil, fmt.Errorf("%s-type artifact needs Name and Alias", family)
		}
		permissions, _ := artifact["Permissions"].(map[string]any)
		variesByCulture, variesBySegment := parseVariations(artifact["Variations"])
		icon, _ := artifact["Icon"].(string)
		if icon == "" {
			icon = "icon-document"
		}

		containers, containerIDByAlias := mapPropertyGroups(artifact)
		properties, err := mapPropertyTypes(artifact, family, containerIDByAlias)
		if err != nil {
			return nil, nil, err
		}

		compositions := []any{}
		refKey := family + "Type"
		for _, item := range asAnySlice(artifact["CompositionContentTypes"]) {
			if ref := udaGUIDRef(item); ref != nil {
				compositions = append(compositions, map[string]any{refKey: ref, "compositionType": "Composition"})
			}
		}
		// Parent is either a folder (…-container) or an inheritance parent
		// (same kind); the Udi kind tells them apart.
		var parentFolder map[string]any
		if parentUdi, _ := artifact["Parent"].(string); parentUdi != "" {
			parentKind, _ := parseUdi(parentUdi)
			if strings.HasSuffix(parentKind, "-container") {
				parentFolder = udaGUIDRef(parentUdi)
			} else if ref := udaGUIDRef(parentUdi); ref != nil {
				compositions = append(compositions, map[string]any{refKey: ref, "compositionType": "Inheritance"})
			}
		}

		body := map[string]any{
			"id":               guid,
			"alias":            alias,
			"name":             name,
			"description":      udaNullableString(artifact, "Description"),
			"icon":             icon,
			"allowedAsRoot":    udaBool(permissions, "AllowedAtRoot"),
			"variesByCulture":  variesByCulture,
			"variesBySegment":  variesBySegment,
			"isElement":        udaBool(permissions, "IsElementType"),
			"allowedInLibrary": udaBool(permissions, "AllowedInLibrary"),
			"properties":       properties,
			"containers":       containers,
			"compositions":     compositions,
		}
		if listView := udaGUIDRef(artifact["ListView"]); listView != nil {
			body["collection"] = listView
		}
		if parentFolder != nil {
			body["parent"] = parentFolder
		}

		switch family {
		case "document", "media":
			allowed := []any{}
			for index, item := range asAnySlice(permissions["AllowedChildContentTypes"]) {
				if ref := udaGUIDRef(item); ref != nil {
					allowed = append(allowed, map[string]any{refKey: ref, "sortOrder": index})
				}
			}
			body["allowed"+strings.ToUpper(family[:1])+family[1:]+"Types"] = allowed
		}
		if family == "document" {
			templates := []any{}
			for _, item := range asAnySlice(artifact["AllowedTemplates"]) {
				if ref := udaGUIDRef(item); ref != nil {
					templates = append(templates, ref)
				}
			}
			body["allowedTemplates"] = templates
			if ref := udaGUIDRef(artifact["DefaultTemplate"]); ref != nil {
				body["defaultTemplate"] = ref
			}
			cleanup := map[string]any{"preventCleanup": false, "keepAllVersionsNewerThanDays": nil, "keepLatestVersionPerDayForDays": nil}
			if history, ok := artifact["HistoryCleanup"].(map[string]any); ok {
				cleanup["preventCleanup"] = udaBool(history, "PreventCleanup")
				cleanup["keepAllVersionsNewerThanDays"] = history["KeepAllVersionsNewerThanDays"]
				cleanup["keepLatestVersionPerDayForDays"] = history["KeepLatestVersionPerDayForDays"]
			}
			body["cleanup"] = cleanup
		}
		return body, withoutKey(body, "id", "parent"), nil
	}
}

// parseVariations reads Deploy's ContentVariation flags ("Nothing",
// "Culture", "Segment", "CultureAndSegment", or comma-separated).
func parseVariations(value any) (bool, bool) {
	text := strings.ToLower(fmt.Sprint(value))
	return strings.Contains(text, "culture"), strings.Contains(text, "segment")
}

// mapPropertyGroups turns PropertyGroups into Management API containers.
// Deploy flattens nested groups with a slash alias ("content/settings"), so
// the parent is the group whose alias is the prefix; Type may be the enum
// name or its number (0 = Group, 1 = Tab).
func mapPropertyGroups(artifact map[string]any) ([]any, map[string]string) {
	groups := asAnySlice(artifact["PropertyGroups"])
	idByAlias := map[string]string{}
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if key, _ := group["Key"].(string); key != "" {
			idByAlias[strings.ToLower(udaStringField(group, "Alias"))] = normalizeGUID(key)
		}
	}
	containers := make([]any, 0, len(groups))
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := group["Key"].(string)
		if key == "" {
			continue
		}
		alias := udaStringField(group, "Alias")
		container := map[string]any{
			"id":        normalizeGUID(key),
			"name":      group["Name"],
			"type":      containerTypeName(group["Type"]),
			"sortOrder": udaNumber(group, "SortOrder"),
		}
		if slash := strings.LastIndex(alias, "/"); slash > 0 {
			if parentID, ok := idByAlias[strings.ToLower(alias[:slash])]; ok {
				container["parent"] = map[string]any{"id": parentID}
			}
		}
		containers = append(containers, container)
	}
	return containers, idByAlias
}

func containerTypeName(value any) string {
	switch typed := value.(type) {
	case string:
		if strings.EqualFold(typed, "Tab") {
			return "Tab"
		}
		return "Group"
	case float64:
		if typed == 1 {
			return "Tab"
		}
	}
	return "Group"
}

// mapPropertyTypes reads both property collections (grouped and ungrouped)
// into Management API property models, binding grouped ones to their
// container by the group's Key.
func mapPropertyTypes(artifact map[string]any, family string, _ map[string]string) ([]any, error) {
	properties := []any{}
	convert := func(property map[string]any, containerKey string) error {
		key, _ := property["Key"].(string)
		alias, _ := property["Alias"].(string)
		name, _ := property["Name"].(string)
		if key == "" || alias == "" {
			return fmt.Errorf("property %q is missing Key or Alias", alias)
		}
		dataType := udaGUIDRef(property["DataType"])
		if dataType == nil {
			return fmt.Errorf("property %q has no DataType udi", alias)
		}
		variesByCulture, variesBySegment := parseVariations(property["Variations"])
		if v, ok := property["VariesByCulture"].(bool); ok {
			variesByCulture = v
		}
		model := map[string]any{
			"id":              normalizeGUID(key),
			"alias":           alias,
			"name":            name,
			"description":     udaNullableString(property, "Description"),
			"dataType":        dataType,
			"sortOrder":       udaNumber(property, "SortOrder"),
			"variesByCulture": variesByCulture,
			"variesBySegment": variesBySegment,
			"validation": map[string]any{
				"mandatory":        udaBool(property, "Mandatory"),
				"mandatoryMessage": udaNullableString(property, "MandatoryMessage"),
				"regEx":            udaNullableString(property, "ValidationRegExp"),
				"regExMessage":     udaNullableString(property, "ValidationRegExpMessage"),
			},
			"appearance": map[string]any{"labelOnTop": udaBool(property, "LabelOnTop")},
		}
		if containerKey != "" {
			model["container"] = map[string]any{"id": normalizeGUID(containerKey)}
		}
		if family == "member" {
			model["isSensitive"] = udaBool(property, "IsSensitive")
			model["visibility"] = map[string]any{
				"memberCanView": udaBool(property, "MemberCanView"),
				"memberCanEdit": udaBool(property, "MemberCanEdit"),
			}
		}
		properties = append(properties, model)
		return nil
	}
	for _, item := range asAnySlice(artifact["PropertyGroups"]) {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		groupKey, _ := group["Key"].(string)
		for _, raw := range asAnySlice(group["PropertyTypes"]) {
			if property, ok := raw.(map[string]any); ok {
				if err := convert(property, groupKey); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, raw := range asAnySlice(artifact["PropertyTypes"]) {
		if property, ok := raw.(map[string]any); ok {
			if err := convert(property, ""); err != nil {
				return nil, err
			}
		}
	}
	return properties, nil
}

// normalizeGUID accepts dashed or 32-hex GUIDs and returns the dashed,
// lower-case form the Management API expects.
func normalizeGUID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if udiHexPattern.MatchString(value) {
		return strings.Join([]string{value[0:8], value[8:12], value[12:16], value[16:20], value[20:32]}, "-")
	}
	return value
}

func asAnySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

// artifactDependencyKeys lists the kind/GUID keys of the artifact's ordering
// dependencies (Deploy marks the ones that must exist first with
// Ordering: true; unmarked ones are soft references).
func artifactDependencyKeys(artifact map[string]any, orderingOnly bool) []string {
	keys := []string{}
	for _, item := range asAnySlice(artifact["Dependencies"]) {
		dep, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if orderingOnly && !udaBool(dep, "Ordering") {
			continue
		}
		if kind, id := parseUdi(udaStringField(dep, "Udi")); id != "" {
			keys = append(keys, udaKey(kind, id))
		}
	}
	return keys
}

// udaUpdatePath resolves the item endpoint for an artifact.
func udaUpdatePath(spec udaWriteSpec, artifact udaArtifact) string {
	return api.JoinPath(spec.UpdatePath, artifact.GUID)
}
