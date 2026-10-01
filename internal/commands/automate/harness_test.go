package automate

import (
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

// buildAutomateRoot builds a root carrying only the automate group.
func buildAutomateRoot(t *testing.T, deps cmdkit.Dependencies) *cobra.Command {
	t.Helper()
	return cmdtest.BuildRoot(t, deps, Register)
}
