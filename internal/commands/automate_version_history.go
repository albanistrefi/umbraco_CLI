package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Version history is the undo path for Automate entities: every save of an
// automation, workspace, or connection becomes a version that can be
// inspected, compared, and rolled back to. Entity types come from
// 'version-history types' (Automation, Workspace, Connection).

func automateVersionHistory(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version-history",
		Short: "Entity version history: list, inspect, compare, roll back",
	}
	cmd.AddCommand(automateVersionHistoryTypes(deps))
	cmd.AddCommand(automateVersionHistoryList(deps))
	cmd.AddCommand(automateVersionHistoryGet(deps))
	cmd.AddCommand(automateVersionHistoryCompare(deps))
	cmd.AddCommand(automateVersionHistoryRollback(deps))
	return cmd
}

func automateVersionHistoryTypes(deps Dependencies) *cobra.Command {
	return automateArrayRead(deps, "types", "List the entity types that keep version history", "/version-history/supported-types")
}

func automateVersionHistoryList(deps Dependencies) *cobra.Command {
	var skip, take int
	cmd := &cobra.Command{
		Use:   "list <entity-type> <entity-id>",
		Short: "List stored versions of an entity (paginated; --skip/--take)",
		Long:  "GET /version-history/{entityType}/{entityId}. entity-type is one of 'version-history types' (e.g. Automation); entity-id is the entity's GUID. The response wraps the versions array with currentVersion/publishedVersion/totalVersions, so it does not follow the standard {items,total} envelope.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/version-history/%s/%s", args[0], args[1]), automateOpts(applyPaginationParams(nil, skip, take), false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	addPaginationFlags(cmd, &skip, &take)
	return cmd
}

func automateVersionHistoryGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <entity-type> <entity-id> <version>",
		Short: "Get one stored version of an entity (the full payload as it was)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/version-history/%s/%s/%s", args[0], args[1], args[2]), automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

func automateVersionHistoryCompare(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "compare <entity-type> <entity-id> <from-version> <to-version>",
		Short: "Compare two stored versions of an entity",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/version-history/%s/%s/%s/compare/%s", args[0], args[1], args[2], args[3]), automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func automateVersionHistoryRollback(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rollback <entity-type> <entity-id> <version>",
		Short: "Roll an entity back to a stored version",
		Long:  "POST /version-history/{entityType}/{entityId}/{entityVersion}/rollback. The undo path after an agent edit goes wrong: pick the version from 'version-history list', confirm with 'get' or 'compare', then roll back.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/version-history/%s/%s/%s/rollback", args[0], args[1], args[2]), nil, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "rolledBack", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
