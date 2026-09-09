// Package version exposes the published umbraco-cli release identifier.
//
// VERSION is the canonical source of truth for the release identifier:
// the Go code embeds it via go:embed and surfaces it through Current().
package version

import (
	_ "embed"
	"runtime"
	"strings"
)

//go:embed VERSION
var raw string

// Current returns the published umbraco-cli release identifier (trimmed of whitespace).
func Current() string {
	return strings.TrimSpace(raw)
}

// UserAgent returns the User-Agent header value sent on every outbound
// request (token, Management API, multipart, raw). Go's default
// "Go-http-client/1.1" is flagged by common bot rules (Cloudflare among
// them), which surfaces as 403s on fronted hosts even with a valid token.
func UserAgent() string {
	return "umbraco-cli/" + Current() + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}
