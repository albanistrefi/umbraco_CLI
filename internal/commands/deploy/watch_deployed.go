package deploy

import (
	"regexp"
	"strings"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/jsonvalue"
)

// deployPass follows Umbraco Deploy's schema pass in the log entries of the
// new process: the disk work item starting, and DiskService's work status
// at its end. Entries are attributed by their own ProcessId/MachineName, so
// it needs the baseline's to tell the new process from the old one; without
// it detection is off.
type deployPass struct {
	baselineProcessID string
	baselineMachine   string
	detection         bool

	started    bool
	ended      bool
	workStatus string
	// endedAt is the timestamp of the entry that logged the work status.
	endedAt string
	// beganID is the id of the latest deployment begun before the pass
	// ended; completedID the first one logged as completed, used when no
	// beginning was read.
	beganID     string
	completedID string
}

var deployDeploymentIDPattern = regexp.MustCompile(`(?i)\bdeployment "([^"]+)"`)

func newDeployPass(baseline watchObservation) *deployPass {
	return &deployPass{baselineProcessID: baseline.ProcessID, baselineMachine: baseline.MachineName, detection: baseline.ProcessID != ""}
}

// observe books the pass's start and end from a read's entries and returns
// the schema-pass lines for them.
func (p *deployPass) observe(entries []map[string]any) []any {
	events := make([]any, 0)
	if !p.detection {
		return events
	}
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
		if processID == "" || (processID == p.baselineProcessID && machine == p.baselineMachine) {
			continue
		}
		source := properties["SourceContext"]
		stamp := jsonvalue.String(entry["timestamp"])
		if strings.HasPrefix(source, "Umbraco.Deploy.") {
			if match := deployDeploymentIDPattern.FindStringSubmatch(cmdkit.LogEntryMessage(entry)); match != nil {
				if strings.Contains(strings.ToLower(cmdkit.LogEntryMessage(entry)), "completed") {
					if p.completedID == "" {
						p.completedID = match[1]
					}
				} else if !p.ended {
					p.beganID = match[1]
				}
			}
		}
		switch {
		case !p.started && strings.HasSuffix(source, ".DiskReadWorkItem"):
			p.started = true
			events = append(events, watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "started"})
		case !p.ended && strings.HasSuffix(source, ".DiskService"):
			match := deployWorkStatusPattern.FindStringSubmatch(cmdkit.LogEntryMessage(entry))
			if match == nil {
				continue
			}
			p.started = true
			p.ended = true
			p.workStatus = match[1]
			p.endedAt = stamp
			events = append(events, watchSchemaPassEvent{Timestamp: stamp, Type: "schema-pass", Status: "ended", Detail: map[string]any{"workStatus": p.workStatus}})
		}
	}
	return events
}

// completed reports whether the pass ended with work status Completed; any
// other work status is never a completed deployment.
func (p *deployPass) completed() bool {
	return p.ended && strings.EqualFold(p.workStatus, "Completed")
}

func (p *deployPass) deploymentID() string {
	if p.beganID != "" {
		return p.beganID
	}
	return p.completedID
}

// deployedDetail is the detail of the deployed phase once it may fire: the
// pass ended with Completed and, with --uda-dir, the schema has settled
// (every tracked artifact in sync, or re-checked after the pass with a
// definitive answer). nil until then.
func (r *watchRun) deployedDetail() map[string]any {
	if r.pass == nil || !r.pass.completed() {
		return nil
	}
	if r.schema != nil && r.schema.pending() {
		return nil
	}
	detail := map[string]any{"workStatus": r.pass.workStatus, "loggedAt": r.pass.endedAt}
	if id := r.pass.deploymentID(); id != "" {
		detail["deploymentId"] = id
	}
	if r.schema != nil {
		confirmed, unconfirmed, unknown := r.schema.classify(true)
		detail["schema"] = map[string]any{"confirmed": len(confirmed), "remaining": len(unconfirmed), "unknown": len(unknown)}
	}
	return detail
}

// deployedBaselineDetail says whether the deployed phase can fire on this
// watch, and why not when it cannot.
func deployedBaselineDetail(pass *deployPass) map[string]any {
	switch {
	case pass == nil:
		return map[string]any{"enabled": false, "reason": "Umbraco Deploy's log is read only with --logs or --uda-dir, so the deployment's completion cannot be observed and deployed is not emitted"}
	case !pass.detection:
		return map[string]any{"enabled": false, "reason": "no baseline process id to tell Umbraco Deploy's entries from older ones, so deployed is not emitted"}
	}
	return map[string]any{"enabled": true}
}
