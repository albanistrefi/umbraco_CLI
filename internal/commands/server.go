package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterServer(root *cobra.Command, deps Dependencies) {
	server := &cobra.Command{Use: "server", Short: "Server information and diagnostics"}
	server.AddCommand(readOnlyEndpoint(deps, "status", "Get server status", "/server/status"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "info", "Get server info", "/server/information", "/server/info"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "config", "Get server config", "/server/configuration", "/server/config"))
	server.AddCommand(readOnlyEndpointWithFallback(deps, "troubleshoot", "Run troubleshooting checks", "/server/troubleshooting", "/server/troubleshoot"))
	server.AddCommand(readOnlyEndpoint(deps, "upgrade-check", "Check upgrade readiness", "/server/upgrade-check"))
	root.AddCommand(server)
}

func readOnlyEndpoint(deps Dependencies, use string, short string, path string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), path, api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func readOnlyEndpointWithFallback(deps Dependencies, use string, short string, paths ...string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		candidates := make([]getRequestCandidate, 0, len(paths))
		for _, path := range paths {
			candidates = append(candidates, getRequestCandidate{path: path, opts: api.RequestOptions{}})
		}

		result, err := getWithFallback(cmd.Context(), deps.Client, candidates...)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}
