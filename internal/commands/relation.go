package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// RegisterRelation wires the relation group: relation types (the catalogue
// of relation kinds) and the relations stored for one of them.
func RegisterRelation(root *cobra.Command, deps Dependencies) {
	relation := &cobra.Command{
		Use:   "relation",
		Short: "Relations and relation types",
		Long: `Relations and relation types.

Task → command:
  Which relation types exist?                          relation type list
  What does one relation type do (bidirectional?)      relation type get <id>
  Resolve relation type GUIDs to names in one call     relation type items --ids <id1,id2>
  What relations are stored for a relation type?       relation list --type <id> --take 5
  Walk every relation of a type                        relation list --type <id> --all
  Only show the two ends of each relation              relation list --type <id> --fields parent,child

The Management API exposes relations per relation type only — there is no
by-parent or by-child read — so start from 'relation type list', then filter
the rows of 'relation list --type <id>' client-side.`,
	}
	relation.AddCommand(relationList(deps))
	relation.AddCommand(relationType(deps))
	root.AddCommand(relation)
}

func relationList(deps Dependencies) *cobra.Command {
	var relationTypeID string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List the relations stored for one relation type (paginated; --skip/--take/--all)",
		Long:  "GET /relation/type/{id}. The relation type GUID comes from 'relation type list'. Each row names the two ends of the relation (parent/child) — the API has no by-parent or by-child endpoint, so narrow the rows with --fields or --all plus client-side filtering. Bookkeeping types such as umbMedia can hold tens of thousands of rows, so keep --take small before reaching for --all.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: api.JoinPath("/relation/type/%s", strings.TrimSpace(relationTypeID)), opts: api.RequestOptions{Params: params}},
			}
		},
	})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(relationTypeID) == "" {
			return fmt.Errorf("relation list requires --type <relation-type-id>; run 'umbraco relation type list' to find one")
		}
		return nil
	}
	cmd.Flags().StringVar(&relationTypeID, "type", "", "Relation type GUID whose relations to list (required)")
	return cmd
}

// relationType groups the relation-type catalogue reads under
// 'relation type'. Relation types are the vocabulary; 'relation list' reads
// the rows recorded against one of them.
func relationType(deps Dependencies) *cobra.Command {
	relationType := &cobra.Command{
		Use:   "type",
		Short: "Relation types: list, inspect, resolve by ID",
	}
	relationType.AddCommand(relationTypeList(deps))
	relationType.AddCommand(relationTypeGet(deps))
	relationType.AddCommand(relationTypeItems(deps))
	return relationType
}

func relationTypeList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List relation types (paginated; --skip/--take/--all)",
		Long:  "GET /relation-type. The catalogue of relation kinds the instance knows about (document/media pickers, tracked references, recycle-bin bookkeeping). The id of a row is the --type argument of 'relation list'.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/relation-type", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func relationTypeGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get one relation type (alias, direction, tracked object types)",
		Long:  "GET /relation-type/{id}. isBidirectional tells you whether the relation is walked from both ends; isDependency tells you whether deleting one end is blocked by the other.",
		Path:  func(args []string) string { return api.JoinPath("/relation-type/%s", args[0]) },
	})
}

func relationTypeItems(deps Dependencies) *cobra.Command {
	var idsCSV string
	var fields string
	cmd := &cobra.Command{
		Use:   "items",
		Short: "Resolve relation type GUIDs to names in one call",
		Long:  "GET /item/relation-type?id=…. The item read for relation types: pass the GUIDs seen in other payloads and get their names back without one request per ID.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("relation type items requires --ids <comma-separated guids>")
			}
			result, err := deps.Client.Get(cmd.Context(), "/item/relation-type", api.RequestOptions{Params: map[string]any{"id": stringsToAny(ids)}, Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated relation type GUIDs (required)")
	addFieldsFlag(cmd, &fields)
	return cmd
}
