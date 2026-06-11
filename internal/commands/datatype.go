package commands

import (
	"context"
	"fmt"
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
	datatype.AddCommand(datatypeBlock(deps))
	datatype.AddCommand(datatypeDelete(deps))
	root.AddCommand(datatype)
}

func datatypeGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get data type by ID",
		Path:  func(args []string) string { return api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]) },
	})
}

func datatypeList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List data types (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: dataTypeFilterPath, opts: api.RequestOptions{Params: params}},
				{path: dataTypeTreeRootPath, opts: api.RequestOptions{Params: params}},
				{path: dataTypeLegacyCollectionPath, opts: api.RequestOptions{}},
			}
		},
	})
}

func datatypeRoot(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "root",
		Short: "Get root data types (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: dataTypeTreeRootPath, opts: api.RequestOptions{Params: params}},
				{path: dataTypeLegacyRootPath, opts: api.RequestOptions{}},
			}
		},
	})
}

func datatypeSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	var editorAlias string
	var skip int
	var take int
	cmd := &cobra.Command{Use: "search", Short: "Search data types", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		userTakeSet := cmd.Flags().Changed("take")
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

		if strings.TrimSpace(editorAlias) != "" {
			userSkip := skip
			userTake := 0
			if userTakeSet {
				userTake = take
			}
			if paramsRaw != "" {
				userSkip = intParam(params, "skip", userSkip)
				userTake = intParam(params, "take", userTake)
			}
			result, err := searchDataTypesByEditorAlias(ctx, deps.Client, params, editorAlias, userSkip, userTake)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
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

		candidates := []getRequestCandidate{
			{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
		}
		if _, hasQuery := searchParams["query"]; hasQuery {
			candidates = []getRequestCandidate{
				{path: dataTypeItemSearchPath, opts: api.RequestOptions{Params: searchParams}},
				{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
				{path: dataTypeLegacySearchPath, opts: api.RequestOptions{Params: searchParams}},
			}
		}

		result, err := getWithFallback(ctx, deps.Client, candidates...)
		if err != nil {
			return err
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

func searchDataTypesByEditorAlias(ctx context.Context, client *api.Client, params map[string]any, editorAlias string, userSkip int, userTake int) (map[string]any, error) {
	const pageSize = 200
	const scanCap = 5000

	baseParams := cloneParams(params)
	delete(baseParams, "skip")
	delete(baseParams, "take")
	delete(baseParams, "editorAlias")
	if _, hasQuery := baseParams["query"]; !hasQuery {
		if _, hasFilter := baseParams["filter"]; !hasFilter {
			baseParams["filter"] = editorAlias
		}
	}

	matches := make([]any, 0)
	total := -1
	scanned := 0
	scanCapReached := false
	var lastPayload map[string]any

	for scanned < scanCap {
		pageParams := cloneParams(baseParams)
		pageParams["skip"] = scanned
		pageParams["take"] = pageSize

		page, err := datatypeSearchPage(ctx, client, pageParams)
		if err != nil {
			return nil, err
		}
		if payload, ok := page.(map[string]any); ok {
			lastPayload = payload
			if pageTotal, ok := intValue(payload["total"]); ok {
				total = pageTotal
			}
		}

		items := resultItems(page)
		matches = append(matches, filterDataTypeItems(items, editorAlias, strings.EqualFold)...)

		scanned += pageSize
		if userTake > 0 && len(matches) >= userSkip+userTake {
			break
		}
		if len(items) == 0 {
			break
		}
		if total >= 0 && scanned >= total {
			break
		}
	}
	if scanned >= scanCap && (total < 0 || scanned < total) {
		scanCapReached = true
	}

	window := applyItemWindow(matches, userSkip, userTake)
	result := map[string]any{
		"items":         window,
		"total":         total,
		"filteredTotal": len(matches),
		"editorAlias":   editorAlias,
		"scanned":       minInt(scanned, scanCap),
	}
	if lastPayload != nil {
		for _, key := range []string{"links", "_links"} {
			if value, ok := lastPayload[key]; ok {
				result[key] = value
			}
		}
	}
	if scanCapReached {
		result["scanCapReached"] = true
		result["warning"] = fmt.Sprintf("stopped after scanning %d data types; narrow --query or increase the CLI scan cap in code if needed", scanCap)
	}
	return result, nil
}

func datatypeSearchPage(ctx context.Context, client *api.Client, params map[string]any) (any, error) {
	searchParams := cloneParams(params)
	filterParams := cloneParams(params)
	if queryValue, ok := searchParams["query"]; ok {
		if _, exists := filterParams["filter"]; !exists {
			filterParams["filter"] = queryValue
		}
	}
	if _, hasQuery := searchParams["query"]; hasQuery {
		return getWithFallback(ctx, client,
			getRequestCandidate{path: dataTypeItemSearchPath, opts: api.RequestOptions{Params: searchParams}},
			getRequestCandidate{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
			getRequestCandidate{path: dataTypeLegacySearchPath, opts: api.RequestOptions{Params: searchParams}},
		)
	}
	return getWithFallback(ctx, client,
		getRequestCandidate{path: dataTypeFilterPath, opts: api.RequestOptions{Params: filterParams}},
	)
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

func applyItemWindow(items []any, skip int, take int) []any {
	if skip < 0 {
		skip = 0
	}
	if skip >= len(items) {
		return []any{}
	}
	end := len(items)
	if take > 0 && skip+take < end {
		end = skip + take
	}
	return items[skip:end]
}

func intParam(params map[string]any, key string, fallback int) int {
	if value, ok := intValue(params[key]); ok {
		return value
	}
	return fallback
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func datatypeIsUsed(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "is-used <id>", Short: "Check whether a data type is in use", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), api.JoinPath(dataTypeLegacyCollectionPath+"/%s/is-used", args[0]), api.RequestOptions{})
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
		result, err := deps.Client.Post(cmd.Context(), dataTypeLegacyCollectionPath, body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body, "editorAlias"))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func datatypeUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update data type",
		Long: `Updates a data type with the uniform CLI update contract:

  --json        full replacement; the server resets any field not mentioned
                (including editorUiAlias, items, multiple)
  --merge-json  fetches the current data type, deep-merges the patch, and
                PUTs the result; fields not mentioned are preserved

Before v0.4.0 --json silently behaved like --merge-json on this resource.
Pass --merge-json for partial edits.`,
		Path: func(args []string) string { return api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]) },
	})
}

func datatypeDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a data type",
		Path: func(args []string) string {
			return api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0])
		},
	})
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

		result, err := mutateDatatypeStringArray(cmd.Context(), deps.Client, args[0], alias, value, dryRun, "add")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&alias, "alias", "", "Datatype array alias to update")
	cmd.Flags().StringVar(&value, "value", "", "String value to append")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func datatypeExtensions(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "extensions <id>", Short: "List enabled data type extension aliases", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := fetchDatatypeObject(cmd.Context(), deps.Client, args[0])
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

		result, err := mutateDatatypeStringArray(cmd.Context(), deps.Client, args[0], alias, value, dryRun, "remove")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&alias, "alias", "", "Datatype array alias to update")
	cmd.Flags().StringVar(&value, "value", "", "String value to remove")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func datatypeAddExtension(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "add-extension <id> <extension-alias>", Short: "Add an extension alias to the datatype extensions array", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.String(args[1]); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(cmd.Context(), deps.Client, args[0], "extensions", args[1], dryRun, "add")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func datatypeRemoveExtension(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "remove-extension <id> <extension-alias>", Short: "Remove an extension alias from the datatype extensions array", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.String(args[1]); err != nil {
			return err
		}

		result, err := mutateDatatypeStringArray(cmd.Context(), deps.Client, args[0], "extensions", args[1], dryRun, "remove")
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	addDryRunFlag(cmd, &dryRun)
	return cmd
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
