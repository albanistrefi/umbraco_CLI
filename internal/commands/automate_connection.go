package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

// Connections are reusable credential sets for external services (Slack,
// Airtable, HTTP APIs, ...). Actions that talk to the outside world
// reference a connection, and a workspace whitelists which connections its
// automations may use — so connection discovery precedes authoring.

func automateConnection(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Connection operations (credentials automations use for external services)",
	}
	cmd.AddCommand(automateConnectionList(deps))
	cmd.AddCommand(automateConnectionGet(deps))
	cmd.AddCommand(automateConnectionCreate(deps))
	cmd.AddCommand(automateConnectionUpdate(deps))
	cmd.AddCommand(automateConnectionDelete(deps))
	cmd.AddCommand(automateConnectionTest(deps))
	return cmd
}

func automateConnectionList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List connections (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/connections", opts: automateOpts(params, false)},
			}
		},
	})
}

func automateConnectionGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:       "get <id>",
		Short:     "Get a connection by ID",
		Path:      func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateConnectionCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a connection",
		Long:  "POST /connections. Required: alias, name, type (from 'catalogue connection-types'), settings (the type's credential fields). Verify it works afterwards with 'connection test'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["automate.connection.create"])
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/connections", body, automateOpts(nil, dryRun))
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

func automateConnectionUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:       "update <id>",
		Short:     "Update a connection",
		Path:      func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateConnectionDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete a connection (automations referencing it will fail)",
		Path:      func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateConnectionTest(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Test that a connection's credentials work against the external service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/connections/%s/test", args[0]), nil, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "tested", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
