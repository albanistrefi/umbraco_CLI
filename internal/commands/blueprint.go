package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// RegisterBlueprint wires the document blueprint family: reusable content
// presets an editor picks when creating a document. A blueprint carries a
// document type plus a full set of property values, lives in its own
// folder tree, and is consumed through its scaffold — the document payload
// skeleton behind 'document create --from-blueprint'.
func RegisterBlueprint(root *cobra.Command, deps Dependencies) {
	blueprint := &cobra.Command{
		Use:     "blueprint",
		Aliases: []string{"document-blueprint"},
		Short:   "Document blueprints: reusable content presets for new documents",
		Long: `Document blueprints: reusable content presets for new documents.

A blueprint is a saved set of property values for one document type. The
backoffice offers it when an editor creates a document; from the CLI the
same thing is 'document create --from-blueprint <id>'.

Task → command:
  Browse the blueprint tree                            blueprint list; blueprint children <folder-id>
  Locate one in the tree                               blueprint ancestors <id>; blueprint siblings <id>
  Read one blueprint and its values                    blueprint get <id>
  Resolve several GUIDs to names in one call           blueprint items --ids <a,b>
  See who changed one, and when                        blueprint audit-log <id>
  Turn an existing document into a blueprint           blueprint create-from-document <document-id> --name <n> [--parent <folder-id>]
  Build a blueprint from scratch                       blueprint create --print-template, then blueprint create --json '{...}'
  Change the stored values                             blueprint update <id> --merge-json '{...}' --backup
  Organize the tree                                    blueprint create-folder --name <n> [--parent <id>]; blueprint move <id> --to <folder-id>
  Inspect the document payload it produces             blueprint scaffold <id>
  Create a document from it                            document create --from-blueprint <id> --parent <doc-id>
  Remove one                                           blueprint delete <id> --force; blueprint delete-folder <id> --force`,
	}
	blueprint.AddCommand(blueprintList(deps))
	blueprint.AddCommand(blueprintChildren(deps))
	blueprint.AddCommand(blueprintAncestors(deps))
	blueprint.AddCommand(blueprintSiblings(deps))
	blueprint.AddCommand(blueprintGet(deps))
	blueprint.AddCommand(blueprintItems(deps))
	blueprint.AddCommand(blueprintAuditLog(deps))
	blueprint.AddCommand(blueprintScaffold(deps))
	blueprint.AddCommand(blueprintCreate(deps))
	blueprint.AddCommand(blueprintCreateFromDocument(deps))
	blueprint.AddCommand(blueprintUpdate(deps))
	blueprint.AddCommand(blueprintMove(deps))
	blueprint.AddCommand(blueprintDelete(deps))
	blueprint.AddCommand(schemaTypeCreateFolder(deps, "blueprint", "document-blueprint", "document blueprint", true))
	blueprint.AddCommand(schemaTypeDeleteFolder(deps, "blueprint", "document-blueprint", "document blueprint"))
	root.AddCommand(blueprint)
}

func blueprintList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List blueprints and folders at the tree root (paginated; --skip/--take/--all)",
		Long:  "GET /tree/document-blueprint/root. Folders come back with isFolder true; use 'blueprint children <id>' to descend into one.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document-blueprint/root", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func blueprintChildren(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "children <parent-id>",
		Short: "List blueprints inside a folder (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/document-blueprint/children", opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
			}
		},
	})
}

