package forms

import (
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

// buildFormsRoot builds a root carrying only the forms group.
func buildFormsRoot(t *testing.T, deps cmdkit.Dependencies) *cobra.Command {
	t.Helper()
	return cmdtest.BuildRoot(t, deps, Register)
}
