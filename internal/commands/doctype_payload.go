package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"umbraco-cli/internal/api"
)

func fetchDoctypeObject(ctx context.Context, client *api.Client, id string) (map[string]any, error) {
	result, err := client.Get(ctx, api.JoinPath("/document-type/%s", id), api.RequestOptions{})
	if err != nil {
		return nil, err
	}

	return decodeResult[map[string]any](result)
}

// findDoctypeContainerID returns the id of the container with the given name on the supplied
// doctype payload (case-insensitive). If multiple containers share that name it returns the
// matching IDs so the caller can disambiguate. Containers in the Umbraco Management API are
// keyed by name, not alias.
func findDoctypeContainerID(doctype map[string]any, name string) (id string, ambiguous bool) {
	containers, ok := doctype["containers"].([]any)
	if !ok {
		return "", false
	}
	target := strings.ToLower(strings.TrimSpace(name))
	matches := 0
	for _, item := range containers {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryName, _ := entry["name"].(string)
		if strings.ToLower(strings.TrimSpace(entryName)) != target {
			continue
		}
		if matchID, _ := entry["id"].(string); matchID != "" {
			id = matchID
			matches++
		}
	}
	return id, matches > 1
}

// hasDoctypeProperty reports whether the doctype already exposes a property with the given alias.
func hasDoctypeProperty(doctype map[string]any, alias string) bool {
	properties, ok := doctype["properties"].([]any)
	if !ok {
		return false
	}
	for _, item := range properties {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if entryAlias, _ := entry["alias"].(string); entryAlias == alias {
			return true
		}
	}
	return false
}

// buildDoctypeProperty assembles a property entry with the defaults the Management API expects.
func buildDoctypeProperty(id, containerID, alias, name, dataTypeID, description string, mandatory bool, sortOrder int) map[string]any {
	return map[string]any{
		"id":              id,
		"container":       map[string]any{"id": containerID},
		"alias":           alias,
		"name":            name,
		"description":     description,
		"dataType":        map[string]any{"id": dataTypeID},
		"variesByCulture": false,
		"variesBySegment": false,
		"sortOrder":       sortOrder,
		"appearance":      map[string]any{"labelOnTop": false},
		"validation": map[string]any{
			"mandatory":        mandatory,
			"mandatoryMessage": nil,
			"regEx":            nil,
			"regExMessage":     nil,
		},
	}
}

// nextDoctypePropertySortOrder returns the next sort order to use for a new property in the
// given container, based on the highest sortOrder already present.
func nextDoctypePropertySortOrder(doctype map[string]any, containerID string) int {
	properties, ok := doctype["properties"].([]any)
	if !ok {
		return 0
	}
	highest := -1
	for _, item := range properties {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		container, ok := entry["container"].(map[string]any)
		if !ok {
			continue
		}
		if id, _ := container["id"].(string); id != containerID {
			continue
		}
		if value, ok := entry["sortOrder"].(float64); ok {
			if int(value) > highest {
				highest = int(value)
			}
		}
	}
	return highest + 1
}

// hasDoctypeContainer reports whether the doctype already exposes a container with the given
// name (case-insensitive). Used to short-circuit add-container before generating an ID.
func hasDoctypeContainer(doctype map[string]any, name string) bool {
	containers, ok := doctype["containers"].([]any)
	if !ok {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(name))
	for _, item := range containers {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryName, _ := entry["name"].(string)
		if strings.ToLower(strings.TrimSpace(entryName)) == target {
			return true
		}
	}
	return false
}

// nextDoctypeContainerSortOrder returns the next sort order to use for a new container at the
// supplied parent scope (parentID is "" for root-level Tabs).
func nextDoctypeContainerSortOrder(doctype map[string]any, parentID string) int {
	containers, ok := doctype["containers"].([]any)
	if !ok {
		return 0
	}
	highest := -1
	for _, item := range containers {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryParentID := ""
		if parent, ok := entry["parent"].(map[string]any); ok {
			entryParentID, _ = parent["id"].(string)
		}
		if entryParentID != parentID {
			continue
		}
		if value, ok := entry["sortOrder"].(float64); ok {
			if int(value) > highest {
				highest = int(value)
			}
		}
	}
	return highest + 1
}

