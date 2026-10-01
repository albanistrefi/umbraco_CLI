// Package safefile turns server-provided names into local file names and
// paths that cannot escape the directory they are written to.
package safefile

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Name reduces a server-provided file name to a single safe path
// component for any host OS: no separators of either flavour, no traversal,
// no control characters.
func Name(name string, fallback string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ' ':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	cleaned := strings.Trim(b.String(), ". ")
	if cleaned == "" || strings.Trim(cleaned, ".") == "" {
		return fallback
	}
	// Windows reserves device names regardless of extension (CON, NUL,
	// COM1, LPT1.txt ...); opening one addresses the device, not a file.
	stem := strings.ToUpper(cleaned)
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	if windowsReservedNames[stem] {
		return "_" + cleaned
	}
	return cleaned
}

var windowsReservedNames = func() map[string]bool {
	names := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for i := 1; i <= 9; i++ {
		names[fmt.Sprintf("COM%d", i)] = true
		names[fmt.Sprintf("LPT%d", i)] = true
	}
	return names
}()

// ChildPath joins name under dir and verifies the result is a direct
// child of dir under the host OS's path semantics.
func ChildPath(dir string, name string) (string, error) {
	joined := filepath.Join(dir, name)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if filepath.Dir(absJoined) != absDir {
		return "", fmt.Errorf("file name %q would escape %s", name, dir)
	}
	return joined, nil
}
