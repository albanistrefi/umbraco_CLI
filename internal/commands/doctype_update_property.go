package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/uuid"
	"umbraco-cli/internal/validate"
)

// doctypeUpdateProperty builds "doctype update-property": GET + PUT of the
// full document type with one property's fields changed. The Management API
// has no property-level endpoint, so like add-property and remove-property
// the server's own record is written back with only that property edited,
// and the type is re-read to confirm the change.
func doctypeUpdateProperty(deps cmdkit.Dependencies) *cobra.Command {
	var alias string
	var name string
	var description string
	var dataType string
	var mandatory bool
	var noMandatory bool
	var backup string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "update-property <id-or-alias>",
		Short: "Change one property of a document type in place (name, description, mandatory, data type)",
		Long: "GET /document-type/{id} + PUT /document-type/{id}. The Management API has no property-level endpoint, so this fetches the document type, changes only the fields you pass on the property with --alias, and writes the type back otherwise unchanged; the type is re-read afterwards and the command fails if the change did not stick. " +
			"Pass at least one of --name, --description, --mandatory / --no-mandatory, --data-type. When the property already has those values nothing is written. " +
			"A property that comes from a composition (or a parent type) is not part of this type's own properties and can only be edited on the type that declares it; the error names that type. " +
			"Changing --data-type swaps the editor behind any stored values: 'doctype property-is-used' tells you whether there are any. " +
			"Pass --backup to save the pre-change type first; 'doctype restore-backup <file>' puts it back.",
		Example: `  umbraco doctype update-property <id> --alias seoTitle --description "Shown in search results" --dry-run
  umbraco doctype update-property <id> --alias seoTitle --no-mandatory --backup
  umbraco doctype update-property <id> --alias heroImage --data-type <data-type-id> --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireValue("--alias", alias); err != nil {
				return err
			}
			changes, err := doctypePropertyChangesFromFlags(cmd, name, description, dataType, mandatory, noMandatory)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			id, err := resolveSchemaTypeID(ctx, deps.Client, "document-type", "doctype", "document type", args[0])
			if err != nil {
				return err
			}
			path := api.JoinPath("/document-type/%s", id)
			current, err := cmdkit.FetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return err
			}

			properties, _ := current["properties"].([]any)
			index, lookupErr := schemaTypePropertyIndex(current, alias)
			if lookupErr != nil {
				owner, err := doctypeCompositionOwner(ctx, deps.Client, current, alias)
				if err != nil {
					return err
				}
				if owner != nil {
					return owner.error(args[0], alias)
				}
				return fmt.Errorf("document type %s: %w", args[0], lookupErr)
			}
			before, _ := properties[index].(map[string]any)
			updated, diff := applyDoctypePropertyChanges(before, changes)

			out := map[string]any{
				"id":       id,
				"property": map[string]any{"alias": alias, "id": before["id"]},
			}
			if len(diff) == 0 {
				out["changed"] = false
				out["message"] = fmt.Sprintf("property %q already has the requested values; nothing was written", alias)
				return cmdkit.PrintResult(cmd, deps, out)
			}
			out["changed"] = true
			out["changes"] = diff

			var backupFile string
			if cmd.Flags().Changed("backup") && !dryRun {
				backupFile, err = cmdkit.WriteBackup(cmdkit.ResolveBackupPath(backup, "doctype", id), "doctype", id, path, current, nil)
				if err != nil {
					return err
				}
			}

			nextProperties := append(make([]any, 0, len(properties)), properties...)
			nextProperties[index] = updated
			body := cmdkit.CloneAnyMap(current)
			body["properties"] = nextProperties

			result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				out["update"] = result
				return cmdkit.PrintResult(cmd, deps, out)
			}

			after, err := cmdkit.FetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return fmt.Errorf("the server accepted the update but reading the document type back failed: %w", err)
			}
			afterIndex, err := schemaTypePropertyIndex(after, alias)
			if err != nil {
				return fmt.Errorf("the server accepted the update but the document type no longer has property %q", alias)
			}
			afterProperties, _ := after["properties"].([]any)
			stored, _ := afterProperties[afterIndex].(map[string]any)
			if mismatched := doctypePropertyMismatches(stored, updated, diff); len(mismatched) > 0 {
				return fmt.Errorf("the server accepted the update but property %q still differs in %s", alias, strings.Join(mismatched, ", "))
			}
			out["verified"] = true
			if backupFile != "" {
				out["backup"] = backupFile
			}
			return cmdkit.PrintResult(cmd, deps, out)
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "Alias of the property to change (required; exact match)")
	cmd.Flags().StringVar(&name, "name", "", "New display name for the property")
	cmd.Flags().StringVar(&description, "description", "", "New property description (--description \"\" clears it)")
	cmd.Flags().BoolVar(&mandatory, "mandatory", false, "Make the property mandatory")
	cmd.Flags().BoolVar(&noMandatory, "no-mandatory", false, "Make the property optional")
	cmd.Flags().StringVar(&dataType, "data-type", "", "Data type ID (GUID) to back the property with")
	cmdkit.AddBackupFlag(cmd, &backup)
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// doctypePropertyChanges holds the fields update-property was asked to set;
// nil means the flag was not passed and the field is left as it is.
type doctypePropertyChanges struct {
	name        *string
	description *string
	dataType    *string
	mandatory   *bool
}

func doctypePropertyChangesFromFlags(cmd *cobra.Command, name, description, dataType string, mandatory, noMandatory bool) (doctypePropertyChanges, error) {
	flags := cmd.Flags()
	var changes doctypePropertyChanges
	if flags.Changed("name") {
		if strings.TrimSpace(name) == "" {
			return changes, fmt.Errorf("--name cannot be empty")
		}
		if err := validate.String(name); err != nil {
			return changes, err
		}
		changes.name = &name
	}
	if flags.Changed("description") {
		changes.description = &description
	}
	if flags.Changed("data-type") {
		trimmed := strings.TrimSpace(dataType)
		if !uuid.Valid(trimmed) {
			return changes, fmt.Errorf("--data-type must be a data type GUID, got %q; 'umbraco datatype list' lists them", dataType)
		}
		changes.dataType = &trimmed
	}
	switch {
	case flags.Changed("mandatory") && flags.Changed("no-mandatory"):
		return changes, fmt.Errorf("--mandatory and --no-mandatory are mutually exclusive")
	case flags.Changed("mandatory"):
		changes.mandatory = &mandatory
	case flags.Changed("no-mandatory"):
		value := !noMandatory
		changes.mandatory = &value
	}
	if changes.name == nil && changes.description == nil && changes.dataType == nil && changes.mandatory == nil {
		return changes, fmt.Errorf("doctype update-property needs at least one change: --name, --description, --mandatory / --no-mandatory or --data-type")
	}
	return changes, nil
}

// doctypePropertyFields reads the fields update-property can change, in the
// form they are compared in: a null description is the empty one, and data
// type GUIDs compare case-insensitively.
func doctypePropertyFields(property map[string]any) map[string]any {
	description, _ := property["description"].(string)
	dataTypeID := ""
	if dataType, ok := property["dataType"].(map[string]any); ok {
		dataTypeID, _ = dataType["id"].(string)
	}
	mandatory := false
	if validation, ok := property["validation"].(map[string]any); ok {
		mandatory, _ = validation["mandatory"].(bool)
	}
	return map[string]any{
		"name":        property["name"],
		"description": description,
		"dataType":    strings.ToLower(dataTypeID),
		"mandatory":   mandatory,
	}
}

// applyDoctypePropertyChanges returns a copy of property with the requested
// changes applied and a {field: {from, to}} entry for each field that
// actually changes. property itself (and the nested maps it shares with the
// fetched type, which a --backup writes out) is not modified.
func applyDoctypePropertyChanges(property map[string]any, changes doctypePropertyChanges) (map[string]any, map[string]any) {
	current := doctypePropertyFields(property)
	updated := cmdkit.CloneAnyMap(property)
	diff := map[string]any{}

	if changes.name != nil && current["name"] != *changes.name {
		diff["name"] = map[string]any{"from": property["name"], "to": *changes.name}
		updated["name"] = *changes.name
	}
	if changes.description != nil && current["description"] != *changes.description {
		diff["description"] = map[string]any{"from": property["description"], "to": *changes.description}
		updated["description"] = *changes.description
	}
	if changes.dataType != nil && current["dataType"] != strings.ToLower(*changes.dataType) {
		var from any
		dataType, _ := property["dataType"].(map[string]any)
		if dataType != nil {
			from = dataType["id"]
		}
		next := cmdkit.CloneAnyMap(dataType)
		next["id"] = *changes.dataType
		diff["dataType"] = map[string]any{"from": from, "to": *changes.dataType}
		updated["dataType"] = next
	}
	if changes.mandatory != nil && current["mandatory"] != *changes.mandatory {
		validation, _ := property["validation"].(map[string]any)
		next := cmdkit.CloneAnyMap(validation)
		next["mandatory"] = *changes.mandatory
		diff["mandatory"] = map[string]any{"from": current["mandatory"], "to": *changes.mandatory}
		updated["validation"] = next
	}
	return updated, diff
}

// doctypePropertyMismatches names the changed fields whose re-read value
// differs from what was written.
func doctypePropertyMismatches(stored map[string]any, written map[string]any, diff map[string]any) []string {
	got := doctypePropertyFields(stored)
	want := doctypePropertyFields(written)
	mismatched := []string{}
	for _, field := range []string{"name", "description", "dataType", "mandatory"} {
		if _, changed := diff[field]; changed && got[field] != want[field] {
			mismatched = append(mismatched, field)
		}
	}
	return mismatched
}

// doctypePropertyOwner is the composition (or parent type) that declares a
// property the composing type shows but does not own.
type doctypePropertyOwner struct {
	ID              string
	Name            string
	Alias           string
	CompositionType string
	Via             []string
}

func (o *doctypePropertyOwner) error(typeArg string, alias string) error {
	relation := fmt.Sprintf("comes from composition %q", o.Name)
	if o.CompositionType == "Inheritance" {
		relation = fmt.Sprintf("is inherited from parent document type %q", o.Name)
	}
	via := ""
	if len(o.Via) > 0 {
		via = fmt.Sprintf(" (through %s)", strings.Join(o.Via, " → "))
	}
	return fmt.Errorf("document type %s does not own property %q: it %s (alias %s, id %s)%s. Composed properties can only be changed on the type that declares them: run 'umbraco doctype update-property %s --alias %s' with the same flags", typeArg, alias, relation, o.Alias, o.ID, via, o.ID, alias)
}

// doctypeCompositionOwner finds the type that declares alias among the
// compositions of doctype, following compositions of compositions. GET
// /document-type/{id} lists only the type's own properties, so a property
// the backoffice shows on the type but that is missing from them comes from
// one of these. It returns nil when no composition declares alias.
func doctypeCompositionOwner(ctx context.Context, client *api.Client, doctype map[string]any, alias string) (*doctypePropertyOwner, error) {
	visited := map[string]struct{}{}
	if id, _ := doctype["id"].(string); id != "" {
		visited[strings.ToLower(id)] = struct{}{}
	}
	var walk func(node map[string]any, edgeType string, via []string) (*doctypePropertyOwner, error)
	walk = func(node map[string]any, edgeType string, via []string) (*doctypePropertyOwner, error) {
		compositions, _ := node["compositions"].([]any)
		for _, item := range compositions {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ref, _ := entry["documentType"].(map[string]any)
			compositionID, _ := ref["id"].(string)
			if compositionID == "" {
				continue
			}
			if _, seen := visited[strings.ToLower(compositionID)]; seen {
				continue
			}
			visited[strings.ToLower(compositionID)] = struct{}{}
			composed, err := fetchDoctypeObject(ctx, client, compositionID)
			if err != nil {
				return nil, fmt.Errorf("property %q is not on the type itself and reading its composition %s failed: %w", alias, compositionID, err)
			}
			relation := edgeType
			if relation == "" {
				relation, _ = entry["compositionType"].(string)
			}
			if hasDoctypeProperty(composed, alias) {
				name, _ := composed["name"].(string)
				composedAlias, _ := composed["alias"].(string)
				return &doctypePropertyOwner{ID: compositionID, Name: name, Alias: composedAlias, CompositionType: relation, Via: via}, nil
			}
			name, _ := composed["name"].(string)
			owner, err := walk(composed, relation, append(append([]string{}, via...), fmt.Sprintf("%q", name)))
			if err != nil || owner != nil {
				return owner, err
			}
		}
		return nil, nil
	}
	return walk(doctype, "", nil)
}
