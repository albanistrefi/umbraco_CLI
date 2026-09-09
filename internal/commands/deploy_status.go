package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

// deployDriftFoundError maps "the command ran cleanly and found drift" to
// exit code 7 — its own documented code, since 2 is reserved for schema
// diff differences by the global exit-code contract. Under explicit
// -o json the error is a quiet exit: the JSON report already carries the
// summary, and CI harnesses that merge stdout+stderr would otherwise
// corrupt the JSON with the summary line — the cause of two consecutive
// field reports of "invalid JSON output".
type deployDriftFoundError struct {
	drifted, missing int
	quiet            bool
}

func (e deployDriftFoundError) Error() string {
	return fmt.Sprintf("deploy status found %d drifted and %d missing artifacts", e.drifted, e.missing)
}
func (deployDriftFoundError) ExitCode() int     { return 7 }
func (e deployDriftFoundError) QuietExit() bool { return e.quiet }

func deployStatus(deps Dependencies) *cobra.Command {
	var udaDir string
	var kinds []string
	var flagStepAliases []string
	var exitZero bool
	var concurrency int

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Compare local .uda deploy artifacts against the environment, read-only",
		Long: `Reads the Umbraco Deploy artifacts in --uda-dir (the site repo's umbraco/Deploy/Revision) and compares each against the target environment's database via the Management API, reporting in-sync vs drifted per entity. Strictly read-only: a pre-flight check that turns "will this deploy blow up or carry surprises?" into an answerable question — in-sync artifacts are skipped by Deploy's schema pass and are therefore safe; drifted ones are processed.

Comparison is per entity kind (data types, document/media/member types, templates, containers, member groups, relation types) over the fields the artifact carries; environment-only additions like migration markers are ignored. Automate artifacts degrade to status "unknown" where the Automate API is unreachable (Cloud basic auth blocks package APIs on non-live environments) — never a false in-sync — but their step aliases are still read locally, and --flag-step-alias marks automations carrying aliases you know your Deploy version cannot validate (configuration, not encoded knowledge: those landmines change as bugs are fixed).

Exit 7 when drift or missing entities are found (suppress with --exit-zero); parse failures and unreachable comparisons are reported per artifact, never silently dropped. The report is stdout; with an explicit -o json the drift exit is silent (the summary is inside the JSON), so even merged-stream captures parse. Without -o json the drift summary line goes to stderr.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if concurrency < 1 {
				return fmt.Errorf("--concurrency must be at least 1")
			}
			artifacts, err := loadUdaArtifacts(udaDir, kinds)
			if err != nil {
				return err
			}
			if len(artifacts) == 0 {
				return fmt.Errorf("no .uda artifacts found in %s (pass --uda-dir pointing at the site repo's umbraco/Deploy/Revision)", udaDir)
			}

			// Pre-flight: an unreachable or unauthenticated environment must
			// surface with its real exit code (3/4), not degrade every
			// comparison to "unknown" and exit 0 — CI would read an
			// unperformed pre-flight as a passed one.
			if _, err := deps.Client.Get(cmd.Context(), "/server/status", api.RequestOptions{}); err != nil {
				return fmt.Errorf("deploy status cannot reach the target environment: %w", err)
			}

			results := compareArtifacts(cmd.Context(), deps, artifacts, flagStepAliases, concurrency)
			summary := map[string]int{}
			flagged := 0
			for _, result := range results {
				summary[result.Status]++
				if len(result.Flags) > 0 {
					flagged++
				}
			}
			payload := map[string]any{
				"udaDir":    udaDir,
				"artifacts": results,
				"summary": map[string]any{
					"total":         len(results),
					"inSync":        summary["in-sync"],
					"drifted":       summary["drifted"],
					"missingRemote": summary["missing-remote"],
					"unknown":       summary["unknown"],
					"errors":        summary["error"],
					"flagged":       flagged,
				},
			}
			if err := printResult(cmd, deps, payload); err != nil {
				return err
			}
			if !exitZero && (summary["drifted"] > 0 || summary["missing-remote"] > 0) {
				return deployDriftFoundError{drifted: summary["drifted"], missing: summary["missing-remote"], quiet: explicitJSONOutput(deps)}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&udaDir, "uda-dir", filepath.Join("umbraco", "Deploy", "Revision"), "Directory holding the .uda artifacts")
	cmd.Flags().StringArrayVar(&kinds, "kind", nil, "Only compare these artifact kinds (Udi entity types, e.g. data-type, document-type; repeatable)")
	cmd.Flags().StringArrayVar(&flagStepAliases, "flag-step-alias", nil, "Flag automations whose steps carry this action-alias substring (repeatable; e.g. a control-flow alias your Deploy version fails to validate)")
	cmd.Flags().BoolVar(&exitZero, "exit-zero", false, "Exit 0 even when drift or missing entities are found")
	cmd.Flags().IntVar(&concurrency, "concurrency", 8, "Maximum concurrent environment lookups")
	return cmd
}

// udaArtifact is one parsed .uda file, discriminated by the Udi entity type
// (authoritative across every artifact kind, unlike the filename prefix or
// the assembly-qualified __type).
type udaArtifact struct {
	File string
	Kind string
	GUID string
	Body map[string]any
	Err  error
}

// udaStatusResult is one artifact's comparison outcome.
type udaStatusResult struct {
	File        string   `json:"file"`
	Kind        string   `json:"kind"`
	Udi         string   `json:"udi,omitempty"`
	Name        string   `json:"name,omitempty"`
	Status      string   `json:"status"`
	Diffs       []string `json:"diffs,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	StepAliases []string `json:"stepAliases,omitempty"`
	Flags       []string `json:"flags,omitempty"`
}

