package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterDoctype(root *cobra.Command, deps Dependencies) {
	doctype := &cobra.Command{Use: "doctype", Short: "Document type schema operations"}
	doctype.AddCommand(doctypeGet(deps))
	doctype.AddCommand(doctypeList(deps))
	doctype.AddCommand(doctypeRoot(deps))
	doctype.AddCommand(doctypeChildren(deps))
	doctype.AddCommand(doctypeSearch(deps))
	doctype.AddCommand(doctypeCreate(deps))
	doctype.AddCommand(doctypeUpdate(deps))
	doctype.AddCommand(doctypeCopy(deps))
	doctype.AddCommand(doctypeMove(deps))
	doctype.AddCommand(doctypeDelete(deps))
	root.AddCommand(doctype)
}

func doctypeGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get <id>", Short: "Get document type by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/document-type/%s", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func doctypeList(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "list", Short: "List document types", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), "/document-type", api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func doctypeRoot(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "root", Short: "Get root document types", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/tree/document-type/root", opts: api.RequestOptions{}},
			getRequestCandidate{path: "/document-type/root", opts: api.RequestOptions{}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func doctypeChildren(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "children <id>", Short: "Get child document types", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{
				path: "/tree/document-type/children",
				opts: api.RequestOptions{Params: map[string]any{"parentId": args[0]}},
			},
			getRequestCandidate{
				path: fmt.Sprintf("/document-type/%s/children", args[0]),
				opts: api.RequestOptions{},
			},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func doctypeSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	cmd := &cobra.Command{Use: "search", Short: "Search document types", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			if query == "" {
				return fmt.Errorf("doctype search requires either --params or --query")
			}
			params = map[string]any{"query": query}
		}
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/item/document-type/search", opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: "/document-type/search", opts: api.RequestOptions{Params: params}},
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

func doctypeCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "create", Short: "Create document type", RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/document-type", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func doctypeUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var dryRun bool
	cmd := &cobra.Command{Use: "update <id>", Short: "Update document type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		hasJSON := strings.TrimSpace(jsonPayload) != ""
		hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
		if hasJSON == hasMergeJSON {
			return fmt.Errorf("doctype update requires exactly one of --json or --merge-json")
		}

		if hasMergeJSON {
			patch, err := parsePayload(mergeJSON)
			if err != nil {
				return err
			}

			current, err := fetchDoctypeObject(context.Background(), deps.Client, args[0])
			if err != nil {
				return err
			}

			merged := mergeDatatypePayload(current, patch)
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document-type/%s", args[0]), merged, api.RequestOptions{DryRun: dryRun, SkipValidation: true})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		}

		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document-type/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into the current document type before update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func doctypeCopy(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{Use: "copy <id>", Short: "Copy document type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if err := requireValue("--to", to); err != nil {
				return err
			}
			body = map[string]any{"target": map[string]any{"id": to}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document-type/%s/copy", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Copy payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func doctypeMove(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{Use: "move <id>", Short: "Move document type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if err := requireValue("--to", to); err != nil {
				return err
			}
			body = map[string]any{"target": map[string]any{"id": to}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document-type/%s/move", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Move payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func doctypeDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "delete <id>", Short: "Delete document type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/document-type/%s", args[0]), api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}
