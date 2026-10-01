package commands

import (
	"net/http"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

func RegisterHealth(root *cobra.Command, deps cmdkit.Dependencies) {
	health := &cobra.Command{Use: "health", Short: "Health check operations"}
	health.AddCommand(healthGroups(deps))
	health.AddCommand(healthGroup(deps))
	health.AddCommand(healthRun(deps))
	health.AddCommand(healthAction(deps))
	root.AddCommand(health)
}

func healthGroups(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "groups", Short: "List health check groups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), "/health-check-group", api.RequestOptions{})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func healthGroup(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "group <name>", Short: "Get health check group details", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/health-check-group/%s", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func healthRun(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "run <group-name>", Short: "Run health checks for group", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/health-check-group/%s/check", args[0]), nil, api.RequestOptions{})
		if api.IsStatus(err, http.StatusNotFound) {
			// Older servers expose GET .../run instead of POST .../check.
			result, err = deps.Client.Get(cmd.Context(), api.JoinPath("/health-check-group/%s/run", args[0]), api.RequestOptions{})
		}
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func healthAction(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "action <id>",
		Short: "Execute a health check action",
		Long:  "POST /health-check/execute-action. On current servers <id> is the health check id from 'health run' results and fills healthCheck.id in the body when --json omits it. On older servers (which 404 the modern route) <id> must be the legacy action id — it is forwarded to POST /health-check/{actionId} with the --json payload unchanged.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cmdkit.OptionalBody(jsonPayload)
			if err != nil {
				return err
			}
			// Modern servers take the full action model on /execute-action, with
			// the owning health check referenced in the body; the positional
			// argument fills that reference when --json doesn't carry one.
			modern := make(map[string]any, len(body)+2)
			for k, v := range body {
				modern[k] = v
			}
			if _, ok := modern["healthCheck"]; !ok {
				modern["healthCheck"] = map[string]any{"id": args[0]}
			}
			if _, ok := modern["valueRequired"]; !ok {
				modern["valueRequired"] = false
			}
			result, err := deps.Client.Post(cmd.Context(), "/health-check/execute-action", modern, api.RequestOptions{DryRun: dryRun})
			if api.IsStatus(err, http.StatusNotFound) {
				// Older servers address the action by id in the path instead.
				result, err = deps.Client.Post(cmd.Context(), api.JoinPath("/health-check/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
			}
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, result)
		}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Action payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned request without executing")
	return cmd
}
