// Package validate guards values that flow into identifier positions:
// path segments, aliases, keys, ISO codes, and similar. It deliberately
// does NOT validate content values (property values, translations, Razor
// markup) — the Management API is the authority on those, and client-side
// heuristics have a history of rejecting legitimate CMS content.
package validate

import (
	"fmt"
	"regexp"
	"strings"
)

var controlChars = regexp.MustCompile(`[\x00-\x1F\x7F]`)

// String rejects control characters in identifier-like inputs (aliases,
// keys, ISO codes, file paths) where they always indicate breakage.
func String(value string) error {
	if controlChars.MatchString(value) {
		return fmt.Errorf("input contains control characters")
	}
	return nil
}

// ResourceID rejects characters that can never appear in an Umbraco
// entity ID and would change how a URL is interpreted.
func ResourceID(value string) error {
	if strings.ContainsAny(value, "?#%") {
		return fmt.Errorf("invalid resource ID: %s", value)
	}
	return nil
}
