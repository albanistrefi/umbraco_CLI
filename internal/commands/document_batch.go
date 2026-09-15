package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Multi-document publish/update behind 'document publish' and 'document
// update': --ids a,b,c or --from-file ids.txt instead of one positional id.
// Field report: 124 pages took 372 separate CLI invocations from a script;
// this runs the same per-document requests sequentially and reports one row
// per document.

// documentBatchPlanWindow is how many documents a --dry-run plans in full
// (with the exact requests); the rest are counted.
const documentBatchPlanWindow = 3

// documentBatchFailedError makes a batch with failed rows exit non-zero
// after the rows have been printed, keeping the documented code of the
// underlying failures: 3 when any row hit an auth failure (the whole run is
// unusable), otherwise the API code 4 when any row failed on a request,
// otherwise 1 (local failures such as a backup file that could not be
// written).
type documentBatchFailedError struct {
	failed int
	total  int
	code   int
}

func (e documentBatchFailedError) Error() string {
	return fmt.Sprintf("%d of %d documents failed; see the items above", e.failed, e.total)
}

func (e documentBatchFailedError) ExitCode() int { return e.code }

// batchExitCodeFor folds one row's error into the run's exit code.
func batchExitCodeFor(current int, err error) int {
	var coder interface{ ExitCode() int }
	code := 1
	if errors.As(err, &coder) {
		code = coder.ExitCode()
	}
	switch {
	case code == 3 || current == 3:
		return 3
	case code > current:
		return code
	default:
		return current
	}
}

type documentBatchOptions struct {
	IDs []string
	// Update, when true, PUTs each document with FullBody or the merge of
	// MergePatch into the current document.
	Update     bool
	FullBody   map[string]any
	MergePatch map[string]any
	// Publish, when true, publishes each document (after the update when
	// both are set; atomic update-and-publish on 18.1+).
	Publish bool
	Culture string
	// PublishBody, when set, is the full publish payload (--json) used
	// instead of the --culture shortcut.
	PublishBody map[string]any
	// Backup, when set, saves each document before its PUT ("auto" or a
	// directory).
	Backup string
	DryRun bool
}

type documentBatchRow struct {
	err     error
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Update  string `json:"update,omitempty"`
	Publish string `json:"publish,omitempty"`
	Backup  string `json:"backup,omitempty"`
	Error   string `json:"error,omitempty"`
	Plan    any    `json:"plan,omitempty"`
}

type documentBatchResult struct {
	exitCode  int
	DryRun    bool               `json:"dryRun"`
	Total     int                `json:"total"`
	Updated   int                `json:"updated"`
	Published int                `json:"published"`
	Skipped   int                `json:"skipped"`
	Failed    int                `json:"failed"`
	Planned   int                `json:"planned,omitempty"`
	Items     []documentBatchRow `json:"items"`
}

// addDocumentBatchFlags registers the multi-id flags shared by publish and
// update. --ids follows the repo-wide comma-separated GUID convention;
// --from-file reads one id per line.
func addDocumentBatchFlags(cmd *cobra.Command, ids *string, fromFile *string, force *bool, verb string) {
	cmd.Flags().StringVar(ids, "ids", "", fmt.Sprintf("Comma-separated document GUIDs to %s instead of one positional id (runs sequentially; one result row per document)", verb))
	cmd.Flags().StringVar(fromFile, "from-file", "", "Path to a file with document GUIDs, one per line (combines with --ids)")
	cmd.Flags().BoolVar(force, "force", false, "Confirm a multi-document run when not using --dry-run")
}

// resolveDocumentBatchTargets validates the positional-vs-batch contract:
// exactly one of a positional id or --ids/--from-file. It returns nil when
// the command should run its single-document path.
func resolveDocumentBatchTargets(cmd *cobra.Command, args []string, idsCSV string, fromFile string, force bool, dryRun bool, consequence string) ([]string, error) {
	batch := strings.TrimSpace(idsCSV) != "" || strings.TrimSpace(fromFile) != ""
	if !batch {
		if len(args) != 1 {
			return nil, fmt.Errorf("%s requires a document id, or --ids/--from-file for several", cmd.CommandPath())
		}
		return nil, nil
	}
	if len(args) != 0 {
		return nil, fmt.Errorf("%s takes either a positional id or --ids/--from-file, not both", cmd.CommandPath())
	}
	ids, err := loadDocumentIDs(uniqueCSV(idsCSV), fromFile)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("--ids/--from-file resolved to no document ids")
	}
	if err := requireForceOrDryRun(cmd, consequence, force, dryRun); err != nil {
		return nil, err
	}
	return ids, nil
}

