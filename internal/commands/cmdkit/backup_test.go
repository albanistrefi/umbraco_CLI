package cmdkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupAutoNamesAreUniqueAndNeverOverwrite(t *testing.T) {
	a := ResolveBackupPath("", "media", "m-1")
	b := ResolveBackupPath("", "media", "m-1")
	if a == b {
		t.Fatalf("expected distinct auto names, got %s twice", a)
	}
	target := filepath.Join(t.TempDir(), "x.json")
	if _, err := WriteBackup(target, "media", "m-1", "/media/m-1", map[string]any{"values": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteBackup(target, "media", "m-1", "/media/m-1", map[string]any{"values": []any{}}, nil); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected exclusive create to refuse, got %v", err)
	}
}

func TestBackupBinaryNameIsSanitizedAndEnvelopeReservedFirst(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.json")
	bin := &BackupBinary{Property: "umbracoFile", Src: `/media/abc/..\evil.txt`, Content: []byte("a")}
	if _, err := WriteBackup(target, "media", "m-1", "/media/m-1", map[string]any{}, bin); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "x.files"))
	if len(entries) != 1 || strings.ContainsAny(entries[0].Name(), `\/`) || strings.Contains(entries[0].Name(), "..") {
		t.Fatalf("expected a sanitized single file, got %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatalf("binary escaped the .files directory")
	}
	// Re-using the envelope path must fail before the old binary is touched.
	before, _ := os.ReadFile(filepath.Join(dir, "x.files", entries[0].Name()))
	if _, err := WriteBackup(target, "media", "m-1", "/media/m-1", map[string]any{}, &BackupBinary{Property: "umbracoFile", Src: bin.Src, Content: []byte("zz")}); err == nil {
		t.Fatalf("expected reuse to fail")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "x.files", entries[0].Name()))
	if string(before) != string(after) {
		t.Fatalf("old binary was overwritten: %q → %q", before, after)
	}
}
