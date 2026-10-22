package deploy

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"umbraco-cli/internal/commands/cmdkit"
)

// The schema tracker is deploy watch's --uda-dir: the artifacts that are
// drifted or missing on the target at baseline are what this deploy should
// change, so they are re-checked after landing until they are in sync or
// Umbraco Deploy's schema pass has ended.
//
// Facts it is built on, from the 01-10 live deploy and earlier ones:
//   - Deploy's schema pass is the disk work item (DiskReadWorkItem) and runs
//     after the new process is serving (two minutes after MainDom on 01-10);
//     DiskService logs its end as Work Status "Completed". An artifact still
//     at baseline before that is expected, not a failure.
//   - Automate artifacts compare as unknown where the Automate API is not
//     reachable (non-live environments behind basic auth); unknown is never
//     counted as in sync or as failed.
//   - Deploy can skip artifacts without failing (15-09: four Automate
//     artifacts new to live stayed missing), so a pass that completes is
//     not proof every artifact landed: only a re-check is.

// schemaUnknownRetries is how many re-checks after the pass ended an
// unknown artifact gets before it is reported as unknown.
const schemaUnknownRetries = 3

const schemaRecheckConcurrency = 8

var deployWorkStatusPattern = regexp.MustCompile(`Work Status "?([A-Za-z]+)"?`)

// deployWatchSchemaUnconfirmedError is the exit when the environment was
// verified but artifacts this deploy should have changed are still drifted
// or missing after Deploy's schema pass ended, or the pass did not complete.
// Exit 7 is the documented "found drifted or missing entities" code.
type deployWatchSchemaUnconfirmedError struct {
	unconfirmed int
	workStatus  string
}

func (e deployWatchSchemaUnconfirmedError) Error() string {
	if e.workStatus != "" && !strings.EqualFold(e.workStatus, "Completed") {
		return fmt.Sprintf("deploy watch verified the environment, but Umbraco Deploy's schema pass ended with work status %q and %d tracked artifacts are still drifted or missing", e.workStatus, e.unconfirmed)
	}
	return fmt.Sprintf("deploy watch verified the environment, but %d artifacts this deploy should have changed are still drifted or missing after Umbraco Deploy's schema pass ended", e.unconfirmed)
}
func (deployWatchSchemaUnconfirmedError) ExitCode() int { return 7 }

type trackedArtifact struct {
	artifact udaArtifact
	baseline udaStatusResult
	current  udaStatusResult
	// unknownAfterEnd counts re-checks after the pass ended that could not
	// compare the artifact.
	unknownAfterEnd int
}

type schemaTracker struct {
	deps   cmdkit.Dependencies
	udaDir string
	// pass is Umbraco Deploy's schema pass as read from the log; its
	// detection is off without a baseline process id, since the pass's
	// entries could not be told apart from those written before the deploy.
	pass *deployPass

	tracked      []*trackedArtifact
	unverifiable []udaStatusResult
	inSync       int

	checkedAfterEnd  bool
	waitingAnnounced bool
}

// newSchemaTracker reads the artifacts and compares each against the
// target, before the watch arms. An unreadable directory or one with no
// artifacts is a usage error.
func newSchemaTracker(ctx context.Context, deps cmdkit.Dependencies, udaDir string, pass *deployPass) (*schemaTracker, error) {
	artifacts, err := loadUdaArtifacts(udaDir, nil)
	if err != nil {
		return nil, err
	}
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no .uda artifacts found in %s (pass --uda-dir pointing at the site repo's umbraco/Deploy/Revision)", udaDir)
	}
	byFile := map[string]udaArtifact{}
	for _, artifact := range artifacts {
		byFile[artifact.File] = artifact
	}
	tracker := &schemaTracker{deps: deps, udaDir: udaDir, pass: pass}
	for _, result := range compareArtifacts(ctx, deps, artifacts, nil, schemaRecheckConcurrency) {
		switch result.Status {
		case "drifted", "missing-remote":
			tracker.tracked = append(tracker.tracked, &trackedArtifact{artifact: byFile[result.File], baseline: result, current: result})
		case "in-sync":
			tracker.inSync++
		default:
			tracker.unverifiable = append(tracker.unverifiable, result)
		}
	}
	return tracker, nil
}

func (s *schemaTracker) baselineDetail() map[string]any {
	tracking := make([]any, 0, len(s.tracked))
	for _, item := range s.tracked {
		tracking = append(tracking, schemaArtifactRef(item.baseline))
	}
	unverifiable := make([]any, 0, len(s.unverifiable))
	for _, result := range s.unverifiable {
		unverifiable = append(unverifiable, map[string]any{"file": result.File, "kind": result.Kind, "status": result.Status, "reason": result.Reason})
	}
	detail := map[string]any{
		"udaDir":       s.udaDir,
		"tracking":     tracking,
		"inSync":       s.inSync,
		"unverifiable": unverifiable,
	}
	if !s.pass.detection {
		detail["passDetection"] = "off: no baseline process id to tell the schema pass's log entries from older ones; verified waits for every tracked artifact to be in sync"
	}
	return detail
}

