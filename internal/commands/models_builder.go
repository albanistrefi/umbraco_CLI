package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// modelsBuilderSourceModes covers the ModelsMode values for which a build
// actually produces source files on disk. For InMemory or Nothing modes a
// build request would be a no-op at best and confusing at worst, so we
// pre-check and report rather than POST blindly.
var modelsBuilderSourceModes = map[string]bool{
	"SourceCodeManual": true,
	"SourceCodeAuto":   true,
}

func RegisterModelsBuilder(root *cobra.Command, deps Dependencies) {
	mb := &cobra.Command{
		Use:     "models-builder",
		Aliases: []string{"modelsbuilder", "models"},
		Short:   "Trigger and inspect ModelsBuilder source generation",
		Long:    "Wraps /umbraco/management/api/v1/models-builder/*. Useful after CLI schema changes (doctype create, etc.) when ModelsMode is SourceCodeManual — the backoffice 'Generate models' button would otherwise be the only trigger.",
	}
	mb.AddCommand(modelsBuilderDashboard(deps))
	mb.AddCommand(modelsBuilderStatus(deps))
	mb.AddCommand(modelsBuilderBuild(deps))
	root.AddCommand(mb)
}

func modelsBuilderDashboard(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Get dashboard: mode, modelsNamespace, outOfDate flag, last error",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(context.Background(), "/models-builder/dashboard", api.RequestOptions{Fields: fields})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func modelsBuilderStatus(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Get out-of-date status: Current | OutOfDate | Unknown",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(context.Background(), "/models-builder/status", api.RequestOptions{})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	return cmd
}

func modelsBuilderBuild(deps Dependencies) *cobra.Command {
	var wait bool
	var timeout time.Duration
	var pollInterval time.Duration
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Trigger source generation (SourceCodeManual / SourceCodeAuto only)",
		Long:  "POSTs to /models-builder/build. Pre-checks the dashboard mode so non-source-generating modes (InMemory, Nothing) fail with a clear message instead of an opaque server error. With --wait, polls status until Current or --timeout elapses. --dry-run runs the dashboard/mode pre-checks and returns the planned POST without triggering generation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun && wait {
				return fmt.Errorf("--dry-run does not trigger a build, so --wait has nothing to poll for; pass one or the other")
			}

			ctx := context.Background()

			dashboard, err := deps.Client.Get(ctx, "/models-builder/dashboard", api.RequestOptions{})
			if err != nil {
				return fmt.Errorf("could not fetch ModelsBuilder dashboard before build: %w", err)
			}
			mode := modelsBuilderModeFrom(dashboard)
			if mode == "" {
				return fmt.Errorf("could not determine ModelsMode from dashboard response; cannot decide whether build is meaningful")
			}
			if !modelsBuilderSourceModes[mode] {
				return fmt.Errorf("ModelsMode is %q; build only generates source files for SourceCodeManual or SourceCodeAuto. Change ModelsBuilder:ModelsMode in appsettings to enable source generation", mode)
			}
			if canGen, ok := dashboardBool(dashboard, "canGenerate"); ok && !canGen {
				lastErr := dashboardString(dashboard, "lastError")
				if lastErr != "" {
					return fmt.Errorf("ModelsBuilder reports canGenerate=false (lastError: %s)", lastErr)
				}
				return fmt.Errorf("ModelsBuilder reports canGenerate=false; check the dashboard")
			}

			result, err := deps.Client.Post(ctx, "/models-builder/build", map[string]any{}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			if !wait {
				return printResult(cmd, deps, result)
			}

			deadline := time.Now().Add(timeout)
			for {
				statusPayload, err := deps.Client.Get(ctx, "/models-builder/status", api.RequestOptions{})
				if err != nil {
					return fmt.Errorf("polling status after build failed: %w", err)
				}
				status := modelsBuilderStatusString(statusPayload)
				if strings.EqualFold(status, "Current") {
					return printResult(cmd, deps, map[string]any{
						"build":  result,
						"status": status,
						"waited": time.Since(deadline.Add(-timeout)).String(),
					})
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("models-builder did not reach Current within %s (last status: %s); try increasing --timeout or check the dashboard for errors", timeout, status)
				}
				time.Sleep(pollInterval)
			}
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll status after triggering the build until it reports Current or --timeout elapses")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "How long to wait when --wait is set (e.g. 30s, 2m)")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", time.Second, "How often to poll status when --wait is set")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run dashboard/mode pre-checks and return the planned POST without triggering generation; incompatible with --wait")
	return cmd
}

func modelsBuilderModeFrom(dashboard any) string {
	envelope, ok := dashboard.(map[string]any)
	if !ok {
		return ""
	}
	mode, _ := envelope["mode"].(string)
	return mode
}

func modelsBuilderStatusString(payload any) string {
	envelope, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	status, _ := envelope["status"].(string)
	return status
}

func dashboardBool(payload any, key string) (bool, bool) {
	envelope, ok := payload.(map[string]any)
	if !ok {
		return false, false
	}
	v, exists := envelope[key]
	if !exists {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func dashboardString(payload any, key string) string {
	envelope, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := envelope[key].(string)
	return s
}