// executeDocumentBatch runs the per-document requests sequentially. A failure
// on one document is recorded on its row and the run continues.
func executeDocumentBatch(ctx context.Context, client *api.Client, opts documentBatchOptions) documentBatchResult {
	result := documentBatchResult{DryRun: opts.DryRun, Total: len(opts.IDs), Items: make([]documentBatchRow, 0, len(opts.IDs))}
	for index, id := range opts.IDs {
		row := documentBatchRow{ID: id}
		if opts.DryRun && index >= documentBatchPlanWindow {
			// Beyond the plan window a dry-run only counts, so a 124-page
			// rehearsal does not turn into 124 GETs.
			if opts.Update {
				row.Update = "planned"
			}
			if opts.Publish {
				row.Publish = "planned"
			}
			result.Planned++
			result.Items = append(result.Items, row)
			continue
		}
		documentBatchOne(ctx, client, opts, &row)
		switch row.Update {
		case "updated":
			result.Updated++
		case "skipped":
			result.Skipped++
		}
		if row.Publish == "published" {
			result.Published++
		}
		if row.Error != "" {
			result.Failed++
			result.exitCode = batchExitCodeFor(result.exitCode, row.err)
		} else if opts.DryRun {
			result.Planned++
		}
		result.Items = append(result.Items, row)
	}
	return result
}

func documentBatchOne(ctx context.Context, client *api.Client, opts documentBatchOptions, row *documentBatchRow) {
	path := api.JoinPath("/document/%s", row.ID)
	reqOpts := api.RequestOptions{DryRun: opts.DryRun}
	plan := map[string]any{}
	fail := func(stage string, err error) {
		row.err = err
		row.Error = err.Error()
		if stage == "update" {
			row.Update = "failed"
			if opts.Publish {
				row.Publish = "not-run"
			}
		} else {
			row.Publish = "failed"
		}
	}

	// One read per document: the name for the row, and the base for a
	// merge. --json full bodies still read, so a typo'd id fails here with
	// a 404 instead of creating confusion later.
	current, err := fetchObject(ctx, client, path, api.RequestOptions{})
	if err != nil {
		if opts.Update {
			fail("update", err)
		} else {
			fail("publish", err)
		}
		return
	}
	row.Name = entityDisplayName(current)

	var body map[string]any
	if opts.Update {
		if opts.FullBody != nil {
			body = opts.FullBody
		} else {
			body = mergeAliasPayload(current, opts.MergePatch)
			if reflect.DeepEqual(current, body) {
				row.Update = "skipped"
				body = nil
			}
		}
		if body != nil && opts.Backup != "" && !opts.DryRun {
			target := ""
			if opts.Backup != backupAutoValue {
				target = strings.TrimRight(opts.Backup, "/") + "/" + resolveBackupPath(backupAutoValue, "document", row.ID)
			}
			backupFile, err := writeBackup(resolveBackupPath(target, "document", row.ID), "document", row.ID, path, current, nil)
			if err != nil {
				fail("update", err)
				return
			}
			row.Backup = backupFile
		}
	}

	if opts.Update && opts.Publish && body != nil {
		atomic := make(map[string]any, len(body)+1)
		for k, v := range body {
			atomic[k] = v
		}
		if _, ok := atomic["culturesToPublish"]; !ok {
			atomic["culturesToPublish"] = culturesToPublishList(opts.Culture)
		}
		atomicResult, err := client.Put(ctx, api.JoinPath("/document/%s/update-and-publish", row.ID), atomic, reqOpts)
		if err == nil {
			row.Update, row.Publish = stateWord(opts.DryRun, "updated"), stateWord(opts.DryRun, "published")
			if opts.DryRun {
				plan["updateAndPublish"] = atomicResult
				row.Plan = plan
			}
			return
		}
		if !isAPIStatus(err, http.StatusNotFound) {
			fail("update", err)
			return
		}
		// Older server: fall through to the two-step path.
	}

	if opts.Update && body != nil {
		putResult, err := client.Put(ctx, path, body, reqOpts)
		if err != nil {
			fail("update", err)
			return
		}
		row.Update = stateWord(opts.DryRun, "updated")
		if opts.DryRun {
			plan["update"] = putResult
		}
	}
	if opts.Publish {
		publishBody := opts.PublishBody
		if publishBody == nil {
			publishBody, err = documentPublishBody("", opts.Culture)
			if err != nil {
				fail("publish", err)
				return
			}
		}
		publishResult, err := publishWithInvariantRaceRetry(ctx, client, row.ID, publishBody, reqOpts)
		if err != nil {
			fail("publish", err)
			return
		}
		row.Publish = stateWord(opts.DryRun, "published")
		if opts.DryRun {
			plan["publish"] = publishResult
		}
	}
	if opts.DryRun && len(plan) > 0 {
		row.Plan = plan
	}
}

func stateWord(dryRun bool, done string) string {
	if dryRun {
		return "planned"
	}
	return done
}

// printDocumentBatch prints the rows and turns failures into exit 4.
func printDocumentBatch(cmd *cobra.Command, deps Dependencies, result documentBatchResult) error {
	if err := printResult(cmd, deps, result); err != nil {
		return err
	}
	if result.Failed > 0 {
		return documentBatchFailedError{failed: result.Failed, total: result.Total, code: result.exitCode}
	}
	return nil
}
