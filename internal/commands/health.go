package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/jsonvalue"
)

func RegisterHealth(root *cobra.Command, deps cmdkit.Dependencies) {
	health := &cobra.Command{Use: "health", Short: "Health check operations"}
	health.AddCommand(healthGroups(deps))
	health.AddCommand(healthGroup(deps))
	health.AddCommand(healthRun(deps))
	health.AddCommand(healthAction(deps))
	root.AddCommand(health)
}

func healthGroups(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "groups", Short: "List health check groups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), "/health-check-group", api.RequestOptions{})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func healthGroup(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "group <name>", Short: "Get health check group details", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/health-check-group/%s", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
}

func healthRun(deps cmdkit.Dependencies) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "run <group-name>",
		Short: "Run health checks for group",
		Long:  "Runs one health check group, or with --all every group GET /health-check-group lists, one after another, printing {groups: [{name, checks} or {name, error}], summary: {groups, ran, failed, results: {<resultType>: count}}}. A group that fails to run is reported in place and the command exits 4 after printing; an authentication failure stops the run.",
		Args: func(cmd *cobra.Command, args []string) error {
			if all {
				if len(args) > 0 {
					return fmt.Errorf("pass a group name or --all, not both")
				}
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all {
				result, err := runHealthGroup(cmd.Context(), deps, args[0])
				if err != nil {
					return err
				}
				return cmdkit.PrintResult(cmd, deps, result)
			}
			return runAllHealthGroups(cmd, deps)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Run every health check group in one call, with a summary of result types")
	return cmd
}

func runHealthGroup(ctx context.Context, deps cmdkit.Dependencies, name string) (any, error) {
	result, err := deps.Client.Post(ctx, api.JoinPath("/health-check-group/%s/check", name), nil, api.RequestOptions{})
	if api.IsStatus(err, http.StatusNotFound) {
		// Older servers expose GET .../run instead of POST .../check.
		result, err = deps.Client.Get(ctx, api.JoinPath("/health-check-group/%s/run", name), api.RequestOptions{})
	}
	return result, err
}

// healthGroupsFailedError is the exit after printing when one or more
// groups could not be run: an API error (4), like a single failed run.
type healthGroupsFailedError struct{ failed, total int }

func (e healthGroupsFailedError) Error() string {
	return fmt.Sprintf("health run --all: %d of %d groups could not be run (see their error fields)", e.failed, e.total)
}
func (healthGroupsFailedError) ExitCode() int { return 4 }

func runAllHealthGroups(cmd *cobra.Command, deps cmdkit.Dependencies) error {
	ctx := cmd.Context()
	listed, err := deps.Client.Get(ctx, "/health-check-group", api.RequestOptions{})
	if err != nil {
		return err
	}
	names := make([]string, 0)
	for _, item := range cmdkit.ResultItems(listed) {
		if group, ok := item.(map[string]any); ok {
			if name := jsonvalue.String(group["name"]); name != "" {
				names = append(names, name)
			}
		}
	}

	groups := make([]any, 0, len(names))
	results := map[string]int{}
	failed := 0
	for _, name := range names {
		result, err := runHealthGroup(ctx, deps, name)
		if err != nil {
			var authErr interface{ ExitCode() int }
			if errors.As(err, &authErr) && authErr.ExitCode() == 3 {
				return err
			}
			failed++
			groups = append(groups, map[string]any{"name": name, "error": err.Error()})
			continue
		}
		object, _ := result.(map[string]any)
		checks, _ := object["checks"].([]any)
		for _, check := range checks {
			checkObject, _ := check.(map[string]any)
			checkResults, _ := checkObject["results"].([]any)
			for _, checkResult := range checkResults {
				resultObject, _ := checkResult.(map[string]any)
				if resultType := jsonvalue.String(resultObject["resultType"]); resultType != "" {
					results[resultType]++
				}
			}
		}
		groups = append(groups, map[string]any{"name": name, "checks": checks})
	}

	payload := map[string]any{
		"groups": groups,
		"summary": map[string]any{
			"groups":  len(names),
			"ran":     len(names) - failed,
			"failed":  failed,
			"results": results,
		},
	}
	if err := cmdkit.PrintResult(cmd, deps, payload); err != nil {
		return err
	}
	if failed > 0 {
		return healthGroupsFailedError{failed: failed, total: len(names)}
	}
	return nil
}

func healthAction(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "action <id>",
		Short: "Execute a health check action",
		Long:  "POST /health-check/execute-action. On current servers <id> is the health check id from 'health run' results and fills healthCheck.id in the body when --json omits it. On older servers (which 404 the modern route) <id> must be the legacy action id — it is forwarded to POST /health-check/{actionId} with the --json payload unchanged.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cmdkit.OptionalBody(jsonPayload)
			if err != nil {
				return err
			}
			// Modern servers take the full action model on /execute-action, with
			// the owning health check referenced in the body; the positional
			// argument fills that reference when --json doesn't carry one.
			modern := make(map[string]any, len(body)+2)
			for k, v := range body {
				modern[k] = v
			}
			if _, ok := modern["healthCheck"]; !ok {
				modern["healthCheck"] = map[string]any{"id": args[0]}
			}
			if _, ok := modern["valueRequired"]; !ok {
				modern["valueRequired"] = false
			}
			result, err := deps.Client.Post(cmd.Context(), "/health-check/execute-action", modern, api.RequestOptions{DryRun: dryRun})
			if api.IsStatus(err, http.StatusNotFound) {
				// Older servers address the action by id in the path instead.
				result, err = deps.Client.Post(cmd.Context(), api.JoinPath("/health-check/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
			}
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, result)
		}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Action payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the planned request without executing")
	return cmd
}
