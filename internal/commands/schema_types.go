package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// schemaTypeSpec parameterizes the shared command surface of the
// folder-organized schema type resources (media-type, member-type). The
// document-type group predates this builder and keeps its richer bespoke
// surface, but shares the tree/folder helpers below.
type schemaTypeSpec struct {
	Use      string // command group name, e.g. "mediatype"
	Resource string // API resource segment, e.g. "media-type"
	Display  string // human name used in help text, e.g. "media type"
	// UpdateStripFields lists response-only keys the update request model
	// rejects (additionalProperties: false); they are stripped from the
	// merged body before the PUT.
	UpdateStripFields []string
}

func RegisterMediaType(root *cobra.Command, deps Dependencies) {
	registerSchemaTypeGroup(root, deps, schemaTypeSpec{
		Use: "mediatype", Resource: "media-type", Display: "media type",
		UpdateStripFields: []string{"id", "isDeletable", "aliasCanBeChanged"},
	})
}

func RegisterMemberType(root *cobra.Command, deps Dependencies) {
	registerSchemaTypeGroup(root, deps, schemaTypeSpec{
		Use: "membertype", Resource: "member-type", Display: "member type",
		UpdateStripFields: []string{"id"},
	})
}

func registerSchemaTypeGroup(root *cobra.Command, deps Dependencies, spec schemaTypeSpec) {
	group := &cobra.Command{Use: spec.Use, Short: fmt.Sprintf("%s operations", strings.ToUpper(spec.Display[:1])+spec.Display[1:])}
	group.AddCommand(schemaTypeList(deps, spec))
	group.AddCommand(schemaTypeGet(deps, spec))
	group.AddCommand(collectionCommand(deps, collectionSpec{
		Use:   "children <id>",
		Short: fmt.Sprintf("Get child %ss of a folder (paginated; --skip/--take/--all)", spec.Display),
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/" + spec.Resource + "/children", opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
				{path: api.JoinPath("/"+spec.Resource+"/%s/children", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
	}))
	group.AddCommand(searchCommand(deps, searchSpec{
		Use:   "search",
		Short: fmt.Sprintf("Search %ss", spec.Display),
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/item/" + spec.Resource + "/search", opts: api.RequestOptions{Params: params}},
			}
		},
	}))
	group.AddCommand(createCommand(deps, createSpec{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s", spec.Display),
		Path:  "/" + spec.Resource,
	}))
	group.AddCommand(updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: fmt.Sprintf("Update a %s (--json replaces, --merge-json merges)", spec.Display),
		Path: func(args []string) string {
			return api.JoinPath("/"+spec.Resource+"/%s", args[0])
		},
		NormalizeMerged: stripFields(spec.UpdateStripFields...),
	}))
	group.AddCommand(deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: fmt.Sprintf("Delete a %s", spec.Display),
		Path: func(args []string) string {
			return api.JoinPath("/"+spec.Resource+"/%s", args[0])
		},
	}))
	group.AddCommand(getCommand(deps, getSpec{
		Use:   "export <id>",
		Short: fmt.Sprintf("Export a %s as a .udt document", spec.Display),
		Path: func(args []string) string {
			return api.JoinPath("/"+spec.Resource+"/%s/export", args[0])
		},
	}))
	root.AddCommand(group)
}

func schemaTypeList(deps Dependencies, spec schemaTypeSpec) *cobra.Command {
	var fields string
	var paramsRaw string
	var skip, take int
	var all bool
	var recursive bool
	var typesOnly bool
	var excludeFolders bool
	var triage readTriageOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %ss (paginated; --skip/--take/--all)", spec.Display),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			params = applyPaginationParams(params, skip, take)
			candidates := []getRequestCandidate{
				{path: "/tree/" + spec.Resource + "/root", opts: api.RequestOptions{Params: params, Fields: fields}},
			}

			ctx := cmd.Context()
			filterFolders := typesOnly || excludeFolders
			var result any
			if recursive {
				rootLimit := triage.FirstN
				if filterFolders {
					rootLimit = 0
				}
				result, err = getAllPagesWithFallback(ctx, deps.Client, take, skip, rootLimit, candidates...)
			} else if all {
				result, err = getAllPagesWithFallback(ctx, deps.Client, take, skip, triage.FirstN, candidates...)
			} else {
				result, err = getWithFallback(ctx, deps.Client, candidates...)
			}
			if err != nil {
				return err
			}

			if recursive {
				items, err := flattenSchemaTypeTree(ctx, deps.Client, spec.Resource, resultItems(result), take, filterFolders, triage.FirstN)
				if err != nil {
					return err
				}
				result = map[string]any{
					"items":     items,
					"total":     len(items),
					"recursive": true,
					"typesOnly": filterFolders,
				}
			} else if filterFolders {
				result = filterSchemaTypeFolders(result)
			}

			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	addFieldsFlag(cmd, &fields)
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	addPaginationFlags(cmd, &skip, &take)
	addAutoPaginationFlag(cmd, &all)
	addReadTriageFlags(cmd, &triage)
	cmd.Flags().BoolVar(&recursive, "recursive", false, fmt.Sprintf("Walk %s folders recursively", spec.Display))
	cmd.Flags().BoolVar(&typesOnly, "types-only", false, fmt.Sprintf("Return %ss only, excluding folders", spec.Display))
	cmd.Flags().BoolVar(&excludeFolders, "exclude-folders", false, "Alias for --types-only")
	return cmd
}

