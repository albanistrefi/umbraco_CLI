package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const blListID = "dt-blocklist-1"
const blPath = "/umbraco/management/api/v1/data-type/dt-blocklist-1"

// blockListPayload returns a realistic Block List datatype payload with two
// pre-existing blocks plus an unrelated values entry, so tests can verify
// that unrelated config survives a block mutation round-trip.
func blockListPayload(t *testing.T) string {
	t.Helper()
	return `{
		"id":"dt-blocklist-1",
		"name":"Test Block List",
		"editorAlias":"Umbraco.BlockList",
		"values":[
			{"alias":"blocks","value":[
				{"contentElementTypeKey":"11111111-1111-1111-1111-111111111111","label":"Text Block","editorSize":"medium","forceHideContentEditorInOverlay":false},
				{"contentElementTypeKey":"22222222-2222-2222-2222-222222222222","label":"Image Block","editorSize":"large","forceHideContentEditorInOverlay":false}
			]},
			{"alias":"validationLimit","value":{"min":1,"max":10}}
		]
	}`
}

func TestDatatypeBlockListReturnsExistingBlocks(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "datatype", "block", "list", blListID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(output), &blocks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(blocks) != 2 || blocks[0]["contentElementTypeKey"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected blocks output: %+v", blocks)
	}
}

func TestDatatypeBlockAddPreservesExistingBlocksAndUnrelatedConfig(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			switch req.Method {
			case http.MethodGet:
				return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
			case http.MethodPut:
				if err := json.NewDecoder(req.Body).Decode(&putBody); err != nil {
					t.Fatalf("decode put: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{"updated":true}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "33333333-3333-3333-3333-333333333333",
		"--settings-element-type", "44444444-4444-4444-4444-444444444444",
		"--label", "Hero Block",
		"--editor-size", "Large",
	)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	values, _ := putBody["values"].([]any)
	if len(values) != 2 {
		t.Fatalf("expected unrelated 'validationLimit' values entry preserved, got %d entries", len(values))
	}

	// Find blocks entry, assert all three blocks present in order.
	var blocks []any
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] == "blocks" {
			blocks = entry["value"].([]any)
			break
		}
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks after append, got %d: %+v", len(blocks), blocks)
	}
	last := blocks[2].(map[string]any)
	if last["contentElementTypeKey"] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("new block should be appended at the end, got %+v", last)
	}
	if last["settingsElementTypeKey"] != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("settings element type missing, got %+v", last)
	}
	if last["editorSize"] != "large" {
		t.Fatalf("editor-size should be lowercased, got %+v", last["editorSize"])
	}

	// Output is the mutation summary.
	var summary map[string]any
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary["changed"] != true || summary["editorAlias"] != "Umbraco.BlockList" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestDatatypeBlockAddIsIdempotent(t *testing.T) {
	var putCount int32
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				atomic.AddInt32(&putCount, 1)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
	)
	if err != nil {
		t.Fatalf("add idempotent failed: %v", err)
	}
	if atomic.LoadInt32(&putCount) != 0 {
		t.Fatalf("expected zero PUTs for idempotent add, got %d", putCount)
	}

	var summary map[string]any
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary["changed"] != false {
		t.Fatalf("expected changed=false on idempotent add, got %+v", summary)
	}
	if !strings.Contains(summary["message"].(string), "already") {
		t.Fatalf("expected helpful message, got %+v", summary)
	}
}

func TestDatatypeBlockAddRejectsNonBlockEditor(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-blocklist-1","editorAlias":"Umbraco.MultipleTextstring","values":[]}`), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "00000000-0000-0000-0000-aaaaaaaaaaaa",
	)
	if err == nil {
		t.Fatalf("expected rejection for non-block editor")
	}
	if !strings.Contains(err.Error(), "Umbraco.BlockList") || !strings.Contains(err.Error(), "MultipleTextstring") {
		t.Fatalf("error should name both editors, got: %v", err)
	}
}

func TestDatatypeBlockAddRejectsInvalidEditorSize(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--editor-size", "huge",
	)
	if err == nil {
		t.Fatalf("expected --editor-size validation")
	}
	if !strings.Contains(err.Error(), "small, medium, large") {
		t.Fatalf("error should list valid sizes, got: %v", err)
	}
}

