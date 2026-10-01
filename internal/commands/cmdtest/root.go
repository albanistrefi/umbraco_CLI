package cmdtest

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
)

// Register attaches one or more command groups to a root; commands.RegisterAll
// and each add-on's Register fit it.
type Register func(root *cobra.Command, deps cmdkit.Dependencies)

// BuildRoot returns a bare "umbraco" root with the persistent --output flag
// bound to deps.OutputFlag and every given group registered. Core tests pass
// commands.RegisterAll for the full tree; an add-on's tests pass its own
// Register.
func BuildRoot(t *testing.T, deps cmdkit.Dependencies, register ...Register) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	for _, r := range register {
		r(root, deps)
	}
	return root
}

// Execute runs root with args and returns stdout; stderr is discarded.
func Execute(root *cobra.Command, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// ExecuteWithErr runs root with args and returns stdout and stderr.
func ExecuteWithErr(root *cobra.Command, args ...string) (string, string, error) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// FindChildCommand returns root's direct child called name, or nil.
func FindChildCommand(root *cobra.Command, name string) *cobra.Command {
	for _, command := range root.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return nil
}

// ExitCode reports the exit code an error carries (1 when it carries none).
func ExitCode(err error) int {
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// AssertQueryValue fails the test unless values[key] is exactly [expected].
func AssertQueryValue(t *testing.T, values map[string][]string, key string, expected string) {
	t.Helper()
	actual := values[key]
	if len(actual) != 1 || actual[0] != expected {
		t.Fatalf("expected query %s=%q, got %+v", key, expected, actual)
	}
}
