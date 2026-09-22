package commands

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The README advertises a command count per collection plus two totals. Both
// are derived from the real command tree here so they cannot drift:
//
//   - a per-group count is the number of visible direct subcommands of that
//     root command. Hidden commands and cobra's auto-added "help"/"completion"
//     are excluded because they are not part of the documented surface. A root
//     command that is itself a leaf, such as `api`, counts as 1.
//   - the totals are the number of runnable leaf commands in the whole tree:
//     every command with a Run/RunE at any depth, so nested subgroups such as
//     `document version`, `document bin`, `relation type`, `datatype block` and
//     the `automate` subgroups are all included. That is what the README means
//     by "counting every nested subcommand".
//
// Groups documented on a shared line (`doctype` (17) / `mediatype` (10) / ...)
// are each matched individually.

var readmeGroupCountPattern = regexp.MustCompile("`([a-z][a-z0-9-]*)` \\((\\d+)")

func isDocumentedCommand(cmd *cobra.Command) bool {
	if cmd.Hidden {
		return false
	}
	switch cmd.Name() {
	case "help", "completion":
		return false
	}
	return true
}

func visibleDirectCommandCount(cmd *cobra.Command) int {
	count := 0
	for _, child := range cmd.Commands() {
		if isDocumentedCommand(child) {
			count++
		}
	}
	if count == 0 && cmd.Runnable() {
		return 1
	}
	return count
}

func countRunnableLeaves(cmd *cobra.Command) int {
	total := 0
	for _, child := range cmd.Commands() {
		if !isDocumentedCommand(child) {
			continue
		}
		if child.Runnable() {
			total++
		}
		total += countRunnableLeaves(child)
	}
	return total
}

func readmeCollectionsSection(t *testing.T, readme string) string {
	t.Helper()
	const heading = "\n## Collections\n"
	start := strings.Index(readme, heading)
	if start < 0 {
		t.Fatalf("README.md has no %q section", "## Collections")
	}
	rest := readme[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}

func TestREADMECommandCountsMatchRegisteredCommands(t *testing.T) {
	root := buildRootWithCollections(t, makeDeps())

	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	readme := string(raw)

	groups := map[string]*cobra.Command{}
	for _, cmd := range root.Commands() {
		if isDocumentedCommand(cmd) {
			groups[cmd.Name()] = cmd
		}
	}

	documented := map[string]bool{}
	for _, line := range strings.Split(readmeCollectionsSection(t, readme), "\n") {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		for _, match := range readmeGroupCountPattern.FindAllStringSubmatch(line, -1) {
			name := match[1]
			cmd, ok := groups[name]
			if !ok {
				t.Errorf("README documents `%s` but no such command is registered on the root", name)
				continue
			}
			documented[name] = true
			stated, convErr := strconv.Atoi(match[2])
			if convErr != nil {
				t.Fatalf("unparsable count for `%s`: %v", name, convErr)
			}
			if actual := visibleDirectCommandCount(cmd); stated != actual {
				t.Errorf("README says `%s` (%d), but %d commands are registered; expected `%s` (%d)",
					name, stated, actual, name, actual)
			}
		}
	}

	// Every registered group has to be documented somewhere in the README. Most
	// belong in the Collections list; a few (`schema`, `generate-skills`) have a
	// section of their own instead, so a mention anywhere is enough here.
	var missing []string
	for name := range groups {
		if documented[name] {
			continue
		}
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(readme) {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("command groups missing from the README: %s", strings.Join(missing, ", "))
	}

	total := countRunnableLeaves(root)
	for _, want := range []string{
		fmt.Sprintf("operations — %d commands", total),
		fmt.Sprintf("Total: **%d runnable commands**", total),
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README total is out of date; expected it to contain %q (%d runnable leaf commands)", want, total)
		}
	}
}