// explicitJSONOutput reports whether the caller explicitly requested JSON
// output, accepting the same spellings ParseOutputFormat does (-o JSON,
// padded values). The env-default output deliberately does not count:
// quiet exits are for machine consumers who asked for machine output.
func explicitJSONOutput(deps Dependencies) bool {
	requested := deps.requestedOutput()
	if strings.TrimSpace(requested) == "" {
		return false
	}
	format, err := config.ParseOutputFormat(requested)
	return err == nil && format == config.OutputJSON
}

func loadUdaArtifacts(dir string, kinds []string) ([]udaArtifact, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read --uda-dir %s: %w", dir, err)
	}
	wanted := map[string]struct{}{}
	for _, kind := range kinds {
		wanted[strings.TrimSpace(kind)] = struct{}{}
	}

	artifacts := make([]udaArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".uda") {
			continue
		}
		artifact := parseUdaFile(filepath.Join(dir, entry.Name()))
		if len(wanted) > 0 {
			// The filter applies to errored artifacts too: an out-of-scope
			// artifact must not pollute a filtered run's error count, and an
			// unparseable kind cannot match any filter.
			if _, ok := wanted[artifact.Kind]; !ok {
				continue
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func parseUdaFile(path string) udaArtifact {
	artifact := udaArtifact{File: filepath.Base(path)}
	raw, err := os.ReadFile(path)
	if err != nil {
		artifact.Err = err
		return artifact
	}
	// Deploy writes the files UTF-8 with BOM; json.Unmarshal rejects it.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		artifact.Err = fmt.Errorf("parse: %w", err)
		return artifact
	}
	artifact.Body = body
	udi, _ := body["Udi"].(string)
	artifact.Kind, artifact.GUID = parseUdi(udi)
	if artifact.Kind == "" || artifact.GUID == "" {
		artifact.Err = fmt.Errorf("no usable Udi in artifact (%q)", udi)
	}
	return artifact
}

// parseUdi splits umb://<entity-type>/<identifier>. A 32-hex identifier is
// normalized to the dashed GUID the Management API accepts; other
// identifiers (languages are keyed by ISO code, e.g. umb://language/en-US)
// are returned verbatim, and each kind's route decides whether a raw
// identifier is acceptable.
func parseUdi(udi string) (string, string) {
	rest, ok := strings.CutPrefix(udi, "umb://")
	if !ok {
		return "", ""
	}
	kind, identifier, ok := strings.Cut(rest, "/")
	if !ok || identifier == "" {
		return kind, ""
	}
	if udiHexPattern.MatchString(identifier) {
		identifier = strings.ToLower(identifier)
		return kind, strings.Join([]string{identifier[0:8], identifier[8:12], identifier[12:16], identifier[16:20], identifier[20:32]}, "-")
	}
	return kind, identifier
}

var udiHexPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

var udiGUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// udaKindAcceptsRawID lists kinds whose Management API routes take a
// non-GUID identifier. Every other kind requires a GUID, and a non-GUID
// identifier there is an artifact error — never a request against a
// collection or invalid-UUID route.
func udaKindAcceptsRawID(kind string) bool {
	return kind == "language"
}

func compareArtifacts(ctx context.Context, deps Dependencies, artifacts []udaArtifact, flagStepAliases []string, concurrency int) []udaStatusResult {
	// Probe Automate availability once up front: on an environment without
	// the package (or with the API blocked by Cloud basic auth) every
	// per-entity lookup 404s, which must read as "unknown — API
	// unavailable", never as the entity missing remotely.
	var automateErr error
	automateProbed := false
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.Kind, "umbraco-automate-") && artifact.Err == nil {
			_, automateErr = deps.Client.Get(ctx, "/automations", api.RequestOptions{APIPrefix: automateAPIPrefix, Params: map[string]any{"skip": 0, "take": 1}})
			automateProbed = true
			break
		}
	}
	_ = automateProbed

	results := make([]udaStatusResult, len(artifacts))
	var wg sync.WaitGroup
	slots := make(chan struct{}, concurrency)
	for i := range artifacts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[index] = compareArtifact(ctx, deps, artifacts[index], flagStepAliases, automateErr)
		}(i)
	}
	wg.Wait()

	statusRank := map[string]int{"error": 0, "drifted": 1, "missing-remote": 2, "unknown": 3, "in-sync": 4}
	sort.SliceStable(results, func(i, j int) bool {
		if statusRank[results[i].Status] != statusRank[results[j].Status] {
			return statusRank[results[i].Status] < statusRank[results[j].Status]
		}
		return results[i].File < results[j].File
	})
	return results
}

