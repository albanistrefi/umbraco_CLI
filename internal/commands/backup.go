package commands

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	Culture  string `json:"culture,omitempty"`
	Segment  string `json:"segment,omitempty"`
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
		// Timestamp plus a random suffix: two writes to the same item in the
		// same second must not share (and truncate) one backup.
		file = fmt.Sprintf("%s-%s-%s-%s.backup.json", resource, sanitizeFileComponent(id), time.Now().UTC().Format("20060102T150405Z"), randomSuffix())
	}
	return file
}

func randomSuffix() string {
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(buf[:])
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
	// Reserve the envelope first (exclusive create), so a reused --backup path
	// fails before any sibling binary of the older backup could be touched.
	handle, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to write backup %s (refusing to overwrite an existing file): %w", file, err)
	}
	defer func() { _ = handle.Close() }()

	if binary != nil {
		// The binary keeps its (sanitized) original file name inside a
		// sibling folder so a restore re-uploads it under the same name
		// (Umbraco derives the stored file name from the upload).
		filesDir := backupFilesDir(file)
		if err := os.MkdirAll(filesDir, 0o700); err != nil {
			return "", err
		}
		binaryPath, err := safeChildPath(filesDir, sanitizeFileName(pathpkg.Base(binary.Src), "file"))
		if err != nil {
			return "", err
		}
		binaryHandle, err := os.OpenFile(binaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", fmt.Errorf("failed to write backup file %s (refusing to overwrite an existing file): %w", binaryPath, err)
		}
		_, writeErr := binaryHandle.Write(binary.Content)
		closeErr := binaryHandle.Close()
		if writeErr != nil || closeErr != nil {
			return "", fmt.Errorf("failed to write backup file %s: %w", binaryPath, errors.Join(writeErr, closeErr))
		}
		relative, err := filepath.Rel(filepath.Dir(file), binaryPath)
		if err != nil {
			relative = binaryPath
		}
		envelope.File = &backupFile{Property: binary.Property, Culture: binary.Culture, Segment: binary.Segment, Src: binary.Src, Path: filepath.ToSlash(relative), Bytes: len(binary.Content)}
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", err
	}
	if _, err := handle.Write(encoded); err != nil {
		return "", fmt.Errorf("failed to write backup %s: %w", file, err)
	}
	return file, nil
}

// sanitizeFileName reduces a server-provided file name to a single safe path
// component for any host OS: no separators of either flavour, no traversal,
// no control characters.
func sanitizeFileName(name string, fallback string) string {
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

// safeChildPath joins name under dir and verifies the result is a direct
// child of dir under the host OS's path semantics.
func safeChildPath(dir string, name string) (string, error) {
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

// backupBinary is a downloaded media file waiting to be written next to its
// envelope.
type backupBinary struct {
	Property string
	Culture  string
	Segment  string
	Src      string
	Content  []byte
}

// backupFilesDir is the sibling folder holding a backup's binaries.
func backupFilesDir(envelopePath string) string {
	return strings.TrimSuffix(envelopePath, ".json") + ".files"
}

// backupBinaryPath resolves a stored relative binary path and refuses
// anything that escapes the envelope's .files directory (including via
// symlinks): a crafted envelope must not make restore upload arbitrary
// local files.
func backupBinaryPath(envelopePath string, stored string) (string, error) {
	filesDir, err := filepath.Abs(backupFilesDir(envelopePath))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(filesDir); err == nil {
		filesDir = resolved
	}
	candidate := filepath.Join(filepath.Dir(envelopePath), filepath.FromSlash(stored))
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(candidateAbs); err == nil {
		candidateAbs = resolved
	}
	if candidateAbs != filesDir && !strings.HasPrefix(candidateAbs, filesDir+string(filepath.Separator)) {
		return "", fmt.Errorf("backup file path %q points outside %s; refusing to upload it", stored, filesDir)
	}
	if candidateAbs == filesDir {
		return "", fmt.Errorf("backup file path %q is a directory", stored)
	}
	return candidateAbs, nil
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
	if strings.ContainsAny(envelope.ID, "/\\?#% \t\r\n") || strings.Contains(envelope.ID, "..") {
		return backupEnvelope{}, fmt.Errorf("backup file %s has an invalid id %q", file, envelope.ID)
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