func TestDatatypeBlockRemoveDropsBlockAndPreservesRest(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			switch req.Method {
			case http.MethodGet:
				return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
			case http.MethodPut:
				if err := json.NewDecoder(req.Body).Decode(&putBody); err != nil {
					t.Fatalf("decode put: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "remove", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
	)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	var blocks []any
	for _, v := range putBody["values"].([]any) {
		entry := v.(map[string]any)
		if entry["alias"] == "blocks" {
			blocks = entry["value"].([]any)
			break
		}
	}
	if len(blocks) != 1 {
		t.Fatalf("expected one remaining block, got %+v", blocks)
	}
	if blocks[0].(map[string]any)["contentElementTypeKey"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("wrong block survived: %+v", blocks[0])
	}
}

func TestDatatypeBlockRemoveIsIdempotent(t *testing.T) {
	var putCount int32
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				atomic.AddInt32(&putCount, 1)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "remove", blListID,
		"--content-element-type", "00000000-0000-0000-0000-bbbbbbbbbbbb",
	)
	if err != nil {
		t.Fatalf("idempotent remove failed: %v", err)
	}
	if atomic.LoadInt32(&putCount) != 0 {
		t.Fatalf("expected zero PUTs for idempotent remove, got %d", putCount)
	}

	var summary map[string]any
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary["changed"] != false {
		t.Fatalf("expected changed=false: %+v", summary)
	}
}

// blockGridAddCaptured runs `datatype block add` against a synthetic
// BlockGrid datatype and returns the block object that landed in the PUT
// payload. Helper so the BlockGrid placement tests stay readable.
func blockGridAddCaptured(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-blocklist-1","editorAlias":"Umbraco.BlockGrid","values":[{"alias":"blocks","value":[]}]}`), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(buildRootWithCollections(t, deps), append([]string{"datatype", "block", "add", blListID}, args...)...); err != nil {
		t.Fatalf("BlockGrid add failed: %v", err)
	}
	if putBody == nil {
		t.Fatalf("expected PUT to fire on first block add against BlockGrid")
	}
	for _, v := range putBody["values"].([]any) {
		entry := v.(map[string]any)
		if entry["alias"] == "blocks" {
			blocks := entry["value"].([]any)
			if len(blocks) != 1 {
				t.Fatalf("expected exactly one block, got %d", len(blocks))
			}
			return blocks[0].(map[string]any)
		}
	}
	t.Fatalf("no blocks entry found in PUT payload")
	return nil
}

func TestDatatypeBlockBlockGridDefaultsAllowAtRootAndAllowInAreasToTrue(t *testing.T) {
	// Both placement flags default to true so the block is actually placeable
	// after registration — the server defaults to false when omitted, which
	// would silently register a block that's invisible in the editor.
	block := blockGridAddCaptured(t, "--content-element-type", "55555555-5555-5555-5555-555555555555")
	if block["allowAtRoot"] != true {
		t.Fatalf("expected allowAtRoot=true default, got %+v", block["allowAtRoot"])
	}
	if block["allowInAreas"] != true {
		t.Fatalf("expected allowInAreas=true default, got %+v", block["allowInAreas"])
	}
}

func TestDatatypeBlockBlockGridHonoursExplicitPlacementOverrides(t *testing.T) {
	block := blockGridAddCaptured(t,
		"--content-element-type", "55555555-5555-5555-5555-555555555555",
		"--allow-at-root=false",
		"--allow-in-areas=false",
	)
	if block["allowAtRoot"] != false {
		t.Fatalf("expected --allow-at-root=false to override, got %+v", block["allowAtRoot"])
	}
	if block["allowInAreas"] != false {
		t.Fatalf("expected --allow-in-areas=false to override, got %+v", block["allowInAreas"])
	}
}

func TestDatatypeBlockBlockListOmitsPlacementFlags(t *testing.T) {
	// Placement flags are BlockGrid-only; setting them on a BlockList add
	// must NOT pollute the payload with fields the editor doesn't understand.
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "66666666-6666-6666-6666-666666666666",
		"--allow-at-root=true", "--allow-in-areas=true",
	); err != nil {
		t.Fatalf("BlockList add failed: %v", err)
	}

	var newBlock map[string]any
	for _, v := range putBody["values"].([]any) {
		entry := v.(map[string]any)
		if entry["alias"] == "blocks" {
			blocks := entry["value"].([]any)
			newBlock = blocks[len(blocks)-1].(map[string]any)
			break
		}
	}
	if _, hasRoot := newBlock["allowAtRoot"]; hasRoot {
		t.Fatalf("BlockList block must not include allowAtRoot, got %+v", newBlock)
	}
	if _, hasAreas := newBlock["allowInAreas"]; hasAreas {
		t.Fatalf("BlockList block must not include allowInAreas, got %+v", newBlock)
	}
}