func compareArtifact(ctx context.Context, deps Dependencies, artifact udaArtifact, flagStepAliases []string, automateErr error) udaStatusResult {
	result := udaStatusResult{File: artifact.File, Kind: artifact.Kind}
	if artifact.Err != nil {
		result.Status = "error"
		result.Reason = artifact.Err.Error()
		return result
	}
	result.Udi, _ = artifact.Body["Udi"].(string)
	result.Name, _ = artifact.Body["Name"].(string)

	isAutomate := strings.HasPrefix(artifact.Kind, "umbraco-automate-")
	fetchPath, comparer := udaComparer(artifact.Kind)

	// The GUID guard applies to every kind that would issue a request —
	// Automate kinds included, which are all GUID-keyed. Kinds with no
	// comparison stay "unknown" regardless of identifier shape, since they
	// never issue a request.
	if (isAutomate || comparer != nil) && !udaKindAcceptsRawID(artifact.Kind) && !udiGUIDPattern.MatchString(artifact.GUID) {
		result.Status = "error"
		result.Reason = fmt.Sprintf("kind %s requires a GUID identifier, got %q", artifact.Kind, artifact.GUID)
		return result
	}

	if isAutomate {
		return compareAutomateArtifact(ctx, deps, artifact, result, flagStepAliases, automateErr)
	}

	if comparer == nil {
		result.Status = "unknown"
		result.Reason = fmt.Sprintf("no comparison implemented for kind %s", artifact.Kind)
		return result
	}

	remote, err := fetchObject(ctx, deps.Client, api.JoinPath(fetchPath, artifact.GUID), api.RequestOptions{})
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			result.Status = "missing-remote"
			result.Reason = "entity does not exist on the target environment"
			return result
		}
		// Never report a false in-sync when the comparison could not run.
		result.Status = "unknown"
		result.Reason = err.Error()
		return result
	}

	diffs := comparer(artifact.Body, remote)
	if len(diffs) > 0 {
		sort.Strings(diffs)
		result.Status = "drifted"
		result.Diffs = diffs
		return result
	}
	result.Status = "in-sync"
	return result
}

// compareAutomateArtifact handles the Automate package artifacts. The
// Automate API is blocked by Cloud basic auth on non-live environments and
// is absent entirely without the package, so unreachable comparisons
// degrade to "unknown" — but step aliases are always read locally, and
// --flag-step-alias marks automations regardless of API reachability.
func compareAutomateArtifact(ctx context.Context, deps Dependencies, artifact udaArtifact, result udaStatusResult, flagStepAliases []string, automateErr error) udaStatusResult {
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
	remote, err := deps.Client.Get(ctx, api.JoinPath(fetchPath, artifact.GUID), api.RequestOptions{APIPrefix: automateAPIPrefix})
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
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
	return sortedKeys(seen)
}

