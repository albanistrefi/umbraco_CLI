package commands

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// guidPattern matches the standard 8-4-4-4-12 lowercase/uppercase hex form
// the Umbraco Management API uses. Used to pre-validate block GUID flags
// so a typo on --content-element-type / --settings-element-type errors with
// "must be a GUID" instead of falling through to "block not found" (which
// would be misleading) or, worse, persisting garbage on the server.
var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validateBlockGUID rejects flag values that don't look like a GUID. flagName
// is included verbatim in the error so the user knows which flag was wrong.
// Caller decides whether empty is valid (it usually means "clear this
// optional field" — see datatypeBlockUpdate's --settings-element-type
// handling).
func validateBlockGUID(flagName string, value string) error {
	if !guidPattern.MatchString(value) {
		return fmt.Errorf("%s must be a GUID (8-4-4-4-12 hex), got %q", flagName, value)
	}
	return nil
}

// datatypeBlockEditorAliases is the set of editorAlias values whose
// configuration includes a 'blocks' value entry shaped as an array of
// block-definition objects. Other editors (e.g. Umbraco.MultipleTextstring)
// may have a 'values' array but its entries are strings, not block defs —
// rejecting up front prevents corrupting unrelated configurations.
var datatypeBlockEditorAliases = map[string]bool{
	"Umbraco.BlockList": true,
	"Umbraco.BlockGrid": true,
}

var datatypeBlockValidEditorSizes = map[string]bool{
	"small":  true,
	"medium": true,
	"large":  true,
}

type datatypeBlockMutationSummary struct {
	Action                string         `json:"action"`
	DatatypeID            string         `json:"datatypeId"`
	EditorAlias           string         `json:"editorAlias"`
	ContentElementTypeKey string         `json:"contentElementTypeKey"`
	Changed               bool           `json:"changed"`
	Message               string         `json:"message,omitempty"`
	Block                 map[string]any `json:"block,omitempty"`
}

func datatypeBlock(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block",
		Short: "Manage allowed blocks on a Block List / Block Grid datatype",
		Long:  "Read-modify-write helpers that mutate the 'blocks' value entry on Umbraco.BlockList and Umbraco.BlockGrid datatypes without clobbering the rest of the configuration. Idempotent: 'add' is a no-op if the element type is already an allowed block; 'remove' is a no-op if it isn't; 'update' is a no-op if the resulting block is byte-identical to the current one.",
	}
	cmd.AddCommand(datatypeBlockList(deps))
	cmd.AddCommand(datatypeBlockAdd(deps))
	cmd.AddCommand(datatypeBlockUpdate(deps))
	cmd.AddCommand(datatypeBlockRemove(deps))
	return cmd
}

func datatypeBlockList(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <datatypeId>",
		Short: "List allowed blocks on a Block List / Block Grid datatype",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := fetchDatatypeObject(context.Background(), deps.Client, args[0])
			if err != nil {
				return err
			}
			if _, err := requireDatatypeBlockEditor(payload, args[0]); err != nil {
				return err
			}
			blocks := loadDatatypeBlocks(payload)
			out := make([]any, 0, len(blocks))
			for _, b := range blocks {
				out = append(out, b)
			}
			return printResult(cmd, deps, out)
		},
	}
	return cmd
}

