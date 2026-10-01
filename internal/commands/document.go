package commands

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

func RegisterDocument(root *cobra.Command, deps cmdkit.Dependencies) {
	document := &cobra.Command{
		Use:     "document",
		Aliases: []string{"doc"},
		Short:   "Document and content management operations",
		Long: `Document and content management operations.

Task → command:
  Read a document, find documents                      document get <id>, document search --query <text>, document grep
  Change one property                                  document update <id> --property <alias> --value <v> --backup
  Change many documents the same way                   document update --ids a,b,c --merge-json '{...}' --force   (or --from-file ids.txt)
  Publish one / many                                   document publish <id>; document publish --ids a,b,c --force
  Change and publish many in one pass                  document update --ids … --merge-json … --save-and-publish --force
  Per-row values from a spreadsheet                    document csv-update --file rows.csv --property <alias>
  Undo a write                                         document restore-backup <file>   (from any --backup)
  Publish a whole subtree                              document publish-descendants <id>`,
	}

	document.AddCommand(documentGet(deps))
	document.AddCommand(documentURLs(deps))
	document.AddCommand(documentRoot(deps))
	document.AddCommand(documentChildren(deps))
	document.AddCommand(documentAncestors(deps))
	document.AddCommand(documentSearch(deps))
	document.AddCommand(documentGrep(deps))
	document.AddCommand(documentCreate(deps))
	document.AddCommand(documentUpdate(deps))
	document.AddCommand(documentBulkUpdate(deps))
	document.AddCommand(documentCSVUpdate(deps))
	document.AddCommand(documentUpdateProperties(deps))
	document.AddCommand(restoreBackupCommand(deps, restoreSpec{Use: "document", Display: "document", PathFormat: "/document/%s", Notes: "Publish state is not part of the backup: re-publish with 'document publish' if the restored version should go live."}))
	document.AddCommand(documentPublish(deps))
	document.AddCommand(documentUnpublish(deps))
	document.AddCommand(documentPublishDescendants(deps))
	document.AddCommand(documentPublishDescendantsResult(deps))
	document.AddCommand(documentSort(deps))
	document.AddCommand(sortChildrenCommand(deps, "document", true))
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
	document.AddCommand(recycleBinCommand(deps, "document"))
	document.AddCommand(documentVersion(deps))
	document.AddCommand(documentAuditLog(deps))

	root.AddCommand(document)
}

func documentGet(deps cmdkit.Dependencies) *cobra.Command {
	var trim cmdkit.OutputTrimOptions
	var withURLs bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a document by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.ValidateDocumentOutputTrim(trim); err != nil {
				return err
			}
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/document/%s", args[0]), api.RequestOptions{Fields: trim.Fields})
			if err != nil {
				return err
			}
			if withURLs {
				result, err = attachDocumentURLs(cmd.Context(), deps, args[0], result)
				if err != nil {
					return err
				}
			}
			result, err = cmdkit.ApplyDocumentOutputTrim(result, trim, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, result)
		},
	}
	cmdkit.AddDocumentOutputTrimFlags(cmd, &trim)
	cmd.Flags().BoolVar(&withURLs, "with-urls", false, "Fetch published document URL info and include it as urls in the response")
	return cmd
}