// udaComparer maps a Udi entity type to its Management API fetch path and a
// field comparer. Comparisons cover the fields the artifact carries;
// environment-only additions (e.g. migration markers in data-type values)
// are not drift.
func udaComparer(kind string) (string, func(artifact map[string]any, remote map[string]any) []string) {
	switch kind {
	case "data-type":
		return "/data-type/%s", compareDataTypeArtifact
	case "document-type":
		return "/document-type/%s", compareContentTypeArtifact
	case "media-type":
		return "/media-type/%s", compareContentTypeArtifact
	case "member-type":
		return "/member-type/%s", compareContentTypeArtifact
	case "template":
		return "/template/%s", compareTemplateArtifact
	case "document-type-container":
		return "/document-type/folder/%s", compareNameOnlyArtifact
	case "data-type-container":
		return "/data-type/folder/%s", compareNameOnlyArtifact
	case "media-type-container":
		return "/media-type/folder/%s", compareNameOnlyArtifact
	case "member-type-container":
		return "/member-type/folder/%s", compareNameOnlyArtifact
	case "member-group":
		return "/member-group/%s", compareNameOnlyArtifact
	case "relation-type":
		return "/relation-type/%s", compareRelationTypeArtifact
	case "language":
		return "/language/%s", compareLanguageArtifact
	}
	return "", nil
}

func compareNameOnlyArtifact(artifact map[string]any, remote map[string]any) []string {
	return diffFields(nil, fieldDiff("name", artifact["Name"], remote["name"]))
}

func compareDataTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("editorAlias", artifact["EditorAlias"], remote["editorAlias"]),
		fieldDiff("editorUiAlias", artifact["EditorUiAlias"], remote["editorUiAlias"]),
	)
	configuration, _ := artifact["Configuration"].(map[string]any)
	remoteValues := map[string]any{}
	if values, ok := remote["values"].([]any); ok {
		for _, item := range values {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := entry["alias"].(string); alias != "" {
				remoteValues[alias] = entry["value"]
			}
		}
	}
	for key, artifactValue := range configuration {
		remoteValue, exists := remoteValues[key]
		if !exists || !jsonValueEqual(artifactValue, remoteValue) {
			diffs = append(diffs, "configuration."+key)
		}
	}
	return diffs
}

func compareTemplateArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
	)
	if artifactContent, ok := artifact["Content"].(string); ok {
		remoteContent, _ := remote["content"].(string)
		if normalizeTemplateContent(artifactContent) != normalizeTemplateContent(remoteContent) {
			diffs = append(diffs, "content")
		}
	}
	return diffs
}