func schemaTypeGet(deps Dependencies, spec schemaTypeSpec) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: fmt.Sprintf("Get %s by ID", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/"+spec.Resource+"/%s", args[0]), api.RequestOptions{Fields: fields})
			if err != nil {
				if isSchemaTypeFolderID(cmd.Context(), deps.Client, spec.Resource, args[0]) {
					return fmt.Errorf("%s id %s is a folder, not a %s; use `umbraco %s children %s` or `umbraco %s list --recursive --types-only`", spec.Display, args[0], spec.Display, spec.Use, args[0], spec.Use)
				}
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

// The helpers below are shared by document-type, media-type, and member-type:
// the three schema type trees expose the same folder semantics.

func flattenSchemaTypeTree(ctx context.Context, client *api.Client, resource string, items []any, pageSize int, excludeFolders bool, limit int) ([]any, error) {
	flattened := make([]any, 0, len(items))
	seenFolders := map[string]struct{}{}
	if err := appendSchemaTypeTreeItems(ctx, client, resource, &flattened, items, pageSize, excludeFolders, limit, seenFolders); err != nil {
		return nil, err
	}
	return flattened, nil
}

func appendSchemaTypeTreeItems(ctx context.Context, client *api.Client, resource string, flattened *[]any, items []any, pageSize int, excludeFolders bool, limit int, seenFolders map[string]struct{}) error {
	for _, item := range items {
		if schemaTypeLimitReached(flattened, limit) {
			return nil
		}
		folder := isSchemaTypeFolderItem(item)
		if !folder || !excludeFolders {
			*flattened = append(*flattened, item)
			if schemaTypeLimitReached(flattened, limit) {
				return nil
			}
		}
		if !folder {
			continue
		}
		id := itemID(item)
		if id == "" {
			continue
		}
		if _, seen := seenFolders[id]; seen {
			continue
		}
		seenFolders[id] = struct{}{}
		childLimit := 0
		if !excludeFolders && limit > 0 {
			childLimit = limit - len(*flattened)
		}
		children, err := fetchSchemaTypeFolderChildren(ctx, client, resource, id, pageSize, childLimit)
		if err != nil {
			return err
		}
		if err := appendSchemaTypeTreeItems(ctx, client, resource, flattened, children, pageSize, excludeFolders, limit, seenFolders); err != nil {
			return err
		}
	}
	return nil
}

func fetchSchemaTypeFolderChildren(ctx context.Context, client *api.Client, resource string, folderID string, pageSize int, limit int) ([]any, error) {
	result, err := getAllPagesWithFallback(ctx, client, pageSize, 0, limit,
		getRequestCandidate{path: "/tree/" + resource + "/children", opts: api.RequestOptions{Params: map[string]any{"parentId": folderID}}},
		getRequestCandidate{path: api.JoinPath("/"+resource+"/%s/children", folderID)},
	)
	if err != nil {
		return nil, err
	}
	return resultItems(result), nil
}

// isSchemaTypeFolderID asks the folder endpoint directly. The earlier
// tree-children probe answered 200 with an empty list for *any* id, so a
// plain 404 (typo, deleted type, wrong environment) was misreported as
// "is a folder"; now only an actual folder record counts.
func isSchemaTypeFolderID(ctx context.Context, client *api.Client, resource string, id string) bool {
	result, err := client.Get(ctx, api.JoinPath("/"+resource+"/folder/%s", id), api.RequestOptions{})
	if err != nil {
		return false
	}
	folder, ok := result.(map[string]any)
	return ok && folder["id"] != nil
}

func isSchemaTypeFolderItem(item any) bool {
	entry, ok := item.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"isFolder", "isContainer"} {
		if value, ok := entry[key].(bool); ok && value {
			return true
		}
	}
	for _, key := range []string{"type", "nodeType", "kind", "entityType"} {
		if value, ok := entry[key].(string); ok && strings.EqualFold(strings.TrimSpace(value), "folder") {
			return true
		}
	}
	alias, hasAlias := entry["alias"].(string)
	return !hasAlias || strings.TrimSpace(alias) == ""
}

func filterSchemaTypeFolders(result any) any {
	items := resultItems(result)
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		if !isSchemaTypeFolderItem(item) {
			filtered = append(filtered, item)
		}
	}
	if payload, ok := result.(map[string]any); ok {
		next := cloneAnyMap(payload)
		next["items"] = filtered
		next["total"] = len(filtered)
		next["typesOnly"] = true
		return next
	}
	return filtered
}

func schemaTypeLimitReached(items *[]any, limit int) bool {
	return limit > 0 && len(*items) >= limit
}
