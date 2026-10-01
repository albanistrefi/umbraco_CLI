package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

func RegisterServer(root *cobra.Command, deps cmdkit.Dependencies) {
	server := &cobra.Command{Use: "server", Short: "Server information and diagnostics"}
	server.AddCommand(readOnlyEndpoint(deps, "status", "Get server status", "/server/status"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "info", "Get server info", "/server/information", "/server/info"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "config", "Get server config", "/server/configuration", "/server/config"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "troubleshoot", "Run troubleshooting checks", "/server/troubleshooting", "/server/troubleshoot"))
	server.AddCommand(readOnlyEndpoint(deps, "upgrade-check", "Check upgrade readiness", "/server/upgrade-check"))
	root.AddCommand(server)
}

func readOnlyEndpoint(deps cmdkit.Dependencies, use string, short string, path string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), path, api.RequestOptions{})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func readOnlyEndpointWithFallback(deps cmdkit.Dependencies, use string, short string, paths ...string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		candidates := make([]cmdkit.GetRequestCandidate, 0, len(paths))
		for _, path := range paths {
			candidates = append(candidates, cmdkit.GetRequestCandidate{Path: path, Opts: api.RequestOptions{}})
		}

		result, err := cmdkit.GetWithFallback(cmd.Context(), deps.Client, candidates...)
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}