func documentRoot(deps cmdkit.Dependencies) *cobra.Command {
	var resolveDoctype bool
	cmd := cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:                "root",
		Short:              "Get root documents (paginated; --skip/--take/--all)",
		DocumentOutputTrim: true,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/tree/document/root", Opts: api.RequestOptions{Params: params}},
				{Path: "/document/root", Opts: api.RequestOptions{Params: params}},
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

func documentChildren(deps cmdkit.Dependencies) *cobra.Command {
	var resolveDoctype bool
	cmd := cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:                "children <id>",
		Short:              "Get child documents (paginated; --skip/--take/--all)",
		NArgs:              1,
		DocumentOutputTrim: true,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/tree/document/children", Opts: api.RequestOptions{Params: cmdkit.WithParam(params, "parentId", args[0])}},
				{Path: api.JoinPath("/document/%s/children", args[0]), Opts: api.RequestOptions{Params: params}},
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
func resolveDocumentTypeAliases(ctx context.Context, deps cmdkit.Dependencies, result any) (any, error) {
	aliases := map[string]string{}
	for _, item := range cmdkit.ResultItems(result) {
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
			detail, err := cmdkit.FetchObject(ctx, deps.Client, api.JoinPath("/document-type/%s", id), api.RequestOptions{})
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

func documentAncestors(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "ancestors <id>",
		Short: "Get ancestor documents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := cmdkit.GetWithFallback(
				cmd.Context(),
				deps.Client,
				cmdkit.GetRequestCandidate{
					Path: "/tree/document/ancestors",
					Opts: api.RequestOptions{Params: map[string]any{"descendantId": args[0]}},
				},
				cmdkit.GetRequestCandidate{
					Path: api.JoinPath("/document/%s/ancestors", args[0]),
					Opts: api.RequestOptions{},
				},
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, result)
		},
	}
}

func documentSearch(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.SearchCommand(deps, cmdkit.SearchSpec{
		Use:                "search",
		Short:              "Search documents",
		DocumentOutputTrim: true,
		Extra: []cmdkit.ParamFlag{
			{Flag: "under", Param: "parentId", Usage: "Limit search to documents under the given parent ID"},
		},
		Endpoints: func(params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/item/document/search", Opts: api.RequestOptions{Params: params}},
				{Path: "/document/search", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func documentCreate(deps cmdkit.Dependencies) *cobra.Command {
	var publish bool
	var cultures string
	var fromBlueprint string
	var parent string
	return cmdkit.CreateCommand(deps, cmdkit.CreateSpec{
		Use:   "create",
		Short: "Create a document",
		Long: "POST /document, or POST /document/create-and-publish with --publish (requires Umbraco 18.1+): the document is created and published in one atomic server-side operation, with --culture naming the cultures to publish (omit for invariant content).\n\n" +
			"--from-blueprint <id> seeds the payload from a document blueprint (GET /document-blueprint/{id}/scaffold, see 'umbraco blueprint'): the blueprint's document type, property values and variant names become the base payload and --json is then optional, deep-merged on top of it to override single values or rename the document. Pass --parent for the target parent, or omit it to create at the content root.",
		Path:         "/document",
		TemplateKey:  "document.create",
		PayloadUsage: "Full JSON payload (optional with --from-blueprint, where it is merged into the blueprint scaffold)",
		Base: func(ctx context.Context) (map[string]any, error) {
			if strings.TrimSpace(fromBlueprint) == "" {
				return nil, nil
			}
			return blueprintDocumentPayload(ctx, deps.Client, strings.TrimSpace(fromBlueprint))
		},
		Validate: func() error {
			if !publish && strings.TrimSpace(cultures) != "" {
				return fmt.Errorf("--culture requires --publish")
			}
			return nil
		},
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			cmd.Flags().BoolVar(&publish, "publish", false, "Create and publish atomically via POST /document/create-and-publish (Umbraco 18.1+)")
			cmd.Flags().StringVar(&cultures, "culture", "", "Comma-separated cultures to publish with --publish; omit for invariant content")
			cmd.Flags().StringVar(&fromBlueprint, "from-blueprint", "", "Seed the payload from this document blueprint's scaffold; --json is then optional and merged on top")
			cmd.Flags().StringVar(&parent, "parent", "", "Parent document GUID; omit to create at the content root (fills parent when the payload does not)")
			return func(body map[string]any) error {
				if trimmed := strings.TrimSpace(parent); trimmed != "" {
					if _, set := body["parent"]; !set {
						body["parent"] = map[string]any{"id": trimmed}
					}
				}
				if strings.TrimSpace(fromBlueprint) != "" && len(blueprintVariantNames(body)) == 0 {
					return fmt.Errorf("blueprint %s carries no variant name: supply one with --json '{\"variants\":[{\"name\":\"…\"}]}'", strings.TrimSpace(fromBlueprint))
				}
				if !publish {
					return nil
				}
				if _, ok := body["culturesToPublish"]; !ok {
					body["culturesToPublish"] = culturesToPublishList(cultures)
				}
				return nil
			}
		},
		RouteOverride: func() string {
			if publish {
				return "/document/create-and-publish"
			}
			return ""
		},
	})
}

// culturesToPublishList maps the --culture shortcut to the culturesToPublish
// array the combined create/update-and-publish operations require. Invariant
// content publishes with an empty list — the backoffice filters invariant
// variants out rather than sending a null culture.
func culturesToPublishList(cultures string) []any {
	names := cmdkit.UniqueCSV(cultures)
	list := make([]any, len(names))
	for i, name := range names {
		list[i] = name
	}
	return list
}

func documentUpdate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var property string
	var value string
	var valueJSON string
	var saveAndPublish bool
	var culture string
	var backup string
	var idsCSV string
	var fromFile string
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <id> | --ids a,b,c | --from-file ids.txt",
		Short: "Update a document (or several with --ids/--from-file)",
		Long: "PUT /document/{id}. Exactly one of --json (full replacement), --merge-json (fetch and deep-merge), or --property/--value. Pass --backup to save the current document first; 'document restore-backup <file>' puts it back. --save-and-publish publishes in the same operation on Umbraco 18.1+.\n\n" +
			"With --ids or --from-file the same change is applied to every listed document in sequence (--merge-json and --property merge into each document's own current state), one result row per document (id, name, update status, publish status, error). " +
			"A failure on one document does not stop the rest; the command exits 4 when any row failed. Multi-document runs require --force or --dry-run; a dry-run shows the planned requests for the first " + fmt.Sprint(documentBatchPlanWindow) + " documents and counts the rest. With --backup each document is saved first (bare --backup: auto-named files in the working directory; --backup=<dir>: inside that directory).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
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

			ids, err := resolveDocumentBatchTargets(cmd, args, idsCSV, fromFile, force, dryRun, "mutates every listed document")
			if err != nil {
				return err
			}
			if ids != nil {
				opts := documentBatchOptions{IDs: ids, Update: true, Publish: saveAndPublish, Culture: culture, DryRun: dryRun}
				if cmd.Flags().Changed("backup") {
					opts.Backup = backup
					if strings.TrimSpace(opts.Backup) == "" {
						opts.Backup = cmdkit.BackupAutoValue
					}
				}
				switch {
				case hasJSON:
					opts.FullBody, err = cmdkit.ParsePayload(jsonPayload)
				case hasMergeJSON:
					opts.MergePatch, err = cmdkit.ParseJSONObject(mergeJSON, "--merge-json")
				default:
					opts.MergePatch, err = documentPropertyPatch(property, value, cmd.Flags().Changed("value"), valueJSON, cmd.Flags().Changed("value-json"))
				}
				if err != nil {
					return err
				}
				return printDocumentBatch(cmd, deps, executeDocumentBatch(ctx, deps.Client, opts))
			}
			path := api.JoinPath("/document/%s", args[0])

			var body map[string]any
			if hasProperty {
				patch, err := documentPropertyPatch(property, value, cmd.Flags().Changed("value"), valueJSON, cmd.Flags().Changed("value-json"))
				if err != nil {
					return err
				}
				current, err := cmdkit.FetchObject(ctx, deps.Client, path, api.RequestOptions{})
				if err != nil {
					return err
				}
				body = cmdkit.MergeAliasPayload(current, patch)
			} else {
				body, err = cmdkit.ResolveUpdateBody(ctx, deps.Client, path, "", jsonPayload, mergeJSON, nil, nil)
				if err != nil {
					return err
				}
			}

			// Content writes are the riskiest ones: the backup is taken from
			// the server right before the PUT (never from the merged body).
			var backupFile string
			if cmd.Flags().Changed("backup") && !dryRun {
				current, err := cmdkit.FetchObject(ctx, deps.Client, path, api.RequestOptions{})
				if err != nil {
					return fmt.Errorf("--backup could not read the current document: %w", err)
				}
				backupFile, err = writeEntityBackup(cmd, backup, "document", args[0], path, current)
				if err != nil {
					return err
				}
			}

			if !saveAndPublish {
				result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
				if err != nil {
					return err
				}
				if backupFile != "" {
					return cmdkit.PrintResult(cmd, deps, withBackupPath(result, backupFile, "updated"))
				}
				return cmdkit.PrintMutationResult(cmd, deps, "updated", result, dryRun)
			}

			// Modern servers (18.1+) update and publish in one atomic
			// operation, which also sidesteps the invariant-content publish
			// race the two-call flow has to retry around.
			atomicBody := make(map[string]any, len(body)+1)
			for k, v := range body {
				atomicBody[k] = v
			}
			if _, ok := atomicBody["culturesToPublish"]; !ok {
				atomicBody["culturesToPublish"] = culturesToPublishList(culture)
			}
			atomicResult, err := deps.Client.Put(ctx, api.JoinPath("/document/%s/update-and-publish", args[0]), atomicBody, api.RequestOptions{DryRun: dryRun})
			if err == nil {
				return cmdkit.PrintResult(cmd, deps, withBackupPath(map[string]any{
					"saveAndPublish": true,
					"atomic":         true,
					"updated":        coalescePutResult(atomicResult, dryRun),
					"published":      coalescePutResult(atomicResult, dryRun),
				}, backupFile, "updated"))
			}
			if !cmdkit.IsAPIStatus(err, http.StatusNotFound) {
				return err
			}

			// Older servers: separate update and publish calls.
			result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			publishBody, err := documentPublishBody("", culture)
			if err != nil {
				return withBackupHint(err, "document", backupFile)
			}
			publishResult, err := publishWithInvariantRaceRetry(ctx, deps.Client, args[0], publishBody, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				// The update already landed; the caller must learn which
				// file undoes it (auto names carry a random suffix).
				return withBackupHint(err, "document", backupFile)
			}

			return cmdkit.PrintResult(cmd, deps, withBackupPath(map[string]any{
				"saveAndPublish": true,
				"updated":        coalescePutResult(result, dryRun),
				"published":      coalescePutResult(publishResult, dryRun),
			}, backupFile, "updated"))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current document before update (fields not mentioned are preserved): values merge by alias+culture+segment and variants by culture+segment, so naming one culture leaves the others untouched. --json replaces the payload wholesale instead")
	cmd.Flags().StringVar(&property, "property", "", "Update a single property alias without constructing the full payload")
	cmd.Flags().StringVar(&value, "value", "", "String value used with --property, sent as given; --value \"\" clears the property")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "JSON value used with --property")
	cmd.Flags().BoolVar(&saveAndPublish, "save-and-publish", false, "Publish the document after a successful update")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --save-and-publish")
	cmdkit.AddBackupFlag(cmd, &backup)
	addDocumentBatchFlags(cmd, &idsCSV, &fromFile, &force, "update")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentUpdateProperties(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var backup string
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
			if err := cmdkit.RequireValue("--json", jsonPayload); err != nil {
				return err
			}
			patch, err := buildUpdatePropertiesPatch(jsonPayload)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			path := api.JoinPath("/document/%s", args[0])
			current, err := cmdkit.FetchObject(ctx, deps.Client, path, api.RequestOptions{})
			if err != nil {
				return err
			}
			merged := cmdkit.MergeAliasPayload(current, patch)
			var backupFile string
			if cmd.Flags().Changed("backup") && !dryRun {
				backupFile, err = writeEntityBackup(cmd, backup, "document", args[0], path, current)
				if err != nil {
					return err
				}
			}
			result, err := deps.Client.Put(ctx, path, merged, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if backupFile != "" {
				return cmdkit.PrintResult(cmd, deps, withBackupPath(result, backupFile, "updated"))
			}
			return cmdkit.PrintMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Properties payload as JSON; accepts object {alias: value}, array [{alias, value, culture?, segment?}], or envelope {\"values\":[...]}")
	cmdkit.AddBackupFlag(cmd, &backup)
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

var invariantRaceBackoffs = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second}

func documentCopy(deps cmdkit.Dependencies) *cobra.Command {
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
			body, err := cmdkit.TargetActionBody(jsonPayload, to)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(ctx, api.JoinPath("/document/%s/copy", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if !publish {
				return cmdkit.PrintMutationResult(cmd, deps, "copied", result, dryRun)
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
			return cmdkit.PrintResult(cmd, deps, map[string]any{
				"copied":    result,
				"published": coalescePutResult(publishResult, dryRun),
			})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Copy payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID shortcut")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish the copied document after a successful copy")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture shortcut for --publish")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
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

func documentMove(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.TargetActionCommand(deps, cmdkit.TargetActionSpec{
		Use:   "move <id>",
		Short: "Move a document",
		Candidates: func(args []string) []cmdkit.MutationCandidate {
			path := api.JoinPath("/document/%s/move", args[0])
			return []cmdkit.MutationCandidate{{Method: "PUT", Path: path}, {Method: "POST", Path: path}}
		},
		Verb: "moved",
	})
}

func documentDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a document (use 'trash' for the recycle bin)",
		Path: func(args []string) string {
			return api.JoinPath("/document/%s", args[0])
		},
	})
}

func documentTrash(deps cmdkit.Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "trash <id>",
		Short: "Move a document to recycle bin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := api.JoinPath("/document/%s/move-to-recycle-bin", args[0])
			result, err := cmdkit.MutateWithFallback(cmd.Context(), deps.Client, map[string]any{}, api.RequestOptions{DryRun: dryRun},
				cmdkit.MutationCandidate{Method: "PUT", Path: path},
				cmdkit.MutationCandidate{Method: "POST", Path: path},
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "trashed", result, dryRun)
		},
	}
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentRestore(deps cmdkit.Dependencies) *cobra.Command {
	return restoreFromBinCommand(deps, "document", "content root")
}

func documentReferences(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.ReferencesCommand(deps, cmdkit.ReferencesSpec{
		Use:   "references <id>",
		Short: "List items that reference this document (paginated; --skip/--take/--all)",
		Long:  "Wraps GET /document/{id}/referenced-by. Used to answer 'what uses this node' for orphan checks, safe-delete verification, and taxonomy usage audits.",
		Path:  func(args []string) string { return api.JoinPath("/document/%s/referenced-by", args[0]) },
	})
}

func documentReferencedDescendants(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.ReferencesCommand(deps, cmdkit.ReferencesSpec{
		Use:   "referenced-descendants <id>",
		Short: "List items that reference this document or any of its descendants",
		Path:  func(args []string) string { return api.JoinPath("/document/%s/referenced-descendants", args[0]) },
	})
}

func documentAreReferenced(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.AreReferencedCommand(deps, "document")
}
