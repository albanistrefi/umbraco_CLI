package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

func RegisterUserGroup(root *cobra.Command, deps Dependencies) {
	userGroup := &cobra.Command{
		Use:   "user-group",
		Short: "Backoffice user group management (permission sets)",
	}
	userGroup.AddCommand(userGroupList(deps))
	userGroup.AddCommand(userGroupGet(deps))
	userGroup.AddCommand(userGroupCreate(deps))
	userGroup.AddCommand(userGroupUpdate(deps))
	userGroup.AddCommand(userGroupDelete(deps))
	userGroup.AddCommand(userGroupAddUsers(deps))
	userGroup.AddCommand(userGroupRemoveUsers(deps))
	root.AddCommand(userGroup)
}

func userGroupList(deps Dependencies) *cobra.Command {
	var filter string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List user groups (paginated; --skip/--take/--all, --filter for substring search)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			if filter != "" {
				return []getRequestCandidate{
					{path: "/filter/user-group", opts: api.RequestOptions{Params: withParam(params, "filter", filter)}},
				}
			}
			return []getRequestCandidate{
				{path: "/user-group", opts: api.RequestOptions{Params: params}},
			}
		},
	})
	cmd.Flags().StringVar(&filter, "filter", "", "Substring filter against group names")
	return cmd
}

func userGroupGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get a user group by ID",
		Path:  func(args []string) string { return api.JoinPath("/user-group/%s", args[0]) },
	})
}

func userGroupCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user group",
		Long:  "POST /user-group. The model is permission-heavy; start from --print-template or 'user-group get' an existing group. sections use the umb-prefixed aliases (Umb.Section.Content, ...); permissions use single-letter verb codes matching the backoffice.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["user-group.create"])
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
			result, err := deps.Client.Post(cmd.Context(), "/user-group", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body, "alias"))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func userGroupUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update a user group",
		Path:  func(args []string) string { return api.JoinPath("/user-group/%s", args[0]) },
	})
}

func userGroupDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, "delete <id>", "Permanently delete a user group", func(args []string) string {
		return api.JoinPath("/user-group/%s", args[0])
	})
}

func userGroupAddUsers(deps Dependencies) *cobra.Command {
	return userGroupMembershipCommand(deps, "add-users", "Add users to a user group", "added",
		func(ctx *cobra.Command, path string, body any, dryRun bool) (any, error) {
			return deps.Client.Post(ctx.Context(), path, body, api.RequestOptions{DryRun: dryRun})
		})
}

func userGroupRemoveUsers(deps Dependencies) *cobra.Command {
	return userGroupMembershipCommand(deps, "remove-users", "Remove users from a user group", "removed",
		func(ctx *cobra.Command, path string, body any, dryRun bool) (any, error) {
			return deps.Client.Request(ctx.Context(), "DELETE", path, body, api.RequestOptions{DryRun: dryRun})
		})
}

// userGroupMembershipCommand builds add-users/remove-users: both send a
// [{id},...] array body to /user-group/{id}/users, differing only in method.
func userGroupMembershipCommand(deps Dependencies, use string, short string, verb string, send func(cmd *cobra.Command, path string, body any, dryRun bool) (any, error)) *cobra.Command {
	var idsCSV string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   use + " <group-id> --ids <id,...>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("user-group %s requires --ids <comma-separated user guids>", use)
			}
			result, err := send(cmd, api.JoinPath("/user-group/%s/users", args[0]), idReferences(ids), dryRun)
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, verb, result, dryRun)
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated user GUIDs (required)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
