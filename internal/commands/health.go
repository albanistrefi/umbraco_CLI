package commands

import (
	"context"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterHealth(root *cobra.Command, deps Dependencies) {
	health := &cobra.Command{Use: "health", Short: "Health check operations"}
	health.AddCommand(healthGroups(deps))
	health.AddCommand(healthGroup(deps))
	health.AddCommand(healthRun(deps))
	health.AddCommand(healthAction(deps))
	root.AddCommand(health)
}

func healthGroups(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "groups", Short: "List health check groups", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), "/health-check-group", api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func healthGroup(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "group <name>", Short: "Get health check group details", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), api.JoinPath("/health-check-group/%s", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func healthRun(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "run <group-name>", Short: "Run health checks for group", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), api.JoinPath("/health-check-group/%s/run", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func healthAction(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "action <action-id>", Short: "Execute a health check action", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body, err := optionalBody(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), api.JoinPath("/health-check/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Action payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned request without executing")
	return cmd
}
