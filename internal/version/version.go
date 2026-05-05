// Package version exposes the published umbraco-cli release identifier.
//
// VERSION is the canonical source of truth for the release identifier across the project:
//   - the Go code embeds it via go:embed and surfaces it through Current()
//   - scripts/sync-version.mjs propagates the same value into package.json and package-lock.json
//   - scripts/verify-skills.mjs fails CI if any of those copies drift from VERSION
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Current returns the published umbraco-cli release identifier (trimmed of whitespace).
func Current() string {
	return strings.TrimSpace(raw)
}
