package cmdtest

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

type exitErr struct{ code int }

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return e.code }

func TestHarnessRoundTrip(t *testing.T) {
	var seen []string
	deps := Deps(func(req *http.Request) (*http.Response, error) {
		return TokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			seen = append(seen, req.URL.Path)
			if req.URL.Path == "/umbraco/management/api/v1/empty" {
				return NoContent(), nil
			}
			AssertQueryValue(t, req.URL.Query(), "q", "1")
			return JSONResponse(http.StatusOK, `{"ok":true}`), nil
		})
	})
	if deps.Config.BaseURL == "" || deps.HTTPClient == nil || ClientDeps(nil).HTTPClient != nil || MakeDeps().Client == nil {
		t.Fatal("unexpected Dependencies wiring")
	}
	group := func(root *cobra.Command, deps cmdkit.Dependencies) {
		root.AddCommand(&cobra.Command{Use: "ping", RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/ping", api.RequestOptions{Params: map[string]any{"q": "1"}})
			if err != nil {
				return err
			}
			if _, err := deps.Client.Get(cmd.Context(), "/empty", api.RequestOptions{}); err != nil {
				return err
			}
			cmd.PrintErrln("note")
			return cmdkit.PrintResult(cmd, deps, result)
		}})
	}
	root := BuildRoot(t, deps, group)
	if FindChildCommand(root, "ping") == nil || FindChildCommand(root, "nope") != nil {
		t.Fatal("expected the registered group on the root")
	}
	out, err := Execute(root, "ping", "--output", "json")
	if err != nil || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("execute: %q %v", out, err)
	}
	out, errOut, err := ExecuteWithErr(root, "ping")
	if err != nil || !strings.Contains(out, "ok") || !strings.Contains(errOut, "note") {
		t.Fatalf("execute with stderr: %q %q %v", out, errOut, err)
	}
	if len(seen) != 4 {
		t.Fatalf("expected two requests per run, got %v", seen)
	}
	if ExitCode(exitErr{code: 4}) != 4 || ExitCode(errors.New("plain")) != 1 {
		t.Fatal("unexpected exit codes")
	}
}