// normalizeDoctypeContainerType maps user-provided type input to the canonical "Tab" or
// "Group" expected by Umbraco. Returns the empty string when the input is not recognized.
func normalizeDoctypeContainerType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tab":
		return "Tab"
	case "group":
		return "Group"
	default:
		return ""
	}
}

// buildDoctypeContainer assembles a container entry that mirrors the Umbraco Management API
// PropertyTypeContainerModelBase shape (id, parent?, name, type, sortOrder).
func buildDoctypeContainer(id, parentID, name, containerType string, sortOrder int) map[string]any {
	container := map[string]any{
		"id":        id,
		"name":      name,
		"type":      containerType,
		"sortOrder": sortOrder,
	}
	if parentID != "" {
		container["parent"] = map[string]any{"id": parentID}
	} else {
		container["parent"] = nil
	}
	return container
}

// doctypeReorderPatch builds the properties sortOrder patch for a full
// reorder: the listed aliases get sortOrder from their position, and the
// remaining properties in the same container follow after them in their
// current relative order, so the resulting order is fully deterministic.
// sortOrder is scoped per container, so every listed alias must live in the
// same container.
func doctypeReorderPatch(doctype map[string]any, ordered []string) ([]any, error) {
	properties, _ := doctype["properties"].([]any)
	byAlias := make(map[string]map[string]any, len(properties))
	for _, item := range properties {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if alias, _ := entry["alias"].(string); alias != "" {
			byAlias[alias] = entry
		}
	}

	containerOf := func(entry map[string]any) string {
		if container, ok := entry["container"].(map[string]any); ok {
			id, _ := container["id"].(string)
			return id
		}
		return ""
	}

	var scope string
	listed := make(map[string]struct{}, len(ordered))
	for i, alias := range ordered {
		entry, ok := byAlias[alias]
		if !ok {
			return nil, fmt.Errorf("doctype has no property with alias %q", alias)
		}
		if i == 0 {
			scope = containerOf(entry)
		} else if containerOf(entry) != scope {
			return nil, fmt.Errorf("properties %q and %q live in different containers; sortOrder is scoped per container, so reorder one container at a time", ordered[0], alias)
		}
		listed[alias] = struct{}{}
	}

	patch := make([]any, 0, len(properties))
	for i, alias := range ordered {
		patch = append(patch, map[string]any{"alias": alias, "sortOrder": i})
	}

	rest := make([]map[string]any, 0, len(properties))
	for _, item := range properties {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		alias, _ := entry["alias"].(string)
		if _, isListed := listed[alias]; isListed || alias == "" || containerOf(entry) != scope {
			continue
		}
		rest = append(rest, entry)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		left, _ := rest[i]["sortOrder"].(float64)
		right, _ := rest[j]["sortOrder"].(float64)
		return left < right
	})
	for i, entry := range rest {
		patch = append(patch, map[string]any{"alias": entry["alias"], "sortOrder": len(ordered) + i})
	}

	return patch, nil
}

func normalizeDoctypePayload(body map[string]any) {
	normalizeDoctypeProperties(body["properties"])
	// Earlier --print-template skeletons called the version-cleanup block
	// historyCleanup (Deploy's name); the Management API field is cleanup.
	if legacy, ok := body["historyCleanup"]; ok {
		if _, exists := body["cleanup"]; !exists {
			body["cleanup"] = legacy
		}
		delete(body, "historyCleanup")
	}
}

// normalizeDoctypePayloadHook adapts normalizeDoctypePayload to the
// error-returning Normalize contract used by update specs.
func normalizeDoctypePayloadHook(body map[string]any) error {
	normalizeDoctypePayload(body)
	return nil
}

func normalizeDoctypeProperties(raw any) {
	properties, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range properties {
		property, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := property["dataType"]; !exists {
			if dataTypeID, ok := property["dataTypeId"].(string); ok && strings.TrimSpace(dataTypeID) != "" {
				property["dataType"] = map[string]any{"id": dataTypeID}
				delete(property, "dataTypeId")
			}
		}
	}
}
