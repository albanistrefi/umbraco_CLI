package commands

import (
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
	cmd.AddCommand(datatypeBlockGroups(deps))
	cmd.AddCommand(datatypeBlockAdd(deps))
	cmd.AddCommand(datatypeBlockUpdate(deps))
	cmd.AddCommand(datatypeBlockReorder(deps))
	cmd.AddCommand(datatypeBlockRemove(deps))
	return cmd
}

func datatypeBlockList(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <datatypeId>",
		Short: "List allowed blocks on a Block List / Block Grid datatype",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := fetchDatatypeObject(cmd.Context(), deps.Client, args[0])
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
	var group string
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
		Long:  "Appends a block to the datatype's blocks array. Idempotent: if a block with the same --content-element-type is already present, no PUT is sent.\n\nBlockGrid: --allow-at-root and --allow-in-areas default to true so the block is actually placeable after registration (server-side both default to false when omitted, which would register a block that's invisible to editors). Pass --allow-at-root=false or --allow-in-areas=false to override. --group <name> places the block in a BlockGrid block group, creating the group in blockGroups when it does not exist yet (groups are matched by name, case-insensitively).\n\nBlockList: --allow-at-root, --allow-in-areas and --group are Block Grid concepts; the first two are ignored, --group is rejected.",
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

			ctx := cmd.Context()
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
			if group != "" {
				if editor != "Umbraco.BlockGrid" {
					return fmt.Errorf("--group applies to Umbraco.BlockGrid only; %s is %s", args[0], editor)
				}
				groupKey, withGroup := ensureBlockGroup(payload, group)
				payload = withGroup
				block["groupKey"] = groupKey
			}

			next := append([]map[string]any{}, blocks...)
			next = append(next, block)
			nextPayload := writeDatatypeBlocks(payload, next)

			result, err := deps.Client.Put(
				ctx,
				api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun},
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
	cmd.Flags().StringVar(&group, "group", "", "BlockGrid only: put the block in this block group (created in blockGroups when it does not exist yet)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

func datatypeBlockUpdate(deps Dependencies) *cobra.Command {
	var group string
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

			ctx := cmd.Context()
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
			if cmd.Flags().Changed("group") {
				if editor != "Umbraco.BlockGrid" {
					return fmt.Errorf("--group applies to Umbraco.BlockGrid only; %s is %s", args[0], editor)
				}
				if strings.TrimSpace(group) == "" {
					delete(updated, "groupKey")
				} else {
					groupKey, withGroup := ensureBlockGroup(payload, group)
					payload = withGroup
					updated["groupKey"] = groupKey
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
				api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun},
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
	cmd.Flags().StringVar(&group, "group", "", "BlockGrid only: move the block into this block group (created when missing). Pass empty string to remove it from its group.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

// datatypeBlockGroups lists a Block Grid's block groups with how many blocks
// each holds.
func datatypeBlockGroups(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "groups <datatypeId>",
		Short: "List a Block Grid's block groups (blockGroups) with block counts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := fetchDatatypeObject(cmd.Context(), deps.Client, args[0])
			if err != nil {
				return err
			}
			editor, err := requireDatatypeBlockEditor(payload, args[0])
			if err != nil {
				return err
			}
			if editor != "Umbraco.BlockGrid" {
				return fmt.Errorf("block groups exist on Umbraco.BlockGrid only; %s is %s", args[0], editor)
			}
			counts := map[string]int{}
			ungrouped := 0
			for _, block := range loadDatatypeBlocks(payload) {
				if key := asString(block["groupKey"]); key != "" {
					counts[strings.ToLower(key)]++
				} else {
					ungrouped++
				}
			}
			groups := []map[string]any{}
			for _, group := range loadBlockGroups(payload) {
				key := asString(group["key"])
				groups = append(groups, map[string]any{"key": key, "name": group["name"], "blocks": counts[strings.ToLower(key)]})
			}
			return printResult(cmd, deps, map[string]any{"datatypeId": args[0], "groups": groups, "ungroupedBlocks": ungrouped})
		},
	}
}

// datatypeBlockReorder rewrites the blocks array in the given order: listed
// keys first, in that order; unlisted blocks keep their relative order after
// them. Array order is what the block picker shows.
func datatypeBlockReorder(deps Dependencies) *cobra.Command {
	var keys []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "reorder <datatypeId>",
		Short: "Reorder the allowed blocks (array order is the picker order)",
		Long:  "Rewrites the datatype's blocks array so the --keys content element types come first in the given order; blocks not listed keep their relative order after them. Idempotent: no PUT when the order is already the requested one. Re-reads the datatype afterwards and fails if the server did not persist the order.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(keys) == 0 {
				return fmt.Errorf("--keys is required (comma-separated content element type GUIDs in the desired order)")
			}
			for _, key := range keys {
				if err := validateBlockGUID("--keys", key); err != nil {
					return err
				}
			}
			ctx := cmd.Context()
			payload, err := fetchDatatypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return err
			}
			editor, err := requireDatatypeBlockEditor(payload, args[0])
			if err != nil {
				return err
			}
			blocks := loadDatatypeBlocks(payload)
			next, err := reorderBlocks(blocks, keys)
			if err != nil {
				return err
			}
			order := blockKeyOrder(next)
			if reflect.DeepEqual(blockKeyOrder(blocks), order) {
				return printResult(cmd, deps, map[string]any{"action": "reorder", "datatypeId": args[0], "editorAlias": editor, "changed": false, "order": order})
			}
			result, err := deps.Client.Put(ctx, api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]), writeDatatypeBlocks(payload, next), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			after, err := fetchDatatypeObject(ctx, deps.Client, args[0])
			if err != nil {
				return fmt.Errorf("the update was accepted but re-reading the datatype failed: %w", err)
			}
			persisted := blockKeyOrder(loadDatatypeBlocks(after))
			if !reflect.DeepEqual(persisted, order) {
				return fmt.Errorf("the server accepted the update but persisted a different block order: %v", persisted)
			}
			return printResult(cmd, deps, map[string]any{"action": "reorder", "datatypeId": args[0], "editorAlias": editor, "changed": true, "order": order, "verified": true})
		},
	}
	cmd.Flags().StringSliceVar(&keys, "keys", nil, "Content element type GUIDs in the desired order (comma-separated or repeated); unlisted blocks follow in their current order")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resulting payload without writing it")
	return cmd
}