// --- datatype block update ---
// These tests cover the acceptance criteria for the v0.3.16-follow-up
// 'block update' command. Mirror the structure of 'block add' tests where
// behaviour is symmetric (idempotency, BlockList-ignores-grid-flags,
// editor-size validation, non-block-editor rejection).

// blockListPayloadWithEditorUI is a variant of blockListPayload that
// includes editorUiAlias and useSingleBlockMode at the top level. Used to
// pin the "must not clobber top-level fields" regression that prompted
// 'block update' to exist in the first place — agents hand-rolling the
// full datatype update --json kept dropping editorUiAlias.
func blockListPayloadWithEditorUI(t *testing.T) string {
	t.Helper()
	return `{
		"id":"dt-blocklist-1",
		"name":"Test Block List",
		"editorAlias":"Umbraco.BlockList",
		"editorUiAlias":"Umb.PropertyEditorUi.BlockList",
		"values":[
			{"alias":"blocks","value":[
				{"contentElementTypeKey":"11111111-1111-1111-1111-111111111111","label":"Text Block","editorSize":"medium","forceHideContentEditorInOverlay":false,"thumbnail":"/img/old.png"},
				{"contentElementTypeKey":"22222222-2222-2222-2222-222222222222","label":"Image Block","editorSize":"large","forceHideContentEditorInOverlay":false}
			]},
			{"alias":"validationLimit","value":{"min":1,"max":10}},
			{"alias":"useSingleBlockMode","value":false}
		]
	}`
}

// Mutates only the targeted block; sibling blocks AND every top-level
// field/value survive untouched. This is the regression guard for the
// "datatype update --json dropped editorUiAlias" bug — block update must
// not reintroduce it.
func TestDatatypeBlockUpdatePreservesEverythingElse(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			switch req.Method {
			case http.MethodGet:
				return datatypeJSONResponse(http.StatusOK, blockListPayloadWithEditorUI(t)), nil
			case http.MethodPut:
				if err := json.NewDecoder(req.Body).Decode(&putBody); err != nil {
					t.Fatalf("decode put: %v", err)
				}
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--label", "Updated Text Block",
	); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if putBody["editorUiAlias"] != "Umb.PropertyEditorUi.BlockList" {
		t.Fatalf("editorUiAlias must survive the PUT (the v0.3.15-era regression), got %+v", putBody["editorUiAlias"])
	}
	values := putBody["values"].([]any)
	if len(values) != 3 {
		t.Fatalf("expected validationLimit and useSingleBlockMode preserved, got %d values entries", len(values))
	}

	// Find blocks entry; siblings unchanged, only 11111111-1111-1111-1111-111111111111's label mutated.
	var blocks []any
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] == "blocks" {
			blocks = entry["value"].([]any)
			break
		}
	}
	if len(blocks) != 2 {
		t.Fatalf("expected both blocks preserved, got %+v", blocks)
	}
	updated := blocks[0].(map[string]any)
	sibling := blocks[1].(map[string]any)
	if updated["contentElementTypeKey"] != "11111111-1111-1111-1111-111111111111" || updated["label"] != "Updated Text Block" {
		t.Fatalf("target block label not updated, got %+v", updated)
	}
	// Unpassed fields on the target must NOT have moved.
	if updated["editorSize"] != "medium" {
		t.Fatalf("editorSize on the target was clobbered (must only mutate passed flags), got %+v", updated["editorSize"])
	}
	if updated["thumbnail"] != "/img/old.png" {
		t.Fatalf("thumbnail on the target was clobbered, got %+v", updated["thumbnail"])
	}
	// Sibling block unchanged.
	if sibling["contentElementTypeKey"] != "22222222-2222-2222-2222-222222222222" || sibling["label"] != "Image Block" || sibling["editorSize"] != "large" {
		t.Fatalf("sibling block was disturbed, got %+v", sibling)
	}
}

