package cmdkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

// ResolveUpdateBody enforces the uniform update contract: --json replaces
// the resource wholesale, --merge-json fetches the current resource and
// deep-merges the patch so unmentioned fields survive. Exactly one of the
// two must be provided. normalize runs on the parsed user input before any
// merge (input conveniences, shape rejection); normalizeMerged runs on the
// final body (output hygiene such as stripping response-only fields) and
// must be idempotent.
func ResolveUpdateBody(ctx context.Context, client *api.Client, fetchPath string, apiPrefix string, jsonPayload string, mergeJSON string, normalize func(map[string]any) error, normalizeMerged func(map[string]any) error) (map[string]any, error) {
	hasJSON := strings.TrimSpace(jsonPayload) != ""
	hasMerge := strings.TrimSpace(mergeJSON) != ""
	if hasJSON == hasMerge {
		return nil, fmt.Errorf("update requires exactly one of --json (full replacement) or --merge-json (fetch and merge)")
	}

	if hasJSON {
		body, err := ParsePayload(jsonPayload)
		if err != nil {
			return nil, err
		}
		if normalize != nil {
			if err := normalize(body); err != nil {
				return nil, err
			}
		}
		if normalizeMerged != nil {
			if err := normalizeMerged(body); err != nil {
				return nil, err
			}
		}
		return body, nil
	}

	patch, err := ParseJSONObject(mergeJSON, "--merge-json")
	if err != nil {
		return nil, err
	}
	if normalize != nil {
		if err := normalize(patch); err != nil {
			return nil, err
		}
	}
	current, err := FetchObject(ctx, client, fetchPath, api.RequestOptions{APIPrefix: apiPrefix})
	if err != nil {
		return nil, err
	}
	merged := MergeAliasPayload(current, patch)
	if normalizeMerged != nil {
		if err := normalizeMerged(merged); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// StripFields returns a Normalize that deletes response-only keys echoed
// by the merge fetch which the update model rejects (update request models
// with additionalProperties: false). Idempotent, as Normalize requires.
func StripFields(keys ...string) func(map[string]any) error {
	return func(body map[string]any) error {
		for _, key := range keys {
			delete(body, key)
		}
		return nil
	}
}

// UpdateSpec configures UpdateCommand.
type UpdateSpec struct {
	Use   string
	Short string
	Long  string
	// Path maps the positional args to the resource path used for both the
	// merge fetch and the PUT.
	Path func(args []string) string
	// Normalize, when non-nil, adjusts or rejects the parsed user input
	// (the --json body or --merge-json patch) before any merge. Use it for
	// input conveniences like accepting alternate shapes.
	Normalize func(map[string]any) error
	// NormalizeMerged, when non-nil, adjusts the final body after the merge
	// fetch. Use it for output hygiene like stripping response-only fields
	// the update model rejects. Must be idempotent.
	NormalizeMerged func(map[string]any) error
	// BindArgs, when non-nil, runs last on the final body with the
	// positional args, for update models that must restate the path id in
	// the body (and must not name a different one).
	BindArgs func(args []string, body map[string]any) error
	// APIPrefix overrides the default core Management API mount.
	APIPrefix string
}

// UpdateCommand builds an update mutation with the uniform contract:
// --json = full replacement, --merge-json = fetch-and-merge, exactly one
// required, empty 204 success reported as {"updated": true}.
func UpdateCommand(deps Dependencies, spec UpdateSpec) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var backup string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			path := spec.Path(args)
			body, err := ResolveUpdateBody(ctx, deps.Client, path, spec.APIPrefix, jsonPayload, mergeJSON, spec.Normalize, spec.NormalizeMerged)
			if err != nil {
				return err
			}
			if spec.BindArgs != nil {
				if err := spec.BindArgs(args, body); err != nil {
					return err
				}
			}
			var backupFile string
			if cmd.Flags().Changed("backup") && !dryRun {
				resource := "resource"
				if parent := cmd.Parent(); parent != nil {
					resource = parent.Name()
				}
				current, err := FetchObject(ctx, deps.Client, path, api.RequestOptions{APIPrefix: spec.APIPrefix})
				if err != nil {
					return fmt.Errorf("--backup could not read the current %s: %w", resource, err)
				}
				backupFile, err = WriteBackup(ResolveBackupPath(backup, resource, args[0]), resource, args[0], path, current, nil)
				if err != nil {
					return err
				}
			}
			result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun, APIPrefix: spec.APIPrefix})
			if err != nil {
				return err
			}
			if backupFile != "" {
				// Always surface the backup path: auto names carry a random
				// suffix, so the caller cannot reconstruct it.
				if result == nil {
					return PrintResult(cmd, deps, map[string]any{"updated": true, "backup": backupFile})
				}
				if body, ok := result.(map[string]any); ok {
					body["backup"] = backupFile
					return PrintResult(cmd, deps, body)
				}
				return PrintResult(cmd, deps, map[string]any{"updated": result, "backup": backupFile})
			}
			return PrintMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved)")
	AddBackupFlag(cmd, &backup)
	AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// CreateSpec configures CreateCommand.
