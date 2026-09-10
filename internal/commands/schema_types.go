package commands

import (
	"context"
	"fmt"
	"net/http"
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
		Use:   "get <id-or-alias>",
		Short: fmt.Sprintf("Get %s by ID (or by exact alias)", spec.Display),
		Long:  fmt.Sprintf("Fetches a %s by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404.", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveSchemaTypeID(cmd.Context(), deps.Client, spec.Resource, spec.Use, spec.Display, args[0])
			if err != nil {
				return err
			}
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/"+spec.Resource+"/%s", id), api.RequestOptions{Fields: fields})
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

// resolveSchemaTypeID accepts a GUID as-is; anything else is treated as an
// alias and resolved via the item search plus a per-candidate fetch (the
// item model carries no alias). Field report: a non-GUID used to fall
// through to GET /<resource>/<alias>, whose 404 hint blamed the Umbraco
// version rather than the argument.
func resolveSchemaTypeID(ctx context.Context, client *api.Client, resource string, use string, display string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if isUUIDLike(value) {
		return value, nil
	}
	if value == "" || strings.ContainsAny(value, "/?#%") {
		return "", fmt.Errorf("%q is not a GUID or alias; use 'umbraco %s get <guid>' or 'umbraco %s search --query <text>'", value, use, use)
	}
	// The item search matches names, not aliases, so query with the first
	// camelCase word of the alias ("blogCategories" → "blog") and check the
	// alias on the candidates' full models. A renamed type whose alias no
	// longer resembles its name misses that search, so an exhaustive walk
	// of the type tree is the fallback.
	result, err := client.Get(ctx, "/item/"+resource+"/search", api.RequestOptions{Params: map[string]any{"query": aliasSearchTerm(value), "skip": 0, "take": 100}})
	if err != nil {
		return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
	}
	ids := []string{}
	for _, item := range resultItems(result) {
		if id := itemID(item); id != "" {
			ids = append(ids, id)
		}
	}
	matches, err := matchSchemaTypeAlias(ctx, client, resource, ids, value)
	if err != nil {
		return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
	}
	if len(matches) == 0 {
		rootResult, err := client.Get(ctx, "/tree/"+resource+"/root", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 500}})
		if err != nil {
			return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
		}
		all, err := flattenSchemaTypeTree(ctx, client, resource, resultItems(rootResult), 500, true, 0)
		if err != nil {
			return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
		}
		allIDs := []string{}
		for _, item := range all {
			if id := itemID(item); id != "" {
				allIDs = append(allIDs, id)
			}
		}
		matches, err = matchSchemaTypeAlias(ctx, client, resource, allIDs, value)
		if err != nil {
			return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("%q is not a GUID and no %s has that alias; use 'umbraco %s get <guid>' or 'umbraco %s search --query %s' to find it", value, display, use, use, value)
	default:
		return "", fmt.Errorf("alias %q matched %d %ss (%s); pass the GUID", value, len(matches), display, strings.Join(matches, ", "))
	}
}

// aliasSearchTerm returns the leading camelCase word of an alias, which is
// what the name-based item search can match.
func aliasSearchTerm(alias string) string {
	for i, r := range alias {
		if i > 0 && (r >= 'A' && r <= 'Z' || r == '_' || r == '-' || r == ' ') {
			return alias[:i]
		}
	}
	return alias
}

// matchSchemaTypeAlias returns the ids among candidates whose full model
// carries the alias (exact, case-insensitive).
func matchSchemaTypeAlias(ctx context.Context, client *api.Client, resource string, ids []string, alias string) ([]string, error) {
	details, err := fetchSchemaTypeBatch(ctx, client, resource, ids)
	if err != nil {
		return nil, err
	}
	matches := []string{}
	for _, detail := range details {
		if got, _ := detail["alias"].(string); strings.EqualFold(got, alias) {
			if id, _ := detail["id"].(string); id != "" {
				matches = append(matches, id)
			}
		}
	}
	return matches, nil
}

// fetchSchemaTypeBatch loads full models for the ids, via the batch route
// when the server has it (ids as repeated query values) and one GET per id
// otherwise. A 404 for one id means it vanished between calls and is
// skipped; any other API failure is returned, never mistaken for "absent".
func fetchSchemaTypeBatch(ctx context.Context, client *api.Client, resource string, ids []string) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// buildURL repeats []any values as separate query parameters.
	idParams := make([]any, 0, len(ids))
	for _, id := range ids {
		idParams = append(idParams, id)
	}
	if result, err := client.Get(ctx, "/"+resource+"/batch", api.RequestOptions{Params: map[string]any{"id": idParams}}); err == nil {
		out := []map[string]any{}
		for _, item := range resultItems(result) {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	} else if !isAPIStatus(err, http.StatusNotFound) {
		return nil, err
	}
	out := []map[string]any{}
	for _, id := range ids {
		detail, err := fetchObject(ctx, client, api.JoinPath("/"+resource+"/%s", id), api.RequestOptions{})
		if err != nil {
			if isAPIStatus(err, http.StatusNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, detail)
	}
	return out, nil
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
