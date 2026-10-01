package deploy

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/jsonvalue"
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
	deps              cmdkit.Dependencies
	udaDir            string
	baselineProcessID string
	baselineMachine   string

	tracked      []*trackedArtifact
	unverifiable []udaStatusResult
	inSync       int

	passStarted      bool
	passEnded        bool
	workStatus       string
	checkedAfterEnd  bool
	waitingAnnounced bool
}

// newSchemaTracker reads the artifacts and compares each against the
// target, before the watch arms. An unreadable directory or one with no
// artifacts is a usage error.
func newSchemaTracker(ctx context.Context, deps cmdkit.Dependencies, udaDir string, baseline watchObservation) (*schemaTracker, error) {
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
	tracker := &schemaTracker{deps: deps, udaDir: udaDir, baselineProcessID: baseline.ProcessID, baselineMachine: baseline.MachineName}
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
	return map[string]any{
		"udaDir":       s.udaDir,
		"tracking":     tracking,
		"inSync":       s.inSync,
		"unverifiable": unverifiable,
	}
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

// observeLogs picks the schema pass's start and end out of a tick's log
// entries. Only entries from the new process count, judged by the entry's
// own ProcessId/MachineName, so a pass is attributed correctly however late
// the watch noticed the landing.
func (s *schemaTracker) observeLogs(entries []map[string]any) []any {
	events := make([]any, 0)
	for _, entry := range entries {
		properties := map[string]string{}
		if list, ok := entry["properties"].([]any); ok {
			for _, item := range list {
				if property, ok := item.(map[string]any); ok {
					properties[jsonvalue.String(property["name"])] = jsonvalue.String(property["value"])
				}
			}
		}
		processID, machine := properties["ProcessId"], properties["MachineName"]
		if processID == "" || (processID == s.baselineProcessID && machine == s.baselineMachine) {
			continue
		}
		source := properties["SourceContext"]
		stamp := jsonvalue.String(entry["timestamp"])
		switch {
		case !s.passStarted && strings.HasSuffix(source, ".DiskReadWorkItem"):
			s.passStarted = true
			events = append(events, watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "started"})
		case !s.passEnded && strings.HasSuffix(source, ".DiskService"):
			match := deployWorkStatusPattern.FindStringSubmatch(cmdkit.LogEntryMessage(entry))
			if match == nil {
				continue
			}
			s.passStarted = true
			s.passEnded = true
			s.workStatus = match[1]
			events = append(events, watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "ended", Detail: map[string]any{"workStatus": s.workStatus}})
		}
	}
	return events
}

// recheck compares the tracked artifacts that are not in sync yet, once
// the deploy has landed, and returns the status changes. An unknown result
// (the API unavailable mid-restart) keeps the last definitive status.
func (s *schemaTracker) recheck(ctx context.Context, landed bool) []any {
	if !landed || !s.pending() {
		return nil
	}
	endedBefore := s.passEnded
	pending := make([]udaArtifact, 0)
	byFile := map[string]*trackedArtifact{}
	for _, item := range s.tracked {
		if item.current.Status != "in-sync" {
			pending = append(pending, item.artifact)
			byFile[item.artifact.File] = item
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	events := make([]any, 0)
	for _, result := range compareArtifacts(ctx, s.deps, pending, nil, schemaRecheckConcurrency) {
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
	if !s.passEnded || !s.checkedAfterEnd {
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
	if s.passStarted {
		reason = "waiting for Umbraco Deploy's schema pass to end"
	}
	if s.passEnded {
		reason = "re-checking artifacts after Umbraco Deploy's schema pass ended"
	}
	return []any{watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "waiting", Detail: map[string]any{"reason": reason, "pending": s.pendingFiles()}}}
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
	confirmed := make([]any, 0)
	unconfirmed := make([]any, 0)
	unknown := make([]any, 0)
	// Until the schema settles nothing is judged: artifacts not in sync yet
	// are pending, not unconfirmed.
	settled := !s.pending()
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
	unconfirmedKey := "unconfirmed"
	if !settled {
		unconfirmedKey = "pending"
	}
	unverifiable := make([]any, 0, len(s.unverifiable))
	for _, result := range s.unverifiable {
		unverifiable = append(unverifiable, map[string]any{"file": result.File, "kind": result.Kind, "status": result.Status, "reason": result.Reason})
	}
	return watchSchemaPassEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Type: "schema-summary", Status: s.verdict(), Detail: map[string]any{
		"phase":        phase,
		"settled":      settled,
		"deployPass":   map[string]any{"started": s.passStarted, "ended": s.passEnded, "workStatus": s.workStatus},
		"confirmed":    confirmed,
		unconfirmedKey: unconfirmed,
		"unknown":      unknown,
		"unverifiable": unverifiable,
		"inSync":       s.inSync,
	}}
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
	if s.unconfirmedCount() > 0 || (s.passEnded && !strings.EqualFold(s.workStatus, "Completed")) {
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
	return deployWatchSchemaUnconfirmedError{unconfirmed: s.unconfirmedCount(), workStatus: s.workStatus}
}