// normalizeTemplateContent normalizes line endings only: leading and
// trailing whitespace in a Razor template can be rendered output, so it is
// significant and compared verbatim.
func normalizeTemplateContent(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func compareContentTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
		fieldDiff("icon", artifact["Icon"], remote["icon"]),
	)
	if permissions, ok := artifact["Permissions"].(map[string]any); ok {
		if isElement, ok := permissions["IsElementType"].(bool); ok {
			remoteElement, _ := remote["isElement"].(bool)
			if isElement != remoteElement {
				diffs = append(diffs, "isElement")
			}
		}
		if allowedAtRoot, ok := permissions["AllowedAtRoot"].(bool); ok {
			remoteRoot, _ := remote["allowedAsRoot"].(bool)
			if allowedAtRoot != remoteRoot {
				diffs = append(diffs, "allowedAsRoot")
			}
		}
		if allowedChildren, ok := permissions["AllowedChildContentTypes"].([]any); ok {
			remoteChildren := referencedGUIDSet(remote["allowedDocumentTypes"], "documentType")
			if remoteChildren == nil {
				remoteChildren = referencedGUIDSet(remote["allowedMediaTypes"], "mediaType")
			}
			if remoteChildren != nil && !udiSetMatches(allowedChildren, remoteChildren) {
				diffs = append(diffs, "allowedChildContentTypes")
			}
		}
	}
	if description, ok := artifact["Description"].(string); ok {
		if normalizeNullableString(description) != normalizeNullableString(udaStringField(remote, "description")) {
			diffs = append(diffs, "description")
		}
	}
	if compositions, ok := artifact["CompositionContentTypes"].([]any); ok {
		remoteCompositions := referencedGUIDSet(remote["compositions"], "documentType")
		if remoteCompositions == nil {
			remoteCompositions = referencedGUIDSet(remote["compositions"], "mediaType")
		}
		if remoteCompositions == nil {
			remoteCompositions = referencedGUIDSet(remote["compositions"], "memberType")
		}
		if remoteCompositions != nil && !udiSetMatches(compositions, remoteCompositions) {
			diffs = append(diffs, "compositions")
		}
	}

	if templates, ok := artifact["AllowedTemplates"].([]any); ok {
		if remoteTemplates := referencedGUIDSetFlat(remote["allowedTemplates"]); remoteTemplates != nil && !udiSetMatches(templates, remoteTemplates) {
			diffs = append(diffs, "allowedTemplates")
		}
	}
	if defaultTemplate, ok := artifact["DefaultTemplate"].(string); ok {
		_, want := parseUdi(defaultTemplate)
		got := ""
		if ref, ok := remote["defaultTemplate"].(map[string]any); ok {
			got, _ = ref["id"].(string)
		}
		if _, hasKey := remote["defaultTemplate"]; hasKey && !guidLikeEqual(want, got) {
			diffs = append(diffs, "defaultTemplate")
		}
	}
	if listView, ok := artifact["ListView"].(string); ok {
		_, want := parseUdi(listView)
		got := ""
		if ref, ok := remote["collection"].(map[string]any); ok {
			got, _ = ref["id"].(string)
		}
		if _, hasKey := remote["collection"]; hasKey && !guidLikeEqual(want, got) {
			diffs = append(diffs, "collection")
		}
	}
	if history, ok := artifact["HistoryCleanup"].(map[string]any); ok {
		if cleanup, ok := remote["cleanup"].(map[string]any); ok {
			diffs = diffFields(diffs,
				fieldDiff("cleanup.preventCleanup", udaBool(history, "PreventCleanup"), udaBool(cleanup, "preventCleanup")),
				fieldDiff("cleanup.keepAllVersionsNewerThanDays", history["KeepAllVersionsNewerThanDays"], cleanup["keepAllVersionsNewerThanDays"]),
				fieldDiff("cleanup.keepLatestVersionPerDayForDays", history["KeepLatestVersionPerDayForDays"], cleanup["keepLatestVersionPerDayForDays"]),
			)
		}
	}
	if variations, ok := artifact["Variations"]; ok {
		wantCulture, wantSegment := parseVariations(variations)
		if got, isBool := remote["variesByCulture"].(bool); isBool && got != wantCulture {
			diffs = append(diffs, "variesByCulture")
		}
		if got, isBool := remote["variesBySegment"].(bool); isBool && got != wantSegment {
			diffs = append(diffs, "variesBySegment")
		}
	}
	if groups, ok := artifact["PropertyGroups"].([]any); ok {
		if remoteContainers, ok := remote["containers"].([]any); ok {
			diffs = append(diffs, compareContainers(groups, remoteContainers)...)
		}
	}

	artifactProperties := artifactPropertyIndex(artifact)
	remoteProperties := map[string]map[string]any{}
	if properties, ok := remote["properties"].([]any); ok {
		for _, item := range properties {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := entry["alias"].(string); alias != "" {
				remoteProperties[alias] = entry
			}
		}
	}
	for alias, artifactProperty := range artifactProperties {
		remoteProperty, exists := remoteProperties[alias]
		if !exists {
			diffs = append(diffs, "property "+alias+" (missing remotely)")
			continue
		}
		if name, _ := artifactProperty["Name"].(string); name != udaStringField(remoteProperty, "name") {
			diffs = append(diffs, "property "+alias+".name")
		}
		if _, dataTypeGUID := parseUdi(udaStringField(artifactProperty, "DataType")); dataTypeGUID != "" {
			remoteDataType := ""
			if reference, ok := remoteProperty["dataType"].(map[string]any); ok {
				remoteDataType, _ = reference["id"].(string)
			}
			if !strings.EqualFold(dataTypeGUID, remoteDataType) {
				diffs = append(diffs, "property "+alias+".dataType")
			}
		}
		if sortOrder, ok := artifactProperty["SortOrder"].(float64); ok {
			if remoteSort, ok := remoteProperty["sortOrder"].(float64); ok && sortOrder != remoteSort {
				diffs = append(diffs, "property "+alias+".sortOrder")
			}
		}
		if description, ok := artifactProperty["Description"].(string); ok {
			if normalizeNullableString(description) != normalizeNullableString(udaStringField(remoteProperty, "description")) {
				diffs = append(diffs, "property "+alias+".description")
			}
		}
		if mandatory, ok := artifactProperty["Mandatory"].(bool); ok {
			remoteMandatory := false
			if validation, ok := remoteProperty["validation"].(map[string]any); ok {
				remoteMandatory, _ = validation["mandatory"].(bool)
			}
			if mandatory != remoteMandatory {
				diffs = append(diffs, "property "+alias+".mandatory")
			}
		}
		if varies, ok := artifactProperty["VariesByCulture"].(bool); ok {
			if remoteVaries, isBool := remoteProperty["variesByCulture"].(bool); isBool && varies != remoteVaries {
				diffs = append(diffs, "property "+alias+".variesByCulture")
			}
		}
	}
	for alias := range remoteProperties {
		if _, exists := artifactProperties[alias]; !exists {
			diffs = append(diffs, "property "+alias+" (missing in artifact)")
		}
	}
	return diffs
}

