package commands

import (
	"context"
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

			merged := mergeAliasPayload(current, patch)
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

			current, err := fetchDoctypeObject(context.Background(), deps.Client, args[0])
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
				context.Background(),
				fmt.Sprintf("/document-type/%s", args[0]),
				merged,
				api.RequestOptions{DryRun: dryRun, SkipValidation: true},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "Property alias (camelCase identifier)")
	cmd.Flags().StringVar(&name, "name", "", "Human-readable property name")
	cmd.Flags().StringVar(&dataType, "data-type", "", "Data type ID (GUID) backing the property")
	cmd.Flags().StringVar(&container, "container", "", "Name of the existing tab/group container that should hold the property (case-insensitive match)")
	cmd.Flags().StringVar(&description, "description", "", "Optional property description")
	cmd.Flags().BoolVar(&mandatory, "mandatory", false, "Mark the property as mandatory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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

			current, err := fetchDoctypeObject(context.Background(), deps.Client, args[0])
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
				context.Background(),
				fmt.Sprintf("/document-type/%s", args[0]),
				merged,
				api.RequestOptions{DryRun: dryRun, SkipValidation: true},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Display name for the new container")
	cmd.Flags().StringVar(&containerType, "type", "", "Container type: Tab or Group")
	cmd.Flags().StringVar(&parent, "parent", "", "Optional name of an existing parent container (typically a Tab when adding a Group)")
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
