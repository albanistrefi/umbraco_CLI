package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterPublishedCache(root *cobra.Command, deps Dependencies) {
	cache := &cobra.Command{Use: "published-cache", Short: "Published content cache operations"}
	cache.AddCommand(readOnlyEndpointWithFallback(deps, "status", "Get published cache rebuild status", "/published-cache/rebuild/status", "/published-cache/status"))
	cache.AddCommand(publishedCacheRebuild(deps))
	cache.AddCommand(publishedCacheReload(deps))
	root.AddCommand(cache)
}

func publishedCacheRebuild(deps Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	var wait bool
	var timeout time.Duration
	var pollInterval time.Duration
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the published content cache from the database",
		Long:  "POST /published-cache/rebuild. Rebuilds the published content cache from the database — the standard fix for stale published content. Expensive on large sites; with --wait, polls the rebuild status until isRebuilding clears or --timeout elapses (mirroring 'indexer rebuild --wait').",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "rebuilds the entire published cache and is expensive on large sites", force, dryRun); err != nil {
				return err
			}
			if dryRun && wait {
				return fmt.Errorf("--dry-run does not trigger a rebuild, so --wait has nothing to poll for; pass one or the other")
			}

			ctx := cmd.Context()
			result, err := deps.Client.Post(ctx, "/published-cache/rebuild", nil, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if !wait {
				return printMutationResult(cmd, deps, "rebuilding", result, dryRun)
			}

			deadline := time.Now().Add(timeout)
			for {
				// Same fallback list as the status command: older servers
				// only expose the legacy route.
				statusPayload, err := getWithFallback(ctx, deps.Client,
					getRequestCandidate{Path: "/published-cache/rebuild/status", Opts: api.RequestOptions{}},
					getRequestCandidate{Path: "/published-cache/status", Opts: api.RequestOptions{}},
				)
				if err != nil {
					return fmt.Errorf("polling rebuild status failed: %w", err)
				}
				rebuilding, known := publishedCacheIsRebuilding(statusPayload)
				if !known {
					// Legacy servers answer with a plain string that has no
					// isRebuilding flag; burning the whole timeout would just
					// delay the same answer.
					return fmt.Errorf("the rebuild was triggered, but this server does not expose the isRebuilding flag so --wait cannot poll it; check 'published-cache status' manually")
				}
				if !rebuilding {
					return printResult(cmd, deps, map[string]any{
						"rebuilt": true,
						"waited":  time.Since(deadline.Add(-timeout)).String(),
					})
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("published cache was still rebuilding after %s; try increasing --timeout or check 'published-cache status'", timeout)
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
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll the rebuild status after triggering until isRebuilding clears or --timeout elapses")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "How long to wait when --wait is set (e.g. 30s, 2m)")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", time.Second, "How often to poll when --wait is set")
	return cmd
}

// publishedCacheIsRebuilding reads the modern status shape
// {"isRebuilding": bool}; the second return reports whether the payload was
// understood at all (older servers return a plain string here).
func publishedCacheIsRebuilding(payload any) (bool, bool) {
	object, ok := payload.(map[string]any)
	if !ok {
		return false, false
	}
	rebuilding, ok := object["isRebuilding"].(bool)
	return rebuilding, ok
}

func publishedCacheReload(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload the in-memory published cache",
		Long:  "POST /published-cache/reload. Reloads the in-memory published cache from the cache store without a database rebuild; much cheaper than 'published-cache rebuild'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), "/published-cache/reload", nil, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "reloaded", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
