package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// mediaReplaceFile swaps the file behind an existing media item while keeping
// every other value. Field report: without this, agents fell back to a raw
// PUT, and a PUT whose temporaryFileId does not resolve (expired, consumed,
// or mistyped) returns 200 while leaving the item with no values at all —
// reproduced live on 18.1. The command therefore verifies after writing and
// refuses to report success for an emptied item.
func mediaReplaceFile(deps Dependencies) *cobra.Command {
	var propertyAlias string
	var name string
	var backup string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "replace-file <id> <file>",
		Short: "Replace the file behind an existing media item, keeping its other values",
		Long: "Uploads <file> as a temporary file, rewrites the file property (default umbracoFile) on the existing item, and verifies the result. " +
			"Every other value is preserved; the server recomputes derived values (umbracoBytes, umbracoExtension, dimensions).\n\n" +
			"After the write the item is fetched again; if the server accepted the PUT but left the item with no values (what happens when a temporary file id does not resolve) the command fails instead of reporting success. " +
			"Pass --backup to save the pre-change item first; 'media restore-backup <file>' puts it back.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id, filePath := args[0], args[1]
			path := api.JoinPath("/media/%s", id)

			current, err := fetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return err
			}
			before, _ := mediaFileValue(current, propertyAlias)
			if before == nil {
				return fmt.Errorf("media item %s has no %q property; check the media type or pass --property", id, propertyAlias)
			}

			var backupFile string
			if cmd.Flags().Changed("backup") && !dryRun {
				binary, err := downloadMediaBinary(ctx, deps.Client, propertyAlias, before)
				if err != nil {
					return fmt.Errorf("--backup could not download the current file: %w", err)
				}
				backupFile, err = writeBackup(resolveBackupPath(backup, "media", id), "media", id, path, current, binary)
				if err != nil {
					return err
				}
			}

			tempID, err := newUUIDv4()
			if err != nil {
				return fmt.Errorf("failed to generate temporary file id: %w", err)
			}
			uploadResult, err := deps.Client.MultipartPost(ctx, "/temporary-file", map[string]string{"id": tempID}, "file", filePath, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			patch := map[string]any{
				"values": []any{map[string]any{
					"alias": propertyAlias,
					"value": map[string]any{"temporaryFileId": tempID},
				}},
			}
			if strings.TrimSpace(name) != "" {
				patch["variants"] = renameVariants(current, name)
			}
			body := mergeAliasPayload(current, patch)

			putResult, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, map[string]any{
					"id":            id,
					"property":      propertyAlias,
					"before":        before,
					"temporaryFile": map[string]any{"id": tempID, "upload": uploadResult},
					"update":        putResult,
				})
			}

			after, err := verifyMediaFileWrite(ctx, deps.Client, path, propertyAlias)
			if err != nil {
				if backupFile != "" {
					return fmt.Errorf("%w; the pre-change item was saved to %s — run 'umbraco media restore-backup %s' to put it back", err, backupFile, backupFile)
				}
				return err
			}
			result := map[string]any{
				"id":            id,
				"property":      propertyAlias,
				"before":        before,
				"after":         after,
				"changed":       fmt.Sprint(before["src"]) != fmt.Sprint(after["src"]),
				"temporaryFile": map[string]any{"id": tempID},
				"verified":      true,
			}
			if backupFile != "" {
				result["backup"] = backupFile
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&propertyAlias, "property", "umbracoFile", "File property alias to replace")
	cmd.Flags().StringVar(&name, "name", "", "Also rename the media item")
	addBackupFlag(cmd, &backup)
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// mediaRestoreBackup PUTs a --backup file back onto its media item.
func mediaRestoreBackup(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "restore-backup <file>",
		Short: "Restore a media item from a --backup JSON file",
		Long: "Reads a file written by 'media replace-file --backup' or 'media update --backup' and PUTs the saved item back to the server, then verifies the item is no longer empty. " +
			"Backups from replace-file also carry the original file: Umbraco deletes the previous file when it is replaced, so the restore re-uploads the saved binary rather than pointing at a path that no longer exists. " +
			"Backups from 'media update --backup' are metadata-only; the restore first checks the referenced file is still served and refuses to write if it is gone. " +
			"This restores values and name; it does not undo a move or delete (use 'media restore' for the recycle bin).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			envelope, err := readBackup(args[0], "media")
			if err != nil {
				return err
			}
			path := envelope.Path
			if path == "" {
				path = api.JoinPath("/media/%s", envelope.ID)
			}
			body := envelope.Entity
			result := map[string]any{"id": envelope.ID, "savedAt": envelope.SavedAt}

			if envelope.File == nil {
				// Metadata-only backup: refuse before writing if the file it
				// points at is gone, rather than restoring a dead reference.
				if fileValue, ok := mediaFileValue(envelope.Entity, "umbracoFile"); ok {
					if src := strings.TrimSpace(fmt.Sprint(fileValue["src"])); src != "" && src != "<nil>" {
						if _, _, err := deps.Client.GetBytes(ctx, src, api.RequestOptions{RawPath: true}); err != nil {
							return fmt.Errorf("refusing to restore: the backup is metadata-only and its file %s is no longer served (%v); only 'media replace-file --backup' captures the binary, use 'media replace-file %s <file>' with a local copy instead", src, err, envelope.ID)
						}
					}
				}
			} else {
				binaryPath := filepath.Join(filepath.Dir(args[0]), filepath.FromSlash(envelope.File.Path))
				tempID, err := newUUIDv4()
				if err != nil {
					return fmt.Errorf("failed to generate temporary file id: %w", err)
				}
				if _, err := deps.Client.MultipartPost(ctx, "/temporary-file", map[string]string{"id": tempID}, "file", binaryPath, api.RequestOptions{DryRun: dryRun}); err != nil {
					return fmt.Errorf("re-uploading the backed-up file %s failed: %w", binaryPath, err)
				}
				body = mergeAliasPayload(envelope.Entity, map[string]any{
					"values": []any{map[string]any{"alias": envelope.File.Property, "value": map[string]any{"temporaryFileId": tempID}}},
				})
				result["reuploaded"] = map[string]any{"file": binaryPath, "property": envelope.File.Property, "temporaryFile": tempID}
			}

			putResult, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				result["update"] = putResult
				return printResult(cmd, deps, result)
			}
			after, err := fetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return err
			}
			if len(mediaValues(after)) == 0 && len(mediaValues(envelope.Entity)) > 0 {
				return fmt.Errorf("the server accepted the restore but media item %s still has no values; the backup at %s is intact, inspect it with 'umbraco media get %s'", envelope.ID, args[0], envelope.ID)
			}
			result["restored"] = true
			result["values"] = len(mediaValues(after))
			return printResult(cmd, deps, result)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// verifyMediaFileWrite re-fetches the item and returns the file property's
