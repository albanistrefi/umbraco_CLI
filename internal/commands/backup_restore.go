package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// restoreSpec parameterizes "<group> restore-backup <file>" for the
// resources whose --backup envelopes are plain entities (documents, data
// types, and the schema types). Media has its own restore because it also
// carries a binary.
type restoreSpec struct {
	// Use is the command group name and the envelope's resource tag
	// (--backup stamps the parent command name), e.g. "document".
	Use string
	// Display is the human name used in help and errors.
	Display string
	// PathFormat is the entity route with one %s for the id.
	PathFormat string
	// StripFields lists response-only keys the update request model rejects.
	StripFields []string
	// Extra lines appended to the help text.
	Notes string
}

// restoreBackupCommand builds the generic restore: read the envelope, PUT
// the captured entity back onto the id it was taken from, re-read, and
// confirm the captured properties are present again.
func restoreBackupCommand(deps Dependencies, spec restoreSpec) *cobra.Command {
	var dryRun bool
	notes := ""
	if spec.Notes != "" {
		notes = " " + spec.Notes
	}
	cmd := &cobra.Command{
		Use:   "restore-backup <file>",
		Short: fmt.Sprintf("Restore a %s from a --backup JSON file", spec.Display),
		Long: fmt.Sprintf("Reads a file written by any '%s … --backup' command and PUTs the saved %s back to the server, then re-reads it and fails if a saved property is missing afterwards. "+
			"The endpoint is derived from the id inside the envelope (the file cannot steer the write elsewhere). "+
			"This restores the entity's fields and values; it does not undo a move, publish state, or delete.%s", spec.Use, spec.Display, notes),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			envelope, err := readBackup(args[0], spec.Use)
			if err != nil {
				return err
			}
			path := api.JoinPath(spec.PathFormat, envelope.ID)
			if envelope.Path != "" && envelope.Path != path {
				return fmt.Errorf("backup file %s names path %q, which does not match %s %s", args[0], envelope.Path, spec.Display, envelope.ID)
			}
			body := cloneAnyMap(envelope.Entity)
			for _, key := range spec.StripFields {
				delete(body, key)
			}
			putResult, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			result := map[string]any{"id": envelope.ID, "savedAt": envelope.SavedAt}
			if dryRun {
				result["update"] = putResult
				return printResult(cmd, deps, result)
			}
			after, err := fetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return fmt.Errorf("the restore was accepted but re-reading the %s failed: %w", spec.Display, err)
			}
			if missing := missingRestoredAliases(envelope.Entity, after); len(missing) > 0 {
				return fmt.Errorf("the restore was accepted but the %s is missing %s afterwards: %s", spec.Display, restoredAliasLabel(envelope.Entity), strings.Join(missing, ", "))
			}
			result["restored"] = true
			result["verified"] = true
			if name := entityDisplayName(after); name != "" {
				result["name"] = name
			}
			return printResult(cmd, deps, result)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// missingRestoredAliases lists the aliases present in the saved entity's
// values (content) or properties (schema types) that the re-read entity no
// longer carries — the signal that a write was accepted but not applied.
func missingRestoredAliases(saved map[string]any, after map[string]any) []string {
	key := "values"
	if _, hasValues := saved["values"]; !hasValues {
		if _, hasProperties := saved["properties"]; hasProperties {
			key = "properties"
		}
	}
	present := map[string]bool{}
	for _, item := range aliasEntries(after[key]) {
		present[item] = true
	}
	missing := []string{}
	seen := map[string]bool{}
	for _, alias := range aliasEntries(saved[key]) {
		if !present[alias] && !seen[alias] {
			missing = append(missing, alias)
			seen[alias] = true
		}
	}
	return missing
}

func restoredAliasLabel(saved map[string]any) string {
	if _, hasValues := saved["values"]; hasValues {
		return "values"
	}
	return "properties"
}

func aliasEntries(raw any) []string {
	items, _ := raw.([]any)
	aliases := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if alias, _ := entry["alias"].(string); alias != "" {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

// entityDisplayName returns the top-level name or, for variant content, the
// first variant's name.
func entityDisplayName(entity map[string]any) string {
	if name, _ := entity["name"].(string); name != "" {
		return name
	}
	variants, _ := entity["variants"].([]any)
	for _, raw := range variants {
		if variant, ok := raw.(map[string]any); ok {
			if name, _ := variant["name"].(string); name != "" {
				return name
			}
		}
	}
	return ""
}

// writeEntityBackup is the --backup step shared by the bespoke document
// update commands: it saves the pre-change entity and returns the file.
func writeEntityBackup(cmd *cobra.Command, target string, resource string, id string, path string, current map[string]any) (string, error) {
	return writeBackup(resolveBackupPath(target, resource, id), resource, id, path, current, nil)
}

// withBackupPath adds the backup file to a mutation result so callers can
// find an auto-named file.
func withBackupPath(result any, backupFile string, verb string) any {
	if backupFile == "" {
		return result
	}
	if result == nil {
		return map[string]any{verb: true, "backup": backupFile}
	}
	if body, ok := result.(map[string]any); ok {
		body["backup"] = backupFile
		return body
	}
	return map[string]any{verb: result, "backup": backupFile}
}
