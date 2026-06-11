package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

// Workspaces partition Automate: every automation lives in exactly one
// workspace, which also decides the connections it may use and the user
// groups that may edit it. Authoring anything starts with a workspace ID.

func automateWorkspace(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Workspace operations (every automation lives in one)",
	}
	cmd.AddCommand(automateWorkspaceList(deps))
	cmd.AddCommand(automateWorkspaceGet(deps))
	cmd.AddCommand(automateWorkspaceCreate(deps))
	cmd.AddCommand(automateWorkspaceUpdate(deps))
	cmd.AddCommand(automateWorkspaceDelete(deps))
	cmd.AddCommand(automateWorkspaceGroup(deps))
	return cmd
}

func automateWorkspaceList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List workspaces (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/workspaces", opts: automateOpts(params, false)},
			}
		},
	})
}

func automateWorkspaceGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:       "get <id>",
		Short:     "Get a workspace by ID",
		Path:      func(args []string) string { return api.JoinPath("/workspaces/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateWorkspaceCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workspace",
		Long:  "POST /workspaces. Required: alias, name, serviceAccountKey (a backoffice user GUID automations run as), userGroups (who may edit), allowedConnections (connection GUIDs automations here may use; [] for none). Use --print-template for the shape.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["automate.workspace.create"])
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/workspaces", body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func automateWorkspaceUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:       "update <id>",
		Short:     "Update a workspace",
		Long:      "PUT /workspaces/{id}. The update model requires the workspace's current version field for optimistic concurrency; --merge-json picks it up from the fetch automatically.",
		Path:      func(args []string) string { return api.JoinPath("/workspaces/%s", args[0]) },
		Normalize: stripFields("id", "dateCreated", "dateModified"),
		APIPrefix: automateAPIPrefix,
	})
}

func automateWorkspaceDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete a workspace",
		Path:      func(args []string) string { return api.JoinPath("/workspaces/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateWorkspaceGroup(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Automation groups within a workspace (folders for organizing automations)",
	}
	cmd.AddCommand(automateWorkspaceGroupList(deps))
	cmd.AddCommand(automateWorkspaceGroupGet(deps))
	cmd.AddCommand(automateWorkspaceGroupAdd(deps))
	cmd.AddCommand(automateWorkspaceGroupUpdate(deps))
	cmd.AddCommand(automateWorkspaceGroupRemove(deps))
	return cmd
}

func automateWorkspaceGroupList(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "list <workspace-id>",
		Short: "List automation groups in a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/workspaces/%s/groups", args[0]), automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

func automateWorkspaceGroupGet(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "get <workspace-id> <group-id>",
		Short: "Get an automation group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/workspaces/%s/groups/%s", args[0], args[1]), automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

// automateWorkspaceGroupBody builds the {name, parentId?} body shared by
// group add and update.
func automateWorkspaceGroupBody(name string, parentID string) (map[string]any, error) {
	if err := requireValue("--name", name); err != nil {
		return nil, err
	}
	body := map[string]any{"name": name}
	if strings.TrimSpace(parentID) != "" {
		body["parentId"] = parentID
	}
	return body, nil
}

func automateWorkspaceGroupAdd(deps Dependencies) *cobra.Command {
	var name string
	var parentID string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add <workspace-id>",
		Short: "Add an automation group to a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := automateWorkspaceGroupBody(name, parentID)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/workspaces/%s/groups", args[0]), body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Group name (required)")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "Parent group ID for nesting")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateWorkspaceGroupUpdate(deps Dependencies) *cobra.Command {
	var name string
	var parentID string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <workspace-id> <group-id>",
		Short: "Rename or move an automation group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := automateWorkspaceGroupBody(name, parentID)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/workspaces/%s/groups/%s", args[0], args[1]), body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Group name (required)")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "Parent group ID for nesting")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateWorkspaceGroupRemove(deps Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remove <workspace-id> <group-id>",
		Short: "Remove an automation group from a workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !dryRun {
				return fmt.Errorf("%s permanently deletes the group; pass --force to confirm or --dry-run to rehearse", cmd.CommandPath())
			}
			result, err := deps.Client.Delete(cmd.Context(), api.JoinPath("/workspaces/%s/groups/%s", args[0], args[1]), automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
