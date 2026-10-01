package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/schema"
)

func RegisterTemplate(root *cobra.Command, deps cmdkit.Dependencies) {
	template := &cobra.Command{Use: "template", Short: "Template operations"}
	template.AddCommand(templateGet(deps))
	template.AddCommand(templateRoot(deps))
	template.AddCommand(templateSearch(deps))
	template.AddCommand(templateCreate(deps))
	template.AddCommand(templateUpdate(deps))
	template.AddCommand(templateDelete(deps))
	root.AddCommand(template)
}

func templateGet(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:   "get <id>",
		Short: "Get template by ID",
		Path:  func(args []string) string { return api.JoinPath("/template/%s", args[0]) },
	})
}

func templateRoot(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "root",
		Short: "Get root templates (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/tree/template/root", Opts: api.RequestOptions{Params: params}},
				{Path: "/template/root", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func templateSearch(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.SearchCommand(deps, cmdkit.SearchSpec{
		Use:   "search",
		Short: "Search templates",
		Endpoints: func(params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/item/template/search", Opts: api.RequestOptions{Params: params}},
				{Path: "/template/search", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func templateCreate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{Use: "create", Short: "Create template", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if printTemplate {
			return cmdkit.PrintResult(cmd, deps, schema.Templates["template.create"])
		}
		if err := cmdkit.RequireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := cmdkit.ParsePayload(jsonPayload)
		if err != nil {
			return err
		}
		if _, err := cmdkit.EnsurePayloadID(body); err != nil {
			return err
		}
		result, err := deps.Client.Post(cmd.Context(), "/template", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, cmdkit.CreateResult(result, body))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func templateUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update <id>",
		Short: "Update template",
		Path:  func(args []string) string { return api.JoinPath("/template/%s", args[0]) },
	})
}

func templateDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a template",
		Path: func(args []string) string {
			return api.JoinPath("/template/%s", args[0])
		},
	})
}
