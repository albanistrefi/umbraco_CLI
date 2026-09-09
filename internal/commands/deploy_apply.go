package commands

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// deployApplyFailedError is returned when one or more planned writes failed
// (or were applied but still drift); it shares exit code 4 with API errors
// because that is what a failed write is.
type deployApplyFailedError struct {
	failed     int
	drifted    int
	planErrors int
	dryRun     bool
}

func (e deployApplyFailedError) Error() string {
	if e.planErrors > 0 {
		if e.dryRun {
			return fmt.Sprintf("deploy apply: %d artifact(s) could not be planned (parse, mapping, or comparison failure); a forced run would not execute until they are fixed or --continue-on-error is passed", e.planErrors)
		}
		return fmt.Sprintf("deploy apply: %d artifact(s) could not be planned; %d write(s) failed, %d applied but still drifted", e.planErrors, e.failed, e.drifted)
	}
	return fmt.Sprintf("deploy apply: %d write(s) failed, %d applied but still drifted", e.failed, e.drifted)
}
func (deployApplyFailedError) ExitCode() int { return 4 }

// udaPlanEntry is one planned (or executed) write.
type udaPlanEntry struct {
	Order    int            `json:"order"`
	File     string         `json:"file"`
	Kind     string         `json:"kind"`
	Udi      string         `json:"udi,omitempty"`
	Name     string         `json:"name,omitempty"`
	Action   string         `json:"action"` // create | update | skip | unsupported | error
	Reason   string         `json:"reason,omitempty"`
	Diffs    []string       `json:"diffs,omitempty"`
	Method   string         `json:"method,omitempty"`
	Path     string         `json:"path,omitempty"`
	Body     map[string]any `json:"body,omitempty"`
	Deferred []string       `json:"deferredReferences,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	// Execution outcome
	Result   string   `json:"result,omitempty"` // applied | applied-drifted | failed | not-run
	Backup   string   `json:"backup,omitempty"`
	Error    string   `json:"error,omitempty"`
	AfterDif []string `json:"remainingDiffs,omitempty"`

	artifact udaArtifact
	spec     udaWriteSpec
	create   bool
}

func deployApply(deps Dependencies) *cobra.Command {
	var udaDir string
	var kinds []string
	var dryRun bool
	var force bool
	var backupDir string
	var noBackup bool
	var continueOnError bool
	var includeBodies bool
	var concurrency int

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply local .uda deploy artifacts to the environment (Deploy's \"Update schema\" from the CLI)",
		Long: `Makes the target environment's schema match the Umbraco Deploy artifacts in --uda-dir: the write side of 'deploy status'. Every artifact is first compared exactly as 'deploy status' does; in-sync artifacts are skipped, missing ones are created (with the artifact's GUID, so references keep resolving), drifted ones are updated with a full replacement built from the artifact — the same semantics as Deploy's schema pass.

