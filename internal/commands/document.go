package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

func RegisterDocument(root *cobra.Command, deps Dependencies) {
	document := &cobra.Command{
		Use:     "document",
		Aliases: []string{"doc"},
		Short:   "Document and content management operations",
	}

	document.AddCommand(documentGet(deps))
	document.AddCommand(documentRoot(deps))
	document.AddCommand(documentChildren(deps))
	document.AddCommand(documentAncestors(deps))
	document.AddCommand(documentSearch(deps))
	document.AddCommand(documentCreate(deps))
	document.AddCommand(documentUpdate(deps))
	document.AddCommand(documentBulkUpdate(deps))
	document.AddCommand(documentCSVUpdate(deps))
	document.AddCommand(documentUpdateProperties(deps))
	document.AddCommand(documentPublish(deps))
	document.AddCommand(documentUnpublish(deps))
	document.AddCommand(documentPublishDescendants(deps))
	document.AddCommand(documentPublishDescendantsResult(deps))
	document.AddCommand(documentSort(deps))
	document.AddCommand(documentDomains(deps))
	document.AddCommand(documentPublicAccess(deps))
	document.AddCommand(documentCopy(deps))
	document.AddCommand(documentMove(deps))
	document.AddCommand(documentDelete(deps))
	document.AddCommand(documentTrash(deps))
	document.AddCommand(documentRestore(deps))
	document.AddCommand(documentReferences(deps))
	document.AddCommand(documentReferencedDescendants(deps))
	document.AddCommand(documentAreReferenced(deps))
	document.AddCommand(documentVersion(deps))
	document.AddCommand(documentAuditLog(deps))

	root.AddCommand(document)
}

func documentGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get a document by ID",
		Path:  func(args []string) string { return api.JoinPath("/document/%s", args[0]) },
	})
}

func documentRoot(deps Dependencies) *cobra.Command {
	var resolveDoctype bool
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "root",
		Short: "Get root documents (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document/root", opts: api.RequestOptions{Params: params}},
				{path: "/document/root", opts: api.RequestOptions{Params: params}},
			}
		},
		Enrich: func(ctx context.Context, result any) (any, error) {
			if !resolveDoctype {
				return result, nil
			}
			return resolveDocumentTypeAliases(ctx, deps, result)
		},
	})
	addResolveDoctypeFlag(cmd, &resolveDoctype)
	return cmd
}

func documentChildren(deps Dependencies) *cobra.Command {
	var resolveDoctype bool
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "children <id>",
		Short: "Get child documents (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document/children", opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
				{path: api.JoinPath("/document/%s/children", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
		Enrich: func(ctx context.Context, result any) (any, error) {
			if !resolveDoctype {
				return result, nil
			}
			return resolveDocumentTypeAliases(ctx, deps, result)
		},
	})
	addResolveDoctypeFlag(cmd, &resolveDoctype)
	return cmd
}

func addResolveDoctypeFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "resolve-doctype", false, "Annotate each item's documentType with its alias (tree responses carry only the id; this fetches each distinct document type once)")
}

// resolveDocumentTypeAliases annotates the documentType reference on each
// item with the type's alias. Tree responses carry only {id, icon} for the
// document type, which leaves agents unable to reason about content types
// without per-item lookups; this resolves each distinct type exactly once.
func resolveDocumentTypeAliases(ctx context.Context, deps Dependencies, result any) (any, error) {
	aliases := map[string]string{}
	for _, item := range resultItems(result) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		docType, ok := entry["documentType"].(map[string]any)
		if !ok {
			continue
		}
		id, _ := docType["id"].(string)
		if id == "" {
			continue
		}
		alias, known := aliases[id]
		if !known {
			detail, err := fetchObject(ctx, deps.Client, api.JoinPath("/document-type/%s", id), api.RequestOptions{})
			if err != nil {
				return nil, fmt.Errorf("could not resolve document type %s: %w", id, err)
			}
			alias, _ = detail["alias"].(string)
			aliases[id] = alias
		}
		if alias != "" {
			docType["alias"] = alias
		}
	}
	return result, nil
}

