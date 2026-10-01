package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// groupAnnotation marks a command group that requireSubcommands made
// runnable only to reject a missing subcommand.
const groupAnnotation = "umbraco-cli/requires-subcommand"

// requireSubcommands makes every command group without an action of its own
// a usage error (exit 1) when it is invoked without a known subcommand.
// Cobra prints a non-runnable command's help and returns nil, so a bare
// "umbraco health" exited 0 and a script read it as a passing check. Groups
// that do run something themselves (schema) are left alone, and --help is
// answered by cobra before RunE, so it still exits 0.
func requireSubcommands(root *cobra.Command) {
	markGroups(root)
	// A runnable command's usage opens with "<path> [flags]"; for these
	// groups that line would advertise the bare invocation being rejected.
	root.SetUsageTemplate(strings.Replace(root.UsageTemplate(),
		"{{if .Runnable}}",
		`{{if and .Runnable (not (index .Annotations "`+groupAnnotation+`"))}}`, 1))
}

func markGroups(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		markGroups(child)
	}
	if !cmd.HasSubCommands() || cmd.Runnable() {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[groupAnnotation] = "true"
	cmd.RunE = runGroupWithoutSubcommand
}

func runGroupWithoutSubcommand(cmd *cobra.Command, args []string) error {
	path := cmd.CommandPath()
	if len(args) > 0 {
		suggestion := ""
		if cmd.SuggestionsMinimumDistance <= 0 {
			cmd.SuggestionsMinimumDistance = 2 // cobra's own default for root
		}
		if matches := cmd.SuggestionsFor(args[0]); len(matches) > 0 {
			suggestion = fmt.Sprintf(" (did you mean %q?)", matches[0])
		}
		return fmt.Errorf("unknown command %q for %q%s; run '%s --help' for usage", args[0], path, suggestion, path)
	}

	names := make([]string, 0, len(cmd.Commands()))
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			names = append(names, child.Name())
		}
	}
	return fmt.Errorf("%s requires a subcommand (%s); run '%s --help' for usage", path, strings.Join(names, ", "), path)
}