Writes run in dependency order (Deploy's Ordering dependencies, then kind precedence: languages → folders → data types → templates → member/media/document types → member groups). Content-type references to items that come later in the same plan (allowed children, compositions) are deferred and re-applied in a fix-up pass once the referenced types exist. After every write the entity is re-read and compared again; a write the server accepted but that still drifts is reported as such, never as success.

Supported kinds: language, data-type(+folders), document-type/media-type/member-type(+folders), template, member-group. Relation types and Automate artifacts are read-only in the Management API and are listed as unsupported. Mutating: refuses to run without --dry-run (plan only, no writes; the plan includes the exact request bodies with --bodies) or --force. Every updated entity is backed up first (see --backup-dir).

Artifacts that cannot be parsed, mapped, or compared are plan errors: they exit 4 (also under --dry-run) and block a forced run entirely unless --continue-on-error is passed, so a partial apply never exits cleanly. Folder parents cannot be compared or moved through the Management API and are flagged as warnings.

Exit 0 when every planned write applied and verified; exit 4 when any artifact could not be planned or any write failed or still drifts; exit 7 is not used here (that is 'deploy status').`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && !force {
				return fmt.Errorf("deploy apply mutates the environment's schema; rehearse with --dry-run or confirm with --force")
			}
			if concurrency < 1 {
				return fmt.Errorf("--concurrency must be at least 1")
			}
			ctx := cmd.Context()
			artifacts, err := loadUdaArtifacts(udaDir, kinds)
			if err != nil {
				return err
			}
			if len(artifacts) == 0 {
				return fmt.Errorf("no .uda artifacts found in %s (pass --uda-dir pointing at the site repo's umbraco/Deploy/Revision)", udaDir)
			}
			if _, err := deps.Client.Get(ctx, "/server/status", api.RequestOptions{}); err != nil {
				return fmt.Errorf("deploy apply cannot reach the target environment: %w", err)
			}

			statuses := compareArtifacts(ctx, deps, artifacts, nil, concurrency)
			plan := buildApplyPlan(artifacts, statuses)
			planErrors := 0
			for i := range plan {
				if plan[i].Action == "error" {
					planErrors++
				}
			}

			// Plan errors (unparseable or unmappable artifacts, failed
			// comparisons) block execution: a partial apply that exits
			// cleanly would hide them. --continue-on-error applies anyway.
			if !dryRun && planErrors > 0 && !continueOnError {
				for i := range plan {
					if plan[i].Action == "create" || plan[i].Action == "update" {
						plan[i].Result = "not-run"
					}
				}
			} else if !dryRun {
				resolvedBackupDir := ""
				if !noBackup {
					resolvedBackupDir = backupDir
					if resolvedBackupDir == "" {
						resolvedBackupDir = filepath.Join(".umbraco-deploy-backup", time.Now().UTC().Format("20060102T150405Z"))
					}
				}
				executeApplyPlan(ctx, deps, plan, resolvedBackupDir, continueOnError)
			}

			summary := map[string]int{}
			for i := range plan {
				summary[plan[i].Action]++
				if plan[i].Result != "" {
					summary[plan[i].Result]++
				}
			}
			out := make([]udaPlanEntry, len(plan))
			copy(out, plan)
			if !includeBodies {
				for i := range out {
					out[i].Body = nil
				}
			}
			payload := map[string]any{
				"udaDir": udaDir,
				"dryRun": dryRun,
				"plan":   out,
				"summary": map[string]any{
					"total":          len(plan),
					"create":         summary["create"],
					"update":         summary["update"],
					"skip":           summary["skip"],
					"unsupported":    summary["unsupported"],
					"errors":         summary["error"],
					"applied":        summary["applied"],
					"appliedDrifted": summary["applied-drifted"],
					"failed":         summary["failed"],
					"notRun":         summary["not-run"],
				},
			}
			if err := printResult(cmd, deps, payload); err != nil {
				return err
			}
			if planErrors > 0 {
				return deployApplyFailedError{failed: summary["failed"], drifted: summary["applied-drifted"], planErrors: planErrors, dryRun: dryRun}
			}
			if !dryRun && (summary["failed"] > 0 || summary["applied-drifted"] > 0) {
				return deployApplyFailedError{failed: summary["failed"], drifted: summary["applied-drifted"]}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&udaDir, "uda-dir", filepath.Join("umbraco", "Deploy", "Revision"), "Directory holding the .uda artifacts")
	cmd.Flags().StringArrayVar(&kinds, "kind", nil, "Only apply these artifact kinds (Udi entity types, e.g. data-type, document-type; repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "Actually write to the environment (required without --dry-run)")
	cmd.Flags().StringVar(&backupDir, "backup-dir", "", "Directory for pre-change backups of every updated entity (default ./.umbraco-deploy-backup/<timestamp>)")
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Do not write pre-change backups")
	cmd.Flags().BoolVar(&continueOnError, "continue-on-error", false, "Keep applying after a failed write (default: stop, leaving later entries not-run)")
	cmd.Flags().BoolVar(&includeBodies, "bodies", false, "Include the exact request bodies in the plan output")
	cmd.Flags().IntVar(&concurrency, "concurrency", 8, "Maximum concurrent environment lookups during comparison")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// kindRank orders kinds so prerequisites land first when Deploy's explicit
// Ordering dependencies do not decide.
func kindRank(kind string) int {
	switch kind {
	case "language":
		return 0
	case "data-type-container", "document-type-container", "media-type-container", "member-type-container":
		return 1
	case "data-type":
		return 2
	case "template":
		return 3
	case "member-type":
		return 4
	case "media-type":
		return 5
	case "document-type":
		return 6
	case "member-group":
		return 7
	}
	return 9
}

// buildApplyPlan turns comparison results into an ordered write plan.
func buildApplyPlan(artifacts []udaArtifact, statuses []udaStatusResult) []udaPlanEntry {
	// compareArtifacts sorts its results by status; match them back to the
	// artifacts by file name.
	statusByFile := make(map[string]udaStatusResult, len(statuses))
	for _, status := range statuses {
		statusByFile[status.File] = status
	}
	entries := make([]udaPlanEntry, 0, len(artifacts))
	planned := map[string]int{} // GUID → index of a create/update entry
	for _, artifact := range artifacts {
		status := statusByFile[artifact.File]
		entry := udaPlanEntry{File: artifact.File, Kind: artifact.Kind, Udi: status.Udi, Name: status.Name, artifact: artifact}
		switch {
		case artifact.Err != nil || status.Status == "error":
			entry.Action, entry.Reason = "error", status.Reason
		case status.Status == "in-sync":
			entry.Action, entry.Reason = "skip", "in sync"
		case status.Status == "unknown":
			if _, ok := udaWriter(artifact.Kind); !ok {
				entry.Action, entry.Reason = "unsupported", "kind is read-only in the Management API or has no CLI writer: "+artifact.Kind
			} else {
				// A supported artifact whose comparison could not run is a
				// failure, not a skip: exiting 0 here would let automation
				// read an unperformed apply as a completed one.
				entry.Action, entry.Reason = "error", "comparison failed: "+status.Reason
			}
		default:
			spec, ok := udaWriter(artifact.Kind)
			if !ok {
				entry.Action, entry.Reason = "unsupported", "kind is read-only in the Management API or has no CLI writer: "+artifact.Kind
				break
			}
			create, update, err := spec.Map(artifact.Body)
			if err != nil {
				entry.Action, entry.Reason = "error", "cannot map artifact: "+err.Error()
				break
			}
			entry.spec = spec
			entry.Diffs = status.Diffs
			if status.Status == "missing-remote" {
				entry.Action, entry.create = "create", true
				entry.Method, entry.Path, entry.Body = http.MethodPost, spec.CreatePath, create
			} else {
				entry.Action = "update"
				entry.Method, entry.Path, entry.Body = http.MethodPut, udaUpdatePath(spec, artifact), update
				entry.Reason = "drifted: " + strings.Join(status.Diffs, ", ")
				if strings.HasSuffix(artifact.Kind, "-container") && artifact.Body["Parent"] != nil {
					entry.Warnings = append(entry.Warnings, "folder parent cannot be compared or changed through the Management API (folder model has no parent, no move route); verify placement in the backoffice")
				}
			}
			planned[udaKey(artifact.Kind, artifact.GUID)] = len(entries)
		}
		entries = append(entries, entry)
	}
	return orderApplyPlan(entries, planned)
}

// orderApplyPlan sorts writes so that every Ordering dependency that is
// itself being written lands first (Kahn over the planned set), breaking
// ties by kind rank then file name. Non-write entries keep their relative
// order after the writes.
func orderApplyPlan(entries []udaPlanEntry, planned map[string]int) []udaPlanEntry {
	writes := []int{}
	others := []int{}
	for i := range entries {
		if entries[i].Action == "create" || entries[i].Action == "update" {
			writes = append(writes, i)
		} else {
			others = append(others, i)
		}
	}
	indegree := map[int]int{}
	dependents := map[int][]int{}
	for _, i := range writes {
		indegree[i] += 0
		for _, dep := range artifactDependencyKeys(entries[i].artifact.Body, true) {
			if j, ok := planned[dep]; ok && j != i {
				indegree[i]++
				dependents[j] = append(dependents[j], i)
			}
		}
	}
	less := func(a, b int) bool {
		ra, rb := kindRank(entries[a].Kind), kindRank(entries[b].Kind)
		if ra != rb {
			return ra < rb
		}
		return entries[a].File < entries[b].File
	}
	ready := []int{}
	for _, i := range writes {
		if indegree[i] == 0 {
			ready = append(ready, i)
		}
	}
	ordered := []int{}
	for len(ready) > 0 {
		sort.Slice(ready, func(x, y int) bool { return less(ready[x], ready[y]) })
		next := ready[0]
		ready = ready[1:]
		ordered = append(ordered, next)
		for _, d := range dependents[next] {
			indegree[d]--
			if indegree[d] == 0 {
				ready = append(ready, d)
			}
		}
	}
	// Cycles (mutual Ordering deps) fall back to kind rank.
	if len(ordered) < len(writes) {
		seen := map[int]bool{}
		for _, i := range ordered {
			seen[i] = true
		}
		rest := []int{}
		for _, i := range writes {
			if !seen[i] {
				rest = append(rest, i)
			}
		}
		sort.Slice(rest, func(x, y int) bool { return less(rest[x], rest[y]) })
		ordered = append(ordered, rest...)
	}

	result := make([]udaPlanEntry, 0, len(entries))
	for _, i := range ordered {
		result = append(result, entries[i])
	}
	for _, i := range others {
		result = append(result, entries[i])
	}
	// Defer forward references among content types: a reference to a type
	// created later in this plan cannot be sent yet.
	position := map[string]int{}
	for i := range result {
		position[udaKey(result[i].Kind, result[i].artifact.GUID)] = i
	}
	for i := range result {
		result[i].Order = i + 1
		if result[i].Body != nil {
			result[i].Deferred = deferForwardReferences(result[i].Body, func(kind string, guid string) bool {
				j, ok := position[udaKey(kind, guid)]
				return ok && j > i && result[j].create
			})
		}
	}
	return result
}

// udaKey identifies an entity by kind and GUID: GUIDs are only unique per
// entity type (an Automate artifact may legitimately share a GUID with a
// document type), so references must never be resolved by GUID alone.
func udaKey(kind string, guid string) string {
	return kind + "/" + strings.ToLower(guid)
}

// deferForwardReferences strips content-type references (allowed children,
// compositions) that point at entities created later in the plan, returning
// the deferred GUIDs so a fix-up pass can restore them.
func deferForwardReferences(body map[string]any, isForward func(kind string, guid string) bool) []string {
	deferred := []string{}
	kindByRefKey := map[string]string{"documentType": "document-type", "mediaType": "media-type", "memberType": "member-type"}
	filter := func(key string, refKeys ...string) {
		items, ok := body[key].([]any)
		if !ok {
			return
		}
		kept := make([]any, 0, len(items))
		for _, item := range items {
			entry, _ := item.(map[string]any)
			forward := false
			for _, refKey := range refKeys {
				if ref, ok := entry[refKey].(map[string]any); ok {
					if id, _ := ref["id"].(string); id != "" && isForward(kindByRefKey[refKey], id) {
						forward = true
						deferred = append(deferred, id)
					}
				}
			}
			if !forward {
				kept = append(kept, item)
			}
		}
		body[key] = kept
	}
	filter("allowedDocumentTypes", "documentType")
	filter("allowedMediaTypes", "mediaType")
	filter("compositions", "documentType", "mediaType", "memberType")
	return deferred
}

// executeApplyPlan performs the writes in order, backing up updated
// entities, verifying each result, and running the fix-up pass for
// deferred references.
func executeApplyPlan(ctx context.Context, deps Dependencies, plan []udaPlanEntry, backupDir string, continueOnError bool) {
	stopped := false
	fixups := []int{}
	for i := range plan {
		entry := &plan[i]
		if entry.Action != "create" && entry.Action != "update" {
			continue
		}
		if stopped {
			entry.Result = "not-run"
			continue
		}
		if err := applyEntry(ctx, deps, entry, backupDir); err != nil {
			entry.Result = "failed"
			entry.Error = err.Error()
			if !continueOnError {
				stopped = true
			}
			continue
		}
		if len(entry.Deferred) > 0 {
			fixups = append(fixups, i)
			continue
		}
		verifyEntry(ctx, deps, entry)
	}
	if stopped {
		return
	}
	// Fix-up pass: re-send the full body (references included) now that
	// every planned entity exists.
	for _, i := range fixups {
		entry := &plan[i]
		_, update, err := entry.spec.Map(entry.artifact.Body)
		if err != nil {
			entry.Result, entry.Error = "failed", err.Error()
			continue
		}
		if _, err := deps.Client.Put(ctx, udaUpdatePath(entry.spec, entry.artifact), update, api.RequestOptions{}); err != nil {
			entry.Result, entry.Error = "failed", "fix-up of deferred references: "+err.Error()
			continue
		}
		verifyEntry(ctx, deps, entry)
	}
}

func applyEntry(ctx context.Context, deps Dependencies, entry *udaPlanEntry, backupDir string) error {
	if entry.create {
		_, err := deps.Client.Post(ctx, entry.Path, entry.Body, api.RequestOptions{})
		return err
	}
	if backupDir != "" {
		current, err := fetchObject(ctx, deps.Client, entry.Path, api.RequestOptions{})
		if err != nil {
			return fmt.Errorf("backup read failed: %w", err)
		}
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return err
		}
		file := filepath.Join(backupDir, sanitizeFileName(entry.Kind+"-"+entry.artifact.GUID, "entity")+".backup.json")
		saved, err := writeBackup(file, entry.Kind, entry.artifact.GUID, entry.Path, current, nil)
		if err != nil {
			return err
		}
		entry.Backup = saved
	}
	_, err := deps.Client.Put(ctx, entry.Path, entry.Body, api.RequestOptions{})
	return err
}

// verifyEntry re-reads the entity and reruns the status comparer, so
// "applied" means the environment now matches the artifact.
func verifyEntry(ctx context.Context, deps Dependencies, entry *udaPlanEntry) {
	fetchPath, comparer := udaComparer(entry.Kind)
	if comparer == nil {
		entry.Result = "applied"
		return
	}
	remote, err := fetchObject(ctx, deps.Client, api.JoinPath(fetchPath, entry.artifact.GUID), api.RequestOptions{})
	if err != nil {
		entry.Result, entry.Error = "failed", "the write was accepted but re-reading the entity failed: "+err.Error()
		return
	}
	diffs := comparer(entry.artifact.Body, remote)
	if len(diffs) > 0 {
		sort.Strings(diffs)
		entry.Result, entry.AfterDif = "applied-drifted", diffs
		return
	}
	entry.Result = "applied"
}
