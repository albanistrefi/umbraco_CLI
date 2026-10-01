package forms

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// Prevalue sources feed the options of list-style form fields (dropdowns,
// checkbox lists, radio buttons) from somewhere other than the form itself:
// a text file, a data type's prevalues, documents, or a SQL query.

func formsPrevalueSource(deps cmdkit.Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prevalue-source",
		Short: "Prevalue sources: shared option lists for list-style form fields",
		Long: "Prevalue sources feed the options of dropdown, checkbox-list and radio-button fields from a text file, a data type, documents or a SQL query. " +
			"Reads print each source's settings verbatim — for a SQL source that can include a connection string — so prefer --fields id,name when the settings are not needed.",
	}
	cmd.AddCommand(formsPrevalueSourceList(deps))
	cmd.AddCommand(formsPrevalueSourceTypes(deps))
	cmd.AddCommand(formsPrevalueSourceGet(deps))
	cmd.AddCommand(formsPrevalueSourceCreate(deps))
	cmd.AddCommand(formsPrevalueSourceUpdate(deps))
	cmd.AddCommand(formsPrevalueSourceDelete(deps))
	return cmd
}

func formsPrevalueSourceList(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List prevalue sources (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/prevalue-source", Opts: formsRequestOpts("", params)},
			}
		},
	})
}

func formsPrevalueSourceTypes(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "types",
		Short: "List prevalue source types (the fieldPreValueSourceTypeId values and their settings)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/prevalue-source-type", formsRequestOpts(fields, nil))
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(result, fields))
		},
	}
	cmdkit.AddFieldsFlag(cmd, &fields)
	return cmd
}

func formsPrevalueSourceGet(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:       "get <id>",
		Short:     "Get a prevalue source by ID",
		Path:      func(args []string) string { return api.JoinPath("/prevalue-source/%s", args[0]) },
		APIPrefix: formsAPIPrefix,
	})
}

func formsPrevalueSourceCreate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CreateCommand(deps, cmdkit.CreateSpec{
		Use:   "create",
		Short: "Create a prevalue source, starting from the server's scaffold",
		Long: "POST /prevalue-source. The CLI fetches GET /prevalue-source/scaffold (a fresh id and the required defaults) and deep-merges --json on top. " +
			"Required: \"name\" and \"fieldPreValueSourceTypeId\" (an id from 'forms prevalue-source types'), plus that type's \"settings\" (setting aliases are listed per type).",
		Path:         "/prevalue-source",
		TemplateKey:  "forms.prevalue-source.create",
		PayloadUsage: "Prevalue source fields as JSON, deep-merged onto GET /prevalue-source/scaffold",
		Base: func(ctx context.Context) (map[string]any, error) {
			return cmdkit.FetchObject(ctx, deps.Client, "/prevalue-source/scaffold", formsRequestOpts("", nil))
		},
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			return func(body map[string]any) error {
				if name, _ := body["name"].(string); strings.TrimSpace(name) == "" {
					return fmt.Errorf("prevalue-source create requires a name: pass --json '{\"name\": …, \"fieldPreValueSourceTypeId\": …}'")
				}
				if typeID := cmdkit.AsString(body["fieldPreValueSourceTypeId"]); !cmdkit.IsUUIDLike(typeID) || typeID == "00000000-0000-0000-0000-000000000000" {
					return fmt.Errorf("prevalue-source create requires \"fieldPreValueSourceTypeId\": an id from 'umbraco forms prevalue-source types'")
				}
				if id, ok := body["id"].(string); ok && strings.TrimSpace(id) != "" {
					if _, set := body["unique"]; set {
						body["unique"] = id
					}
				}
				return nil
			}
		},
		ResultKeys: []string{"fieldPreValueSourceTypeId"},
		APIPrefix:  formsAPIPrefix,
	})
}

func formsPrevalueSourceUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update <id>",
		Short: "Update a prevalue source",
		Long: "PUT /prevalue-source/{id}. --merge-json fetches the source and deep-merges the patch (the safe default); --json replaces it wholesale. " +
			"The body's id is filled in from the argument and a mismatching one is refused.",
		Path:      func(args []string) string { return api.JoinPath("/prevalue-source/%s", args[0]) },
		BindArgs:  bindFormsBodyID,
		APIPrefix: formsAPIPrefix,
	})
}

func formsPrevalueSourceDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete a prevalue source",
		Path:      func(args []string) string { return api.JoinPath("/prevalue-source/%s", args[0]) },
		APIPrefix: formsAPIPrefix,
	})
}
