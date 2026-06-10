package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

func RegisterTemplate(root *cobra.Command, deps Dependencies) {
	template := &cobra.Command{Use: "template", Short: "Template operations"}
	template.AddCommand(templateGet(deps))
	template.AddCommand(templateRoot(deps))
	template.AddCommand(templateSearch(deps))
	template.AddCommand(templateCreate(deps))
	template.AddCommand(templateUpdate(deps))
	template.AddCommand(templateDelete(deps))
	root.AddCommand(template)
}

func templateGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get template by ID",
		Path:  func(args []string) string { return api.JoinPath("/template/%s", args[0]) },
	})
}

func templateRoot(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "root",
		Short: "Get root templates (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/template/root", opts: api.RequestOptions{Params: params}},
				{path: "/template/root", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func templateSearch(deps Dependencies) *cobra.Command {
	return searchCommand(deps, searchSpec{
		Use:   "search",
		Short: "Search templates",
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/item/template/search", opts: api.RequestOptions{Params: params}},
				{path: "/template/search", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func templateCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{Use: "create", Short: "Create template", RunE: func(cmd *cobra.Command, args []string) error {
		if printTemplate {
			return printResult(cmd, deps, schema.Templates["template.create"])
		}
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		if _, err := ensurePayloadID(body); err != nil {
			return err
		}
		result, err := deps.Client.Post(cmd.Context(), "/template", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func templateUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update template",
		Path:  func(args []string) string { return api.JoinPath("/template/%s", args[0]) },
	})
}

func templateDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, "delete <id>", "Permanently delete a template", func(args []string) string {
		return api.JoinPath("/template/%s", args[0])
	})
}
