package commands

import (
	"context"
	"errors"
	"fmt"
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
	document.AddCommand(documentCopy(deps))
	document.AddCommand(documentMove(deps))
	document.AddCommand(documentDelete(deps))
	document.AddCommand(documentTrash(deps))
	document.AddCommand(documentRestore(deps))

	root.AddCommand(document)
}

func documentGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a document by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/document/%s", args[0]), api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func documentRoot(deps Dependencies) *cobra.Command {
	var fields string
	var paramsRaw string
	var skip, take int
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "root",
		Short: "Get root documents (paginated; use --skip/--take to walk past the server page size)",
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			params = applyPaginationParams(params, skip, take)
			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{path: "/tree/document/root", opts: api.RequestOptions{Fields: fields, Params: params}},
				getRequestCandidate{path: "/document/root", opts: api.RequestOptions{Fields: fields, Params: params}},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	addPaginationFlags(cmd, &skip, &take)
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func documentChildren(deps Dependencies) *cobra.Command {
	var fields string
	var skip, take int
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "children <id>",
		Short: "Get child documents (paginated; use --skip/--take to walk past the server page size)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			treeParams := applyPaginationParams(map[string]any{"parentId": args[0]}, skip, take)
			legacyParams := applyPaginationParams(nil, skip, take)
			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{
					path: "/tree/document/children",
					opts: api.RequestOptions{Fields: fields, Params: treeParams},
				},
				getRequestCandidate{
					path: fmt.Sprintf("/document/%s/children", args[0]),
					opts: api.RequestOptions{Fields: fields, Params: legacyParams},
				},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addPaginationFlags(cmd, &skip, &take)
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func documentAncestors(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ancestors <id>",
		Short: "Get ancestor documents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{
					path: "/tree/document/ancestors",
					opts: api.RequestOptions{Params: map[string]any{"descendantId": args[0]}},
				},
				getRequestCandidate{
					path: fmt.Sprintf("/document/%s/ancestors", args[0]),
					opts: api.RequestOptions{},
				},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	return cmd
}

func documentSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	var under string
	var skip int
	var take int

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			if params == nil {
				params = map[string]any{}
				if query != "" {
					params["query"] = query
				}
				if under != "" {
					params["parentId"] = under
				}
				if skip >= 0 {
					params["skip"] = skip
				}
				if take >= 0 {
					params["take"] = take
				}
			} else if under != "" {
				if _, exists := params["parentId"]; exists {
					return fmt.Errorf("--under cannot be combined with --params containing parentId")
				}
				params = cloneParams(params)
				params["parentId"] = under
			}
			if len(params) == 0 {
				return fmt.Errorf("document search requires either --params or --query")
			}

			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{path: "/item/document/search", opts: api.RequestOptions{Params: params}},
				getRequestCandidate{path: "/document/search", opts: api.RequestOptions{Params: params}},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search parameters as JSON")
	cmd.Flags().StringVar(&query, "query", "", "Search query (convenience)")
	cmd.Flags().StringVar(&under, "under", "", "Limit search to documents under the given parent ID")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
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
			result, err := deps.Client.Post(context.Background(), "/document", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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
			hasJSON := strings.TrimSpace(jsonPayload) != ""
			hasMergeJSON := strings.TrimSpace(mergeJSON) != ""
			hasProperty := strings.TrimSpace(property) != ""
			modeCount := 0
			if hasJSON {
				modeCount++
			}
			if hasMergeJSON {
				modeCount++
			}
			if hasProperty {
				modeCount++
			}
			if modeCount != 1 {
				return fmt.Errorf("document update requires exactly one of --json, --merge-json, or --property")
			}

			var body map[string]any
			var err error
			if hasProperty {
				patch, err := documentPropertyPatch(property, value, valueJSON)
				if err != nil {
					return err
				}

				current, err := fetchDocumentObject(context.Background(), deps.Client, args[0])
				if err != nil {
					return err
				}
				body = mergeAliasPayload(current, patch)
			} else if hasMergeJSON {
				patch, err := parsePayload(mergeJSON)
				if err != nil {
					return err
				}

				current, err := fetchDocumentObject(context.Background(), deps.Client, args[0])
				if err != nil {
					return err
				}
				body = mergeAliasPayload(current, patch)
			} else {
				body, err = parsePayload(jsonPayload)
				if err != nil {
					return err
				}
			}

			requestOptions := api.RequestOptions{DryRun: dryRun}
			if hasProperty || hasMergeJSON {
				requestOptions.SkipValidation = true
			}
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document/%s", args[0]), body, requestOptions)
			if err != nil {
				return err
			}

			if !saveAndPublish {
				if !dryRun && result == nil {
					return printResult(cmd, deps, map[string]any{"updated": true})
				}
				return printResult(cmd, deps, result)
			}

			publishBody, err := documentPublishBody("", culture)
			if err != nil {
				return err
			}
			publishResult, err := publishWithInvariantRaceRetry(context.Background(), deps.Client, args[0], publishBody, api.RequestOptions{DryRun: dryRun})
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
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into the current document before update")
	cmd.Flags().StringVar(&property, "property", "", "Update a single property alias without constructing the full payload")
	cmd.Flags().StringVar(&value, "value", "", "String value used with --property")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "JSON value used with --property")
	cmd.Flags().BoolVar(&saveAndPublish, "save-and-publish", false, "Publish the document after a successful update")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --save-and-publish")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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

			result := executeDocumentBulkUpdate(context.Background(), deps.Client, resolvedIDs, fullBody, mergePatch, dryRun)
			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringArrayVar(&ids, "id", nil, "Document ID to update; repeat for multiple documents")
	cmd.Flags().StringVar(&idFile, "id-file", "", "Path to a file containing document IDs, one per line")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload applied to every document")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON payload merged into each current document before update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate requests without executing")
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

			result, err := executeDocumentCSVUpdate(context.Background(), deps.Client, documentCSVUpdateOptions{
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the CSV-driven updates without executing them")
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

			current, err := fetchDocumentObject(context.Background(), deps.Client, args[0])
			if err != nil {
				return err
			}
			merged := mergeAliasPayload(current, patch)
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document/%s", args[0]), merged, api.RequestOptions{DryRun: dryRun, SkipValidation: true})
			if err != nil {
				return err
			}
			if !dryRun && result == nil {
				return printResult(cmd, deps, map[string]any{"updated": true})
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Properties payload as JSON; accepts object {alias: value}, array [{alias, value, culture?, segment?}], or envelope {\"values\":[...]}")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document/%s/publish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Publish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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
	path := fmt.Sprintf("/document/%s/publish", id)
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
			result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document/%s/unpublish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Unpublish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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
			var body map[string]any
			var err error
			if jsonPayload != "" {
				body, err = parsePayload(jsonPayload)
			} else {
				if err := requireValue("--to", to); err != nil {
					return err
				}
				body = map[string]any{"target": map[string]any{"id": to}}
			}
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document/%s/copy", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if !publish {
				return printResult(cmd, deps, result)
			}

			copiedID := extractResultID(result)
			if copiedID == "" {
				return fmt.Errorf("document copy --publish requires the copy response to include the new document id")
			}
			publishBody, err := documentPublishBody("", culture)
			if err != nil {
				return err
			}
			publishResult, err := deps.Client.Put(context.Background(), fmt.Sprintf("/document/%s/publish", copiedID), publishBody, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, map[string]any{
				"copied":    result,
				"published": publishResult,
			})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Copy payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish the copied document after a successful copy")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --publish")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
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
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Move a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			var err error
			if jsonPayload != "" {
				body, err = parsePayload(jsonPayload)
			} else {
				if err := requireValue("--to", to); err != nil {
					return err
				}
				body = map[string]any{"target": map[string]any{"id": to}}
			}
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document/%s/move", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Move payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func documentDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/document/%s", args[0]), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func documentTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "trash <id>",
		Short: "Move a document to recycle bin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document/%s/move-to-recycle-bin", args[0]), map[string]any{}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func documentRestore(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/document/%s/restore", args[0]), map[string]any{}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}