// blueprintAncestors is a plain read rather than a collectionCommand: the
// ancestors route answers with a bare array, not the {items, total}
// envelope pagination and triage are built on.
func blueprintAncestors(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "ancestors <id>",
		Short: "List the folders a blueprint sits under, root first",
		Long:  "GET /tree/document-blueprint/ancestors?descendantId={id}. Answers with the folder chain above the blueprint (or folder), root first and ending with the entry itself, so an id seen in a payload can be placed in the tree without walking it from the root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/tree/document-blueprint/ancestors", api.RequestOptions{Params: map[string]any{"descendantId": args[0]}, Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

// blueprintSiblings is a plain read rather than a collectionCommand: the
// route is windowed with --before/--after around the target instead of
// skip/take, and answers with {totalBefore, totalAfter, items}.
func blueprintSiblings(deps Dependencies) *cobra.Command {
	var before int
	var after int
	var foldersOnly bool
	var fields string
	cmd := &cobra.Command{
		Use:   "siblings <id>",
		Short: "List the tree entries around a blueprint or folder",
		Long:  "GET /tree/document-blueprint/siblings?target={id}. The window is counted from the target: --before entries above it and --after below it. The response carries totalBefore/totalAfter alongside items, so a wider window is only worth asking for when those are non-zero.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{"target": args[0], "before": before, "after": after}
			if foldersOnly {
				params["foldersOnly"] = true
			}
			result, err := deps.Client.Get(cmd.Context(), "/tree/document-blueprint/siblings", api.RequestOptions{Params: params, Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().IntVar(&before, "before", 10, "How many siblings above the target to return")
	cmd.Flags().IntVar(&after, "after", 10, "How many siblings below the target to return")
	cmd.Flags().BoolVar(&foldersOnly, "folders-only", false, "Return only folders, skipping the blueprints themselves")
	addFieldsFlag(cmd, &fields)
	return cmd
}

func blueprintItems(deps Dependencies) *cobra.Command {
	var idsCSV string
	var fields string
	cmd := &cobra.Command{
		Use:   "items",
		Short: "Resolve blueprint GUIDs to names in one call",
		Long:  "GET /item/document-blueprint?id=…. The item read for blueprints: pass the GUIDs seen in other payloads and get their names and document types back without one request per ID.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("blueprint items requires --ids <comma-separated guids>")
			}
			result, err := deps.Client.Get(cmd.Context(), "/item/document-blueprint", api.RequestOptions{Params: map[string]any{"id": stringsToAny(ids)}, Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated blueprint GUIDs (required)")
	addFieldsFlag(cmd, &fields)
	return cmd
}

func blueprintAuditLog(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "audit-log <id>",
		Short: "List the audit trail for a blueprint (who did what, when)",
		Long:  "GET /document-blueprint/{id}/audit-log. Pass --params for orderDirection or sinceDate filters, e.g. --params '{\"sinceDate\":\"2026-01-01T00:00:00Z\"}'.",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: api.JoinPath("/document-blueprint/%s/audit-log", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func blueprintGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get a blueprint by ID",
		Long:  "GET /document-blueprint/{id}. The response carries documentType, values and variants; the blueprint's name lives on variants[].name, not at the top level.",
		Path:  func(args []string) string { return api.JoinPath("/document-blueprint/%s", args[0]) },
	})
}

func blueprintScaffold(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "scaffold <id>",
		Short: "Print the document skeleton a blueprint produces",
		Long:  "GET /document-blueprint/{id}/scaffold. Returns the blueprint's values and variants as the server would seed a new document with them. 'document create --from-blueprint <id>' consumes this directly.",
		Path:  func(args []string) string { return api.JoinPath("/document-blueprint/%s/scaffold", args[0]) },
	})
}

func blueprintCreate(deps Dependencies) *cobra.Command {
	return createCommand(deps, createSpec{
		Use:          "create",
		Short:        "Create a blueprint from a full JSON payload",
		Long:         "POST /document-blueprint. Required payload fields: documentType ({\"id\":…}), values, variants (variants[].name is the blueprint name); parent ({\"id\":…} of a blueprint folder) is optional. To capture an existing document instead, use 'blueprint create-from-document'.",
		Path:         "/document-blueprint",
		TemplateKey:  "blueprint.create",
		PayloadUsage: "Full JSON payload",
	})
}

func blueprintCreateFromDocument(deps Dependencies) *cobra.Command {
	var name string
	var parent string
	var id string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create-from-document <document-id>",
		Short: "Capture an existing document as a blueprint",
		Long:  "POST /document-blueprint/from-document. Copies the document's current property values into a new blueprint; --name is the blueprint's name (it does not have to match the document) and --parent nests it in a blueprint folder.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--name", name); err != nil {
				return err
			}
			body := map[string]any{
				"document": map[string]any{"id": strings.TrimSpace(args[0])},
				"name":     strings.TrimSpace(name),
			}
			if trimmed := strings.TrimSpace(parent); trimmed != "" {
				body["parent"] = map[string]any{"id": trimmed}
			}
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				body["id"] = trimmed
			}
			if _, err := ensurePayloadID(body); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/document-blueprint/from-document", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Blueprint name (required)")
	cmd.Flags().StringVar(&parent, "parent", "", "Blueprint folder GUID; omit for the tree root")
	cmd.Flags().StringVar(&id, "id", "", "Blueprint GUID to use (generated when omitted)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func blueprintUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update a blueprint's stored values",
		Long:  "PUT /document-blueprint/{id}. The update model takes values and variants; --merge-json keeps the rest of the blueprint as it is, --json replaces it wholesale. Rename a blueprint by patching variants[].name.",
		Path:  func(args []string) string { return api.JoinPath("/document-blueprint/%s", args[0]) },
	})
}

func blueprintMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move a blueprint into another folder",
		Long:  "PUT /document-blueprint/{id}/move. --to takes the destination folder GUID; --json '{\"target\":null}' moves the blueprint back to the tree root.",
		Candidates: func(args []string) []mutationCandidate {
			return []mutationCandidate{{method: "PUT", path: api.JoinPath("/document-blueprint/%s/move", args[0])}}
		},
		Verb: "moved",
	})
}

func blueprintDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a blueprint",
		Path: func(args []string) string {
			return api.JoinPath("/document-blueprint/%s", args[0])
		},
	})
}

// blueprintDocumentPayload turns a blueprint scaffold into a document
// create payload. The scaffold answers with the blueprint response shape
// (GET /document-blueprint/{id}/scaffold returns
// DocumentBlueprintResponseModel), so the response-only decoration —
// flags, the scaffold's own id, per-value editorAlias, and the variant
// state/date fields — is dropped, documentType is reduced to the
// {id} reference the create model takes, and the required template key is
// seeded as null. The caller's own JSON is merged on top afterwards.
func blueprintDocumentPayload(ctx context.Context, client *api.Client, blueprintID string) (map[string]any, error) {
	path := api.JoinPath("/document-blueprint/%s/scaffold", blueprintID)
	scaffold, err := fetchObject(ctx, client, path, api.RequestOptions{})
	if err != nil {
		return nil, fmt.Errorf("--from-blueprint could not read the blueprint scaffold: %w", err)
	}

	body := map[string]any{"template": nil}
	if documentType, ok := scaffold["documentType"].(map[string]any); ok {
		if id, ok := documentType["id"].(string); ok && strings.TrimSpace(id) != "" {
			body["documentType"] = map[string]any{"id": id}
		}
	}
	if _, ok := body["documentType"]; !ok {
		return nil, fmt.Errorf("blueprint %s scaffold carries no documentType id", blueprintID)
	}
	body["values"] = projectObjects(scaffold["values"], "alias", "culture", "segment", "value")
	body["variants"] = projectObjects(scaffold["variants"], "name", "culture", "segment")
	return body, nil
}

// projectObjects keeps only the named keys of every object in an array,
// dropping entries that are not objects. Absent keys stay absent so a
// merged patch can still add them.
func projectObjects(value any, keys ...string) []any {
	items, ok := value.([]any)
	if !ok {
		return []any{}
	}
	projected := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kept := make(map[string]any, len(keys))
		for _, key := range keys {
			if entry, ok := object[key]; ok {
				kept[key] = entry
			}
		}
		projected = append(projected, kept)
	}
	return projected
}

// blueprintVariantNames lists the variant names present in a document
// payload, so a create can refuse a nameless document before the server
// answers with a validation problem.
func blueprintVariantNames(body map[string]any) []string {
	variants, ok := body["variants"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(variants))
	for _, variant := range variants {
		object, ok := variant.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := object["name"].(string); ok && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return names
}
