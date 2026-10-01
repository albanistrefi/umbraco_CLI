package safefile

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFileNameRejectsWindowsDeviceNames(t *testing.T) {
	for _, name := range []string{"CON", "nul", "COM1", "LPT1.txt", "aux.svg"} {
		if got := Name(name, "file"); !strings.HasPrefix(got, "_") {
			t.Fatalf("expected %q to be rewritten, got %q", name, got)
		}
	}
	if got := Name("console.svg", "file"); got != "console.svg" {
		t.Fatalf("expected ordinary names untouched, got %q", got)
	}
}

func TestNameReplacesSeparatorsAndFallsBack(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":     "_.._etc_passwd",
		"a\\b:c":               "a_b_c",
		"...":                  "fallback",
		"":                     "fallback",
		" .hidden. ":           "hidden",
		"r\u00e9sum\u00e9.pdf": "r_sum_.pdf",
	}
	for name, want := range cases {
		if got := Name(name, "fallback"); got != want {
			t.Fatalf("Name(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestChildPathRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	got, err := ChildPath(dir, "file.txt")
	if err != nil || got != filepath.Join(dir, "file.txt") {
		t.Fatalf("expected a direct child, got %q %v", got, err)
	}
	for _, name := range []string{"../x", "sub/file", ".."} {
		if _, err := ChildPath(dir, name); err == nil || !strings.Contains(err.Error(), "would escape") {
			t.Fatalf("expected %q to be refused, got %v", name, err)
		}
	}
}
