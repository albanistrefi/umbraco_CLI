package commands

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
	"umbraco-cli/internal/validate"
)

const (
	dataTypeLegacyCollectionPath = "/data-type"
	dataTypeLegacyRootPath       = "/data-type/root"
	dataTypeLegacySearchPath     = "/data-type/search"
	dataTypeFilterPath           = "/filter/data-type"
	dataTypeItemSearchPath       = "/item/data-type/search"
	dataTypeTreeRootPath         = "/tree/data-type/root"
)

type dataTypeRequestCandidate struct {
	path string
	opts api.RequestOptions
}

func RegisterDatatype(root *cobra.Command, deps Dependencies) {
	datatype := &cobra.Command{Use: "datatype", Short: "Data type operations"}
	datatype.AddCommand(datatypeGet(deps))
	datatype.AddCommand(datatypeList(deps))
	datatype.AddCommand(datatypeRoot(deps))
	datatype.AddCommand(datatypeSearch(deps))
	datatype.AddCommand(datatypeIsUsed(deps))
	datatype.AddCommand(datatypeCreate(deps))
	datatype.AddCommand(datatypeUpdate(deps))
	datatype.AddCommand(datatypeExtensions(deps))
	datatype.AddCommand(datatypeAddExtension(deps))
	datatype.AddCommand(datatypeRemoveExtension(deps))
	datatype.AddCommand(datatypeAddValue(deps))
	datatype.AddCommand(datatypeRemoveValue(deps))
	datatype.AddCommand(datatypeDelete(deps))
	root.AddCommand(datatype)
}

func datatypeGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get <id>", Short: "Get data type by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func datatypeList(deps Dependencies) *cobra.Command {
	var fields string
	var paramsRaw string
	var skip int
	var take int

	cmd := &cobra.Command{Use: "list", Short: "List data types", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(fields) != "" {
			if cmd.Flags().Changed("params") || cmd.Flags().Changed("skip") || cmd.Flags().Changed("take") {
				return fmt.Errorf("--fields cannot be combined with --params, --skip, or --take on datatype list")
			}

			result, err := deps.Client.Get(context.Background(), dataTypeLegacyCollectionPath, api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		}

		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			params = map[string]any{}
			if skip >= 0 {
				params["skip"] = skip
			}
			if take > 0 {
				params["take"] = take
			}
		}

		result, err := datatypeGetWithFallback(context.Background(), deps.Client,
			dataTypeRequestCandidate{path: dataTypeFilterPath, opts: api.RequestOptions{Params: params}},
			dataTypeRequestCandidate{path: dataTypeTreeRootPath, opts: api.RequestOptions{Params: params}},
			dataTypeRequestCandidate{path: dataTypeLegacyCollectionPath, opts: api.RequestOptions{}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	cmd.Flags().IntVar(&skip, "skip", 0, "Pagination offset")
	cmd.Flags().IntVar(&take, "take", 100, "Pagination page size")
	return cmd
}

func datatypeRoot(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var skip int
	var take int
	var triage readTriageOptions
	cmd := &cobra.Command{Use: "root", Short: "Get root data types", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			params = map[string]any{}
			if skip >= 0 {
				params["skip"] = skip
			}
			if take > 0 {
				params["take"] = take
			}
		}

		result, err := datatypeGetWithFallback(context.Background(), deps.Client,
			dataTypeRequestCandidate{path: dataTypeTreeRootPath, opts: api.RequestOptions{Params: params}},
			dataTypeRequestCandidate{path: dataTypeLegacyRootPath, opts: api.RequestOptions{}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyReadTriage(result, triage))
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	cmd.Flags().IntVar(&skip, "skip", 0, "Pagination offset")
	cmd.Flags().IntVar(&take, "take", 100, "Pagination page size")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func datatypeSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	var editorAlias string
	var skip int
	var take int
	cmd := &cobra.Command{Use: "search", Short: "Search data types", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			if strings.TrimSpace(query) == "" && strings.TrimSpace(editorAlias) == "" {
				return fmt.Errorf("datatype search requires --params, --query, or --editor-alias")
			}
			params = map[string]any{}
			if strings.TrimSpace(query) != "" {
				params["query"] = query
			} else if strings.TrimSpace(editorAlias) != "" {
				params["filter"] = editorAlias
			}
			if skip >= 0 {
				params["skip"] = skip
			}
			if take > 0 {
				params["take"] = take
			}
		} else if strings.TrimSpace(editorAlias) != "" {
			if _, exists := params["editorAlias"]; exists {
				return fmt.Errorf("--editor-alias cannot be combined with --params containing editorAlias")
			}
			params = cloneParams(params)
		}

		searchParams := cloneParams(params)
		filterParams := cloneParams(params)
		if queryValue, ok := searchParams["query"]; ok {
			if _, exists := filterParams["filter"]; !exists {
				filterParams["filter"] = queryValue
			}
		}
		if filterValue, ok := filterParams["filter"]; ok && strings.TrimSpace(editorAlias) == "" {
			if _, exists := searchParams["query"]; !exists {
				searchParams["query"] = filterValue
			}
		}

		candidates := []dataTypeRequestCandidate{
			{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
		}
		if _, hasQuery := searchParams["query"]; hasQuery {
			candidates = []dataTypeRequestCandidate{
				{path: dataTypeItemSearchPath, opts: api.RequestOptions{Params: searchParams}},
				{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
				{path: dataTypeLegacySearchPath, opts: api.RequestOptions{Params: searchParams}},
			}
		}

		result, err := datatypeGetWithFallback(context.Background(), deps.Client, candidates...)
		if err != nil {
			return err
		}
		if strings.TrimSpace(editorAlias) != "" {
			result = filterDataTypesByEditorAlias(result, editorAlias)
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().StringVar(&editorAlias, "editor-alias", "", "Filter by property editor alias, e.g. Umbraco.TextBox")
	cmd.Flags().IntVar(&skip, "skip", 0, "Pagination offset")
	cmd.Flags().IntVar(&take, "take", 100, "Pagination page size")
	return cmd
}

func filterDataTypesByEditorAlias(result any, editorAlias string) any {
	normalized := strings.EqualFold
	payload, ok := result.(map[string]any)
	if ok {
		items, ok := payload["items"].([]any)
		if !ok {
			return result
		}
		filtered := filterDataTypeItems(items, editorAlias, normalized)
		next := cloneAnyMap(payload)
		next["items"] = filtered
		next["filteredTotal"] = len(filtered)
		next["editorAlias"] = editorAlias
		return next
	}
	items, ok := result.([]any)
	if !ok {
		return result
	}
	return filterDataTypeItems(items, editorAlias, normalized)
}

func filterDataTypeItems(items []any, editorAlias string, equal func(string, string) bool) []any {
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := entry["editorAlias"].(string); ok && equal(value, editorAlias) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func datatypeIsUsed(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "is-used <id>", Short: "Check whether a data type is in use", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("%s/%s/is-used", dataTypeLegacyCollectionPath, args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func datatypeCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{Use: "create", Short: "Create data type", RunE: func(cmd *cobra.Command, args []string) error {
		if printTemplate {
			return printResult(cmd, deps, schema.Templates["datatype.create"])
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
		result, err := deps.Client.Post(context.Background(), dataTypeLegacyCollectionPath, body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body, "editorAlias"))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func datatypeUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var dryRun bool
	cmd := &cobra.Command{Use: "update <id>", Short: "Update data type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		hasJSON := strings.TrimSpace(jsonPayload) != ""
		hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
		if hasJSON == hasMergeJSON {
			return fmt.Errorf("datatype update requires exactly one of --json or --merge-json")
		}

		if hasMergeJSON {
			patch, err := parsePayload(mergeJSON)
			if err != nil {
				return err
			}

			current, err := fetchDatatypeObject(context.Background(), deps.Client, args[0])
			if err != nil {
				return err
			}

			merged := mergeAliasPayload(current, patch)
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]), merged, api.RequestOptions{DryRun: dryRun, SkipValidation: true})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		}

		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Put(context.Background(), fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into the current data type before update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "delete <id>", Short: "Delete data type", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]), api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeAddValue(deps Dependencies) *cobra.Command {
	var alias string
	var value string
	var dryRun bool

	cmd := &cobra.Command{Use: "add-value <id>", Short: "Append a string value to a datatype array setting", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--alias", alias); err != nil {
			return err
		}
		if err := requireValue("--value", value); err != nil {
			return err
		}
		if err := validate.String(alias); err != nil {
			return err
		}
		if err := validate.String(value); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(context.Background(), deps.Client, args[0], alias, value, dryRun, "add")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&alias, "alias", "", "Datatype array alias to update")
	cmd.Flags().StringVar(&value, "value", "", "String value to append")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeExtensions(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "extensions <id>", Short: "List enabled data type extension aliases", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := fetchDatatypeObject(context.Background(), deps.Client, args[0])
		if err != nil {
			return err
		}

		result := map[string]any{
			"id":         payload["id"],
			"name":       payload["name"],
			"extensions": datatypeStringArrayValue(payload, "extensions"),
		}
		return printResult(cmd, deps, result)
	}}
}

func datatypeRemoveValue(deps Dependencies) *cobra.Command {
	var alias string
	var value string
	var dryRun bool

	cmd := &cobra.Command{Use: "remove-value <id>", Short: "Remove a string value from a datatype array setting", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--alias", alias); err != nil {
			return err
		}
		if err := requireValue("--value", value); err != nil {
			return err
		}
		if err := validate.String(alias); err != nil {
			return err
		}
		if err := validate.String(value); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(context.Background(), deps.Client, args[0], alias, value, dryRun, "remove")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&alias, "alias", "", "Datatype array alias to update")
	cmd.Flags().StringVar(&value, "value", "", "String value to remove")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeAddExtension(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "add-extension <id> <extension-alias>", Short: "Add an extension alias to the datatype extensions array", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.String(args[1]); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(context.Background(), deps.Client, args[0], "extensions", args[1], dryRun, "add")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeRemoveExtension(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "remove-extension <id> <extension-alias>", Short: "Remove an extension alias from the datatype extensions array", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.String(args[1]); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(context.Background(), deps.Client, args[0], "extensions", args[1], dryRun, "remove")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func datatypeGetWithFallback(ctx context.Context, client *api.Client, candidates ...dataTypeRequestCandidate) (any, error) {
	var lastNotFound error

	for _, candidate := range candidates {
		result, err := client.Get(ctx, candidate.path, candidate.opts)
		if err == nil {
			return result, nil
		}

		apiErr, ok := err.(*api.APIError)
		if ok && apiErr.StatusCode == http.StatusNotFound {
			lastNotFound = err
			continue
		}

		return nil, err
	}

	if lastNotFound != nil {
		return nil, lastNotFound
	}

	return nil, fmt.Errorf("no datatype endpoint candidates were configured")
}

func cloneParams(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
