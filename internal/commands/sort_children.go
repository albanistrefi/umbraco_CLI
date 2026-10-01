package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// sortChildrenCommand builds "<resource> sort-children [parent-id]" for the
// server-side reorder operations PUT /<resource>/root/sort-children and
// PUT /<resource>/{id}/sort-children (Umbraco 18.1+). Unlike "<resource>
// sort", which PUTs an explicit id order, these sort every child of the
// parent by a field. withCulture wires --culture for variant-aware
// resources (documents); the media model has no culture.
func sortChildrenCommand(deps cmdkit.Dependencies, resource string, withCulture bool) *cobra.Command {
	var field string
	var direction string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sort-children [parent-id]",
		Short: "Sort all children of a node by a field",
		Long:  fmt.Sprintf("PUT /%[1]s/root/sort-children or /%[1]s/{id}/sort-children (Umbraco 18.1+). Reorders every child of the parent (root when [parent-id] is omitted) server-side by --field. For explicit manual ordering use '%[1]s sort' instead.", resource),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedField, err := matchEnumValue("--field", field, "Name", "CreateDate", "UpdateDate")
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(direction)) {
			case "asc":
				direction = "Ascending"
			case "desc":
				direction = "Descending"
			}
			normalizedDirection, err := matchEnumValue("--direction", direction, "Ascending", "Descending")
			if err != nil {
				return err
			}
			body := map[string]any{"field": normalizedField, "direction": normalizedDirection}
			if withCulture && strings.TrimSpace(culture) != "" {
				body["culture"] = culture
			}
			path := "/" + resource + "/root/sort-children"
			if len(args) == 1 {
				path = api.JoinPath("/"+resource+"/%s/sort-children", args[0])
			}
			result, err := deps.Client.Put(cmd.Context(), path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "sorted", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&field, "field", "", "Sort field: Name, CreateDate, or UpdateDate (required)")
	cmd.Flags().StringVar(&direction, "direction", "Ascending", "Sort direction: Ascending or Descending (asc/desc accepted)")
	if withCulture {
		cmd.Flags().StringVar(&culture, "culture", "", "Sort by the variant name of this culture (variant content only)")
	}
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// matchEnumValue case-insensitively matches value against the allowed enum
// members, returning the canonical casing the Management API expects.
func matchEnumValue(flag, value string, allowed ...string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", fmt.Errorf("%s is required (one of %s)", flag, strings.Join(allowed, ", "))
	}
	for _, candidate := range allowed {
		if strings.EqualFold(v, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s must be one of %s, got %q", flag, strings.Join(allowed, ", "), value)
}