type CreateSpec struct {
	Use   string
	Short string
	Long  string
	// Path is the collection endpoint the payload is POSTed to.
	Path string
	// TemplateKey, when non-empty, wires --print-template to the schema
	// template with this key.
	TemplateKey string
	// PayloadUsage overrides the --json flag help text.
	PayloadUsage string
	// Base, when non-nil, is consulted at run time for a starting payload
	// fetched from the server (e.g. a blueprint scaffold). Returning a
	// non-nil body makes --json optional and deep-merges the caller's JSON
	// on top of that base; returning nil keeps the plain --json contract.
	Base func(ctx context.Context) (map[string]any, error)
	// Validate, when non-nil, checks the flag values on their own. It runs
	// before Base, so a rejected flag combination costs no request. Checks
	// that need the payload belong in the Flags hook instead.
	Validate func() error
	// Normalize, when non-nil, adjusts or rejects the parsed --json payload
	// (input conveniences, shape rejection). It sees the caller's input, not
	// the Base payload it is later merged onto, and is skipped when no
	// --json was given.
	Normalize func(map[string]any) error
	// Flags, when non-nil, registers extra convenience flags on the command
	// and returns a hook applied to the final payload (after any Base merge).
	Flags func(cmd *cobra.Command) func(map[string]any) error
	// RouteOverride, when non-nil, is consulted after the flag hook and may
	// return a sibling collection endpoint for the POST; returning "" keeps
	// Path. Lets a convenience flag redirect to a combined operation (e.g.
	// document create --publish → /document/create-and-publish).
	RouteOverride func() string
	// ResultKeys are extra payload fields echoed into the create result
	// alongside the defaults (e.g. "icon", "kind").
	ResultKeys []string
	// APIPrefix overrides the default core Management API mount for the
	// POST. A Base fetch picks its own mount.
	APIPrefix string
}