// Partial-update semantics: passing only --editor-size leaves label,
// thumbnail, and settingsElementTypeKey alone. cmd.Flags().Changed is the
// gate, not zero-value detection.
func TestDatatypeBlockUpdateOnlyMutatesPassedFlags(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
			}
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--editor-size", "large",
	); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	values := putBody["values"].([]any)
	var target map[string]any
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] != "blocks" {
			continue
		}
		for _, b := range entry["value"].([]any) {
			block := b.(map[string]any)
			if block["contentElementTypeKey"] == "11111111-1111-1111-1111-111111111111" {
				target = block
				break
			}
		}
	}
	if target["editorSize"] != "large" {
		t.Fatalf("editorSize not updated, got %+v", target)
	}
	if target["label"] != "Text Block" {
		t.Fatalf("label must remain 'Text Block' (unpassed flag), got %+v", target)
	}
}

// --label "" / --thumbnail "" / --settings-element-type "" should remove
// the key entirely (server falls back). Without this, agents have no way
// to clear an override label they set earlier.
func TestDatatypeBlockUpdateEmptyStringClearsOptionalFields(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodGet {
				return datatypeJSONResponse(http.StatusOK, blockListPayloadWithEditorUI(t)), nil
			}
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--label", "",
		"--thumbnail", "",
	); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	values := putBody["values"].([]any)
	var target map[string]any
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] != "blocks" {
			continue
		}
		target = entry["value"].([]any)[0].(map[string]any)
	}
	if _, hasLabel := target["label"]; hasLabel {
		t.Fatalf("empty --label must REMOVE the key, got %+v", target)
	}
	if _, hasThumbnail := target["thumbnail"]; hasThumbnail {
		t.Fatalf("empty --thumbnail must REMOVE the key, got %+v", target)
	}
}

func TestDatatypeBlockUpdateErrorsWhenBlockMissing(t *testing.T) {
	var putCount int
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				putCount++
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	const absentGUID = "deadbeef-dead-beef-dead-beefdeadbeef"
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", absentGUID,
		"--label", "X",
	)
	if err == nil {
		t.Fatalf("expected update to error on missing block")
	}
	if !strings.Contains(err.Error(), absentGUID) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should name the missing block clearly, got: %v", err)
	}
	if putCount != 0 {
		t.Fatalf("missing-block error must short-circuit before any PUT, got %d", putCount)
	}
}

func TestDatatypeBlockUpdateIsIdempotent(t *testing.T) {
	var putCount int
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				putCount++
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	// Re-applying the existing label is a no-op.
	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--label", "Text Block",
	)
	if err != nil {
		t.Fatalf("idempotent update failed: %v", err)
	}
	if putCount != 0 {
		t.Fatalf("identical update must not PUT, got %d", putCount)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary["changed"] != false {
		t.Fatalf("expected changed=false on no-op, got %+v", summary)
	}
}

func TestDatatypeBlockUpdateRejectsInvalidEditorSize(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--editor-size", "huge",
	)
	if err == nil {
		t.Fatalf("expected --editor-size validation")
	}
	if !strings.Contains(err.Error(), "small, medium, large") {
		t.Fatalf("error should list valid sizes, got: %v", err)
	}
}

func TestDatatypeBlockUpdateBlockListIgnoresGridFlags(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--label", "Touch Label",
		"--allow-at-root=false",
		"--allow-in-areas=false",
	); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	values := putBody["values"].([]any)
	var target map[string]any
	for _, v := range values {
		entry := v.(map[string]any)
		if entry["alias"] != "blocks" {
			continue
		}
		target = entry["value"].([]any)[0].(map[string]any)
	}
	if _, set := target["allowAtRoot"]; set {
		t.Fatalf("BlockList block must not carry allowAtRoot (grid-only), got %+v", target)
	}
	if _, set := target["allowInAreas"]; set {
		t.Fatalf("BlockList block must not carry allowInAreas (grid-only), got %+v", target)
	}
}

func TestDatatypeBlockUpdateBlockGridHonoursPlacementFlags(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-blocklist-1","editorAlias":"Umbraco.BlockGrid","values":[{"alias":"blocks","value":[{"contentElementTypeKey":"77777777-7777-7777-7777-777777777777","label":"Old","allowAtRoot":true,"allowInAreas":true}]}]}`), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "77777777-7777-7777-7777-777777777777",
		"--allow-at-root=false",
	); err != nil {
		t.Fatalf("BlockGrid update failed: %v", err)
	}
	values := putBody["values"].([]any)
	target := values[0].(map[string]any)["value"].([]any)[0].(map[string]any)
	if target["allowAtRoot"] != false {
		t.Fatalf("expected allowAtRoot toggled false, got %+v", target["allowAtRoot"])
	}
	// allowInAreas was not passed → must remain the existing value (true).
	if target["allowInAreas"] != true {
		t.Fatalf("unpassed --allow-in-areas must leave the existing value (true), got %+v", target["allowInAreas"])
	}
}

