package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func buildTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}

	// Simulate a collection with read and mutation subcommands
	doc := &cobra.Command{Use: "document", Short: "Document operations"}

	get := &cobra.Command{Use: "get <id>", Short: "Get a document by ID"}
	get.Flags().String("fields", "", "Limit response fields")

	update := &cobra.Command{Use: "update <id>", Short: "Update a document"}
	update.Flags().String("json", "", "Update payload as JSON")
	update.Flags().Bool("dry-run", false, "Validate request without executing")

	del := &cobra.Command{Use: "delete <id>", Short: "Delete a document"}
	del.Flags().Bool("dry-run", false, "Validate request without executing")

	doc.AddCommand(get, update, del)
	root.AddCommand(doc)

	// Simulate a meta command that should be skipped
	schema := &cobra.Command{Use: "schema", Short: "Introspect schemas"}
	root.AddCommand(schema)

	return root
}

func TestGenerateCreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Shared skill
	assertFileExists(t, filepath.Join(dir, "umbraco-shared", "SKILL.md"))

	// Collection skill
	assertFileExists(t, filepath.Join(dir, "umbraco-document", "SKILL.md"))

	// Index
	assertFileExists(t, filepath.Join(dir, "INDEX.md"))

	// Recipe skills
	for _, recipe := range recipes {
		assertFileExists(t, filepath.Join(dir, "recipe-"+recipe.Name, "SKILL.md"))
	}

	// Schema should NOT be generated (meta command)
	schemaPath := filepath.Join(dir, "umbraco-schema", "SKILL.md")
	if _, err := os.Stat(schemaPath); err == nil {
		t.Fatalf("schema skill should not be generated, but found %s", schemaPath)
	}
}

func TestBlockedMethodsExcluded(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "umbraco-document", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read document skill: %v", err)
	}

	md := string(content)

	// "delete" should be blocked
	if strings.Contains(md, "### delete") {
		t.Fatal("blocked method 'delete' should not appear in generated skill")
	}

	// "get" and "update" should be present
	if !strings.Contains(md, "### get") {
		t.Fatal("expected 'get' subcommand in generated skill")
	}
	if !strings.Contains(md, "### update") {
		t.Fatal("expected 'update' subcommand in generated skill")
	}
}

func TestMutationCommandsHaveSafetyPattern(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "umbraco-document", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read document skill: %v", err)
	}

	md := string(content)

	if !strings.Contains(md, "## Mutation Commands") {
		t.Fatal("expected Mutation Commands section")
	}
	if !strings.Contains(md, "**Safe pattern:**") {
		t.Fatal("expected safe pattern block for mutation commands")
	}
	if !strings.Contains(md, "--dry-run") {
		t.Fatal("expected --dry-run in safety pattern")
	}
}

func TestFlagsTableRendered(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "umbraco-document", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read document skill: %v", err)
	}

	md := string(content)

	if !strings.Contains(md, "`--fields`") {
		t.Fatal("expected --fields flag in get subcommand")
	}
	if !strings.Contains(md, "`--json`") {
		t.Fatal("expected --json flag in update subcommand")
	}
}

func TestFilterLimitsOutput(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "document", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Document skill should exist
	assertFileExists(t, filepath.Join(dir, "umbraco-document", "SKILL.md"))

	// Shared skill should NOT exist (doesn't match "document")
	sharedPath := filepath.Join(dir, "umbraco-shared", "SKILL.md")
	if _, err := os.Stat(sharedPath); err == nil {
		t.Fatal("shared skill should not be generated when filter is 'document'")
	}

	// Index should NOT be written when filter is active
	indexPath := filepath.Join(dir, "INDEX.md")
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatal("INDEX.md should not be generated when filter is active")
	}
}

func TestRecipeSkillsGenerated(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, recipe := range recipes {
		path := filepath.Join(dir, "recipe-"+recipe.Name, "SKILL.md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read recipe skill %s: %v", recipe.Name, err)
		}

		md := string(content)

		if !strings.Contains(md, "## Steps") {
			t.Fatalf("recipe %s missing Steps section", recipe.Name)
		}
		if !strings.Contains(md, "PREREQUISITE") {
			t.Fatalf("recipe %s missing prerequisite note", recipe.Name)
		}
	}
}

func TestSharedSkillContent(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "umbraco-shared", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read shared skill: %v", err)
	}

	md := string(content)

	for _, expected := range []string{
		"## Authentication",
		"## Config Precedence",
		"## Global Flags",
		"## Safety Rules",
		"## Schema Introspection",
		"--dry-run",
		"--fields",
	} {
		if !strings.Contains(md, expected) {
			t.Fatalf("shared skill missing expected content: %s", expected)
		}
	}
}

func TestIndexContent(t *testing.T) {
	dir := t.TempDir()
	root := buildTestRoot()

	if err := Generate(root, dir, "", "0.0.1-test"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "INDEX.md"))
	if err != nil {
		t.Fatalf("failed to read index: %v", err)
	}

	md := string(content)

	if !strings.Contains(md, "## Reference") {
		t.Fatal("index missing Reference section")
	}
	if !strings.Contains(md, "## Collections") {
		t.Fatal("index missing Collections section")
	}
	if !strings.Contains(md, "## Recipes") {
		t.Fatal("index missing Recipes section")
	}
	if !strings.Contains(md, "umbraco-document") {
		t.Fatal("index missing umbraco-document entry")
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %s", path)
	}
}
