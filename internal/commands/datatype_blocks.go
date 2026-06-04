package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

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
		Long:  "Read-modify-write helpers that mutate the 'blocks' value entry on Umbraco.BlockList and Umbraco.BlockGrid datatypes without clobbering the rest of the configuration. Idempotent: 'add' is a no-op if the element type is already an allowed block; 'remove' is a no-op if it isn't.",
	}
	cmd.AddCommand(datatypeBlockList(deps))
	cmd.AddCommand(datatypeBlockAdd(deps))
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
