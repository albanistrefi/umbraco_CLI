package engage

import (
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

// buildEngageRoot builds a root carrying only the engage group.
func buildEngageRoot(t *testing.T, deps cmdkit.Dependencies) *cobra.Command {
	t.Helper()
	return cmdtest.BuildRoot(t, deps, Register)
}