func reorderBlocks(blocks []map[string]any, keys []string) ([]map[string]any, error) {
	next := make([]map[string]any, 0, len(blocks))
	used := make(map[int]bool, len(blocks))
	for _, key := range keys {
		idx := findBlockIndex(blocks, strings.ToLower(key))
		if idx < 0 {
			idx = findBlockIndex(blocks, key)
		}
		if idx < 0 {
			return nil, fmt.Errorf("datatype has no block with content element type %s", key)
		}
		if used[idx] {
			return nil, fmt.Errorf("content element type %s listed twice", key)
		}
		used[idx] = true
		next = append(next, blocks[idx])
	}
	for i, block := range blocks {
		if !used[i] {
			next = append(next, block)
		}
	}
	return next, nil
}

func blockKeyOrder(blocks []map[string]any) []string {
	order := make([]string, 0, len(blocks))
	for _, block := range blocks {
		order = append(order, strings.ToLower(asString(block["contentElementTypeKey"])))
	}
	return order
}

// loadBlockGroups reads the blockGroups value entry ([{key, name}]).
func loadBlockGroups(payload map[string]any) []map[string]any {
	values, _ := payload["values"].([]any)
	for _, item := range values {
		entry, _ := item.(map[string]any)
		if entry == nil || entry["alias"] != "blockGroups" {
			continue
		}
		raw, _ := entry["value"].([]any)
		out := make([]map[string]any, 0, len(raw))
		for _, g := range raw {
			if asMap, ok := g.(map[string]any); ok {
				out = append(out, asMap)
			}
		}
		return out
	}
	return nil
}

// ensureBlockGroup returns the key of the block group named name, creating
// the group (with a fresh GUID) in a cloned payload when it does not exist.
// Names match case-insensitively.
func ensureBlockGroup(payload map[string]any, name string) (string, map[string]any) {
	name = strings.TrimSpace(name)
	for _, group := range loadBlockGroups(payload) {
		if strings.EqualFold(asString(group["name"]), name) {
			return asString(group["key"]), payload
		}
	}
	key, err := newUUIDv4()
	if err != nil {
		key = strings.ToLower(name)
	}
	groups := append([]map[string]any{}, loadBlockGroups(payload)...)
	groups = append(groups, map[string]any{"key": key, "name": name})
	return key, writeDatatypeValue(payload, "blockGroups", groups)
}

// writeDatatypeValue returns a deep-cloned payload with the given value entry
// replaced (or appended), preserving every other field.
func writeDatatypeValue(payload map[string]any, alias string, next []map[string]any) map[string]any {
	cloned := cloneObject(payload)
	encoded := make([]any, 0, len(next))
	for _, item := range next {
		encoded = append(encoded, cloneObject(item))
	}
	values, ok := cloned["values"].([]any)
	if !ok {
		cloned["values"] = []any{map[string]any{"alias": alias, "value": encoded}}
		return cloned
	}
	for i, item := range values {
		entry, entryOk := item.(map[string]any)
		if !entryOk || entry["alias"] != alias {
			continue
		}
		nextEntry := cloneObject(entry)
		nextEntry["value"] = encoded
		values[i] = nextEntry
		cloned["values"] = values
		return cloned
	}
	cloned["values"] = append(values, map[string]any{"alias": alias, "value": encoded})
	return cloned
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

			ctx := cmd.Context()
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
				api.JoinPath(dataTypeLegacyCollectionPath+"/%s", args[0]),
				nextPayload,
				api.RequestOptions{DryRun: dryRun},
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
