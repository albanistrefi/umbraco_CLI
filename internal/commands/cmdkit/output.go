package cmdkit

import (
	"sort"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/output"
)

// PrintResult writes data to the command's stdout in the requested output
// format (--output, else the configured default).
func PrintResult(cmd *cobra.Command, deps Dependencies, data any) error {
	return output.Print(data, deps.RequestedOutput(), deps.CurrentEnvOutput(), cmd.OutOrStdout())
}

// PrintMutationResult prints the outcome of a mutating command. Umbraco
// answers 204 No Content for most successful mutations; printing the raw
// nil surfaced as `null`, which scripts could not tell apart from failure.
// A real (non-dry-run) empty success becomes {"<verb>": true} instead.
// Dry-run plans pass through verbatim.
func PrintMutationResult(cmd *cobra.Command, deps Dependencies, verb string, result any, dryRun bool) error {
	if !dryRun && result == nil {
		return PrintResult(cmd, deps, map[string]any{verb: true})
	}
	return PrintResult(cmd, deps, result)
}

// SortedKeys lists a string set in order, as an empty rather than nil slice
// so it prints as [] in JSON output.
func SortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
