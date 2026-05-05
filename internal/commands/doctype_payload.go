package commands

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"umbraco-cli/internal/api"
)

func fetchDoctypeObject(ctx context.Context, client *api.Client, id string) (map[string]any, error) {
	result, err := client.Get(ctx, fmt.Sprintf("/document-type/%s", id), api.RequestOptions{})
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

// newUUIDv4 returns a freshly generated random UUID (RFC 4122 v4).
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
