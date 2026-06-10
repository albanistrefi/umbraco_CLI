package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// This file holds the shared builders for the command shapes that repeat
// across resources: get-by-ID, paginated collection reads, searches,
// updates, and target-style mutations (move/copy). Behavior differences
// between resources live in the spec structs; the flag wiring, fallback
// resolution, pagination, projection, and output contracts are defined
// once here so they cannot drift per resource.

func addFieldsFlag(cmd *cobra.Command, fields *string) {
	cmd.Flags().StringVar(fields, "fields", "", "Limit response fields (comma-separated top-level keys)")
}

func addDryRunFlag(cmd *cobra.Command, dryRun *bool) {
	cmd.Flags().BoolVar(dryRun, "dry-run", false, "Print the planned request without executing")
}

// printMutationResult prints the outcome of a mutating command. Umbraco
// answers 204 No Content for most successful mutations; printing the raw
// nil surfaced as `null`, which scripts could not tell apart from failure.
// A real (non-dry-run) empty success becomes {"<verb>": true} instead.
// Dry-run plans pass through verbatim.
func printMutationResult(cmd *cobra.Command, deps Dependencies, verb string, result any, dryRun bool) error {
	if !dryRun && result == nil {
		return printResult(cmd, deps, map[string]any{verb: true})
	}
	return printResult(cmd, deps, result)
}

// fetchObject retrieves a resource as a generic object, for merge flows
// that need the current server-side state.
func fetchObject(ctx context.Context, client *api.Client, path string) (map[string]any, error) {
	result, err := client.Get(ctx, path, api.RequestOptions{})
	if err != nil {
		return nil, err
	}
	return decodeResult[map[string]any](result)
}

// mergeParams folds convenience-flag values into a --params map. The
// documented precedence on every command that accepts both: --params wins
// on key collisions, flags fill the gaps.
func mergeParams(params map[string]any, flagValues map[string]any) map[string]any {
	if len(flagValues) == 0 {
		return params
	}
	if params == nil {
		params = map[string]any{}
	}
	for key, value := range flagValues {
		if _, exists := params[key]; !exists {
			params[key] = value
		}
	}
	return params
}

type getSpec struct {
	Use   string
	Short string
	Long  string
	// Path maps the positional args to the resource path.
	Path func(args []string) string
}

// getCommand builds a get-by-ID read: --fields wiring plus client-side
// projection for endpoints that ignore the fields hint server-side.
func getCommand(deps Dependencies, spec getSpec) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), spec.Path(args), api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

type collectionSpec struct {
	Use   string
	Short string
	Long  string
	NArgs int
	// Args overrides the default cobra.ExactArgs(NArgs) validation for
	// commands with optional positional arguments.
	Args cobra.PositionalArgs
	// Endpoints maps the positional args and resolved query params to the
	// candidate endpoints in fallback order. Params must not be mutated;
	// candidates that need extra keys clone via withParam.
	Endpoints func(args []string, params map[string]any) []getRequestCandidate
}

// collectionCommand builds a paginated collection read (root/children/list):
// --fields/--params/--skip/--take/--all/triage wiring, endpoint fallback,
// auto-pagination, and projection.
func collectionCommand(deps Dependencies, spec collectionSpec) *cobra.Command {
	var fields string
	var paramsRaw string
	var skip, take int
	var all bool
	var triage readTriageOptions
	positionalArgs := spec.Args
	if positionalArgs == nil {
		positionalArgs = cobra.ExactArgs(spec.NArgs)
	}
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  positionalArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			params = applyPaginationParams(params, skip, take)
			candidates := spec.Endpoints(args, params)
			for i := range candidates {
				candidates[i].opts.Fields = fields
			}

			ctx := cmd.Context()
			var result any
			if all {
				result, err = getAllPagesWithFallback(ctx, deps.Client, take, skip, triage.FirstN, candidates...)
			} else {
				result, err = getWithFallback(ctx, deps.Client, candidates...)
			}
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	addFieldsFlag(cmd, &fields)
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	addPaginationFlags(cmd, &skip, &take)
	addAutoPaginationFlag(cmd, &all)
	addReadTriageFlags(cmd, &triage)
	return cmd
}

// withParam clones a params map and sets one extra key, for fallback
// candidates whose endpoints take an ID as a query parameter.
func withParam(params map[string]any, key string, value any) map[string]any {
	next := make(map[string]any, len(params)+1)
	for k, v := range params {
		next[k] = v
	}
	next[key] = value
	return next
}

// paramFlag declares a string convenience flag that maps onto a query
// parameter for search commands.
type paramFlag struct {
	Flag  string
	Param string
	Usage string
}

type searchSpec struct {
	Use   string
	Short string
	Long  string
	// Flags beyond the always-present --query, e.g. --under → parentId.
	Extra []paramFlag
	// Endpoints maps the resolved query params to candidates in fallback order.
	Endpoints func(params map[string]any) []getRequestCandidate
}

// searchCommand builds a search read with the uniform parameter contract:
// convenience flags (--query, --skip, --take, spec extras) merge into
// --params, with --params winning on key collisions.
func searchCommand(deps Dependencies, spec searchSpec) *cobra.Command {
	var paramsRaw string
	var query string
	var skip, take int
	extraValues := make([]string, len(spec.Extra))

	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}

			flagValues := map[string]any{}
			if strings.TrimSpace(query) != "" {
				flagValues["query"] = query
			}
			for i, extra := range spec.Extra {
				if strings.TrimSpace(extraValues[i]) != "" {
					flagValues[extra.Param] = extraValues[i]
				}
			}
			if skip >= 0 {
				flagValues["skip"] = skip
			}
			if take >= 0 {
				flagValues["take"] = take
			}

			params = mergeParams(params, flagValues)
			if len(params) == 0 {
				return fmt.Errorf("%s requires either --params or --query", cmd.CommandPath())
			}

			result, err := getWithFallback(cmd.Context(), deps.Client, spec.Endpoints(params)...)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	for i, extra := range spec.Extra {
		cmd.Flags().StringVar(&extraValues[i], extra.Flag, "", extra.Usage)
	}
	addPaginationFlags(cmd, &skip, &take)
	return cmd
}