func documentAncestors(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "ancestors <id>",
		Short: "Get ancestor documents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := getWithFallback(
				cmd.Context(),
				deps.Client,
				getRequestCandidate{
					path: "/tree/document/ancestors",
					opts: api.RequestOptions{Params: map[string]any{"descendantId": args[0]}},
				},
				getRequestCandidate{
					path: api.JoinPath("/document/%s/ancestors", args[0]),
					opts: api.RequestOptions{},
				},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func documentSearch(deps Dependencies) *cobra.Command {
	return searchCommand(deps, searchSpec{
		Use:   "search",
		Short: "Search documents",
		Extra: []paramFlag{
			{Flag: "under", Param: "parentId", Usage: "Limit search to documents under the given parent ID"},
		},
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/item/document/search", opts: api.RequestOptions{Params: params}},
				{path: "/document/search", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func documentCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a document",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return printResult(cmd, deps, schema.Templates["document.create"])
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}
			if _, err := ensurePayloadID(body); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/document", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func documentUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var property string
	var value string
	var valueJSON string
	var saveAndPublish bool
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			path := api.JoinPath("/document/%s", args[0])
			hasProperty := strings.TrimSpace(property) != ""
			hasJSON := strings.TrimSpace(jsonPayload) != ""
			hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
			modes := 0
			for _, set := range []bool{hasProperty, hasJSON, hasMergeJSON} {
				if set {
					modes++
				}
			}
			if modes != 1 {
				return fmt.Errorf("document update requires exactly one of --json, --merge-json, or --property")
			}

			var body map[string]any
			var err error
			if hasProperty {
				patch, err := documentPropertyPatch(property, value, valueJSON)
				if err != nil {
					return err
				}
				current, err := fetchObject(ctx, deps.Client, path, api.RequestOptions{})
				if err != nil {
					return err
				}
				body = mergeAliasPayload(current, patch)
			} else {
				body, err = resolveUpdateBody(ctx, deps.Client, path, "", jsonPayload, mergeJSON, nil, nil)
				if err != nil {
					return err
				}
			}

			result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			if !saveAndPublish {
				return printMutationResult(cmd, deps, "updated", result, dryRun)
			}

			publishBody, err := documentPublishBody("", culture)
			if err != nil {
				return err
			}
			publishResult, err := publishWithInvariantRaceRetry(ctx, deps.Client, args[0], publishBody, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			return printResult(cmd, deps, map[string]any{
				"saveAndPublish": true,
				"updated":        coalescePutResult(result, dryRun),
				"published":      coalescePutResult(publishResult, dryRun),
			})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current document before update (fields not mentioned are preserved)")
	cmd.Flags().StringVar(&property, "property", "", "Update a single property alias without constructing the full payload")
	cmd.Flags().StringVar(&value, "value", "", "String value used with --property")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "JSON value used with --property")
	cmd.Flags().BoolVar(&saveAndPublish, "save-and-publish", false, "Publish the document after a successful update")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --save-and-publish")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentBulkUpdate(deps Dependencies) *cobra.Command {
	var ids []string
	var idFile string
	var jsonPayload string
	var mergeJSON string
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "bulk-update",
		Short: "Update multiple documents from an explicit ID list",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && !force {
				return fmt.Errorf("document bulk-update requires --dry-run or --force")
			}

			hasJSON := strings.TrimSpace(jsonPayload) != ""
			hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
			if hasJSON == hasMergeJSON {
				return fmt.Errorf("document bulk-update requires exactly one of --json or --merge-json")
			}

			resolvedIDs, err := loadDocumentIDs(ids, idFile)
			if err != nil {
				return err
			}
			if len(resolvedIDs) == 0 {
				return fmt.Errorf("document bulk-update requires at least one --id or --id-file entry")
			}

			var fullBody map[string]any
			var mergePatch map[string]any
			if hasMergeJSON {
				mergePatch, err = parsePayload(mergeJSON)
				if err != nil {
					return err
				}
			} else {
				fullBody, err = parsePayload(jsonPayload)
				if err != nil {
					return err
				}
			}

			result := executeDocumentBulkUpdate(cmd.Context(), deps.Client, resolvedIDs, fullBody, mergePatch, dryRun)
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringArrayVar(&ids, "id", nil, "Document ID to update; repeat for multiple documents")
	cmd.Flags().StringVar(&idFile, "id-file", "", "Path to a file containing document IDs, one per line")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload applied to every document")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into each current document before update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned requests without executing")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the bulk update when not using --dry-run")
	return cmd
}

func documentCSVUpdate(deps Dependencies) *cobra.Command {
	var file string
	var idColumn string
	var properties []string
	var fieldMappings []string
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "csv-update",
		Short: "Update multiple documents from a CSV file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && !force {
				return fmt.Errorf("document csv-update requires --dry-run or --force")
			}
			if err := requireValue("--file", file); err != nil {
				return err
			}

			mappings, err := parseDocumentCSVFieldMappings(properties, fieldMappings)
			if err != nil {
				return err
			}
			if len(mappings) == 0 {
				return fmt.Errorf("document csv-update requires at least one --property or --field mapping")
			}

			result, err := executeDocumentCSVUpdate(cmd.Context(), deps.Client, documentCSVUpdateOptions{
				File:     file,
				IDColumn: idColumn,
				Mappings: mappings,
				DryRun:   dryRun,
			})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Path to the CSV file")
	cmd.Flags().StringVar(&idColumn, "id-column", "id", "CSV column containing document IDs")
	cmd.Flags().StringArrayVar(&properties, "property", nil, "Property alias to update from a CSV column with the same name; repeat for multiple properties")
	cmd.Flags().StringArrayVar(&fieldMappings, "field", nil, "Explicit alias=column CSV mapping; repeat for multiple properties")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned CSV-driven updates without executing them")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the CSV-driven updates when not using --dry-run")
	return cmd
}

