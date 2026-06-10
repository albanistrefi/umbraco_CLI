package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

func RegisterUser(root *cobra.Command, deps Dependencies) {
	user := &cobra.Command{
		Use:   "user",
		Short: "Backoffice user management (accounts, state, groups, API credentials)",
		Long:  "Manages the backoffice users the CLI itself authenticates as — not front-office members (see 'member'). 'user client-credentials' manages the OAuth client IDs/secrets that API users like this CLI log in with.",
	}
	user.AddCommand(userList(deps))
	user.AddCommand(userGet(deps))
	user.AddCommand(userCreate(deps))
	user.AddCommand(userInvite(deps))
	user.AddCommand(userUpdate(deps))
	user.AddCommand(userDelete(deps))
	user.AddCommand(userStateCommand(deps, "enable", "Enable disabled user accounts", "enabled"))
	user.AddCommand(userStateCommand(deps, "disable", "Disable user accounts (they keep existing but cannot log in)", "disabled"))
	user.AddCommand(userStateCommand(deps, "unlock", "Unlock user accounts locked out by failed logins", "unlocked"))
	user.AddCommand(userSetGroups(deps))
	user.AddCommand(userCurrent(deps))
	user.AddCommand(userPermissions(deps))
	user.AddCommand(userClientCredentials(deps))
	root.AddCommand(user)
}

func userList(deps Dependencies) *cobra.Command {
	var filter string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List backoffice users (paginated; --skip/--take/--all, --filter for substring search)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			if strings.TrimSpace(filter) != "" {
				params = withParam(params, "filter", filter)
			}
			return []getRequestCandidate{
				{path: "/filter/user", opts: api.RequestOptions{Params: params}},
				{path: "/user", opts: api.RequestOptions{Params: params}},
			}
		},
	})
	cmd.Flags().StringVar(&filter, "filter", "", "Substring filter against user name/email")
	return cmd
}

func userGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get a backoffice user by ID",
		Path:  func(args []string) string { return api.JoinPath("/user/%s", args[0]) },
	})
}

func userCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a backoffice user",
		Long:  "POST /user. Required: email, userName, name, userGroupIds ([{\"id\":\"<guid>\"}] from 'user-group list'), kind (\"Default\" for humans, \"Api\" for credential-only API users). API-kind users get credentials via 'user client-credentials create'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["user.create"])
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
			result, err := deps.Client.Post(cmd.Context(), "/user", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body, "kind"))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func userInvite(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Invite a user by email (they choose their own password)",
		Long:  "POST /user/invite. Same required fields as 'user create' minus kind, plus an optional message included in the invitation email. Requires the server to have SMTP configured.",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			result, err := deps.Client.Post(cmd.Context(), "/user/invite", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Invite payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func userUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update a backoffice user",
		Path:  func(args []string) string { return api.JoinPath("/user/%s", args[0]) },
	})
}

func userDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, "delete <id>", "Permanently delete a backoffice user", func(args []string) string {
		return api.JoinPath("/user/%s", args[0])
	})
}

// userStateCommand builds the enable/disable/unlock trio: each POSTs a
// {userIds:[{id},...]} body to /user/<action>.
func userStateCommand(deps Dependencies, action string, short string, verb string) *cobra.Command {
	var idsCSV string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   action + " --ids <id,...>",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("user %s requires --ids <comma-separated user guids>", action)
			}
			body := map[string]any{"userIds": idReferences(ids)}
			result, err := deps.Client.Post(cmd.Context(), "/user/"+action, body, api.RequestOptions{DryRun: dryRun})
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

func userSetGroups(deps Dependencies) *cobra.Command {
	var userIDsCSV string
	var groupIDsCSV string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set-groups",
		Short: "Replace the group memberships of one or more users",
		Long:  "POST /user/set-user-groups. Replaces each listed user's groups with exactly the listed group set. Group GUIDs come from 'user-group list'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			userIDs := uniqueCSV(userIDsCSV)
			groupIDs := uniqueCSV(groupIDsCSV)
			if len(userIDs) == 0 {
				return fmt.Errorf("user set-groups requires --user-ids <comma-separated user guids>")
			}
			body := map[string]any{
				"userIds":      idReferences(userIDs),
				"userGroupIds": idReferences(groupIDs),
			}
			result, err := deps.Client.Post(cmd.Context(), "/user/set-user-groups", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&userIDsCSV, "user-ids", "", "Comma-separated user GUIDs (required)")
	cmd.Flags().StringVar(&groupIDsCSV, "group-ids", "", "Comma-separated user-group GUIDs; the users' groups become exactly this set")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func userCurrent(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Get the user the CLI is authenticated as",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/user/current", api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

func userPermissions(deps Dependencies) *cobra.Command {
	var idsCSV string
	var resource string
	cmd := &cobra.Command{
		Use:   "permissions --ids <id,...>",
		Short: "Check the current user's permissions on specific items",
		Long:  "GET /user/current/permissions[/document|/media]. Lets an agent verify it may write or publish a node before issuing the mutation. --type selects the permission surface: entity (default), document, or media.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("user permissions requires --ids <comma-separated guids>")
			}
			path := "/user/current/permissions"
			switch resource {
			case "", "entity":
				// base path
			case "document", "media":
				path += "/" + resource
			default:
				return fmt.Errorf("--type must be entity, document, or media (got %q)", resource)
			}
			result, err := deps.Client.Get(cmd.Context(), path, api.RequestOptions{Params: map[string]any{"id": stringsToAny(ids)}})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated entity GUIDs to check (required)")
	cmd.Flags().StringVar(&resource, "type", "entity", "Permission surface: entity, document, or media")
	return cmd
}

func userClientCredentials(deps Dependencies) *cobra.Command {
	credentials := &cobra.Command{
		Use:   "client-credentials",
		Short: "OAuth client credentials for API users (what this CLI logs in with)",
	}
	credentials.AddCommand(userClientCredentialsList(deps))
	credentials.AddCommand(userClientCredentialsCreate(deps))
	credentials.AddCommand(userClientCredentialsDelete(deps))
	return credentials
}

func userClientCredentialsList(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list <user-id>",
		Short: "List the client IDs registered for an API user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/user/%s/client-credentials", args[0]), api.RequestOptions{})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func userClientCredentialsCreate(deps Dependencies) *cobra.Command {
	var clientID string
	var clientSecret string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create <user-id>",
		Short: "Register a client ID/secret pair on an API user",
		Long:  "POST /user/{id}/client-credentials. The user must be of kind Api ('user create' with \"kind\":\"Api\"). Client IDs are conventionally prefixed umbraco-back-office-.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--client-id", clientID); err != nil {
				return err
			}
			if err := requireValue("--client-secret", clientSecret); err != nil {
				return err
			}
			body := map[string]any{"clientId": clientID, "clientSecret": clientSecret}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/user/%s/client-credentials", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID (required)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret (required)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func userClientCredentialsDelete(deps Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <user-id> <client-id>",
		Short: "Remove a client ID from an API user (revokes its access)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !dryRun {
				return fmt.Errorf("user client-credentials delete revokes API access; pass --force to confirm or --dry-run to rehearse")
			}
			result, err := deps.Client.Delete(cmd.Context(), api.JoinPath("/user/%s/client-credentials/%s", args[0], args[1]), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm revoking the credential")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// idReferences converts GUID strings into the [{id: ...}] reference shape
// the user endpoints take for userIds/userGroupIds.
func idReferences(ids []string) []any {
	refs := make([]any, len(ids))
	for i, id := range ids {
		refs[i] = map[string]any{"id": id}
	}
	return refs
}
