package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
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
	var fields string
	cmd := &cobra.Command{Use: "get <id>", Short: "Get template by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/template/%s", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func templateRoot(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "root", Short: "Get root templates", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/tree/template/root", opts: api.RequestOptions{}},
			getRequestCandidate{path: "/template/root", opts: api.RequestOptions{}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func templateSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	cmd := &cobra.Command{Use: "search", Short: "Search templates", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			if query == "" {
				return fmt.Errorf("template search requires either --params or --query")
			}
			params = map[string]any{"query": query}
		}
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/item/template/search", opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: "/template/search", opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	return cmd
}

func templateCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "create", Short: "Create template", RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/template", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func templateUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "update <id>", Short: "Update template", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/template/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func templateDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "delete <id>", Short: "Delete template", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/template/%s", args[0]), api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}
