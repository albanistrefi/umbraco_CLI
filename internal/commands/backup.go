package commands

import (
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// backupAutoValue is the sentinel a bare --backup (no path) resolves to; the
// file name is then derived from the resource and id.
const backupAutoValue = "auto"

// backupEnvelope is the on-disk shape written by --backup and read by
// restore-backup. The entity is the exact GET response captured before the
// write, so a restore can PUT it back unchanged.
type backupEnvelope struct {
	Resource string         `json:"resource"`
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	SavedAt  string         `json:"savedAt"`
	Entity   map[string]any `json:"entity"`
	// File, when present, is the binary behind a media file property, saved
	// next to the envelope. Umbraco deletes the previous file on replace, so
	// an entity-only restore would point at a file that no longer exists.
	File *backupFile `json:"file,omitempty"`
}

type backupFile struct {
	Property string `json:"property"`
	Src      string `json:"src"`
	Path     string `json:"path"`
	Bytes    int    `json:"bytes"`
}

// addBackupFlag registers --backup [path]. Without a value the file lands in
// the working directory as <resource>-<id>-<timestamp>.backup.json.
func addBackupFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "backup", "", "Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available")
	cmd.Flags().Lookup("backup").NoOptDefVal = backupAutoValue
}

// resolveBackupPath turns the --backup value into a concrete file path.
func resolveBackupPath(target string, resource string, id string) string {
	file := strings.TrimSpace(target)
	if file == "" || file == backupAutoValue {
		file = fmt.Sprintf("%s-%s-%s.backup.json", resource, sanitizeFileComponent(id), time.Now().UTC().Format("20060102T150405Z"))
	}
	return file
}

// writeBackup persists the pre-change entity (and, when given, the binary
// behind it as a sibling file) and returns the envelope path.
func writeBackup(file string, resource string, id string, path string, entity map[string]any, binary *backupBinary) (string, error) {
	envelope := backupEnvelope{
		Resource: resource,
		ID:       id,
		Path:     path,
		SavedAt:  time.Now().UTC().Format(time.RFC3339),
		Entity:   entity,
	}
	if dir := filepath.Dir(file); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if binary != nil {
		// The binary keeps its original file name inside a sibling folder so a
		// restore re-uploads it under the same name (Umbraco derives the
		// stored file name from the upload and strips extra dots).
		filesDir := strings.TrimSuffix(file, ".json") + ".files"
		if err := os.MkdirAll(filesDir, 0o700); err != nil {
			return "", err
		}
		baseName := pathpkg.Base(binary.Src)
		if baseName == "" || baseName == "." || baseName == "/" {
			baseName = "file"
		}
		binaryPath := filepath.Join(filesDir, baseName)
		if err := os.WriteFile(binaryPath, binary.Content, 0o600); err != nil {
			return "", fmt.Errorf("failed to write backup file %s: %w", binaryPath, err)
		}
		relative, err := filepath.Rel(filepath.Dir(file), binaryPath)
		if err != nil {
			relative = binaryPath
		}
		envelope.File = &backupFile{Property: binary.Property, Src: binary.Src, Path: filepath.ToSlash(relative), Bytes: len(binary.Content)}
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(file, encoded, 0o600); err != nil {
		return "", fmt.Errorf("failed to write backup %s: %w", file, err)
	}
	return file, nil
}

// backupBinary is a downloaded media file waiting to be written next to its
// envelope.
type backupBinary struct {
	Property string
	Src      string
	Content  []byte
}

// readBackup loads a backup envelope and checks it belongs to the expected
// resource, so a document backup cannot be restored onto a media item.
func readBackup(file string, resource string) (backupEnvelope, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return backupEnvelope{}, err
	}
	var envelope backupEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return backupEnvelope{}, fmt.Errorf("invalid backup file %s: %w", file, err)
	}
	if envelope.Resource != resource {
		return backupEnvelope{}, fmt.Errorf("backup file %s holds a %q, not a %s", file, envelope.Resource, resource)
	}
	if envelope.ID == "" || len(envelope.Entity) == 0 {
		return backupEnvelope{}, fmt.Errorf("backup file %s is missing the id or entity", file)
	}
	return envelope, nil
}

func sanitizeFileComponent(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
