package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Bulk update operations for the document command group: explicit ID
// lists and CSV-driven updates.

func documentBulkUpdate(deps Dependencies) *cobra.Command {
	var ids []string
	var idsCSV string
	var idFile string
	var jsonPayload string
	var mergeJSON string
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "bulk-update",
		Short: "Update multiple documents from an explicit ID list (see also 'document update --ids')",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "mutates every listed document", force, dryRun); err != nil {
				return err
			}

			hasJSON := strings.TrimSpace(jsonPayload) != ""
			hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
			if hasJSON == hasMergeJSON {
				return fmt.Errorf("document bulk-update requires exactly one of --json or --merge-json")
			}

			resolvedIDs, err := loadDocumentIDs(append(append([]string{}, ids...), uniqueCSV(idsCSV)...), idFile)
			if err != nil {
				return err
			}
			if len(resolvedIDs) == 0 {
				return fmt.Errorf("document bulk-update requires at least one --id or --id-file entry")
			}

			var fullBody map[string]any
			var mergePatch map[string]any
			if hasMergeJSON {
				mergePatch, err = parsePayload(mergeJSON)
				if err != nil {
					return err
				}
			} else {
				fullBody, err = parsePayload(jsonPayload)
				if err != nil {
					return err
				}
			}

			result := executeDocumentBulkUpdate(cmd.Context(), deps.Client, resolvedIDs, fullBody, mergePatch, dryRun)
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringArrayVar(&ids, "id", nil, "Document ID to update; repeat for multiple documents")
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated document IDs (same as repeating --id)")
	cmd.Flags().StringVar(&idFile, "id-file", "", "Path to a file containing document IDs, one per line")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload applied to every document")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into each current document before update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned requests without executing")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the bulk update when not using --dry-run")
	return cmd
}

func documentCSVUpdate(deps Dependencies) *cobra.Command {
	var file string
	var idColumn string
	var properties []string
	var fieldMappings []string
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "csv-update",
		Short: "Update multiple documents from a CSV file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "mutates every document in the CSV", force, dryRun); err != nil {
				return err
			}
			if err := requireValue("--file", file); err != nil {
				return err
			}

			mappings, err := parseDocumentCSVFieldMappings(properties, fieldMappings)
			if err != nil {
				return err
			}
			if len(mappings) == 0 {
				return fmt.Errorf("document csv-update requires at least one --property or --field mapping")
			}

			result, err := executeDocumentCSVUpdate(cmd.Context(), deps.Client, documentCSVUpdateOptions{
				File:     file,
				IDColumn: idColumn,
				Mappings: mappings,
				DryRun:   dryRun,
			})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Path to the CSV file")
	cmd.Flags().StringVar(&idColumn, "id-column", "id", "CSV column containing document IDs")
	cmd.Flags().StringArrayVar(&properties, "property", nil, "Property alias to update from a CSV column with the same name; repeat for multiple properties")
	cmd.Flags().StringArrayVar(&fieldMappings, "field", nil, "Explicit alias=column CSV mapping; repeat for multiple properties")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned CSV-driven updates without executing them")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the CSV-driven updates when not using --dry-run")
	return cmd
}
