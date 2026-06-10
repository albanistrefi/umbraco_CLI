package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// documentVersion groups the version-history commands under
// 'document version'. Together with 'document audit-log' this is the undo
// path for content edits: list versions, inspect one, roll back to it.
func documentVersion(deps Dependencies) *cobra.Command {
	version := &cobra.Command{
		Use:   "version",
		Short: "Document version history: list, inspect, roll back",
	}
	version.AddCommand(documentVersionList(deps))
	version.AddCommand(documentVersionGet(deps))
	version.AddCommand(documentVersionRollback(deps))
	version.AddCommand(documentVersionPreventCleanup(deps))
	return version
}

func documentVersionList(deps Dependencies) *cobra.Command {
	var culture string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list <document-id>",
		Short: "List stored versions of a document (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			versionParams := withParam(params, "documentId", args[0])
			if culture != "" {
				versionParams["culture"] = culture
			}
			return []getRequestCandidate{
				{path: "/document-version", opts: api.RequestOptions{Params: versionParams}},
			}
		},
	})
	cmd.Flags().StringVar(&culture, "culture", "", "Limit versions to one culture on variant content")
	return cmd
}

func documentVersionGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <version-id>",
		Short: "Get a stored document version (the full payload as it was)",
		Path:  func(args []string) string { return api.JoinPath("/document-version/%s", args[0]) },
	})
}

func documentVersionRollback(deps Dependencies) *cobra.Command {
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rollback <version-id>",
		Short: "Roll the document back to this version",
		Long:  "POST /document-version/{id}/rollback. Version IDs come from 'document version list'. On variant content pass --culture to roll back a single culture; omitting it rolls back the invariant data.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if culture != "" {
				params["culture"] = culture
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/document-version/%s/rollback", args[0]), nil, api.RequestOptions{DryRun: dryRun, Params: params})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "rolledBack", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&culture, "culture", "", "Culture to roll back on variant content")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentVersionPreventCleanup(deps Dependencies) *cobra.Command {
	var disable bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prevent-cleanup <version-id>",
		Short: "Pin a version so scheduled history cleanup never deletes it",
		Long:  "PUT /document-version/{id}/prevent-cleanup. Pins the version by default; pass --disable to unpin it again.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{"preventCleanup": !disable}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document-version/%s/prevent-cleanup", args[0]), nil, api.RequestOptions{DryRun: dryRun, Params: params})
			if err != nil {
				return err
			}
			verb := "pinned"
			if disable {
				verb = "unpinned"
			}
			return printMutationResult(cmd, deps, verb, result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&disable, "disable", false, "Allow cleanup to delete this version again")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentAuditLog(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "audit-log <id>",
		Short: "List the audit trail for a document (who did what, when)",
		Long:  "GET /document/{id}/audit-log. Pass --params for orderDirection or sinceDate filters, e.g. --params '{\"sinceDate\":\"2026-01-01T00:00:00Z\"}'.",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: api.JoinPath("/document/%s/audit-log", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
	})
}