func TestDatatypeBlockUpdateDryRunSendsNoPut(t *testing.T) {
	var putCount int
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				putCount++
			}
			return datatypeJSONResponse(http.StatusOK, blockListPayload(t)), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--label", "New label",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if putCount != 0 {
		t.Fatalf("dry-run must not PUT, got %d", putCount)
	}
	var dry map[string]any
	if err := json.Unmarshal([]byte(output), &dry); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if dry["dryRun"] != true || dry["method"] != "PUT" {
		t.Fatalf("expected DryRunResult envelope, got %+v", dry)
	}
}

// --- GUID flag validation across block add/update/remove ---
// Codex re-review on PR #5 flagged that --content-element-type and
// --settings-element-type were unchecked: a typo'd content GUID would fall
// through to "block not found" (misleading) and a typo'd settings GUID
// would persist as-is (broken settings overlay in the backoffice). Pinned
// across all three subcommands since the surface is symmetric.

func TestDatatypeBlockAddRejectsMalformedContentGUID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("invalid GUID must short-circuit before any HTTP call")
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "not-a-uuid",
	)
	if err == nil {
		t.Fatalf("expected GUID rejection")
	}
	if !strings.Contains(err.Error(), "must be a GUID") || !strings.Contains(err.Error(), "--content-element-type") {
		t.Fatalf("error should name flag and reason, got: %v", err)
	}
}

func TestDatatypeBlockAddRejectsMalformedSettingsGUID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("invalid GUID must short-circuit before any HTTP call")
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--settings-element-type", "garbage",
	)
	if err == nil {
		t.Fatalf("expected settings GUID rejection")
	}
	if !strings.Contains(err.Error(), "--settings-element-type") {
		t.Fatalf("error should name the offending flag, got: %v", err)
	}
}

func TestDatatypeBlockUpdateRejectsMalformedContentGUID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("invalid GUID must short-circuit before any HTTP call")
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "not-a-uuid",
		"--label", "X",
	)
	if err == nil || !strings.Contains(err.Error(), "--content-element-type") {
		t.Fatalf("expected --content-element-type rejection, got: %v", err)
	}
}

func TestDatatypeBlockUpdateRejectsMalformedSettingsGUID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("invalid GUID must short-circuit before any HTTP call")
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--settings-element-type", "garbage",
	)
	if err == nil || !strings.Contains(err.Error(), "--settings-element-type") {
		t.Fatalf("expected --settings-element-type rejection, got: %v", err)
	}
}

// Empty --settings-element-type on update is the "clear this field" signal;
// must NOT trigger the GUID validator (that was the whole point of guarding
// behind cmd.Flags().Changed && value != "").
func TestDatatypeBlockUpdateEmptySettingsClearsWithoutGUIDValidation(t *testing.T) {
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case blPath:
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&putBody)
				return datatypeJSONResponse(http.StatusOK, `{}`), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"dt-blocklist-1","editorAlias":"Umbraco.BlockList","values":[{"alias":"blocks","value":[{"contentElementTypeKey":"11111111-1111-1111-1111-111111111111","settingsElementTypeKey":"44444444-4444-4444-4444-444444444444","label":"L"}]}]}`), nil
		}
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "update", blListID,
		"--content-element-type", "11111111-1111-1111-1111-111111111111",
		"--settings-element-type", "",
	); err != nil {
		t.Fatalf("empty --settings-element-type should clear, not error: %v", err)
	}
	target := putBody["values"].([]any)[0].(map[string]any)["value"].([]any)[0].(map[string]any)
	if _, present := target["settingsElementTypeKey"]; present {
		t.Fatalf("empty --settings-element-type must delete the key, got %+v", target)
	}
}

func TestDatatypeBlockRemoveRejectsMalformedContentGUID(t *testing.T) {
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("invalid GUID must short-circuit before any HTTP call")
		return datatypeJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "remove", blListID,
		"--content-element-type", "not-a-uuid",
	)
	if err == nil || !strings.Contains(err.Error(), "--content-element-type") {
		t.Fatalf("expected --content-element-type rejection, got: %v", err)
	}
}
