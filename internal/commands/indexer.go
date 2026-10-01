package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterIndexer(root *cobra.Command, deps Dependencies) {
	indexer := &cobra.Command{Use: "indexer", Short: "Examine search index operations"}
	indexer.AddCommand(indexerList(deps))
	indexer.AddCommand(getCommand(deps, getSpec{
		Use:   "get <index-name>",
		Short: "Get one Examine index (health, document count, fields)",
		Path: func(args []string) string {
			return api.JoinPath("/indexer/%s", args[0])
		},
	}))
	indexer.AddCommand(indexerRebuild(deps))
	root.AddCommand(indexer)
}

func indexerList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List Examine indexes with health and document counts",
		Long:  "GET /indexer. The classic first stop when search results are missing or stale: healthStatus.status of Rebuilding, Unhealthy, or Corrupt explains it.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: "/indexer", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func indexerRebuild(deps Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	var wait bool
	var timeout time.Duration
	var pollInterval time.Duration
	cmd := &cobra.Command{
		Use:   "rebuild <index-name>",
		Short: "Rebuild an Examine index",
		Long:  "POST /indexer/{indexName}/rebuild. Rebuilds the index from scratch — the standard fix for missing or stale search results. Expensive on large indexes; with --wait, polls the index until healthStatus leaves Rebuilding or --timeout elapses.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "rebuilds the index from scratch and is expensive on large indexes", force, dryRun); err != nil {
				return err
			}
			if dryRun && wait {
				return fmt.Errorf("--dry-run does not trigger a rebuild, so --wait has nothing to poll for; pass one or the other")
			}

			ctx := cmd.Context()
			result, err := deps.Client.Post(ctx, api.JoinPath("/indexer/%s/rebuild", args[0]), nil, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if !wait {
				return printMutationResult(cmd, deps, "rebuilding", result, dryRun)
			}

			deadline := time.Now().Add(timeout)
			for {
				indexPayload, err := deps.Client.Get(ctx, api.JoinPath("/indexer/%s", args[0]), api.RequestOptions{})
				if err != nil {
					return fmt.Errorf("polling index after rebuild failed: %w", err)
				}
				status := indexerHealthStatus(indexPayload)
				if strings.EqualFold(status, "Healthy") {
					return printResult(cmd, deps, map[string]any{
						"rebuilt": true,
						"status":  status,
						"waited":  time.Since(deadline.Add(-timeout)).String(),
					})
				}
				// Corrupt/Unhealthy are terminal: the rebuild finished but
				// search is still broken, so exiting 0 would let automation
				// proceed on a bad index. Only Rebuilding is worth waiting on.
				if status != "" && !strings.EqualFold(status, "Rebuilding") {
					return fmt.Errorf("index %s finished rebuilding in status %s — search is still broken; check 'indexer get %s' and the server logs", args[0], status, args[0])
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("index %s did not leave Rebuilding within %s (last status: %s); try increasing --timeout or check 'indexer get %s'", args[0], timeout, status, args[0])
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(pollInterval):
				}
			}
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the rebuild")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll the index after triggering the rebuild until healthStatus leaves Rebuilding or --timeout elapses")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "How long to wait when --wait is set (e.g. 30s, 2m)")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", time.Second, "How often to poll when --wait is set")
	return cmd
}
