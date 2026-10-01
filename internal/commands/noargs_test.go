package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdtest"
)

// Every leaf command whose Use declares no positional arguments must reject
// stray positionals: silently ignoring one turned "bin delete <id>" typos
// into a full-bin wipe (#41). This invariant keeps the whole tree honest.
func TestLeafCommandsWithoutPositionalsRejectStrayArgs(t *testing.T) {
	root := buildRootWithCollections(t, cmdtest.MakeDeps())

	var violations []string
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		for _, child := range cmd.Commands() {
			name := child.Name()
			if name == "help" || name == "completion" {
				continue
			}
			childPath := strings.TrimSpace(path + " " + name)
			if len(child.Commands()) > 0 {
				walk(child, childPath)
				continue
			}
			// Only placeholders BEFORE the first flag token are positionals;
			// "<...>"/"[...]" after "--flag" are flag metavars ("validate
			// --file <export.json>") and declare nothing positional.
			use := child.Use
			if idx := strings.Index(use, " --"); idx >= 0 {
				use = use[:idx]
			}
			if strings.ContainsAny(use, "<[") {
				continue // declares positionals; arity is its own business
			}
			if err := child.ValidateArgs([]string{"stray-positional"}); err == nil {
				violations = append(violations, childPath)
			}
		}
	}
	walk(root, "")

	if len(violations) > 0 {
		t.Fatalf("%d commands declare no positionals but accept stray args (add Args: cobra.NoArgs):\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
