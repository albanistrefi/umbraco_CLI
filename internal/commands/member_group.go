package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

const memberGroupPath = "/member-group"

func RegisterMemberGroup(root *cobra.Command, deps Dependencies) {
	mg := &cobra.Command{
		Use:     "member-group",
		Aliases: []string{"member-groups", "membergroup"},
		Short:   "Member group lookups (for 'member set-groups' GUID discovery)",
	}
	mg.AddCommand(memberGroupList(deps))
	mg.AddCommand(memberGroupGet(deps))
	root.AddCommand(mg)
}

func memberGroupList(deps Dependencies) *cobra.Command {
	var fields string
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all member groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{path: memberGroupPath, opts: api.RequestOptions{Fields: fields}},
				getRequestCandidate{path: "/tree/member-group/root", opts: api.RequestOptions{Fields: fields}},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func memberGroupGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a member group by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(context.Background(), fmt.Sprintf("%s/%s", memberGroupPath, args[0]), api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}