// value, failing when the write emptied the item or dropped the property.
func verifyMediaFileWrite(ctx context.Context, client *api.Client, path string, propertyAlias string) (map[string]any, error) {
	after, err := fetchObject(ctx, client, path, api.RequestOptions{})
	if err != nil {
		return nil, fmt.Errorf("the update was accepted but re-reading the item failed: %w", err)
	}
	if len(mediaValues(after)) == 0 {
		return nil, fmt.Errorf("the server accepted the update but left the media item with no values (this is what happens when the temporary file id does not resolve)")
	}
	value, _ := mediaFileValue(after, propertyAlias)
	if value == nil || strings.TrimSpace(fmt.Sprint(value["src"])) == "" || value["src"] == nil {
		return nil, fmt.Errorf("the server accepted the update but the %q property has no file afterwards", propertyAlias)
	}
	return value, nil
}

func mediaValues(entity map[string]any) []any {
	values, _ := entity["values"].([]any)
	return values
}

// mediaFileValue returns the object value of the given property (e.g.
// {"src": "/media/.../logo.svg"}) and whether the property exists at all.
func mediaFileValue(entity map[string]any, alias string) (map[string]any, bool) {
	for _, raw := range mediaValues(entity) {
		entry, ok := raw.(map[string]any)
		if !ok || entry["alias"] != alias {
			continue
		}
		value, _ := entry["value"].(map[string]any)
		if value == nil {
			value = map[string]any{}
		}
		return value, true
	}
	return nil, false
}

// renameVariants copies the current variants with the name replaced, so a
// rename rides along on the same PUT without touching culture/segment.
func renameVariants(current map[string]any, name string) []any {
	variants, _ := current["variants"].([]any)
	renamed := make([]any, 0, len(variants))
	for _, raw := range variants {
		variant, ok := raw.(map[string]any)
		if !ok {
			renamed = append(renamed, raw)
			continue
		}
		copied := make(map[string]any, len(variant))
		for key, value := range variant {
			copied[key] = value
		}
		copied["name"] = name
		renamed = append(renamed, copied)
	}
	return renamed
}

// downloadMediaBinary fetches the file currently behind a media file property
// so a backup can restore it after the server deletes the replaced file.
func downloadMediaBinary(ctx context.Context, client *api.Client, propertyAlias string, value map[string]any) (*backupBinary, error) {
	src := strings.TrimSpace(fmt.Sprint(value["src"]))
	if src == "" || src == "<nil>" {
		return nil, nil
	}
	content, _, err := client.GetBytes(ctx, src, api.RequestOptions{RawPath: true})
	if err != nil {
		return nil, err
	}
	return &backupBinary{Property: propertyAlias, Src: src, Content: content}, nil
}
