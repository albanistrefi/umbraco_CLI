package commands

import (
	"encoding/json"
	"fmt"
	"reflect"
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
	var expectID string
	var dryRun bool
	notes := ""
	if spec.Notes != "" {
		notes = " " + spec.Notes
	}
	cmd := &cobra.Command{
		Use:   "restore-backup <file>",
		Short: fmt.Sprintf("Restore a %s from a --backup JSON file", spec.Display),
		Long: fmt.Sprintf("Reads a file written by any '%s … --backup' command and PUTs the saved %s back to the server, then re-reads it and compares the saved values/properties (alias, culture and segment) and names with what the server now holds; any difference fails the command. "+
			"The write goes to the id recorded in the envelope; pass --id <guid> to assert which %s the file must belong to before anything is written (the file is refused when it names another id), and --dry-run to see the target without writing. "+
			"This restores the entity's fields and values; it does not undo a move, publish state, or delete.%s", spec.Use, spec.Display, spec.Display, notes),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			envelope, err := readBackup(args[0], spec.Use)
			if err != nil {
				return err
			}
			if strings.TrimSpace(expectID) != "" && !strings.EqualFold(strings.TrimSpace(expectID), envelope.ID) {
				return fmt.Errorf("backup file %s belongs to %s %s, not %s; refusing to restore", args[0], spec.Display, envelope.ID, expectID)
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
			if diffs := restoreDifferences(envelope.Entity, after); len(diffs) > 0 {
				return fmt.Errorf("the restore was accepted but the %s does not match the backup afterwards: %s", spec.Display, strings.Join(diffs, "; "))
			}
			result["restored"] = true
			result["verified"] = true
			if name := entityDisplayName(after); name != "" {
				result["name"] = name
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&expectID, "id", "", "Assert the envelope belongs to this id before writing (refuses otherwise)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// restoreDifferences compares what the backup asked the server to hold with
// what it holds after the write: every saved value (keyed by alias, culture
// and segment) must be present with an equal value, every saved property
// alias must exist, and variant names must match. An accepted PUT that
// silently kept an old value is therefore reported, not called verified.
func restoreDifferences(saved map[string]any, after map[string]any) []string {
	diffs := []string{}
	if savedValues, ok := saved["values"].([]any); ok {
		afterValues := indexValueEntries(after["values"])
		for _, raw := range savedValues {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			key := valueEntryKey(entry)
			got, present := afterValues[key]
			switch {
			case !present:
				diffs = append(diffs, fmt.Sprintf("value %s is missing", key))
			case !reflect.DeepEqual(normalizeJSON(entry["value"]), normalizeJSON(got["value"])):
				diffs = append(diffs, fmt.Sprintf("value %s differs from the backup", key))
			}
		}
	}
	if savedProperties, ok := saved["properties"].([]any); ok {
		present := map[string]bool{}
		for _, alias := range aliasEntries(after["properties"]) {
			present[alias] = true
		}
		for _, alias := range aliasEntries(savedProperties) {
			if !present[alias] {
				diffs = append(diffs, fmt.Sprintf("property %s is missing", alias))
			}
		}
	}
	if savedVariants, ok := saved["variants"].([]any); ok {
		afterVariants := indexValueEntries(after["variants"])
		for _, raw := range savedVariants {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			wantName, _ := entry["name"].(string)
			got, present := afterVariants[valueEntryKey(entry)]
			gotName, _ := got["name"].(string)
			if !present || gotName != wantName {
				diffs = append(diffs, fmt.Sprintf("name %s is %q, backup has %q", valueEntryKey(entry), gotName, wantName))
			}
		}
	} else if wantName, _ := saved["name"].(string); wantName != "" {
		if gotName, _ := after["name"].(string); gotName != wantName {
			diffs = append(diffs, fmt.Sprintf("name is %q, backup has %q", gotName, wantName))
		}
	}
	return diffs
}

// valueEntryKey identifies a values[]/variants[] entry by alias, culture
// and segment ("title@en-US/segment"; invariant entries have no suffix).
func valueEntryKey(entry map[string]any) string {
	alias, _ := entry["alias"].(string)
	culture, _ := entry["culture"].(string)
	segment, _ := entry["segment"].(string)
	key := alias
	if culture != "" || segment != "" {
		key += "@" + culture
	}
	if segment != "" {
		key += "/" + segment
	}
	return key
}

func indexValueEntries(raw any) map[string]map[string]any {
	items, _ := raw.([]any)
	index := make(map[string]map[string]any, len(items))
	for _, item := range items {
		if entry, ok := item.(map[string]any); ok {
			index[valueEntryKey(entry)] = entry
		}
	}
	return index
}

// normalizeJSON round-trips a value through JSON so a saved number and the
// server's echo of it compare equal regardless of their Go representation.
func normalizeJSON(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return value
	}
	return out
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

// withBackupHint appends the backup file to an error raised after the
// backed-up entity was already changed, so the undo path is never lost.
func withBackupHint(err error, use string, backupFile string) error {
	if backupFile == "" {
		return err
	}
	return fmt.Errorf("%w; the pre-change %s was saved to %s — run 'umbraco %s restore-backup %s' to put it back", err, use, backupFile, use, backupFile)
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
