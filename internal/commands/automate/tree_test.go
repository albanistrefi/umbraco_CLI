// Tests that need the full production tree (the core schema and
// generate-skills commands) live in the external test package so they can
// import commands without an import cycle.
package automate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands"
	"umbraco-cli/internal/commands/automate"
	"umbraco-cli/internal/commands/cmdtest"
)

func TestAutomateCommandRegistersAndSchemaResolves(t *testing.T) {
	root := buildFullRoot(t)
	automateCmd := cmdtest.FindChildCommand(root, "automate")
	if automateCmd == nil {
		t.Fatal("missing automate command")
	}
	if automateCmd.Hidden {
		t.Fatal("automate command should be visible now that Automate is publicly launched")
	}

	output, err := cmdtest.Execute(root, "schema", "automate.automation.list")
	if err != nil {
		t.Fatalf("Automate schema lookup failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode schema output: %v", err)
	}
	if payload["apiRoot"] != automate.APIPrefix || payload["path"] != "/automations" {
		t.Fatalf("unexpected Automate schema payload: %+v", payload)
	}
}

func TestGenerateSkillsIncludesAutomateByDefault(t *testing.T) {
	_, err := cmdtest.Execute(buildFullRoot(t), "generate-skills", "--include-hidden", "--output-dir", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "--include-hidden requires --filter") {
		t.Fatalf("expected --include-hidden without --filter to fail, got %v", err)
	}

	dir := t.TempDir()
	if _, err := cmdtest.Execute(buildFullRoot(t),
		"generate-skills", "--filter", "automate", "--output-dir", dir); err != nil {
		t.Fatalf("generate-skills failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "umbraco-automate", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected automate skill in default generation: %v", err)
	}
	for _, want := range []string{"automation list", "automation validate", "catalogue operators", "workspace group add", "version-history rollback"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated automate skill missing %q:\n%s", want, string(content)[:500])
		}
	}
}

// buildFullRoot builds the whole production command tree, core included.
func buildFullRoot(t *testing.T) *cobra.Command {
	t.Helper()
	return cmdtest.BuildRoot(t, cmdtest.MakeDeps(), commands.RegisterAll)
}
