package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterRedirect(root *cobra.Command, deps Dependencies) {
	redirect := &cobra.Command{Use: "redirect", Short: "Redirect URL management (tracked 301s from renamed/moved documents)"}
	redirect.AddCommand(redirectList(deps))
	redirect.AddCommand(redirectGet(deps))
	redirect.AddCommand(redirectDelete(deps))
	redirect.AddCommand(readOnlyEndpoint(deps, "status", "Get redirect tracking status (enabled/disabled)", "/redirect-management/status"))
	redirect.AddCommand(redirectSetStatus(deps, "enable", "Enabled", "Enable redirect URL tracking"))
	redirect.AddCommand(redirectSetStatus(deps, "disable", "Disabled", "Disable redirect URL tracking"))
	root.AddCommand(redirect)
}

func redirectList(deps Dependencies) *cobra.Command {
	var fields string
	var filter string
	var skip, take int
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked redirects (paginated; use --filter for URL substring search)",
		Long:  "GET /redirect-management. Lists the redirects Umbraco recorded when published documents were renamed or moved. --filter matches against the original URL.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if strings.TrimSpace(filter) != "" {
				params["filter"] = filter
			}
			params = applyPaginationParams(params, skip, take)
			result, err := deps.Client.Get(cmd.Context(), "/redirect-management", api.RequestOptions{Fields: fields, Params: params})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	addFieldsFlag(cmd, &fields)
	cmd.Flags().StringVar(&filter, "filter", "", "Substring filter against the original redirect URL")
	addPaginationFlags(cmd, &skip, &take)
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func redirectGet(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "get <document-id>",
		Short: "List redirects recorded for one document",
		Long:  "GET /redirect-management/{id}. Returns the redirects pointing at the given document key, paginated.",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: api.JoinPath("/redirect-management/%s", args[0]), Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func redirectDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Delete a tracked redirect",
		Path: func(args []string) string {
			return api.JoinPath("/redirect-management/%s", args[0])
		},
	})
}

func redirectSetStatus(deps Dependencies, use string, status string, short string) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  "POST /redirect-management/status?status=" + status + ". Toggles redirect URL tracking site-wide; the inverse command reverses it.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), "/redirect-management/status", nil, api.RequestOptions{
				DryRun: dryRun,
				Params: map[string]any{"status": status},
			})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, strings.ToLower(status), result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