func documentUpdateProperties(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update-properties <id>",
		Short: "Update document properties (merges into values[] by alias)",
		Long: `Updates one or more property values on a document by merging into its values[] array.

Three input shapes are accepted:

  Object form (most common for invariant docs):
    --json '{"isFeatured": true, "products": ["Umbraco CMS"]}'
    Each key becomes a values[] entry with culture=null, segment=null.

  Array form (for culture/segment-variant properties):
    --json '[{"alias":"title","value":"Hi","culture":"en-US","segment":null}]'
    Used verbatim as values[].

  Envelope form (matches 'document update --merge-json'):
    --json '{"values":[{"alias":"title","value":"Hi","culture":null,"segment":null}]}'

In all shapes the resulting values[] is merged by alias into the current document, so untouched properties survive.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			patch, err := buildUpdatePropertiesPatch(jsonPayload)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			path := api.JoinPath("/document/%s", args[0])
			current, err := fetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return err
			}
			merged := mergeAliasPayload(current, patch)
			result, err := deps.Client.Put(ctx, path, merged, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Properties payload as JSON; accepts object {alias: value}, array [{alias, value, culture?, segment?}], or envelope {\"values\":[...]}")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentPublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := documentPublishBody(jsonPayload, culture)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/publish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "published", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Publish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// invariantRaceMaxAttempts is the upper bound on retries for the spurious
// "culture for invariant content" 400 that the Management API throws under
// rapid back-to-back save-and-publish loops. The error is timing-dependent
// and clears on retry; 4 attempts with exponential-ish backoff matches what
// the bug report saw work in practice.
const invariantRaceMaxAttempts = 4

var invariantRaceBackoffs = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second}

// publishWithInvariantRaceRetry PUTs the publish body and retries on the
// specific 400 "culture for invariant content" error that Umbraco intermittently
// returns under tight save-and-publish loops on invariant content. The
// payload is valid (verified via --dry-run in the bug report) — the same
// request succeeds on retry, so the retry is the right correctness-preserving
// workaround at the CLI layer. Other 400s are surfaced immediately.
func publishWithInvariantRaceRetry(ctx context.Context, client *api.Client, id string, body map[string]any, opts api.RequestOptions) (any, error) {
	path := api.JoinPath("/document/%s/publish", id)
	var lastErr error
	for attempt := 0; attempt < invariantRaceMaxAttempts; attempt++ {
		result, err := client.Put(ctx, path, body, opts)
		if err == nil {
			return result, nil
		}
		if opts.DryRun || !isInvariantContentRaceError(err) || attempt == invariantRaceMaxAttempts-1 {
			return nil, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(invariantRaceBackoffs[attempt]):
		}
	}
	return nil, lastErr
}

// isInvariantContentRaceError matches the spurious 400 the Management API
// returns under the save-and-publish race. The payload looks like
// {"detail":"One or more property values specify a culture for an [invariant content]"}.
// Substring-match on "invariant content" inside the rendered error is robust
// to message phrasing tweaks without false-positiving on unrelated 400s.
func isInvariantContentRaceError(err error) bool {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 {
		return false
	}
	return strings.Contains(apiErr.Error(), "invariant content")
}

func documentPublishBody(jsonPayload string, culture string) (map[string]any, error) {
	if strings.TrimSpace(jsonPayload) != "" {
		return parsePayload(jsonPayload)
	}
	if strings.TrimSpace(culture) != "" {
		return map[string]any{"cultures": []any{culture}}, nil
	}
	return map[string]any{
		"publishSchedules": []any{
			map[string]any{"culture": nil},
		},
	}, nil
}

func documentUnpublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "unpublish <id>",
		Short: "Unpublish a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			var err error
			if jsonPayload != "" {
				body, err = parsePayload(jsonPayload)
			} else if culture != "" {
				body = map[string]any{"cultures": []any{culture}}
			} else {
				body = map[string]any{}
			}
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/unpublish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "unpublished", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Unpublish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentCopy(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var publish bool
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "copy <id>",
		Short: "Copy a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			body, err := targetActionBody(jsonPayload, to)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(ctx, api.JoinPath("/document/%s/copy", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if !publish {
				return printMutationResult(cmd, deps, "copied", result, dryRun)
			}

			// On dry-run no copy happens, so there is no real ID to chain;
			// the publish step is planned against a placeholder instead.
			copiedID := "copied-document-id"
			if !dryRun {
				copiedID = extractResultID(result)
				if copiedID == "" {
					return fmt.Errorf("document copy --publish requires the copy response to include the new document id")
				}
			}
			publishBody, err := documentPublishBody("", culture)
			if err != nil {
				return err
			}
			publishResult, err := deps.Client.Put(ctx, api.JoinPath("/document/%s/publish", copiedID), publishBody, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, map[string]any{
				"copied":    result,
				"published": coalescePutResult(publishResult, dryRun),
			})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Copy payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish the copied document after a successful copy")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --publish")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func extractResultID(result any) string {
	payload, ok := result.(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := payload["id"].(string); ok {
		return id
	}
	return ""
}

func documentMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move a document",
		Candidates: func(args []string) []mutationCandidate {
			path := api.JoinPath("/document/%s/move", args[0])
			return []mutationCandidate{{method: "PUT", path: path}, {method: "POST", path: path}}
		},
		Verb: "moved",
	})
}

func documentDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a document (use 'trash' for the recycle bin)",
		Path: func(args []string) string {
			return api.JoinPath("/document/%s", args[0])
		},
	})
}

func documentTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "trash <id>",
		Short: "Move a document to recycle bin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := api.JoinPath("/document/%s/move-to-recycle-bin", args[0])
			result, err := mutateWithFallback(cmd.Context(), deps.Client, map[string]any{}, api.RequestOptions{DryRun: dryRun},
				mutationCandidate{method: "PUT", path: path},
				mutationCandidate{method: "POST", path: path},
			)
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "trashed", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentRestore(deps Dependencies) *cobra.Command {
	var to string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore a document from the recycle bin",
		Long:  "PUT /recycle-bin/document/{id}/restore. The restore target defaults to the document's original parent (looked up via the recycle-bin API); pass --to for a different parent, or --to root to restore at the content root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			var target any
			switch {
			case strings.EqualFold(strings.TrimSpace(to), "root"):
				target = nil
			case strings.TrimSpace(to) != "":
				target = map[string]any{"id": to}
			default:
				original, err := deps.Client.Get(ctx, api.JoinPath("/recycle-bin/document/%s/original-parent", args[0]), api.RequestOptions{})
				if err != nil {
					// A 404 means the recycle-bin API is absent (older
					// servers, where the legacy restore needs no target
					// anyway) or the lookup has nothing to report — either
					// way the restore call itself gives the real answer.
					var apiErr *api.APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
						return fmt.Errorf("could not resolve the original parent (pass --to <parent-id> or --to root): %w", err)
					}
				}
				if id := extractResultID(original); id != "" {
					target = map[string]any{"id": id}
				}
			}

			result, err := mutateWithFallback(ctx, deps.Client, map[string]any{"target": target}, api.RequestOptions{DryRun: dryRun},
				mutationCandidate{method: "PUT", path: api.JoinPath("/recycle-bin/document/%s/restore", args[0])},
				mutationCandidate{method: "POST", path: api.JoinPath("/document/%s/restore", args[0])},
			)
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "restored", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Restore target parent ID, or 'root' (defaults to the original parent)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentReferences(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "references <id>",
		Short: "List items that reference this document (paginated; --skip/--take/--all)",
		Long:  "Wraps GET /document/{id}/referenced-by. Used to answer 'what uses this node' for orphan checks, safe-delete verification, and taxonomy usage audits.",
		Path:  func(args []string) string { return api.JoinPath("/document/%s/referenced-by", args[0]) },
	})
}

func documentReferencedDescendants(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "referenced-descendants <id>",
		Short: "List items that reference this document or any of its descendants",
		Path:  func(args []string) string { return api.JoinPath("/document/%s/referenced-descendants", args[0]) },
	})
}

func documentAreReferenced(deps Dependencies) *cobra.Command {
	return areReferencedCommand(deps, "document")
}
