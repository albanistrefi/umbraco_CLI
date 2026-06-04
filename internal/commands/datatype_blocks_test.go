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
				{"contentElementTypeKey":"existing-1","label":"Text Block","editorSize":"medium","forceHideContentEditorInOverlay":false},
				{"contentElementTypeKey":"existing-2","label":"Image Block","editorSize":"large","forceHideContentEditorInOverlay":false}
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
	if len(blocks) != 2 || blocks[0]["contentElementTypeKey"] != "existing-1" {
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
		"--content-element-type", "new-block-guid",
		"--settings-element-type", "settings-guid",
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
	if last["contentElementTypeKey"] != "new-block-guid" {
		t.Fatalf("new block should be appended at the end, got %+v", last)
	}
	if last["settingsElementTypeKey"] != "settings-guid" {
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
		"--content-element-type", "existing-1",
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
		"--content-element-type", "some-guid",
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
		"--content-element-type", "x",
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
		"--content-element-type", "existing-1",
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
	if blocks[0].(map[string]any)["contentElementTypeKey"] != "existing-2" {
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
		"--content-element-type", "never-there",
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

func TestDatatypeBlockAcceptsBlockGridEditor(t *testing.T) {
	// BlockGrid uses the same blocks-array shape; just confirm the editor
	// allowlist accepts it. --group support is a deferred follow-up; this
	// test just verifies a v1 add on a BlockGrid datatype doesn't error.
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

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"datatype", "block", "add", blListID,
		"--content-element-type", "grid-block-1",
	); err != nil {
		t.Fatalf("BlockGrid add failed: %v", err)
	}
	if putBody == nil {
		t.Fatalf("expected PUT to fire on first block add against BlockGrid")
	}
}
