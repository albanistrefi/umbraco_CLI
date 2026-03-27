package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

func makeDeps() Dependencies {
	cfg := config.Config{BaseURL: "https://example.test"}
	client := api.NewClient(cfg, http.DefaultClient, nil)
	output := "json"
	return Dependencies{Client: client, EnvOutput: config.OutputJSON, OutputFlag: &output}
}

func buildRootWithCollections(t *testing.T, deps Dependencies) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	RegisterDocument(root, deps)
	RegisterDictionary(root, deps)
	RegisterMedia(root, deps)
	RegisterDoctype(root, deps)
	RegisterDatatype(root, deps)
	RegisterTemplate(root, deps)
	RegisterLogs(root, deps)
	RegisterServer(root, deps)
	RegisterHealth(root, deps)
	RegisterTree(root, deps)
	RegisterSchema(root, deps)
	return root
}

func execute(root *cobra.Command, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestCommandCountsMatchMVP(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	total := 0
	for collection, expected := range ExpectedCollectionCommandCounts {
		var found *cobra.Command
		for _, command := range root.Commands() {
			if command.Name() == collection {
				found = command
				break
			}
		}
		if found == nil {
			t.Fatalf("missing collection %s", collection)
		}
		if len(found.Commands()) != expected {
			t.Fatalf("collection %s expected %d commands, got %d", collection, expected, len(found.Commands()))
		}
		total += len(found.Commands())
	}

	if total != 78 {
		t.Fatalf("expected 78 collection commands, got %d", total)
	}
}

func TestSchemaCommandListAndCollectionLookup(t *testing.T) {
	deps := makeDeps()
	output, err := execute(buildRootWithCollections(t, deps), "schema", "--list")
	if err != nil {
		t.Fatalf("schema --list failed: %v", err)
	}
	var listPayload map[string]any
	if err := json.Unmarshal([]byte(output), &listPayload); err != nil {
		t.Fatalf("failed to parse list payload: %v", err)
	}
	endpoints, ok := listPayload["endpoints"].([]any)
	if !ok || len(endpoints) == 0 {
		t.Fatalf("expected non-empty endpoints list")
	}

	output, err = execute(buildRootWithCollections(t, deps), "schema", "document")
	if err != nil {
		t.Fatalf("schema collection lookup failed: %v", err)
	}
	var collectionPayload map[string]any
	if err := json.Unmarshal([]byte(output), &collectionPayload); err != nil {
		t.Fatalf("failed to parse collection payload: %v", err)
	}
	if collectionPayload["collection"] != "document" {
		t.Fatalf("unexpected collection payload: %+v", collectionPayload)
	}
}

func TestDocumentPublishPrefersJSONOverCultureInDryRun(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	output, err := execute(root,
		"document", "publish", "abc-123",
		"--json", `{"cultures":["da-DK"]}`,
		"--culture", "en-US",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("document publish failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to parse dry-run payload: %v", err)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("missing body in dry-run payload: %+v", payload)
	}
	cultures, ok := body["cultures"].([]any)
	if !ok || len(cultures) != 1 || cultures[0] != "da-DK" {
		t.Fatalf("expected --json cultures to take precedence, got: %+v", body)
	}
}

func TestDatatypeSchemaMatchesCompatibilityPrimaryEndpoints(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	output, err := execute(root, "schema", "datatype.list")
	if err != nil {
		t.Fatalf("schema datatype.list failed: %v", err)
	}
	var listPayload map[string]any
	if err := json.Unmarshal([]byte(output), &listPayload); err != nil {
		t.Fatalf("failed to decode datatype.list schema: %v", err)
	}
	if listPayload["path"] != "/filter/data-type" {
		t.Fatalf("unexpected datatype.list path: %+v", listPayload)
	}

	output, err = execute(root, "schema", "datatype.root")
	if err != nil {
		t.Fatalf("schema datatype.root failed: %v", err)
	}
	var rootPayload map[string]any
	if err := json.Unmarshal([]byte(output), &rootPayload); err != nil {
		t.Fatalf("failed to decode datatype.root schema: %v", err)
	}
	if rootPayload["path"] != "/tree/data-type/root" {
		t.Fatalf("unexpected datatype.root path: %+v", rootPayload)
	}

	output, err = execute(root, "schema", "datatype.search")
	if err != nil {
		t.Fatalf("schema datatype.search failed: %v", err)
	}
	var searchPayload map[string]any
	if err := json.Unmarshal([]byte(output), &searchPayload); err != nil {
		t.Fatalf("failed to decode datatype.search schema: %v", err)
	}
	if searchPayload["path"] != "/item/data-type/search" {
		t.Fatalf("unexpected datatype.search path: %+v", searchPayload)
	}
}

func TestSchemaMatchesTemplateDoctypeAndServerPrimaryEndpoints(t *testing.T) {
	deps := makeDeps()
	root := buildRootWithCollections(t, deps)

	cases := map[string]string{
		"document.root":       "/tree/document/root",
		"document.children":   "/tree/document/children",
		"document.ancestors":  "/tree/document/ancestors",
		"document.search":     "/item/document/search",
		"media.search":        "/item/media/search",
		"template.root":       "/tree/template/root",
		"template.search":     "/item/template/search",
		"doctype.root":        "/tree/document-type/root",
		"doctype.children":    "/tree/document-type/children",
		"doctype.search":      "/item/document-type/search",
		"server.info":         "/server/information",
		"server.config":       "/server/configuration",
		"server.troubleshoot": "/server/troubleshooting",
	}

	for endpoint, expectedPath := range cases {
		output, err := execute(root, "schema", endpoint)
		if err != nil {
			t.Fatalf("schema %s failed: %v", endpoint, err)
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			t.Fatalf("failed to decode %s schema payload: %v", endpoint, err)
		}
		if payload["path"] != expectedPath {
			t.Fatalf("unexpected %s path: %+v", endpoint, payload)
		}
	}
}
