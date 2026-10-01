package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"umbraco-cli/internal/commands/cmdtest"
)

func TestGeneratedSkillsFlattenNestedSubgroups(t *testing.T) {
	dir := t.TempDir()
	if _, err := cmdtest.Execute(buildRootWithCollections(t, cmdtest.MakeDeps()),
		"generate-skills", "--filter", "document", "--output-dir", dir); err != nil {
		t.Fatalf("generate-skills failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "umbraco-document", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generated skill: %v", err)
	}
	if !strings.Contains(string(content), "### version rollback") {
		t.Fatal("expected nested 'document version rollback' to document as a leaf command")
	}
	if strings.Contains(string(content), "```bash\numbraco document version\n```") {
		t.Fatal("subgroup must not render as an empty stub")
	}
}
