package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"UMBRACO_BASE_URL", "UMBRACO_CLIENT_ID", "UMBRACO_CLIENT_SECRET", "UMBRACO_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}
}

func executeRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCommand()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// groupPaths lists every command that has subcommands, as argument paths.
func groupPaths(t *testing.T) (guarded [][]string, runnable [][]string) {
	t.Helper()
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd.HasSubCommands() {
			if cmd.Annotations[groupAnnotation] != "" {
				guarded = append(guarded, path)
			} else {
				runnable = append(runnable, path)
			}
		}
		for _, child := range cmd.Commands() {
			walk(child, append(append([]string{}, path...), child.Name()))
		}
	}
	walk(NewRootCommand(), nil)
	return guarded, runnable
}

func TestEveryBareCommandGroupIsAUsageError(t *testing.T) {
	isolateConfig(t)
	guarded, _ := groupPaths(t)
	if len(guarded) < 50 {
		t.Fatalf("expected the whole tree's groups to be guarded, got %d", len(guarded))
	}

	for _, path := range guarded {
		name := strings.TrimSpace("umbraco " + strings.Join(path, " "))
		stdout, _, err := executeRoot(t, path...)
		if err == nil {
			t.Errorf("%s: bare group succeeded; want a usage error", name)
			continue
		}
		if code := ExitCode(err); code != 1 {
			t.Errorf("%s: exit %d, want 1", name, code)
		}
		if !strings.Contains(err.Error(), name+" requires a subcommand") {
			t.Errorf("%s: unexpected error %q", name, err)
		}
		if stdout != "" {
			t.Errorf("%s: wrote to stdout on a usage error: %q", name, stdout)
		}
	}
}

func TestCommandGroupHelpStillSucceeds(t *testing.T) {
	isolateConfig(t)
	guarded, _ := groupPaths(t)
	for _, path := range guarded {
		name := strings.TrimSpace("umbraco " + strings.Join(path, " "))
		stdout, _, err := executeRoot(t, append(append([]string{}, path...), "--help")...)
		if err != nil {
			t.Errorf("%s --help: %v", name, err)
			continue
		}
		if !strings.Contains(stdout, "Available Commands:") {
			t.Errorf("%s --help: no command list in %q", name, stdout)
		}
		if strings.Contains(stdout, "\n  "+name+" [flags]\n") {
			t.Errorf("%s --help advertises the bare invocation:\n%s", name, stdout)
		}
	}
}

func TestBareHealthGroupFailsWithProfileFlag(t *testing.T) {
	isolateConfig(t)
	_, _, err := executeRoot(t, "health", "--output", "json")
	if ExitCode(err) != 1 {
		t.Fatalf("umbraco health: exit %d (%v), want 1", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "(action, group, groups, run)") {
		t.Fatalf("error does not list the subcommands: %v", err)
	}
}

func TestUnknownSubcommandOfGroupIsAUsageError(t *testing.T) {
	isolateConfig(t)
	_, _, err := executeRoot(t, "health", "rnu")
	if ExitCode(err) != 1 {
		t.Fatalf("exit %d (%v), want 1", ExitCode(err), err)
	}
	want := `unknown command "rnu" for "umbraco health" (did you mean "run"?)`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}

	_, _, err = executeRoot(t, "deploy", "queue", "zzz")
	if ExitCode(err) != 1 || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("unexpected result for an unmatched name: exit %d, %v", ExitCode(err), err)
	}
}

func TestRunnableGroupKeepsItsOwnAction(t *testing.T) {
	isolateConfig(t)
	_, runnable := groupPaths(t)
	found := false
	for _, path := range runnable {
		if strings.Join(path, " ") == "schema" {
			found = true
		}
	}
	if !found {
		t.Fatalf("schema should stay a runnable group, not guarded; runnable groups: %v", runnable)
	}
	stdout, _, err := executeRoot(t, "schema", "--list")
	if err != nil || stdout == "" {
		t.Fatalf("schema --list: err=%v stdout=%q", err, stdout)
	}
	help, _, err := executeRoot(t, "schema", "--help")
	if err != nil || !strings.Contains(help, "umbraco schema [") {
		t.Fatalf("schema --help lost its usage line: err=%v\n%s", err, help)
	}
}

func TestCompletionGroupIsGuarded(t *testing.T) {
	isolateConfig(t)
	_, _, err := executeRoot(t, "completion")
	if ExitCode(err) != 1 {
		t.Fatalf("umbraco completion: exit %d (%v), want 1", ExitCode(err), err)
	}
	// The script itself goes to the process stdout: cobra binds the
	// completion commands' writer when they are created.
	if _, _, err := executeRoot(t, "completion", "zsh"); err != nil {
		t.Fatalf("completion zsh broke: %v", err)
	}
}
