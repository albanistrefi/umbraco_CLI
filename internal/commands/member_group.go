package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

const memberGroupPath = "/member-group"

func RegisterMemberGroup(root *cobra.Command, deps cmdkit.Dependencies) {
	mg := &cobra.Command{
		Use:     "member-group",
		Aliases: []string{"member-groups", "membergroup"},
		Short:   "Member group lookups (for 'member set-groups' GUID discovery)",
	}
	mg.AddCommand(memberGroupList(deps))
	mg.AddCommand(memberGroupGet(deps))
	root.AddCommand(mg)
}

func memberGroupList(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	var triage cmdkit.ReadTriageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all member groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := cmdkit.GetWithFallback(
				cmd.Context(),
				deps.Client,
				cmdkit.GetRequestCandidate{Path: memberGroupPath, Opts: api.RequestOptions{Fields: fields}},
				cmdkit.GetRequestCandidate{Path: "/tree/member-group/root", Opts: api.RequestOptions{Fields: fields}},
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyReadTriage(cmdkit.ApplyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmdkit.AddReadTriageFlags(cmd, &triage)
	return cmd
}

func memberGroupGet(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a member group by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath(memberGroupPath+"/%s", args[0]), api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}