func datatypeBlockAdd(deps Dependencies) *cobra.Command {
	var contentElementType string
	var settingsElementType string
	var label string
	var editorSize string
	var thumbnail string
	var forceHideContentEditor bool
	var allowAtRoot bool
	var allowInAreas bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "add <datatypeId>",
		Short: "Register an element type as an allowed block",
		Long:  "Appends a block to the datatype's blocks array. Idempotent: if a block with the same --content-element-type is already present, no PUT is sent.\n\nBlockGrid: --allow-at-root and --allow-in-areas default to true so the block is actually placeable after registration (server-side both default to false when omitted, which would register a block that's invisible to editors). Pass --allow-at-root=false or --allow-in-areas=false to override. --group support over BlockGrid's blockGroups array is a deferred follow-up.\n\nBlockList: --allow-at-root and --allow-in-areas are ignored (those flags only apply to Block Grid).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--content-element-type", contentElementType); err != nil {
				return err
			}
			if err := validateBlockGUID("--content-element-type", contentElementType); err != nil {
				return err
			}
			if settingsElementType != "" {
				if err := validateBlockGUID("--settings-element-type", settingsElementType); err != nil {
					return err
				}
			}
			if editorSize != "" && !datatypeBlockValidEditorSizes[strings.ToLower(editorSize)] {
				return fmt.Errorf("--editor-size must be one of small, medium, large (got %q)", editorSize)
			}

			ctx := context.Background()
			payload, err := fetchDatatypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}
			editor, err := requireDatatypeBlockEditor(payload, args[0])
			if err != nil {
				return err
			}

			blocks := loadDatatypeBlocks(payload)
			if findBlockIndex(blocks, contentElementType) >= 0 {
				return printResult(cmd, deps, datatypeBlockMutationSummary{
					Action:                "add",
					DatatypeID:            args[0],
					EditorAlias:           editor,
					ContentElementTypeKey: contentElementType,
					Changed:               false,
					Message:               "element type is already an allowed block",
				})
			}

			block := map[string]any{
				"contentElementTypeKey":           contentElementType,
				"forceHideContentEditorInOverlay": forceHideContentEditor,
			}
			if settingsElementType != "" {
				block["settingsElementTypeKey"] = settingsElementType
			}
			if label != "" {
				block["label"] = label
			}
			if editorSize != "" {
				block["editorSize"] = strings.ToLower(editorSize)
			}
			if thumbnail != "" {
				block["thumbnail"] = thumbnail
			}
			// BlockGrid placement flags. Server defaults both to false when
			// omitted, which produces a block that's registered but invisible
			// in the editor — so we default to true here and let users
			// override with --allow-at-root=false / --allow-in-areas=false.
			// BlockList ignores these fields, so omit them entirely there.
			if editor == "Umbraco.BlockGrid" {
				block["allowAtRoot"] = allowAtRoot
				block["allowInAreas"] = allowInAreas
			}

			next := append([]map[string]any{}, blocks...)
			next = append(next, block)
			nextPayload := writeDatatypeBlocks(payload, next)

			result, err := deps.Client.Put(
				ctx,
				fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun, SkipValidation: true},
			)
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			return printResult(cmd, deps, datatypeBlockMutationSummary{
				Action:                "add",
				DatatypeID:            args[0],
				EditorAlias:           editor,
				ContentElementTypeKey: contentElementType,
				Changed:               true,
				Block:                 block,
			})
		},
	}

	cmd.Flags().StringVar(&contentElementType, "content-element-type", "", "GUID of the element type to register as a block (required)")
	cmd.Flags().StringVar(&settingsElementType, "settings-element-type", "", "GUID of the element type to use for the block's settings overlay (optional)")
	cmd.Flags().StringVar(&label, "label", "", "Optional label shown in the block picker; defaults to the element type's name")
	cmd.Flags().StringVar(&editorSize, "editor-size", "", "Overlay size: small | medium | large")
	cmd.Flags().StringVar(&thumbnail, "thumbnail", "", "Optional path/URL to a thumbnail image")
	cmd.Flags().BoolVar(&forceHideContentEditor, "force-hide-content-editor", false, "Hide the content editor in the overlay (settings-only blocks)")
	cmd.Flags().BoolVar(&allowAtRoot, "allow-at-root", true, "BlockGrid only: allow placing the block at the grid's root level (default true). Ignored for BlockList.")
	cmd.Flags().BoolVar(&allowInAreas, "allow-in-areas", true, "BlockGrid only: allow placing the block inside areas of other blocks (default true). Ignored for BlockList.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

func datatypeBlockUpdate(deps Dependencies) *cobra.Command {
	var contentElementType string
	var settingsElementType string
	var label string
	var editorSize string
	var thumbnail string
	var forceHideContentEditor bool
	var allowAtRoot bool
	var allowInAreas bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "update <datatypeId>",
		Short: "Update an existing block's properties (partial; flags only mutate what you pass)",
		Long: `Mutates a single existing block on a Block List / Block Grid datatype. The deliberate difference from 'block add': if no block with --content-element-type is present, this errors instead of creating one.

Partial-update semantics: only flags you pass on the command line are applied. Unpassed flags leave that property untouched, so 'block update <dt> --content-element-type <guid> --editor-size large' will not wipe the label.

Clearing optional fields: pass an empty string. --thumbnail "" and --settings-element-type "" remove those fields entirely. --label "" is also accepted and removes the override label (the editor falls back to the element type's name).

Idempotent: if the resulting block is byte-identical to the current one, no PUT is sent.

BlockGrid: --allow-at-root and --allow-in-areas are honored when explicitly passed. Both are ignored for BlockList (mirror of 'block add').`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--content-element-type", contentElementType); err != nil {
				return err
			}
			if err := validateBlockGUID("--content-element-type", contentElementType); err != nil {
				return err
			}
			// Empty --settings-element-type is the "clear this field" signal;
			// only validate when the caller actually supplied a value.
			if cmd.Flags().Changed("settings-element-type") && settingsElementType != "" {
				if err := validateBlockGUID("--settings-element-type", settingsElementType); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("editor-size") && editorSize != "" && !datatypeBlockValidEditorSizes[strings.ToLower(editorSize)] {
				return fmt.Errorf("--editor-size must be one of small, medium, large (got %q)", editorSize)
			}

			ctx := context.Background()
			payload, err := fetchDatatypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}
			editor, err := requireDatatypeBlockEditor(payload, args[0])
			if err != nil {
				return err
			}

			blocks := loadDatatypeBlocks(payload)
			idx := findBlockIndex(blocks, contentElementType)
			if idx < 0 {
				return fmt.Errorf("block %s not found on datatype %s; use 'datatype block add' to create it", contentElementType, args[0])
			}

			// Deep-clone the target so unrelated keys on the original payload
			// pass through untouched and we have a clean before/after pair
			// for the idempotency check.
			updated := cloneObject(blocks[idx])
			if cmd.Flags().Changed("label") {
				if label == "" {
					delete(updated, "label")
				} else {
					updated["label"] = label
				}
			}
			if cmd.Flags().Changed("editor-size") {
				if editorSize == "" {
					delete(updated, "editorSize")
				} else {
					updated["editorSize"] = strings.ToLower(editorSize)
				}
			}
			if cmd.Flags().Changed("thumbnail") {
				if thumbnail == "" {
					delete(updated, "thumbnail")
				} else {
					updated["thumbnail"] = thumbnail
				}
			}
			if cmd.Flags().Changed("settings-element-type") {
				if settingsElementType == "" {
					delete(updated, "settingsElementTypeKey")
				} else {
					updated["settingsElementTypeKey"] = settingsElementType
				}
			}
			if cmd.Flags().Changed("force-hide-content-editor") {
				updated["forceHideContentEditorInOverlay"] = forceHideContentEditor
			}
			// BlockGrid placement flags are only meaningful on BlockGrid; on
			// BlockList we ignore them silently (matches 'block add').
			if editor == "Umbraco.BlockGrid" {
				if cmd.Flags().Changed("allow-at-root") {
					updated["allowAtRoot"] = allowAtRoot
				}
				if cmd.Flags().Changed("allow-in-areas") {
					updated["allowInAreas"] = allowInAreas
				}
			}

			if reflect.DeepEqual(blocks[idx], updated) {
				return printResult(cmd, deps, datatypeBlockMutationSummary{
					Action:                "update",
					DatatypeID:            args[0],
					EditorAlias:           editor,
					ContentElementTypeKey: contentElementType,
					Changed:               false,
					Message:               "no changes (resulting block is byte-identical to current)",
					Block:                 updated,
				})
			}

			next := make([]map[string]any, len(blocks))
			copy(next, blocks)
			next[idx] = updated
			nextPayload := writeDatatypeBlocks(payload, next)

			result, err := deps.Client.Put(
				ctx,
				fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun, SkipValidation: true},
			)
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			return printResult(cmd, deps, datatypeBlockMutationSummary{
				Action:                "update",
				DatatypeID:            args[0],
				EditorAlias:           editor,
				ContentElementTypeKey: contentElementType,
				Changed:               true,
				Block:                 updated,
			})
		},
	}

	cmd.Flags().StringVar(&contentElementType, "content-element-type", "", "GUID of the block to update (required; identity key — same as 'block add' / 'block remove')")
	cmd.Flags().StringVar(&settingsElementType, "settings-element-type", "", "Set the settings overlay element type. Pass empty string to clear.")
	cmd.Flags().StringVar(&label, "label", "", "New block label. Pass empty string to clear (editor falls back to element type name).")
	cmd.Flags().StringVar(&editorSize, "editor-size", "", "Overlay size: small | medium | large. Pass empty string to clear.")
	cmd.Flags().StringVar(&thumbnail, "thumbnail", "", "Path/URL to a thumbnail image. Pass empty string to clear.")
	cmd.Flags().BoolVar(&forceHideContentEditor, "force-hide-content-editor", false, "Hide the content editor in the overlay (settings-only blocks)")
	cmd.Flags().BoolVar(&allowAtRoot, "allow-at-root", true, "BlockGrid only: allow placing the block at the grid's root level. Ignored for BlockList.")
	cmd.Flags().BoolVar(&allowInAreas, "allow-in-areas", true, "BlockGrid only: allow placing the block inside areas of other blocks. Ignored for BlockList.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

func datatypeBlockRemove(deps Dependencies) *cobra.Command {
	var contentElementType string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "remove <datatypeId>",
		Short: "Unregister an element type from a Block List / Block Grid",
		Long:  "Idempotent: if no block with --content-element-type is registered, no PUT is sent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--content-element-type", contentElementType); err != nil {
				return err
			}
			if err := validateBlockGUID("--content-element-type", contentElementType); err != nil {
				return err
			}

			ctx := context.Background()
			payload, err := fetchDatatypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}
			editor, err := requireDatatypeBlockEditor(payload, args[0])
			if err != nil {
				return err
			}

			blocks := loadDatatypeBlocks(payload)
			idx := findBlockIndex(blocks, contentElementType)
			if idx < 0 {
				return printResult(cmd, deps, datatypeBlockMutationSummary{
					Action:                "remove",
					DatatypeID:            args[0],
					EditorAlias:           editor,
					ContentElementTypeKey: contentElementType,
					Changed:               false,
					Message:               "element type is not currently an allowed block",
				})
			}

			next := make([]map[string]any, 0, len(blocks)-1)
			next = append(next, blocks[:idx]...)
			next = append(next, blocks[idx+1:]...)
			nextPayload := writeDatatypeBlocks(payload, next)

			result, err := deps.Client.Put(
				ctx,
				fmt.Sprintf("%s/%s", dataTypeLegacyCollectionPath, args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun, SkipValidation: true},
			)
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			return printResult(cmd, deps, datatypeBlockMutationSummary{
				Action:                "remove",
				DatatypeID:            args[0],
				EditorAlias:           editor,
				ContentElementTypeKey: contentElementType,
				Changed:               true,
			})
		},
	}

	cmd.Flags().StringVar(&contentElementType, "content-element-type", "", "GUID of the element type to unregister (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

// loadDatatypeBlocks extracts the blocks array from a Block List/Grid
// payload. Returns an empty slice when no blocks value entry is present
// (so 'add' on a brand-new datatype works without special casing).
func loadDatatypeBlocks(payload map[string]any) []map[string]any {
	values, _ := payload["values"].([]any)
	for _, item := range values {
		entry, _ := item.(map[string]any)
		if entry == nil || entry["alias"] != "blocks" {
			continue
		}
		raw, _ := entry["value"].([]any)
		out := make([]map[string]any, 0, len(raw))
		for _, b := range raw {
			if asMap, ok := b.(map[string]any); ok {
				out = append(out, asMap)
			}
		}
		return out
	}
	return nil
}

// writeDatatypeBlocks returns a deep-cloned payload with the blocks value
// entry replaced by next. Preserves every other field on the datatype
// (label, sortOrder, other values entries) so unrelated settings survive
// the round-trip.
func writeDatatypeBlocks(payload map[string]any, next []map[string]any) map[string]any {
	cloned := cloneObject(payload)
	encoded := make([]any, 0, len(next))
	for _, block := range next {
		encoded = append(encoded, cloneObject(block))
	}

	values, ok := cloned["values"].([]any)
	if !ok {
		cloned["values"] = []any{map[string]any{"alias": "blocks", "value": encoded}}
		return cloned
	}
	for i, item := range values {
		entry, entryOk := item.(map[string]any)
		if !entryOk {
			continue
		}
		if entry["alias"] != "blocks" {
			continue
		}
		nextEntry := cloneObject(entry)
		nextEntry["value"] = encoded
		values[i] = nextEntry
		cloned["values"] = values
		return cloned
	}
	cloned["values"] = append(values, map[string]any{"alias": "blocks", "value": encoded})
	return cloned
}

func requireDatatypeBlockEditor(payload map[string]any, datatypeID string) (string, error) {
	editor, _ := payload["editorAlias"].(string)
	if editor == "" {
		return "", fmt.Errorf("datatype %s has no editorAlias; cannot determine whether it supports blocks", datatypeID)
	}
	if !datatypeBlockEditorAliases[editor] {
		return "", fmt.Errorf("datatype %s uses editorAlias %q; block commands only support Umbraco.BlockList and Umbraco.BlockGrid", datatypeID, editor)
	}
	return editor, nil
}

func findBlockIndex(blocks []map[string]any, contentElementTypeKey string) int {
	for i, b := range blocks {
		if asString(b["contentElementTypeKey"]) == contentElementTypeKey {
			return i
		}
	}
	return -1
}
