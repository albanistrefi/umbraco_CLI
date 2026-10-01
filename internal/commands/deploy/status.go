package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/automate"
	"umbraco-cli/internal/commands/cmdkit"
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

func deployStatus(deps cmdkit.Dependencies) *cobra.Command {
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
			if err := cmdkit.PrintResult(cmd, deps, payload); err != nil {
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
func explicitJSONOutput(deps cmdkit.Dependencies) bool {
	requested := deps.RequestedOutput()
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

func compareArtifacts(ctx context.Context, deps cmdkit.Dependencies, artifacts []udaArtifact, flagStepAliases []string, concurrency int) []udaStatusResult {
	// Probe Automate availability once up front: on an environment without
	// the package (or with the API blocked by Cloud basic auth) every
	// per-entity lookup 404s, which must read as "unknown — API
	// unavailable", never as the entity missing remotely.
	var automateErr error
	automateProbed := false
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.Kind, "umbraco-automate-") && artifact.Err == nil {
			_, automateErr = deps.Client.Get(ctx, "/automations", api.RequestOptions{APIPrefix: automate.APIPrefix, Params: map[string]any{"skip": 0, "take": 1}})
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

func compareArtifact(ctx context.Context, deps cmdkit.Dependencies, artifact udaArtifact, flagStepAliases []string, automateErr error) udaStatusResult {
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

	remote, err := cmdkit.FetchObject(ctx, deps.Client, api.JoinPath(fetchPath, artifact.GUID), api.RequestOptions{})
	if err != nil {
		if api.IsStatus(err, http.StatusNotFound) {
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
