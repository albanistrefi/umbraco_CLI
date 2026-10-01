package automate

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/schema"
)

// Connections are reusable credential sets for external services (Slack,
// Airtable, HTTP APIs, ...). Actions that talk to the outside world
// reference a connection, and a workspace whitelists which connections its
// automations may use — so connection discovery precedes authoring.

func automateConnection(deps cmdkit.Dependencies) *cobra.Command {
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

func automateConnectionList(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List connections (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/connections", Opts: automateOpts(params, false)},
			}
		},
	})
}

func automateConnectionGet(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:       "get <id>",
		Short:     "Get a connection by ID",
		Path:      func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		APIPrefix: APIPrefix,
	})
}

func automateConnectionCreate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a connection",
		Long:  "POST /connections. Required: alias, name, type (from 'catalogue connection-types'), settings (the type's credential fields). Verify it works afterwards with 'connection test'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return cmdkit.PrintResult(cmd, deps, schema.Templates["automate.connection.create"])
			}
			if err := cmdkit.RequireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := cmdkit.ParsePayload(jsonPayload)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/connections", body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func automateConnectionUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:             "update <id>",
		Short:           "Update a connection",
		Path:            func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		NormalizeMerged: cmdkit.StripFields("id", "dateCreated", "dateModified"),
		APIPrefix:       APIPrefix,
	})
}

func automateConnectionDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete a connection (automations referencing it will fail)",
		Path:      func(args []string) string { return api.JoinPath("/connections/%s", args[0]) },
		APIPrefix: APIPrefix,
	})
}

func automateConnectionTest(deps cmdkit.Dependencies) *cobra.Command {
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
			return cmdkit.PrintMutationResult(cmd, deps, "tested", result, dryRun)
		},
	}
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}
