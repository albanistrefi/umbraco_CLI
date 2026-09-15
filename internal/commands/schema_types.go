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
		Use: "children <id>",
		Enrich: func(ctx context.Context, result any) (any, error) {
			return enrichSchemaTypeAliases(ctx, deps.Client, spec.Resource, result)
		},
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
		Use: "search",
		Enrich: func(ctx context.Context, result any) (any, error) {
			return enrichSchemaTypeAliases(ctx, deps.Client, spec.Resource, result)
		},
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
	group.AddCommand(schemaTypeCreateFolder(deps, spec.Use, spec.Resource, spec.Display, false))
	group.AddCommand(schemaTypeDeleteFolder(deps, spec.Use, spec.Resource, spec.Display))
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
			if result, err = enrichSchemaTypeAliases(ctx, deps.Client, spec.Resource, result); err != nil {
				return err
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

// schemaTypeCreateFolder builds "<group> create-folder": POST /<resource>/folder.
// Folders are how the type trees are organized, yet the Management API has
// no folder concept in the type payloads beyond parent — so this is the
// only way to create the parent a 'create --json {"parent":{"id":…}}' needs.
// Like every create, a --json payload is accepted; --name/--parent/--id fill
// fields the payload leaves out.
func schemaTypeCreateFolder(deps Dependencies, use string, resource string, display string, hasMove bool) *cobra.Command {
	var jsonPayload string
	var name string
	var parent string
	var id string
	var dryRun bool
	placement := fmt.Sprintf("Put a type inside it with 'umbraco %s create --json '{..., \"parent\": {\"id\": \"<folder id>\"}}''", use)
	if hasMove {
		placement += fmt.Sprintf(" or 'umbraco %s move <id> --to <folder id>'", use)
	}
	cmd := &cobra.Command{
		Use:   "create-folder",
		Short: fmt.Sprintf("Create a %s folder (optionally inside another folder)", display),
		Long: fmt.Sprintf("POST /%s/folder. Creates a folder in the %s tree; --parent nests it inside an existing folder. "+
			"Pass the folder as --json '{\"name\": …, \"parent\": {\"id\": …}}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). "+
			"%s. After the create the folder is read back, so the result is the persisted record.", resource, display, placement),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := parseJSONObject(jsonPayload, "--json")
				if err != nil {
					return err
				}
				body = parsed
			}
			if strings.TrimSpace(name) != "" {
				if _, set := body["name"]; !set {
					body["name"] = strings.TrimSpace(name)
				}
			}
			if strings.TrimSpace(parent) != "" {
				if _, set := body["parent"]; !set {
					body["parent"] = map[string]any{"id": strings.TrimSpace(parent)}
				}
			}
			if strings.TrimSpace(id) != "" {
				if _, set := body["id"]; !set {
					body["id"] = strings.TrimSpace(id)
				}
			}
			folderName, _ := body["name"].(string)
			if strings.TrimSpace(folderName) == "" {
				return fmt.Errorf("create-folder requires a folder name: pass --name or a --json payload with \"name\"")
			}
			if parentRef, set := body["parent"]; set && parentRef != nil {
				parentMap, ok := parentRef.(map[string]any)
				parentID, _ := parentMap["id"].(string)
				if !ok || !isUUIDLike(strings.TrimSpace(parentID)) {
					return fmt.Errorf("parent must be {\"id\": \"<folder GUID>\"}, got %v", parentRef)
				}
			}
			folderID, err := ensurePayloadID(body)
			if err != nil {
				return err
			}
			if !isUUIDLike(folderID) {
				return fmt.Errorf("id must be a GUID, got %q", folderID)
			}
			ctx := cmd.Context()
			result, err := deps.Client.Post(ctx, "/"+resource+"/folder", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			created, err := fetchObject(ctx, deps.Client, api.JoinPath("/"+resource+"/folder/%s", folderID), api.RequestOptions{})
			if err != nil {
				return fmt.Errorf("the server accepted the folder but reading it back failed: %w", err)
			}
			out := map[string]any{"id": folderID, "name": created["name"], "created": true, "parent": nil}
			if parentRef, ok := body["parent"]; ok && parentRef != nil {
				out["parent"] = parentRef
			}
			return printResult(cmd, deps, out)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Folder payload as JSON: {\"id\"?, \"name\", \"parent\"?: {\"id\"}}")
	cmd.Flags().StringVar(&name, "name", "", "Folder name (fills name when --json omits it)")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent folder GUID; omit for a root-level folder")
	cmd.Flags().StringVar(&id, "id", "", "Folder GUID to use (generated when omitted)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// schemaTypeDeleteFolder builds "<group> delete-folder": DELETE /<resource>/folder/{id}.
// The server refuses to delete a folder that still has children.
func schemaTypeDeleteFolder(deps Dependencies, use string, resource string, display string) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete-folder <id>",
		Short: fmt.Sprintf("Delete an empty %s folder", display),
		Path: func(args []string) string {
			return api.JoinPath("/"+resource+"/folder/%s", args[0])
		},
	})
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
	// The item search matches names, not aliases, so query with words of
	// the alias ("sharedSector" → "shared", then "sector": types in a
	// folder often carry the folder as an alias prefix their name lacks)
	// and check the alias on the candidates' full models. A renamed type
	// whose alias no longer resembles its name misses every search, so an
	// exhaustive walk of the type tree is the fallback.
	ids := []string{}
	seen := map[string]struct{}{}
	for _, term := range aliasSearchTerms(value) {
		result, err := client.Get(ctx, "/item/"+resource+"/search", api.RequestOptions{Params: map[string]any{"query": term, "skip": 0, "take": 100}})
		if err != nil {
			return "", fmt.Errorf("%q is not a GUID and the alias lookup failed: %w", value, err)
		}
		for _, item := range resultItems(result) {
			id := itemID(item)
			if id == "" {
				continue
			}
			if _, dup := seen[strings.ToLower(id)]; dup {
				continue
			}
			seen[strings.ToLower(id)] = struct{}{}
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

// aliasSearchTerms returns the words the name-based item search should be
// tried with, in order: the leading camelCase word, the trailing word, and
// the whole alias (each once). "sharedSector" → shared, sector.
func aliasSearchTerms(alias string) []string {
	words := aliasWords(alias)
	candidates := []string{}
	if len(words) > 0 {
		candidates = append(candidates, words[0])
		if len(words) > 1 {
			candidates = append(candidates, words[len(words)-1])
		}
	}
	candidates = append(candidates, alias)
	terms := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, term := range candidates {
		key := strings.ToLower(term)
		if term == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}

// aliasWords splits an alias on camelCase boundaries and separators.
func aliasWords(alias string) []string {
	words := []string{}
	start := 0
	flush := func(end int) {
		if word := strings.Trim(alias[start:end], "_- "); word != "" {
			words = append(words, word)
		}
	}
	for i, r := range alias {
		if i > 0 && (r >= 'A' && r <= 'Z' || r == '_' || r == '-' || r == ' ') {
			flush(i)
			start = i
		}
	}
	flush(len(alias))
	return words
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

// schemaTypeBatchSize caps the ids per batch request. Field report: the
// alias fallback once sent every type in the tree (200+) as one query
// string and the server never answered — the request timed out client-side
// and the alias was reported as unknown.
const schemaTypeBatchSize = 100

// fetchSchemaTypeBatch loads full models for the ids, via the batch route
// when the server has it (ids as repeated query values, at most
// schemaTypeBatchSize per request) and one GET per id otherwise. A 404 for
// one id means it vanished between calls and is skipped; any other API
// failure is returned, never mistaken for "absent".
func fetchSchemaTypeBatch(ctx context.Context, client *api.Client, resource string, ids []string) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > schemaTypeBatchSize {
		out := make([]map[string]any, 0, len(ids))
		for start := 0; start < len(ids); start += schemaTypeBatchSize {
			end := start + schemaTypeBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk, err := fetchSchemaTypeBatch(ctx, client, resource, ids[start:end])
			if err != nil {
				return nil, err
			}
			out = append(out, chunk...)
		}
		return out, nil
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
	// An explicit folder flag is authoritative in both directions: tree
	// items carry isFolder but never an alias, so the alias heuristic below
	// must only run when no flag is present (it used to classify every
	// tree item as a folder, making --types-only return nothing).
	for _, key := range []string{"isFolder", "isContainer"} {
		if value, ok := entry[key].(bool); ok {
			return value
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

// enrichSchemaTypeAliases adds alias (and isElement when missing) to tree
// and search items, which the Management API returns without an alias.
// Non-folder items lacking an alias are loaded through the batch route in
// chunks; folders are left alone. Failures propagate: a silently
// alias-less list would be indistinguishable from a real one.
func enrichSchemaTypeAliases(ctx context.Context, client *api.Client, resource string, result any) (any, error) {
	items := resultItems(result)
	missing := []string{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Only an explicit folder flag excludes an item; search items carry
		// neither isFolder nor alias and must still be enriched.
		if folder, ok := entry["isFolder"].(bool); ok && folder {
			continue
		}
		if _, has := entry["alias"]; !has {
			if id := itemID(entry); id != "" {
				missing = append(missing, id)
			}
		}
	}
	if len(missing) == 0 {
		return result, nil
	}
	details := map[string]map[string]any{}
	batch, err := fetchSchemaTypeBatch(ctx, client, resource, missing)
	if err != nil {
		return nil, fmt.Errorf("resolving aliases for %ss failed: %w", strings.ReplaceAll(resource, "-", " "), err)
	}
	for _, detail := range batch {
		if id, _ := detail["id"].(string); id != "" {
			details[strings.ToLower(id)] = detail
		}
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		detail, found := details[strings.ToLower(itemID(entry))]
		if !found {
			continue
		}
		if _, has := entry["alias"]; !has {
			entry["alias"] = detail["alias"]
		}
		if _, has := entry["isElement"]; !has {
			if isElement, ok := detail["isElement"]; ok {
				entry["isElement"] = isElement
			}
		}
	}
	return result, nil
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