// CreateCommand builds a create mutation with the uniform contract:
// required --json payload, optional --print-template skeleton, CLI-generated
// id when omitted, and a create result echoing identity fields from the
// payload when the server answers with an empty body.
func CreateCommand(deps Dependencies, spec CreateSpec) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	var applyFlags func(map[string]any) error
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return PrintResult(cmd, deps, schema.Templates[spec.TemplateKey])
			}
			// Everything that can be decided from the command line alone
			// runs before Base: a Base fetch is a network round trip, and a
			// malformed --json or a rejected flag combination must surface
			// as a usage error rather than as whatever the server (or the
			// token request in front of it) says first.
			var userBody map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := ParsePayload(jsonPayload)
				if err != nil {
					return err
				}
				userBody = parsed
			}
			if spec.Validate != nil {
				if err := spec.Validate(); err != nil {
					return err
				}
			}
			if userBody != nil && spec.Normalize != nil {
				if err := spec.Normalize(userBody); err != nil {
					return err
				}
			}
			var base map[string]any
			if spec.Base != nil {
				fetched, err := spec.Base(cmd.Context())
				if err != nil {
					return err
				}
				base = fetched
			}
			if base == nil {
				if err := RequireValue("--json", jsonPayload); err != nil {
					return err
				}
			}
			body := userBody
			if body == nil {
				body = map[string]any{}
			}
			if base != nil {
				body = MergeAliasPayload(base, body)
			}
			if applyFlags != nil {
				if err := applyFlags(body); err != nil {
					return err
				}
			}
			if _, err := EnsurePayloadID(body); err != nil {
				return err
			}
			path := spec.Path
			if spec.RouteOverride != nil {
				if override := spec.RouteOverride(); override != "" {
					path = override
				}
			}
			result, err := deps.Client.Post(cmd.Context(), path, body, api.RequestOptions{DryRun: dryRun, APIPrefix: spec.APIPrefix})
			if err != nil {
				return err
			}
			return PrintResult(cmd, deps, CreateResult(result, body, spec.ResultKeys...))
		},
	}
	payloadUsage := spec.PayloadUsage
	if payloadUsage == "" {
		payloadUsage = "Create payload as JSON"
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", payloadUsage)
	AddDryRunFlag(cmd, &dryRun)
	if spec.TemplateKey != "" {
		cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	}
	if spec.Flags != nil {
		applyFlags = spec.Flags(cmd)
	}
	return cmd
}

// TargetActionSpec configures TargetActionCommand.
type TargetActionSpec struct {
	Use   string
	Short string
	Long  string
	// Candidates lists method+path attempts in modern-first order; later
	// candidates are tried on 404/405 (older Umbraco versions).
	Candidates func(args []string) []MutationCandidate
	// Verb names the action in empty-success output (e.g. "moved").
	Verb string
	// APIPrefix overrides the default core Management API mount.
	APIPrefix string
}

// TargetActionCommand builds a move/copy-style mutation with either a raw
// --json body or the --to shortcut that expands to {target:{id}}.
func TargetActionCommand(deps Dependencies, spec TargetActionSpec) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := TargetActionBody(jsonPayload, to)
			if err != nil {
				return err
			}
			result, err := MutateWithFallback(cmd.Context(), deps.Client, body, api.RequestOptions{DryRun: dryRun, APIPrefix: spec.APIPrefix}, spec.Candidates(args)...)
			if err != nil {
				return err
			}
			return PrintMutationResult(cmd, deps, spec.Verb, result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Action payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut for {\"target\":{\"id\":...}}")
	AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// TargetActionBody resolves the --json/--to pair shared by move/copy
// commands. --to expands to the {target:{id}} envelope; passing a null
// target via --json '{\"target\":null}' moves to the root.
func TargetActionBody(jsonPayload string, to string) (map[string]any, error) {
	if strings.TrimSpace(jsonPayload) != "" {
		return ParsePayload(jsonPayload)
	}
	if err := RequireValue("--to", to); err != nil {
		return nil, err
	}
	return map[string]any{"target": map[string]any{"id": to}}, nil
}

// DeleteSpec configures DeleteCommand.
type DeleteSpec struct {
	Use   string
	Short string
	// Path maps the positional args to the resource path.
	Path func(args []string) string
	// APIPrefix overrides the default core Management API mount.
	APIPrefix string
}

// DeleteCommand builds a hard-delete mutation. Hard deletes require --force
// or --dry-run, matching the gate on bulk updates: an agent must rehearse
// or explicitly confirm before destroying data. Recycle-bin moves (trash)
// are reversible and intentionally not gated.
func DeleteCommand(deps Dependencies, spec DeleteSpec) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireForceOrDryRun(cmd, "permanently deletes", force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), spec.Path(args), api.RequestOptions{DryRun: dryRun, APIPrefix: spec.APIPrefix})
			if err != nil {
				return err
			}
			return PrintMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	AddDryRunFlag(cmd, &dryRun)
	return cmd
}
