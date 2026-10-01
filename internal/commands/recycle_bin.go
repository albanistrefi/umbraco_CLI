package commands

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// recycleBinCommand builds the 'bin' subgroup shared by document and media —
// the recycle-bin API is symmetric across the two resources. Trash and
// restore live on the parent groups (document trash/restore, media trash);
// the bin group covers looking inside the bin and permanently emptying it.
func recycleBinCommand(deps cmdkit.Dependencies, resource string) *cobra.Command {
	bin := &cobra.Command{Use: "bin", Short: fmt.Sprintf("%s recycle bin operations", resource)}

	bin.AddCommand(cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: fmt.Sprintf("List %s items at the recycle bin root", resource),
		Long:  fmt.Sprintf("GET /recycle-bin/%s/root. Paginated; use 'bin children <id>' to descend into trashed subtrees.", resource),
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/recycle-bin/" + resource + "/root", Opts: api.RequestOptions{Params: params}},
			}
		},
	}))

	bin.AddCommand(cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "children <id>",
		Short: fmt.Sprintf("List children of a trashed %s item", resource),
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/recycle-bin/" + resource + "/children", Opts: api.RequestOptions{Params: cmdkit.WithParam(params, "parentId", args[0])}},
			}
		},
	}))

	bin.AddCommand(cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:   "original-parent <id>",
		Short: fmt.Sprintf("Get the original parent of a trashed %s item (the default restore target)", resource),
		Path: func(args []string) string {
			return api.JoinPath("/recycle-bin/"+resource+"/%s/original-parent", args[0])
		},
	}))

	bin.AddCommand(cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:   "delete <id>",
		Short: fmt.Sprintf("Permanently delete one %s item from the recycle bin", resource),
		Path: func(args []string) string {
			return api.JoinPath("/recycle-bin/"+resource+"/%s", args[0])
		},
	}))

	bin.AddCommand(recycleBinEmpty(deps, resource))

	return bin
}

func recycleBinEmpty(deps cmdkit.Dependencies, resource string) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "empty",
		Short: fmt.Sprintf("Permanently delete everything in the %s recycle bin", resource),
		Long:  fmt.Sprintf("DELETE /recycle-bin/%s. Destroys every trashed %s item; there is no undo.", resource, resource),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireForceOrDryRun(cmd, "permanently destroys every item in the recycle bin", force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), "/recycle-bin/"+resource, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "emptied", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm emptying the recycle bin")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// restoreFromBinCommand builds the restore-from-recycle-bin mutation shared
// by document and media: the restore target defaults to the item's original
// parent (looked up via the recycle-bin API), --to overrides it, and
// --to root restores at the tree root.
func restoreFromBinCommand(deps cmdkit.Dependencies, resource string, rootName string) *cobra.Command {
	var to string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: fmt.Sprintf("Restore a %s item from the recycle bin", resource),
		Long:  fmt.Sprintf("PUT /recycle-bin/%s/{id}/restore. The restore target defaults to the item's original parent (looked up via the recycle-bin API); pass --to for a different parent, or --to root to restore at the %s.", resource, rootName),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			var target any
			switch {
			case strings.EqualFold(strings.TrimSpace(to), "root"):
				target = nil
			case strings.TrimSpace(to) != "":
				target = map[string]any{"id": to}
			default:
				original, err := deps.Client.Get(ctx, api.JoinPath("/recycle-bin/"+resource+"/%s/original-parent", args[0]), api.RequestOptions{})
				if err != nil {
					// A 404 means the recycle-bin API is absent (older
					// servers, where the legacy restore needs no target
					// anyway) or the lookup has nothing to report — either
					// way the restore call itself gives the real answer.
					var apiErr *api.APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
						return fmt.Errorf("could not resolve the original parent (pass --to <parent-id> or --to root): %w", err)
					}
				}
				if id := extractResultID(original); id != "" {
					target = map[string]any{"id": id}
				}
			}

			result, err := cmdkit.MutateWithFallback(ctx, deps.Client, map[string]any{"target": target}, api.RequestOptions{DryRun: dryRun},
				cmdkit.MutationCandidate{Method: "PUT", Path: api.JoinPath("/recycle-bin/"+resource+"/%s/restore", args[0])},
				cmdkit.MutationCandidate{Method: "POST", Path: api.JoinPath("/"+resource+"/%s/restore", args[0])},
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "restored", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Restore target parent ID, or 'root' (defaults to the original parent)")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}
