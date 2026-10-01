package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// RegisterElement wires the element content family introduced by Umbraco
// 18.1: reusable content items living in a folder library instead of the
// page tree, with their own publish lifecycle, version history, and recycle
// bin. The commands mirror the document family and reuse its builders; the
// whole group requires an 18.1+ server (older versions 404 with the usual
// version hint).
func RegisterElement(root *cobra.Command, deps Dependencies) {
	element := &cobra.Command{
		Use:   "element",
		Short: "Element library content (Umbraco 18.1+): reusable content items with publish lifecycle",
		Long:  "Manages elements — reusable content items introduced in Umbraco 18.1 that live in a folder library rather than the page tree. Element types are document types with allowedInLibrary set ('doctype allowed-in-library' lists them). Requires an 18.1+ server.",
	}
	element.AddCommand(elementRoot(deps))
	element.AddCommand(elementChildren(deps))
	element.AddCommand(elementAncestors(deps))
	element.AddCommand(elementSearch(deps))
	element.AddCommand(elementGet(deps))
	element.AddCommand(elementPublished(deps))
	element.AddCommand(elementCreate(deps))
	element.AddCommand(elementUpdate(deps))
	element.AddCommand(elementPublish(deps))
	element.AddCommand(elementUnpublish(deps))
	element.AddCommand(elementCopy(deps))
	element.AddCommand(elementMove(deps))
	element.AddCommand(elementTrash(deps))
	element.AddCommand(elementDelete(deps))
	element.AddCommand(elementAuditLog(deps))
	element.AddCommand(elementReferences(deps))
	element.AddCommand(elementReferencedDescendants(deps))
	element.AddCommand(areReferencedCommand(deps, "element"))
	element.AddCommand(restoreFromBinCommand(deps, "element", "library root"))
	element.AddCommand(recycleBinCommand(deps, "element"))
	element.AddCommand(elementVersion(deps))
	root.AddCommand(element)
}

func elementRoot(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List elements and folders at the library root (paginated; --skip/--take/--all)",
		Long:  "GET /tree/element/root. Use 'element children <id>' to descend into folders.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: "/tree/element/root", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func elementChildren(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "children <parent-id>",
		Short: "List children of a library folder (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: "/tree/element/children", Opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
			}
		},
	})
}

func elementAncestors(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "ancestors <id>",
		Short: "Get ancestor folders of an element",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/tree/element/ancestors", api.RequestOptions{Params: map[string]any{"descendantId": args[0]}})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func elementSearch(deps Dependencies) *cobra.Command {
	return searchCommand(deps, searchSpec{
		Use:   "search",
		Short: "Search elements",
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: "/item/element/search", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func elementGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get an element by ID",
		Path:  func(args []string) string { return api.JoinPath("/element/%s", args[0]) },
	})
}

func elementPublished(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "published <id>",
		Short: "Get the published snapshot of an element",
		Path:  func(args []string) string { return api.JoinPath("/element/%s/published", args[0]) },
	})
}

func elementCreate(deps Dependencies) *cobra.Command {
	var publish bool
	var cultures string
	return createCommand(deps, createSpec{
		Use:          "create",
		Short:        "Create an element",
		Long:         "POST /element, or POST /element/create-and-publish with --publish: created and published in one atomic server-side operation. Required payload fields: documentType ({\"id\":...} of a type with allowedInLibrary), values, variants; parent ({\"id\":...} of a library folder) is optional.",
		Path:         "/element",
		PayloadUsage: "Full JSON payload",
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			cmd.Flags().BoolVar(&publish, "publish", false, "Create and publish atomically via POST /element/create-and-publish")
			cmd.Flags().StringVar(&cultures, "culture", "", "Comma-separated cultures to publish with --publish; omit for invariant content")
			return func(body map[string]any) error {
				if !publish {
					if strings.TrimSpace(cultures) != "" {
						return fmt.Errorf("--culture requires --publish")
					}
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
				return "/element/create-and-publish"
			}
			return ""
		},
	})
}

func elementUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var mergeJSON string
	var saveAndPublish bool
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an element",
		Long:  "PUT /element/{id}, or PUT /element/{id}/update-and-publish with --save-and-publish (one atomic operation; --culture names the cultures to publish, omit for invariant content).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			path := api.JoinPath("/element/%s", args[0])
			if !saveAndPublish && strings.TrimSpace(culture) != "" {
				return fmt.Errorf("--culture requires --save-and-publish")
			}
			body, err := resolveUpdateBody(ctx, deps.Client, path, "", jsonPayload, mergeJSON, nil, nil)
			if err != nil {
				return err
			}
			if !saveAndPublish {
				result, err := deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
				if err != nil {
					return err
				}
				return printMutationResult(cmd, deps, "updated", result, dryRun)
			}
			if _, ok := body["culturesToPublish"]; !ok {
				body["culturesToPublish"] = culturesToPublishList(culture)
			}
			result, err := deps.Client.Put(ctx, api.JoinPath("/element/%s/update-and-publish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, map[string]any{
				"saveAndPublish": true,
				"updated":        coalescePutResult(result, dryRun),
				"published":      coalescePutResult(result, dryRun),
			})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current element before update (fields not mentioned are preserved)")
	cmd.Flags().BoolVar(&saveAndPublish, "save-and-publish", false, "Publish atomically with the update via PUT /element/{id}/update-and-publish")
	cmd.Flags().StringVar(&culture, "culture", "", "Comma-separated cultures to publish with --save-and-publish; omit for invariant content")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func elementPublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish an element",
		Long:  "PUT /element/{id}/publish. Defaults to the invariant publish schedule; pass --culture for one culture or --json for a full publishSchedules payload.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := documentPublishBody(jsonPayload, culture)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/element/%s/publish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "published", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Publish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture to publish on variant content")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func elementUnpublish(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "unpublish <id>",
		Short: "Unpublish an element",
		Long:  "PUT /element/{id}/unpublish. Unpublishes all cultures by default; pass --culture for one culture or --json for a full cultures payload.",
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
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/element/%s/unpublish", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "unpublished", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Unpublish payload as JSON")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture to unpublish on variant content")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func elementCopy(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "copy <id>",
		Short: "Copy an element",
		Candidates: func(args []string) []mutationCandidate {
			return []mutationCandidate{{Method: "POST", Path: api.JoinPath("/element/%s/copy", args[0])}}
		},
		Verb: "copied",
	})
}

func elementMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move an element",
		Candidates: func(args []string) []mutationCandidate {
			return []mutationCandidate{{Method: "PUT", Path: api.JoinPath("/element/%s/move", args[0])}}
		},
		Verb: "moved",
	})
}

func elementTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "trash <id>",
		Short: "Move an element to the recycle bin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/element/%s/move-to-recycle-bin", args[0]), map[string]any{}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "trashed", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func elementDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete an element (use 'trash' for the recycle bin)",
		Path: func(args []string) string {
			return api.JoinPath("/element/%s", args[0])
		},
	})
}

func elementAuditLog(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "audit-log <id>",
		Short: "List the audit trail for an element (who did what, when)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{Path: api.JoinPath("/element/%s/audit-log", args[0]), Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func elementReferences(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "references <id>",
		Short: "List items that reference this element (paginated; --skip/--take/--all)",
		Long:  "Wraps GET /element/{id}/referenced-by. Answers 'what uses this element' before unpublishing or deleting it.",
		Path:  func(args []string) string { return api.JoinPath("/element/%s/referenced-by", args[0]) },
	})
}

func elementReferencedDescendants(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "referenced-descendants <id>",
		Short: "List items that reference this element folder or anything inside it",
		Path:  func(args []string) string { return api.JoinPath("/element/folder/%s/referenced-descendants", args[0]) },
	})
}

// elementVersion groups version-history commands under 'element version',
// mirroring 'document version': list, inspect, roll back, pin.
func elementVersion(deps Dependencies) *cobra.Command {
	version := &cobra.Command{
		Use:   "version",
		Short: "Element version history: list, inspect, roll back",
	}
	version.AddCommand(elementVersionList(deps))
	version.AddCommand(elementVersionGet(deps))
	version.AddCommand(elementVersionRollback(deps))
	version.AddCommand(elementVersionPreventCleanup(deps))
	return version
}

func elementVersionList(deps Dependencies) *cobra.Command {
	var culture string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list <element-id>",
		Short: "List stored versions of an element (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			versionParams := withParam(params, "elementId", args[0])
			if culture != "" {
				versionParams["culture"] = culture
			}
			return []getRequestCandidate{
				{Path: "/element-version", Opts: api.RequestOptions{Params: versionParams}},
			}
		},
	})
	cmd.Flags().StringVar(&culture, "culture", "", "Limit versions to one culture on variant content")
	return cmd
}

func elementVersionGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <version-id>",
		Short: "Get a stored element version (the full payload as it was)",
		Path:  func(args []string) string { return api.JoinPath("/element-version/%s", args[0]) },
	})
}

func elementVersionRollback(deps Dependencies) *cobra.Command {
	var culture string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rollback <version-id>",
		Short: "Roll the element back to this version",
		Long:  "POST /element-version/{id}/rollback. Version IDs come from 'element version list'. On variant content pass --culture to roll back a single culture; omitting it rolls back the invariant data.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if culture != "" {
				params["culture"] = culture
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/element-version/%s/rollback", args[0]), nil, api.RequestOptions{DryRun: dryRun, Params: params})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "rolledBack", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&culture, "culture", "", "Culture to roll back on variant content")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func elementVersionPreventCleanup(deps Dependencies) *cobra.Command {
	var disable bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prevent-cleanup <version-id>",
		Short: "Pin a version so scheduled history cleanup never deletes it",
		Long:  "PUT /element-version/{id}/prevent-cleanup. Pins the version by default; pass --disable to unpin it again.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{"preventCleanup": !disable}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/element-version/%s/prevent-cleanup", args[0]), nil, api.RequestOptions{DryRun: dryRun, Params: params})
			if err != nil {
				return err
			}
			verb := "pinned"
			if disable {
				verb = "unpinned"
			}
			return printMutationResult(cmd, deps, verb, result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&disable, "disable", false, "Allow cleanup to delete this version again")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
