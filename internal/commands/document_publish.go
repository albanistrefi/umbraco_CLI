package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Publish and unpublish operations for the document command group,
// including the invariant-content race retry.

func documentPublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var idsCSV string
	var fromFile string
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "publish <id> | --ids a,b,c | --from-file ids.txt",
		Short: "Publish a document (or several with --ids/--from-file)",
		Long: "PUT /document/{id}/publish. With --ids or --from-file the same publish runs for every listed document in sequence, one result row per document (id, name, publish status, error); " +
			"a failure on one document does not stop the rest, and the command exits 4 when any row failed. Multi-document runs require --force or --dry-run; a dry-run shows the planned requests for the first " + fmt.Sprint(documentBatchPlanWindow) + " documents and counts the rest. " +
			"To change values and publish in one pass use 'document update --ids … --merge-json … --save-and-publish'.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := resolveDocumentBatchTargets(cmd, args, idsCSV, fromFile, force, dryRun, "publishes every listed document")
			if err != nil {
				return err
			}
			if ids != nil {
				if strings.TrimSpace(jsonPayload) != "" {
					return fmt.Errorf("--json cannot be combined with --ids/--from-file; use --culture")
				}
				return printDocumentBatch(cmd, deps, executeDocumentBatch(cmd.Context(), deps.Client, documentBatchOptions{IDs: ids, Publish: true, Culture: culture, DryRun: dryRun}))
			}
			body, err := documentPublishBody(jsonPayload, culture)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/publish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "published", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Publish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	addDocumentBatchFlags(cmd, &idsCSV, &fromFile, &force, "publish")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// invariantRaceMaxAttempts is the upper bound on retries for the spurious
// "culture for invariant content" 400 that the Management API throws under
// rapid back-to-back save-and-publish loops. The error is timing-dependent
// and clears on retry; 4 attempts with exponential-ish backoff matches what
// the bug report saw work in practice.
const invariantRaceMaxAttempts = 4

// publishWithInvariantRaceRetry PUTs the publish body and retries on the
// specific 400 "culture for invariant content" error that Umbraco intermittently
// returns under tight save-and-publish loops on invariant content. The
// payload is valid (verified via --dry-run in the bug report) — the same
// request succeeds on retry, so the retry is the right correctness-preserving
// workaround at the CLI layer. Other 400s are surfaced immediately.
func publishWithInvariantRaceRetry(ctx context.Context, client *api.Client, id string, body map[string]any, opts api.RequestOptions) (any, error) {
	path := api.JoinPath("/document/%s/publish", id)
	var lastErr error
	for attempt := 0; attempt < invariantRaceMaxAttempts; attempt++ {
		result, err := client.Put(ctx, path, body, opts)
		if err == nil {
			return result, nil
		}
		if opts.DryRun || !isInvariantContentRaceError(err) || attempt == invariantRaceMaxAttempts-1 {
			return nil, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(invariantRaceBackoffs[attempt]):
		}
	}
	return nil, lastErr
}

// isInvariantContentRaceError matches the spurious 400 the Management API
// returns under the save-and-publish race. The payload looks like
// {"detail":"One or more property values specify a culture for an [invariant content]"}.
// Substring-match on "invariant content" inside the rendered error is robust
// to message phrasing tweaks without false-positiving on unrelated 400s.
func isInvariantContentRaceError(err error) bool {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 {
		return false
	}
	return strings.Contains(apiErr.Error(), "invariant content")
}

func documentPublishBody(jsonPayload string, culture string) (map[string]any, error) {
	if strings.TrimSpace(jsonPayload) != "" {
		return parsePayload(jsonPayload)
	}
	// The publish model requires publishSchedules on every spec version the
	// CLI has vendored; "cultures" belongs to the unpublish model and the
	// server rejects it here.
	if strings.TrimSpace(culture) != "" {
		return map[string]any{
			"publishSchedules": []any{
				map[string]any{"culture": culture},
			},
		}, nil
	}
	return map[string]any{
		"publishSchedules": []any{
			map[string]any{"culture": nil},
		},
	}, nil
}

func documentUnpublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "unpublish <id>",
		Short: "Unpublish a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			var err error
			if jsonPayload != "" {
				body, err = parsePayload(jsonPayload)
			} else if culture != "" {
				body = map[string]any{"cultures": []any{culture}}
			} else {
				body = map[string]any{}
			}
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/unpublish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "unpublished", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Unpublish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
