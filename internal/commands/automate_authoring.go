package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

// Authoring commands: create, change, and manage the lifecycle of
// automations. Together with the catalogue (step discovery), workspaces
// (where automations live), and connections (external credentials), these
// close the loop that lets an agent build an automation end to end.

func automateAutomationCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an automation (draft; publish separately)",
		Long: `POST /automations. Required: alias, name, workspaceId (from 'workspace list'), steps (the action sequence), connections (connection GUIDs used by the steps; [] for none). trigger defines what starts the flow; step aliases come from the catalogue commands.

Creating leaves the automation as a draft -- 'automation publish <id>' makes it live. For building from an existing automation, prefer the export/validate/import round-trip.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["automate.automation.create"])
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/automations", body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func automateAutomationUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update an automation",
		Long: `PUT /automations/{id}. The update model requires the automation's current version field for optimistic concurrency; --merge-json picks it up from the fetch automatically, making it the safe default for partial edits (e.g. renaming, tweaking one step's settings).

Updating creates a new draft version; 'automation publish <id>' makes it live.`,
		Path: func(args []string) string { return api.JoinPath("/automations/%s", args[0]) },
		// UpdateAutomationRequestModel declares additionalProperties: false;
		// strip the response-only fields the merge fetch echoes back.
		NormalizeMerged: stripFields("id", "workspaceId", "status", "health", "publishedVersion", "dateCreated", "dateModified", "disabledUtc", "warningIssuedUtc"),
		APIPrefix:       automateAPIPrefix,
	})
}

func automateAutomationDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete an automation (including its run history)",
		Path:      func(args []string) string { return api.JoinPath("/automations/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

// automateAutomationLifecycle builds the body-less POST lifecycle actions:
// publish (make the current draft live), unpublish (stop triggering), and
// re-enable (clear the disabled state after repeated failures).
func automateAutomationLifecycle(deps Dependencies, action string, short string, verb string) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   action + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/automations/%s/"+action, args[0]), nil, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, verb, result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateAutomationAncestors(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "ancestors <id>",
		Short: "Get an automation's location (workspace and group breadcrumb)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/automations/%s/ancestors", args[0]), automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}