// referencedGUIDSetFlat reads [{id}] reference lists (allowedTemplates).
func referencedGUIDSetFlat(value any) map[string]struct{} {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range items {
		if ref, ok := item.(map[string]any); ok {
			if id, _ := ref["id"].(string); id != "" {
				set[strings.ToLower(id)] = struct{}{}
			}
		}
	}
	return set
}

// compareContainers matches artifact PropertyGroups to remote containers by
// key and compares name, type, and sort order; unmatched entries on either
// side are diffs.
func compareContainers(groups []any, remoteContainers []any) []string {
	diffs := []string{}
	remoteByID := map[string]map[string]any{}
	for _, item := range remoteContainers {
		if container, ok := item.(map[string]any); ok {
			if id, _ := container["id"].(string); id != "" {
				remoteByID[strings.ToLower(id)] = container
			}
		}
	}
	seen := map[string]bool{}
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := normalizeGUID(udaStringField(group, "Key"))
		name := udaStringField(group, "Name")
		remote, exists := remoteByID[key]
		if !exists {
			diffs = append(diffs, "container "+name+" (missing remotely)")
			continue
		}
		seen[key] = true
		if name != udaStringField(remote, "name") {
			diffs = append(diffs, "container "+name+".name")
		}
		if containerTypeName(group["Type"]) != udaStringField(remote, "type") {
			diffs = append(diffs, "container "+name+".type")
		}
		if sortOrder, ok := group["SortOrder"].(float64); ok {
			if remoteSort, ok := remote["sortOrder"].(float64); ok && sortOrder != remoteSort {
				diffs = append(diffs, "container "+name+".sortOrder")
			}
		}
	}
	for id, remote := range remoteByID {
		if !seen[id] {
			diffs = append(diffs, "container "+udaStringField(remote, "name")+" (missing in artifact)")
		}
	}
	return diffs
}

// artifactPropertyIndex reads BOTH property collections a content-type
// artifact carries: PropertyGroups[].PropertyTypes (grouped, the normal
// case) and the top-level PropertyTypes[] (ungrouped). Reading only the
// grouped collection silently ignores ungrouped properties.
func artifactPropertyIndex(artifact map[string]any) map[string]map[string]any {
	index := map[string]map[string]any{}
	collect := func(items []any) {
		for _, item := range items {
			property, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if alias, _ := property["Alias"].(string); alias != "" {
				index[alias] = property
			}
		}
	}
	if groups, ok := artifact["PropertyGroups"].([]any); ok {
		for _, item := range groups {
			group, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if properties, ok := group["PropertyTypes"].([]any); ok {
				collect(properties)
			}
		}
	}
	if properties, ok := artifact["PropertyTypes"].([]any); ok {
		collect(properties)
	}
	return index
}

func udaStringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func fieldDiff(field string, artifactValue any, remoteValue any) string {
	if artifactValue == nil {
		return ""
	}
	if jsonValueEqual(artifactValue, remoteValue) {
		return ""
	}
	return field
}

func diffFields(diffs []string, candidates ...string) []string {
	for _, candidate := range candidates {
		if candidate != "" {
			diffs = append(diffs, candidate)
		}
	}
	return diffs
}

func jsonValueEqual(a any, b any) bool {
	return reflect.DeepEqual(a, b)
}

// compareRelationTypeArtifact compares the behavioral relation-type fields
// the artifact carries: directionality, dependency behavior, and the
// parent/child object types — a shared name alone says nothing.
func compareRelationTypeArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("alias", artifact["Alias"], remote["alias"]),
	)
	if bidirectional, ok := artifact["IsBidirectional"].(bool); ok {
		if remoteValue, isBool := remote["isBidirectional"].(bool); isBool && bidirectional != remoteValue {
			diffs = append(diffs, "isBidirectional")
		}
	}
	if dependency, ok := artifact["IsDependency"].(bool); ok {
		if remoteValue, isBool := remote["isDependency"].(bool); isBool && dependency != remoteValue {
			diffs = append(diffs, "isDependency")
		}
	}
	for artifactKey, remoteKey := range map[string]string{"ParentObjectType": "parentObjectType", "ChildObjectType": "childObjectType"} {
		if value, ok := artifact[artifactKey].(string); ok && value != "" {
			if !guidLikeEqual(value, udaStringField(remote, remoteKey)) {
				diffs = append(diffs, remoteKey)
			}
		}
	}
	return diffs
}

// referencedGUIDSet extracts the lowercase GUID set from response arrays
// shaped [{"<refKey>": {"id": ...}, ...}]; nil when the field is absent or
// not that shape, so callers skip rather than false-drift.
func referencedGUIDSet(value any, refKey string) map[string]struct{} {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	set := map[string]struct{}{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		reference, ok := entry[refKey].(map[string]any)
		if !ok {
			return nil
		}
		if id, _ := reference["id"].(string); id != "" {
			set[strings.ToLower(id)] = struct{}{}
		}
	}
	return set
}

// udiSetMatches compares an artifact's udi list against a remote GUID set.
func udiSetMatches(udis []any, remote map[string]struct{}) bool {
	local := map[string]struct{}{}
	for _, item := range udis {
		udi, ok := item.(string)
		if !ok {
			return false
		}
		if _, guid := parseUdi(udi); guid != "" {
			local[strings.ToLower(guid)] = struct{}{}
		}
	}
	if len(local) != len(remote) {
		return false
	}
	for guid := range local {
		if _, ok := remote[guid]; !ok {
			return false
		}
	}
	return true
}

// guidLikeEqual compares two identifiers that may each be a bare GUID or a
// udi, case-insensitively.
func guidLikeEqual(a string, b string) bool {
	normalize := func(value string) string {
		if _, guid := parseUdi(value); guid != "" {
			return strings.ToLower(guid)
		}
		return strings.ToLower(strings.TrimSpace(value))
	}
	return normalize(a) == normalize(b)
}

func normalizeNullableString(value string) string {
	return strings.TrimSpace(value)
}

// compareLanguageArtifact compares the language fields the artifact
// carries. Languages are keyed by ISO code, not GUID.
func compareLanguageArtifact(artifact map[string]any, remote map[string]any) []string {
	diffs := diffFields(nil,
		fieldDiff("name", artifact["Name"], remote["name"]),
		fieldDiff("isoCode", artifact["IsoCode"], remote["isoCode"]),
	)
	for artifactKey, remoteKey := range map[string]string{"IsDefault": "isDefault", "IsMandatory": "isMandatory"} {
		if value, ok := artifact[artifactKey].(bool); ok {
			if remoteValue, isBool := remote[remoteKey].(bool); isBool && value != remoteValue {
				diffs = append(diffs, remoteKey)
			}
		}
	}
	if fallback, ok := artifact["FallbackIsoCode"].(string); ok && fallback != "" {
		if fallback != udaStringField(remote, "fallbackIsoCode") {
			diffs = append(diffs, "fallbackIsoCode")
		}
	}
	return diffs
}
