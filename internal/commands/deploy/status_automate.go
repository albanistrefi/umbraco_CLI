package deploy

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/automate"
	"umbraco-cli/internal/commands/cmdkit"
)

// compareAutomateArtifact handles the Automate package artifacts. The
// Automate API is blocked by Cloud basic auth on non-live environments and
// is absent entirely without the package, so unreachable comparisons
// degrade to "unknown" — but step aliases are always read locally, and
// --flag-step-alias marks automations regardless of API reachability.
func compareAutomateArtifact(ctx context.Context, deps cmdkit.Dependencies, artifact udaArtifact, result udaStatusResult, flagStepAliases []string, automateErr error) udaStatusResult {
	if artifact.Kind == "umbraco-automate-automation" {
		result.StepAliases = automationStepAliases(artifact.Body)
		for _, needle := range flagStepAliases {
			needle = strings.TrimSpace(needle)
			if needle == "" {
				continue
			}
			for _, alias := range result.StepAliases {
				if strings.Contains(alias, needle) {
					result.Flags = append(result.Flags, fmt.Sprintf("step alias %q matches --flag-step-alias %q", alias, needle))
				}
			}
		}
	}

	if automateErr != nil {
		result.Status = "unknown"
		result.Reason = "Automate API unavailable on this environment: " + automateErr.Error()
		return result
	}
	fetchPath := map[string]string{
		"umbraco-automate-automation": "/automations/%s/export",
		"umbraco-automate-workspace":  "/workspaces/%s",
		"umbraco-automate-connection": "/connections/%s",
	}[artifact.Kind]
	if fetchPath == "" {
		result.Status = "unknown"
		result.Reason = fmt.Sprintf("no comparison implemented for kind %s", artifact.Kind)
		return result
	}
	remote, err := deps.Client.Get(ctx, api.JoinPath(fetchPath, artifact.GUID), api.RequestOptions{APIPrefix: automate.APIPrefix})
	if err != nil {
		if api.IsStatus(err, http.StatusNotFound) {
			result.Status = "missing-remote"
			result.Reason = "entity does not exist on the target environment"
			return result
		}
		result.Status = "unknown"
		result.Reason = "Automate API unreachable: " + err.Error()
		return result
	}
	remoteObject, _ := remote.(map[string]any)

	if artifact.Kind == "umbraco-automate-automation" {
		// The export representation carries the full behavioral definition
		// (trigger, steps, connections), so this is a real comparison.
		automation, _ := remoteObject["automation"].(map[string]any)
		if automation == nil {
			result.Status = "unknown"
			result.Reason = "export endpoint returned no automation body"
			return result
		}
		diffs := compareAutomationExport(artifact.Body, automation)
		if len(diffs) > 0 {
			sort.Strings(diffs)
			result.Status = "drifted"
			result.Diffs = diffs
			return result
		}
		result.Status = "in-sync"
		return result
	}

	// Workspaces and connections expose no export representation, so only
	// identity fields are comparable: an identity mismatch is real drift,
	// but an identity match must not claim the full definition is in sync.
	diffs := diffFields(nil,
		fieldDiff("name", artifact.Body["Name"], remoteObject["name"]),
		fieldDiff("alias", artifact.Body["Alias"], remoteObject["alias"]),
	)
	if len(diffs) > 0 {
		result.Status = "drifted"
		result.Diffs = diffs
		return result
	}
	result.Status = "unknown"
	result.Reason = "identity fields match; the API exposes no full definition to compare for this kind"
	return result
}

// compareAutomationExport compares an automation artifact against the
// export representation's behavioral fields. Canvas state and step
// positions are layout, not behavior, and are ignored.
func compareAutomationExport(body map[string]any, automation map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", body["Name"], automation["name"]),
		fieldDiff("alias", body["Alias"], automation["alias"]),
	)
	if description, ok := body["Description"].(string); ok {
		if normalizeNullableString(description) != normalizeNullableString(udaStringField(automation, "description")) {
			diffs = append(diffs, "description")
		}
	}
	if trigger, ok := body["Trigger"].(map[string]any); ok {
		remoteTrigger, _ := automation["trigger"].(map[string]any)
		if remoteTrigger == nil {
			diffs = append(diffs, "trigger")
		} else {
			if alias, ok := trigger["TriggerAlias"].(string); ok && alias != udaStringField(remoteTrigger, "triggerAlias") {
				diffs = append(diffs, "trigger.alias")
			}
			if settings, ok := trigger["Settings"]; ok && !jsonValueEqual(settings, remoteTrigger["settings"]) {
				diffs = append(diffs, "trigger.settings")
			}
		}
	}

	artifactSteps := stepIndex(body["Steps"], "Id")
	remoteSteps := stepIndex(automation["steps"], "id")
	for id, artifactStep := range artifactSteps {
		remoteStep, exists := remoteSteps[id]
		if !exists {
			diffs = append(diffs, "step "+id+" (missing remotely)")
			continue
		}
		for artifactKey, remoteKey := range map[string]string{"ActionAlias": "actionAlias", "Alias": "alias", "Name": "name"} {
			if value, ok := artifactStep[artifactKey].(string); ok && value != udaStringField(remoteStep, remoteKey) {
				diffs = append(diffs, "step "+id+"."+remoteKey)
			}
		}
		for artifactKey, remoteKey := range map[string]string{"Settings": "settings", "InputMappings": "inputMappings"} {
			if value, ok := artifactStep[artifactKey]; ok && !jsonValueEqual(value, remoteStep[remoteKey]) {
				diffs = append(diffs, "step "+id+"."+remoteKey)
			}
		}
	}
	for id := range remoteSteps {
		if _, exists := artifactSteps[id]; !exists {
			diffs = append(diffs, "step "+id+" (missing in artifact)")
		}
	}

	if connections, ok := body["Connections"].([]any); ok {
		if !connectionSetsEqual(connections, automation["connections"]) {
			diffs = append(diffs, "connections")
		}
	}
	return diffs
}

func stepIndex(value any, idKey string) map[string]map[string]any {
	index := map[string]map[string]any{}
	items, _ := value.([]any)
	for _, item := range items {
		step, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := step[idKey].(string); id != "" {
			index[id] = step
		}
	}
	return index
}

// connectionSetsEqual compares step-graph edges as unordered
// (source, target, outcome) tuples.
func connectionSetsEqual(artifact []any, remote any) bool {
	tuple := func(entry map[string]any, source, target, outcome string) string {
		outcomeValue, _ := entry[outcome].(string)
		sourceValue, _ := entry[source].(string)
		targetValue, _ := entry[target].(string)
		return sourceValue + "→" + targetValue + "|" + outcomeValue
	}
	local := map[string]int{}
	for _, item := range artifact {
		if entry, ok := item.(map[string]any); ok {
			local[tuple(entry, "SourceStepId", "TargetStepId", "Outcome")]++
		}
	}
	remoteItems, _ := remote.([]any)
	remoteSet := map[string]int{}
	for _, item := range remoteItems {
		if entry, ok := item.(map[string]any); ok {
			remoteSet[tuple(entry, "sourceStepId", "targetStepId", "outcome")]++
		}
	}
	if len(local) != len(remoteSet) {
		return false
	}
	for key, count := range local {
		if remoteSet[key] != count {
			return false
		}
	}
	return true
}

func automationStepAliases(body map[string]any) []string {
	steps, _ := body["Steps"].([]any)
	seen := map[string]struct{}{}
	for _, item := range steps {
		step, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if alias, _ := step["ActionAlias"].(string); alias != "" {
			seen[alias] = struct{}{}
		}
	}
	return cmdkit.SortedKeys(seen)
}