// resolveUpdateBody enforces the uniform update contract: --json replaces
// the resource wholesale, --merge-json fetches the current resource and
// deep-merges the patch so unmentioned fields survive. Exactly one of the
// two must be provided. normalize, when non-nil, runs on the parsed input
// before the merge (so patch entries take their canonical shape and merge
// correctly) and again on the final body (so fields echoed back by the
// fetch but rejected by the update model are stripped). It must therefore
// be idempotent.
func resolveUpdateBody(ctx context.Context, client *api.Client, fetchPath string, jsonPayload string, mergeJSON string, normalize func(map[string]any)) (map[string]any, error) {
	hasJSON := strings.TrimSpace(jsonPayload) != ""
	hasMerge := strings.TrimSpace(mergeJSON) != ""
	if hasJSON == hasMerge {
		return nil, fmt.Errorf("update requires exactly one of --json (full replacement) or --merge-json (fetch and merge)")
	}

	if hasJSON {
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return nil, err
		}
		if normalize != nil {
			normalize(body)
		}
		return body, nil
	}

	patch, err := parseJSONObject(mergeJSON, "--merge-json")
	if err != nil {
		return nil, err
	}
	if normalize != nil {
		normalize(patch)
	}
	current, err := fetchObject(ctx, client, fetchPath)
	if err != nil {
		return nil, err
	}
	merged := mergeAliasPayload(current, patch)
	if normalize != nil {
		normalize(merged)
	}
	return merged, nil
}

type updateSpec struct {
	Use   string
	Short string
	Long  string
	// Path maps the positional args to the resource path used for both the
	// merge fetch and the PUT.
	Path func(args []string) string
	// Normalize, when non-nil, adjusts the parsed payload or patch before use.
	Normalize func(map[string]any)
}

// updateCommand builds an update mutation with the uniform contract:
// --json = full replacement, --merge-json = fetch-and-merge, exactly one
// required, empty 204 success reported as {"updated": true}.
func updateCommand(deps Dependencies, spec updateSpec) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			path := spec.Path(args)
			body, err := resolveUpdateBody(ctx, deps.Client, path, jsonPayload, mergeJSON, spec.Normalize)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

type targetActionSpec struct {
	Use   string
	Short string
	Long  string
	// Path maps the positional args to the action path (e.g. /media/{id}/move).
	Path func(args []string) string
	// Verb names the action in empty-success output (e.g. "moved").
	Verb string
}

// targetActionCommand builds a move/copy-style mutation: POST with either
// a raw --json body or the --to shortcut that expands to {target:{id}}.
func targetActionCommand(deps Dependencies, spec targetActionSpec) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := targetActionBody(jsonPayload, to)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), spec.Path(args), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, spec.Verb, result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Action payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut for {\"target\":{\"id\":...}}")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// targetActionBody resolves the --json/--to pair shared by move/copy
// commands. --to expands to the {target:{id}} envelope; passing a null
// target via --json '{\"target\":null}' moves to the root.
func targetActionBody(jsonPayload string, to string) (map[string]any, error) {
	if strings.TrimSpace(jsonPayload) != "" {
		return parsePayload(jsonPayload)
	}
	if err := requireValue("--to", to); err != nil {
		return nil, err
	}
	return map[string]any{"target": map[string]any{"id": to}}, nil
}

// deleteCommand builds a hard-delete mutation. Hard deletes require --force
// or --dry-run, matching the gate on bulk updates: an agent must rehearse
// or explicitly confirm before destroying data. Recycle-bin moves (trash)
// are reversible and intentionally not gated.
func deleteCommand(deps Dependencies, use string, short string, path func(args []string) string) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !dryRun {
				return fmt.Errorf("%s permanently deletes; pass --force to confirm or --dry-run to rehearse", cmd.CommandPath())
			}
			result, err := deps.Client.Delete(cmd.Context(), path(args), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

type referencesSpec struct {
	Use   string
	Short string
	Long  string
	Path  func(args []string) string
}

// referencesCommand builds the paginated 'what references this' reads
// (referenced-by / referenced-descendants) shared by document and media.
// The endpoints return the standard {items, total} envelope, so pagination
// and triage compose the same way they do on children/root.
func referencesCommand(deps Dependencies, spec referencesSpec) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: spec.Path(args), opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

// areReferencedCommand builds the bulk reference check shared by document
// and media: GET /<resource>/are-referenced?id=...&id=...
func areReferencedCommand(deps Dependencies, resource string) *cobra.Command {
	var idsCSV string
	cmd := &cobra.Command{
		Use:   "are-referenced",
		Short: fmt.Sprintf("Bulk check: which of these %s IDs are referenced by something", resource),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("%s are-referenced requires --ids <comma-separated guids>", resource)
			}
			result, err := deps.Client.Get(cmd.Context(), "/"+resource+"/are-referenced", api.RequestOptions{Params: map[string]any{"id": stringsToAny(ids)}})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", fmt.Sprintf("Comma-separated %s GUIDs to check (required)", resource))
	return cmd
}
