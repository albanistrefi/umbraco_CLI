package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
	"umbraco-cli/internal/validate"
)

func RegisterDoctype(root *cobra.Command, deps Dependencies) {
	doctype := &cobra.Command{
		Use:     "doctype",
		Aliases: []string{"document-type"},
		Short:   "Document type schema operations",
	}
	doctype.AddCommand(doctypeGet(deps))
	doctype.AddCommand(doctypeList(deps))
	doctype.AddCommand(doctypeRoot(deps))
	doctype.AddCommand(doctypeChildren(deps))
	doctype.AddCommand(doctypeSearch(deps))
	doctype.AddCommand(doctypeCreate(deps))
	doctype.AddCommand(doctypeUpdate(deps))
	doctype.AddCommand(doctypeAddProperty(deps))
	doctype.AddCommand(doctypeAddContainer(deps))
	doctype.AddCommand(doctypeCopy(deps))
	doctype.AddCommand(doctypeMove(deps))
	doctype.AddCommand(doctypeDelete(deps))
	root.AddCommand(doctype)
}

func doctypeGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get document type by ID",
		Path:  func(args []string) string { return api.JoinPath("/document-type/%s", args[0]) },
	})
}

func doctypeList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List document types (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document-type/root", opts: api.RequestOptions{Params: params}},
				{path: "/document-type/root", opts: api.RequestOptions{Params: params}},
				{path: "/document-type", opts: api.RequestOptions{Params: params}},
			}
		},
	})
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

func doctypeCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	var element bool
	cmd := &cobra.Command{Use: "create", Short: "Create document type (pass --element to create an element type)", RunE: func(cmd *cobra.Command, args []string) error {
		if printTemplate {
			return printResult(cmd, deps, schema.Templates["doctype.create"])
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
		normalizeDoctypePayload(body)
		if element {
			body["isElement"] = true
		}
		result, err := deps.Client.Post(cmd.Context(), "/document-type", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body, "icon"))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	cmd.Flags().BoolVar(&element, "element", false, "Convenience flag for --json '{...,\"isElement\":true}'; overrides any isElement set in --json")
	return cmd
}

func doctypeUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:       "update <id>",
		Short:     "Update document type",
		Path:      func(args []string) string { return api.JoinPath("/document-type/%s", args[0]) },
		Normalize: normalizeDoctypePayload,
	})
}

func doctypeAddProperty(deps Dependencies) *cobra.Command {
	var alias string
	var name string
	var dataType string
	var container string
	var description string
	var mandatory bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "add-property <id>",
		Short: "Append a property to a document type under an existing container alias",
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

			containerID, ambiguous := findDoctypeContainerID(current, container)
			if containerID == "" {
				return fmt.Errorf("doctype %s has no container named %q", args[0], container)
			}
			if ambiguous {
				return fmt.Errorf("doctype %s has multiple containers named %q; rename one or pick a unique name", args[0], container)
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

			merged := mergeAliasPayload(current, map[string]any{
				"properties": []any{property},
			})
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
		Path:  func(args []string) string { return api.JoinPath("/document-type/%s/copy", args[0]) },
		Verb:  "copied",
	})
}

func doctypeMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move document type",
		Path:  func(args []string) string { return api.JoinPath("/document-type/%s/move", args[0]) },
		Verb:  "moved",
	})
}

func doctypeDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, "delete <id>", "Permanently delete a document type (content of this type loses its definition)", func(args []string) string {
		return api.JoinPath("/document-type/%s", args[0])
	})
}
