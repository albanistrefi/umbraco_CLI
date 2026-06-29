package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// The export/validate/import round-trip is the agent-friendliest authoring
// path: export a working automation as a portable definition, adjust the
// JSON, validate create/import payloads server-side without writing anything,
// then import it as a new automation. Existing automation edits use
// import-update; rehearse those with --dry-run.

// automateExportModelInput resolves the export model from --file or --json
// (exactly one required). The model is what 'automation export' returns.
func automateExportModelInput(file string, jsonRaw string) (map[string]any, error) {
	hasFile := strings.TrimSpace(file) != ""
	hasJSON := strings.TrimSpace(jsonRaw) != ""
	if hasFile == hasJSON {
		return nil, fmt.Errorf("provide the export model via exactly one of --file or --json")
	}
	if hasJSON {
		return parsePayload(jsonRaw)
	}
	payload, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return parseJSONObject(string(payload), "--file")
}

func automateAutomationValidate(deps Dependencies) *cobra.Command {
	var workspaceID string
	var file string
	var jsonRaw string
	cmd := &cobra.Command{
		Use:   "validate --workspace-id <id> --file <export.json>",
		Short: "Validate a new automation import server-side without writing anything",
		Long:  "POST /automations/import/validate. Checks an export model against a workspace -- step aliases, connection references, binding syntax -- and reports success/errors/warnings for creating/importing a new automation. It does not validate overwriting an existing automation; for edits use 'automation import-update <id> --dry-run' to preflight the update request shape.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--workspace-id", workspaceID); err != nil {
				return err
			}
			exportModel, err := automateExportModelInput(file, jsonRaw)
			if err != nil {
				return err
			}
			body := map[string]any{"workspaceId": workspaceID, "exportModel": exportModel}
			result, err := deps.Client.Post(cmd.Context(), "/automations/import/validate", body, automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace the definition would be imported into (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to an export-model JSON file (from 'automation export')")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Export model as inline JSON")
	return cmd
}

func automateAutomationImport(deps Dependencies) *cobra.Command {
	var workspaceID string
	var file string
	var jsonRaw string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import --workspace-id <id> --file <export.json>",
		Short: "Import an automation definition as a new automation",
		Long:  "POST /automations/import. Creates a new draft automation from an export model. Run 'automation validate' first -- it performs the same create/import checks without writing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--workspace-id", workspaceID); err != nil {
				return err
			}
			exportModel, err := automateExportModelInput(file, jsonRaw)
			if err != nil {
				return err
			}
			body := map[string]any{"workspaceId": workspaceID, "exportModel": exportModel}
			result, err := deps.Client.Post(cmd.Context(), "/automations/import", body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "imported", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace to import into (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to an export-model JSON file (from 'automation export')")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Export model as inline JSON")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateAutomationImportUpdate(deps Dependencies) *cobra.Command {
	var file string
	var jsonRaw string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import-update <id> --file <export.json>",
		Short: "Overwrite an existing automation from an export model",
		Long:  "PUT /automations/{id}/import. Unlike 'automation import' this takes the bare export model as the body (no workspace wrapper -- the automation already lives somewhere) and replaces the automation's definition with it. Use --dry-run as the update preflight path; 'automation validate' targets only new imports and can reject existing automation IDs.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exportModel, err := automateExportModelInput(file, jsonRaw)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/automations/%s/import", args[0]), exportModel, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "imported", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Path to an export-model JSON file (from 'automation export')")
	cmd.Flags().StringVar(&jsonRaw, "json", "", "Export model as inline JSON")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
