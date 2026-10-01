package cmdkit

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

	"umbraco-cli/internal/safefile"
)

// BackupAutoValue is the sentinel a bare --backup (no path) resolves to; the
// file name is then derived from the resource and id.
const BackupAutoValue = "auto"

// BackupEnvelope is the on-disk shape written by --backup and read by
// restore-backup. The entity is the exact GET response captured before the
// write, so a restore can PUT it back unchanged.
type BackupEnvelope struct {
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

// AddBackupFlag registers --backup [path]. Without a value the file lands in
// the working directory as <resource>-<id>-<timestamp>.backup.json.
func AddBackupFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "backup", "", "Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available")
	cmd.Flags().Lookup("backup").NoOptDefVal = BackupAutoValue
}

// ResolveBackupPath turns the --backup value into a concrete file path.
func ResolveBackupPath(target string, resource string, id string) string {
	file := strings.TrimSpace(target)
	if file == "" || file == BackupAutoValue {
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

// WriteBackup persists the pre-change entity (and, when given, the binary
// behind it as a sibling file) and returns the envelope path.
func WriteBackup(file string, resource string, id string, path string, entity map[string]any, binary *BackupBinary) (string, error) {
	envelope := BackupEnvelope{
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
		binaryPath, err := safefile.ChildPath(filesDir, safefile.Name(pathpkg.Base(binary.Src), "file"))
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

// BackupBinary is a downloaded media file waiting to be written next to its
// envelope.
type BackupBinary struct {
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

// BackupBinaryPath resolves a stored relative binary path and refuses
// anything that escapes the envelope's .files directory (including via
// symlinks): a crafted envelope must not make restore upload arbitrary
// local files.
func BackupBinaryPath(envelopePath string, stored string) (string, error) {
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

// ReadBackup loads a backup envelope and checks it belongs to the expected
// resource, so a document backup cannot be restored onto a media item.
func ReadBackup(file string, resource string) (BackupEnvelope, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return BackupEnvelope{}, err
	}
	var envelope BackupEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return BackupEnvelope{}, fmt.Errorf("invalid backup file %s: %w", file, err)
	}
	if envelope.Resource != resource {
		return BackupEnvelope{}, fmt.Errorf("backup file %s holds a %q, not a %s", file, envelope.Resource, resource)
	}
	if envelope.ID == "" || len(envelope.Entity) == 0 {
		return BackupEnvelope{}, fmt.Errorf("backup file %s is missing the id or entity", file)
	}
	if strings.ContainsAny(envelope.ID, "/\\?#% \t\r\n") || strings.Contains(envelope.ID, "..") {
		return BackupEnvelope{}, fmt.Errorf("backup file %s has an invalid id %q", file, envelope.ID)
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
