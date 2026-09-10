package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/validate"
)

func RegisterDoctype(root *cobra.Command, deps Dependencies) {
	doctype := &cobra.Command{
		Use:     "doctype",
		Aliases: []string{"document-type"},
		Short:   "Document type schema operations",
		Long: `Document type schema operations.

Task → command:
  Read a type by GUID or alias                         doctype get <id-or-alias>
  Find types by name                                   doctype search --query <text>
  Add / reorder properties, add tabs or groups         doctype add-property, reorder-properties, add-container
  Allowed blocks, block order, Block Grid groups       datatype block add|reorder|groups … (blocks belong to the Block List/Grid DATA TYPE, not the document type)
  Which data type does a property use?                 doctype get <id> --fields properties
  Apply a whole schema from Deploy artifacts           deploy apply --uda-dir <dir> --dry-run`,
	}
	doctype.AddCommand(doctypeGet(deps))
	doctype.AddCommand(doctypeList(deps))
	doctype.AddCommand(doctypeRoot(deps))
	doctype.AddCommand(doctypeChildren(deps))
	doctype.AddCommand(doctypeSearch(deps))
	doctype.AddCommand(doctypeAllowedInLibrary(deps))
	doctype.AddCommand(doctypeCreate(deps))
	doctype.AddCommand(doctypeUpdate(deps))
	doctype.AddCommand(doctypeAddProperty(deps))
	doctype.AddCommand(doctypeAddContainer(deps))
	doctype.AddCommand(doctypeReorderProperties(deps))
	doctype.AddCommand(doctypeCopy(deps))
	doctype.AddCommand(doctypeMove(deps))
	doctype.AddCommand(doctypeDelete(deps))
	root.AddCommand(doctype)
}

func doctypeGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id-or-alias>",
		Short: "Get document type by ID (or by exact alias)",
		Long:  "Fetches a document type by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404. Blocks (allowed blocks, their order, Block Grid groups) live on the Block List/Grid data type: see 'umbraco datatype block --help'.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveSchemaTypeID(cmd.Context(), deps.Client, "document-type", "doctype", "document type", args[0])
			if err != nil {
				return err
			}
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/document-type/%s", id), api.RequestOptions{Fields: fields})
			if err != nil {
				if isSchemaTypeFolderID(cmd.Context(), deps.Client, "document-type", id) {
					return fmt.Errorf("document type id %s is a folder, not a document type; use `umbraco doctype children %s` or `umbraco doctype list --recursive --types-only`", id, id)
				}
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

func doctypeList(deps Dependencies) *cobra.Command {
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
		Short: "List document types (paginated; --skip/--take/--all)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			params = applyPaginationParams(params, skip, take)
			candidates := []getRequestCandidate{
				{path: "/tree/document-type/root", opts: api.RequestOptions{Params: params, Fields: fields}},
				{path: "/document-type/root", opts: api.RequestOptions{Params: params, Fields: fields}},
				{path: "/document-type", opts: api.RequestOptions{Params: params, Fields: fields}},
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
				items, err := flattenSchemaTypeTree(ctx, deps.Client, "document-type", resultItems(result), take, filterFolders, triage.FirstN)
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
	cmd.Flags().BoolVar(&recursive, "recursive", false, "Walk document type folders recursively")
	cmd.Flags().BoolVar(&typesOnly, "types-only", false, "Return document types only, excluding folders")
	cmd.Flags().BoolVar(&excludeFolders, "exclude-folders", false, "Alias for --types-only")
	return cmd
}

func doctypeRoot(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "root",
		Short: "Get root document types (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document-type/root", opts: api.RequestOptions{Params: params}},
				{path: "/document-type/root", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func doctypeChildren(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "children <id>",
		Short: "Get child document types (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document-type/children", opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
				{path: api.JoinPath("/document-type/%s/children", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func doctypeSearch(deps Dependencies) *cobra.Command {
	return searchCommand(deps, searchSpec{
		Use:   "search",
		Short: "Search document types",
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/item/document-type/search", opts: api.RequestOptions{Params: params}},
				{path: "/document-type/search", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func itemID(item any) string {
	entry, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := entry["id"].(string)
	return strings.TrimSpace(id)
}

func doctypeAllowedInLibrary(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "allowed-in-library",
		Short: "List document types usable as library elements (Umbraco 18.1+)",
		Long:  "GET /document-type/allowed-in-library. Lists the element types with allowedInLibrary set — the types 'element create' accepts.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/document-type/allowed-in-library", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func doctypeCreate(deps Dependencies) *cobra.Command {
	return createCommand(deps, createSpec{
		Use:         "create",
		Short:       "Create document type (pass --element to create an element type)",
		Path:        "/document-type",
		TemplateKey: "doctype.create",
		ResultKeys:  []string{"icon"},
		Normalize: func(body map[string]any) error {
			normalizeDoctypePayload(body)
			return nil
		},
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			var element bool
			cmd.Flags().BoolVar(&element, "element", false, "Convenience flag for --json '{...,\"isElement\":true}'; overrides any isElement set in --json")
			return func(body map[string]any) error {
				if element {
					body["isElement"] = true
				}
				return nil
			}
		},
	})
}

func doctypeUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:       "update <id>",
		Short:     "Update document type",
		Path:      func(args []string) string { return api.JoinPath("/document-type/%s", args[0]) },
		Normalize: normalizeDoctypePayloadHook,
	})
}

func doctypeAddProperty(deps Dependencies) *cobra.Command {
	var alias string
	var name string
	var dataType string
	var container string
	var description string
	var mandatory bool
	var createContainer bool
	var newContainerType string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "add-property <id>",
		Short: "Append a property to a document type, creating its container with it if needed",
		Long:  "GET /document-type/{id} + PUT /document-type/{id}. The property lands under the --container tab/group. With --create-container the container is created in the same update when it does not exist yet — the two must travel in one request, because the server prunes containers that are saved empty (which is why a bare 'add-container' followed by 'add-property' cannot work).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for flag, value := range map[string]string{
				"--alias":     alias,
				"--name":      name,
				"--data-type": dataType,
				"--container": container,
			} {
				if err := requireValue(flag, value); err != nil {
					return err
				}
			}
			for _, value := range []string{alias, name, dataType, container} {
				if err := validate.String(value); err != nil {
					return err
				}
			}

			ctx := cmd.Context()
			current, err := fetchDoctypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}

			if !createContainer && cmd.Flags().Changed("container-type") {
				return fmt.Errorf("--container-type only applies with --create-container")
			}
			containerID, ambiguous := findDoctypeContainerID(current, container)
			if ambiguous {
				return fmt.Errorf("doctype %s has multiple containers named %q; rename one or pick a unique name", args[0], container)
			}
			var nextContainers []any
			if containerID == "" {
				if !createContainer {
					return fmt.Errorf("doctype %s has no container named %q; pass --create-container to create it together with this property (empty containers cannot be created alone — the server prunes them on save)", args[0], container)
				}
				normalizedType := normalizeDoctypeContainerType(newContainerType)
				if normalizedType == "" {
					return fmt.Errorf("--container-type must be Tab or Group, got %q", newContainerType)
				}
				newID, err := newUUIDv4()
				if err != nil {
					return fmt.Errorf("failed to generate container id: %w", err)
				}
				created := buildDoctypeContainer(newID, "", container, normalizedType, nextDoctypeContainerSortOrder(current, ""))
				existing, _ := current["containers"].([]any)
				nextContainers = append(append(make([]any, 0, len(existing)+1), existing...), created)
				containerID = newID
			}
			if hasDoctypeProperty(current, alias) {
				return fmt.Errorf("doctype %s already has a property with alias %q", args[0], alias)
			}

			propertyID, err := newUUIDv4()
			if err != nil {
				return fmt.Errorf("failed to generate property id: %w", err)
			}
			sortOrder := nextDoctypePropertySortOrder(current, containerID)
			property := buildDoctypeProperty(propertyID, containerID, alias, name, dataType, description, mandatory, sortOrder)

			patch := map[string]any{"properties": []any{property}}
			if nextContainers != nil {
				// Containers have no alias field, so the alias-keyed merge
				// replaces the whole array with this hand-built slice.
				patch["containers"] = nextContainers
			}
			merged := mergeAliasPayload(current, patch)
			result, err := deps.Client.Put(
				ctx,
				api.JoinPath("/document-type/%s", args[0]),
				merged,
				api.RequestOptions{DryRun: dryRun},
			)
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "Property alias (camelCase identifier)")
	cmd.Flags().StringVar(&name, "name", "", "Human-readable property name")
	cmd.Flags().StringVar(&dataType, "data-type", "", "Data type ID (GUID) backing the property")
	cmd.Flags().StringVar(&container, "container", "", "Name of the existing tab/group container that should hold the property (case-insensitive match)")
	cmd.Flags().StringVar(&description, "description", "", "Optional property description")
	cmd.Flags().BoolVar(&mandatory, "mandatory", false, "Mark the property as mandatory")
	cmd.Flags().BoolVar(&createContainer, "create-container", false, "Create the --container together with this property when it does not exist yet")
	cmd.Flags().StringVar(&newContainerType, "container-type", "Group", "Container type for --create-container: Group or Tab")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func doctypeReorderProperties(deps Dependencies) *cobra.Command {
	var aliasesCSV string
	var alias string
	var sortOrder int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "reorder-properties <id>",
		Short: "Change the order of properties on a document type",
		Long:  "GET /document-type/{id} + PUT /document-type/{id}. The Management API has no dedicated reorder operation — property order is the per-container sortOrder field — so this fetches the document type, rewrites sortOrder values, and PUTs the result back. Two modes: --aliases assigns positions 0..n to the listed properties (all in one container) with the container's remaining properties following in their current relative order; --alias with --sort-order sets a single property's sortOrder verbatim (other properties keep theirs, so equal values sort arbitrarily — prefer --aliases for a full deterministic order).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasList := strings.TrimSpace(aliasesCSV) != ""
			hasSingle := strings.TrimSpace(alias) != ""
			if hasList == hasSingle {
				return fmt.Errorf("doctype reorder-properties requires exactly one of --aliases (full order) or --alias with --sort-order (single move)")
			}
			if hasSingle && sortOrder < 0 {
				return fmt.Errorf("--alias requires --sort-order <n> (0-based)")
			}
			if hasList && sortOrder >= 0 {
				return fmt.Errorf("--sort-order only applies to --alias")
			}

			ordered := uniqueCSV(aliasesCSV)
			if hasList && len(ordered) == 0 {
				return fmt.Errorf("--aliases parsed to no property aliases; pass a comma-separated list like --aliases title,subtitle")
			}

			ctx := cmd.Context()
			current, err := fetchDoctypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}

			var patch []any
			if hasSingle {
				if !hasDoctypeProperty(current, alias) {
					return fmt.Errorf("doctype %s has no property with alias %q", args[0], alias)
				}
				patch = []any{map[string]any{"alias": alias, "sortOrder": sortOrder}}
			} else {
				patch, err = doctypeReorderPatch(current, ordered)
				if err != nil {
					return err
				}
			}

			merged := mergeAliasPayload(current, map[string]any{"properties": patch})
			result, err := deps.Client.Put(
				ctx,
				api.JoinPath("/document-type/%s", args[0]),
				merged,
				api.RequestOptions{DryRun: dryRun},
			)
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}

	cmd.Flags().StringVar(&aliasesCSV, "aliases", "", "Comma-separated property aliases in the desired order (positions become sortOrder; unlisted properties in the container follow in their current order)")
	cmd.Flags().StringVar(&alias, "alias", "", "Single property alias to move (requires --sort-order)")
	cmd.Flags().IntVar(&sortOrder, "sort-order", -1, "Target sortOrder for --alias (0-based)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func doctypeAddContainer(deps Dependencies) *cobra.Command {
	var name string
	var containerType string
	var parent string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "add-container <id>",
		Short: "Append a tab or group container to a document type",
		Long:  "GET /document-type/{id} + PUT /document-type/{id}. Note the server prunes containers that are saved with no properties (verified on 18.1) — this command verifies the container survived the save and errors when it was pruned. To create a container and its first property in one step use 'doctype add-property --create-container'.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for flag, value := range map[string]string{
				"--name": name,
				"--type": containerType,
			} {
				if err := requireValue(flag, value); err != nil {
					return err
				}
			}
			for _, value := range []string{name, containerType, parent} {
				if value == "" {
					continue
				}
				if err := validate.String(value); err != nil {
					return err
				}
			}

			normalizedType := normalizeDoctypeContainerType(containerType)
			if normalizedType == "" {
				return fmt.Errorf("--type must be Tab or Group, got %q", containerType)
			}

			ctx := cmd.Context()
			current, err := fetchDoctypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}

			if hasDoctypeContainer(current, name) {
				return fmt.Errorf("doctype %s already has a container named %q", args[0], name)
			}

			parentID := ""
			if parent != "" {
				resolved, ambiguous := findDoctypeContainerID(current, parent)
				if resolved == "" {
					return fmt.Errorf("doctype %s has no parent container named %q", args[0], parent)
				}
				if ambiguous {
					return fmt.Errorf("doctype %s has multiple containers named %q; rename one or pick a unique name", args[0], parent)
				}
				parentID = resolved
			}

			containerID, err := newUUIDv4()
			if err != nil {
				return fmt.Errorf("failed to generate container id: %w", err)
			}
			sortOrder := nextDoctypeContainerSortOrder(current, parentID)
			container := buildDoctypeContainer(containerID, parentID, name, normalizedType, sortOrder)

			// Containers have no alias field, so the alias-keyed merge replaces the whole array.
			// Build the next containers slice ourselves and let the rest of the doctype stay intact.
			existing, _ := current["containers"].([]any)
			nextContainers := make([]any, 0, len(existing)+1)
			nextContainers = append(nextContainers, existing...)
			nextContainers = append(nextContainers, container)
			merged := mergeAliasPayload(current, map[string]any{"containers": nextContainers})
			result, err := deps.Client.Put(
				ctx,
				api.JoinPath("/document-type/%s", args[0]),
				merged,
				api.RequestOptions{DryRun: dryRun},
			)
			if err != nil {
				return err
			}
			if !dryRun {
				// The server prunes containers saved with no properties and
				// still answers success, so trust nothing: verify the
				// container survived. A failed verification read does not
				// fail the command (the write itself succeeded); only a
				// confirmed prune does.
				after, fetchErr := fetchDoctypeObject(ctx, deps.Client, args[0])
				if fetchErr == nil {
					if id, _ := findDoctypeContainerID(after, name); id == "" {
						return fmt.Errorf("the server accepted the update but pruned the empty container %q — Umbraco discards containers with no properties on save. Create it together with its first property instead: 'doctype add-property %s --container %q --create-container', or send a full payload via 'doctype update'", name, args[0], name)
					}
				}
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Display name for the new container")
	cmd.Flags().StringVar(&containerType, "type", "", "Container type: Tab or Group")
	cmd.Flags().StringVar(&parent, "parent", "", "Optional name of an existing parent container (typically a Tab when adding a Group)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func doctypeCopy(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "copy <id>",
		Short: "Copy document type",
		Candidates: func(args []string) []mutationCandidate {
			return []mutationCandidate{{method: "POST", path: api.JoinPath("/document-type/%s/copy", args[0])}}
		},
		Verb: "copied",
	})
}

func doctypeMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move document type",
		Candidates: func(args []string) []mutationCandidate {
			path := api.JoinPath("/document-type/%s/move", args[0])
			return []mutationCandidate{{method: "PUT", path: path}, {method: "POST", path: path}}
		},
		Verb: "moved",
	})
}

func doctypeDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a document type (content of this type loses its definition)",
		Path: func(args []string) string {
			return api.JoinPath("/document-type/%s", args[0])
		},
	})
}