func schemaArtifactRef(result udaStatusResult) map[string]any {
	ref := map[string]any{"file": result.File, "kind": result.Kind, "status": result.Status}
	if result.Name != "" {
		ref["name"] = result.Name
	}
	if result.Udi != "" {
		ref["udi"] = result.Udi
	}
	return ref
}

// watchSchemaEvent is a per-artifact status change, type "schema".
type watchSchemaEvent struct {
	Timestamp string   `json:"timestamp"`
	Type      string   `json:"type"`
	File      string   `json:"file"`
	Kind      string   `json:"kind"`
	Udi       string   `json:"udi,omitempty"`
	Name      string   `json:"name,omitempty"`
	Status    string   `json:"status"`
	Previous  string   `json:"previous"`
	Diffs     []string `json:"diffs,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// watchSchemaPassEvent reports Umbraco Deploy's schema pass, type
// "schema-pass": waiting (everything else is clear), started, ended.
type watchSchemaPassEvent struct {
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// A recheck compares the tracked artifacts that are not in sync yet, once
// the deploy has landed, and books the status changes. An unknown result
// (the API unavailable mid-restart) keeps the last definitive status. It
// runs in three steps so the comparison holds no lock (watchRun.recheckSchema).

// recheckPlan lists the artifacts a recheck compares (none before landing
// or once the schema has settled) and whether the pass had already ended
// when it started: only a comparison begun after the end counts as a
// re-check after it.
func (s *schemaTracker) recheckPlan(landed bool) ([]udaArtifact, bool) {
	if !landed || !s.pending() {
		return nil, false
	}
	pending := make([]udaArtifact, 0)
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			pending = append(pending, item.artifact)
		}
	}
	return pending, s.pass.ended
}

// recheckCompare compares the planned artifacts against the target. It
// reads no tracker state, so it needs no lock.
func (s *schemaTracker) recheckCompare(ctx context.Context, pending []udaArtifact) []udaStatusResult {
	return compareArtifacts(ctx, s.deps, pending, nil, schemaRecheckConcurrency)
}

// recheckApply books a comparison's results and returns the status changes.
func (s *schemaTracker) recheckApply(results []udaStatusResult, endedBefore bool) []any {
	byFile := map[string]*trackedArtifact{}
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			byFile[item.artifact.File] = item
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	events := make([]any, 0)
	for _, result := range results {
		item := byFile[result.File]
		if item == nil {
			continue
		}
		if result.Status == "unknown" || result.Status == "error" {
			if endedBefore {
				item.unknownAfterEnd++
				if item.unknownAfterEnd >= schemaUnknownRetries && item.current.Status != "unknown" {
					previous := item.current.Status
					item.current = result
					events = append(events, schemaChangeEvent(now, result, previous))
				}
			}
			continue
		}
		item.unknownAfterEnd = 0
		if result.Status != item.current.Status {
			previous := item.current.Status
			item.current = result
			events = append(events, schemaChangeEvent(now, result, previous))
		} else {
			item.current = result
		}
	}
	if endedBefore {
		s.checkedAfterEnd = true
	}
	return events
}

func schemaChangeEvent(stamp string, result udaStatusResult, previous string) watchSchemaEvent {
	return watchSchemaEvent{Timestamp: stamp, Type: "schema", File: result.File, Kind: result.Kind, Udi: result.Udi, Name: result.Name, Status: result.Status, Previous: previous, Diffs: result.Diffs, Reason: result.Reason}
}

// pending reports whether verified must still wait for the schema: until
// every tracked artifact is in sync, or the schema pass has ended and the
// re-checks after it have a definitive answer for each artifact (an
// unknown one gets schemaUnknownRetries tries).
func (s *schemaTracker) pending() bool {
	allInSync := true
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			allInSync = false
		}
	}
	if allInSync {
		return false
	}
	if !s.pass.ended || !s.checkedAfterEnd {
		return true
	}
	for _, item := range s.tracked {
		if item.current.Status == "in-sync" || item.current.Status == "unknown" {
			continue
		}
		if item.unknownAfterEnd > 0 && item.unknownAfterEnd < schemaUnknownRetries {
			return true
		}
	}
	return false
}

// waiting announces once that everything but the schema is clear, so the
// silence before verified has a stated reason.
func (s *schemaTracker) waiting(stamp string) []any {
	if s.waitingAnnounced || !s.pending() {
		return nil
	}
	s.waitingAnnounced = true
	reason := "waiting for Umbraco Deploy's schema pass to start"
	if s.pass.started {
		reason = "waiting for Umbraco Deploy's schema pass to end"
	}
	if s.pass.ended {
		reason = "re-checking artifacts after Umbraco Deploy's schema pass ended"
	}
	if !s.pass.detection {
		reason = "waiting for the tracked artifacts to be in sync; schema pass detection is off without a baseline process id"
	}
	return []any{watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "waiting", Detail: map[string]any{"reason": reason, "pending": s.pendingFiles()}}}
}

func (s *schemaTracker) deployPassDetail() map[string]any {
	pass := map[string]any{"started": s.pass.started, "ended": s.pass.ended, "workStatus": s.pass.workStatus}
	if !s.pass.detection {
		pass["detection"] = "off"
	}
	return pass
}

// pendingReason is the schema part of a timeout reason.
func (s *schemaTracker) pendingReason() string {
	if !s.pass.detection {
		return fmt.Sprintf("schema: %d tracked artifacts unconfirmed, schema pass detection off without a baseline process id", len(s.pendingFiles()))
	}
	return fmt.Sprintf("schema: %d tracked artifacts unconfirmed, Umbraco Deploy's schema pass end not observed", len(s.pendingFiles()))
}

func (s *schemaTracker) pendingFiles() []string {
	files := make([]string, 0)
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			files = append(files, item.artifact.File)
		}
	}
	sort.Strings(files)
	return files
}

// summary is the final schema event, type "schema-summary".
func (s *schemaTracker) summary(phase string) watchSchemaPassEvent {
	// Until the schema settles nothing is judged: artifacts not in sync yet
	// are pending, not unconfirmed.
	settled := !s.pending()
	confirmed, unconfirmed, unknown := s.classify(settled)
	unconfirmedKey := "unconfirmed"
	if !settled {
		unconfirmedKey = "pending"
	}
	unverifiable := make([]any, 0, len(s.unverifiable))
	for _, result := range s.unverifiable {
		unverifiable = append(unverifiable, map[string]any{"file": result.File, "kind": result.Kind, "status": result.Status, "reason": result.Reason})
	}
	detail := map[string]any{
		"phase":        phase,
		"settled":      settled,
		"deployPass":   s.deployPassDetail(),
		"confirmed":    confirmed,
		unconfirmedKey: unconfirmed,
		"unknown":      unknown,
		"unverifiable": unverifiable,
		"inSync":       s.inSync,
	}
	verdict := s.verdict()
	if verdict == "nothing-to-confirm" {
		// 08-10: a deploy without schema changes, and the new process
		// logged no schema pass at all; started/ended false read as missed.
		detail["reason"] = "nothing was drifted or missing at baseline, so this deploy had no schema to confirm and verified did not wait for Umbraco Deploy's schema pass; deployPass says whether the new process logged one"
	}
	return watchSchemaPassEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Type: "schema-summary", Status: verdict, Detail: detail}
}

// classify sorts the tracked artifacts by their current status: confirmed
// (in sync), unconfirmed (drifted or missing) and unknown. Before the schema
// has settled an unknown artifact counts as unconfirmed.
func (s *schemaTracker) classify(settled bool) (confirmed []any, unconfirmed []any, unknown []any) {
	confirmed, unconfirmed, unknown = make([]any, 0), make([]any, 0), make([]any, 0)
	for _, item := range s.tracked {
		ref := schemaArtifactRef(item.current)
		ref["baseline"] = item.baseline.Status
		switch item.current.Status {
		case "in-sync":
			confirmed = append(confirmed, ref)
		case "drifted", "missing-remote":
			if len(item.current.Diffs) > 0 {
				ref["diffs"] = item.current.Diffs
			}
			unconfirmed = append(unconfirmed, ref)
		case "unknown", "error":
			if !settled {
				unconfirmed = append(unconfirmed, ref)
				continue
			}
			ref["reason"] = item.current.Reason
			unknown = append(unknown, ref)
		}
	}
	return confirmed, unconfirmed, unknown
}

// verdict is the summary status: confirmed (every tracked artifact in
// sync), unconfirmed (some still drifted or missing after the pass, or the
// pass did not complete), pending (the watch ended before the schema
// settled; nothing is inferred), or nothing-to-confirm.
func (s *schemaTracker) verdict() string {
	if len(s.tracked) == 0 {
		return "nothing-to-confirm"
	}
	if s.pending() {
		return "pending"
	}
	if s.unconfirmedCount() > 0 || (s.pass.ended && !strings.EqualFold(s.pass.workStatus, "Completed")) {
		return "unconfirmed"
	}
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			return "partly-unknown"
		}
	}
	return "confirmed"
}

func (s *schemaTracker) unconfirmedCount() int {
	count := 0
	for _, item := range s.tracked {
		if item.current.Status == "drifted" || item.current.Status == "missing-remote" {
			count++
		}
	}
	return count
}

// exitError is the exit after verified: 7 when tracked artifacts are still
// drifted or missing after the pass ended, or the pass did not complete.
func (s *schemaTracker) exitError() error {
	if s.verdict() != "unconfirmed" {
		return nil
	}
	return deployWatchSchemaUnconfirmedError{unconfirmed: s.unconfirmedCount(), workStatus: s.pass.workStatus}
}
