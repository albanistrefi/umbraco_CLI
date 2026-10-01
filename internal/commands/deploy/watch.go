package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// deployWatchFailedError maps the watch's failed terminal phase to exit
// code 5 so CI can gate on it.
type deployWatchFailedError struct{ reason string }

func (e deployWatchFailedError) Error() string { return "deploy watch failed: " + e.reason }
func (deployWatchFailedError) ExitCode() int   { return 5 }

// deployWatchTimeoutError maps the watch's timeout terminal phase to exit
// code 6. Timeout is an honest "status unknown", never an inferred success
// or failure.
type deployWatchTimeoutError struct{ reason string }

func (e deployWatchTimeoutError) Error() string { return "deploy watch timeout: " + e.reason }
func (deployWatchTimeoutError) ExitCode() int   { return 6 }

func deployWatch(deps cmdkit.Dependencies) *cobra.Command {
	var healthPaths []string
	var publicURL string
	var interval time.Duration
	var timeout time.Duration
	var escalation time.Duration
	var settle time.Duration
	var heartbeat time.Duration
	var jsonOut bool
	var skipIndexVerify bool
	var logFlags watchLogFlags

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch an environment for the effects of a deployment and report phase transitions",
		Long: `Observes the target environment for state deltas only a deployment can cause — no pipeline or portal API involved, so it works identically on Umbraco Cloud and on-prem, and it is strictly read-only.

Signals: the newest log entry's ProcessId/MachineName (an app recycle means the deploy landed), the management token endpoint probed unauthenticated (503/unreachable = down; 401 = app alive and rejecting the probe — the earliest all-clear, typically ~15s before public pages return), configured health paths on the public host, and Examine index health (deploys can trigger full index rebuilds, during which search is empty — "deploy succeeded" and "the site works" are different questions).

Phases: baseline → restarting → app-alive → serving → landed → settling → verified | failed | timeout. Everything is baselined before arming — a signal already true on the target is not a signal. Verified requires the environment to stay healthy for a full --settle window after everything first looks good: deployment pipelines can disturb the environment AFTER the app is already serving (observed in production: Umbraco Deploy wiped every Examine index 27 seconds after a single-sample check had passed, leaving search empty for 17 minutes), so a single passing sample is not verification. An interrupted settle (index rebuild, health flap) is emitted as settle-interrupted and the window restarts once the environment recovers. Transitions are emitted with timestamps as they are observed (fast recycles may skip phases); silence between transitions means "still in the current phase", and --heartbeat writes a periodic still-alive line to stderr so silence is never ambiguous. Success is never inferred from silence: reaching --timeout without verification exits 6 (status unknown), and sustained downtime or post-landing health failure beyond --escalation exits 5.

Every --json line has a "type": "phase" for the transitions above. --logs adds the deploy's log entries to the same stream, read with the 'logs tail' poller from the newest entry at baseline: "log" lines (timestamp, category, level, sourceContext, message, exception's first line; e-mail addresses, tokens and secrets masked) for app start/stop (lifecycle), migrations and upgrades (migration), Examine indexer suspend/resume/rebuild (indexer), Umbraco Deploy entries (deploy), --logs-match hits (match) and anything at --logs-level or above (level, default Error); and "log-monitor" lines when the log viewer is unavailable (expected during the restart) or resumes, when a burst outran the poller (gap, with the unread window), and once when the watch ends (stopped, with counts of what was read, emitted and excluded). Known chronic noise is dropped before categories apply: delivery-api-disabled (the Delivery API index populator's "not enabled" line), ready-probe-upgrading (the umbraco-ready health check failing with "Level: Upgrading" while migrations run) and automate-workflow-lock-pk (SQL 2627 on PK_umbracoAutomateWorkflowLock); --logs-keep turns one off, --logs-exclude adds more. Log lines never change phases or the exit code, and the monitor stops with the watch.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return fmt.Errorf("--interval must be greater than zero")
			}
			if err := logFlags.validate(cmd); err != nil {
				return err
			}
			base := strings.TrimRight(deps.CurrentConfig().BaseURL, "/")
			public := base
			if strings.TrimSpace(publicURL) != "" {
				public = strings.TrimRight(publicURL, "/")
			}
			if len(healthPaths) == 0 {
				healthPaths = []string{"/"}
			}

			if settle < 0 {
				return fmt.Errorf("--settle cannot be negative")
			}
			probes := &watchProbes{
				deps:        deps,
				cfg:         deps.CurrentConfig(),
				httpClient:  watchHTTPClient(deps),
				tokenURL:    base + "/umbraco/management/api/v1/security/back-office/token",
				publicURL:   public,
				healthPaths: healthPaths,
				skipIndexes: skipIndexVerify,
			}

			ctx := cmd.Context()
			baseline := probes.observe(ctx)
			machine, err := newWatchMachine(baseline, escalation, settle, skipIndexVerify)
			if err != nil {
				return err
			}
			emit := watchEmitter(cmd.OutOrStdout(), jsonOut)
			baselineDetail := map[string]any{
				"processId":      baseline.ProcessID,
				"machineName":    baseline.MachineName,
				"healthyPaths":   machine.baselineHealthy,
				"unhealthyPaths": unhealthyPathNames(baseline.Health),
				"ignoredIndexes": cmdkit.SortedKeys(machine.baselineBadIndexes),
			}
			var logs *logMonitor
			if logFlags.enabled {
				logs = newLogMonitor(ctx, deps.Client, baseline.NewestLogAt, logFlags)
				baselineDetail["logs"] = map[string]any{"excluding": logs.exclusionNames(), "level": logFlags.level}
			}
			emit(watchEvent{
				Timestamp: baseline.At.UTC().Format(time.RFC3339),
				Phase:     "baseline",
				Detail:    baselineDetail,
			})
			// Log events of a tick go out before its phase transitions (the
			// entries happened first). At a terminal state the monitor drains
			// once more and stops with the watch.
			drainLogs := func() {
				if logs == nil {
					return
				}
				for _, event := range logs.poll(ctx) {
					emit(event)
				}
			}
			stopLogs := func() {
				if logs == nil {
					return
				}
				emit(logs.stopped(machine.phase))
			}

			started := time.Now()
			deadline := started.Add(timeout)
			lastHeartbeat := started
			timeoutErr := func() error {
				drainLogs()
				reason := fmt.Sprintf("no verification within %s (last phase: %s) — deployment status unknown", timeout, machine.phase)
				emit(watchEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Phase: "timeout", Detail: map[string]any{"reason": reason}})
				stopLogs()
				return deployWatchTimeoutError{reason: reason}
			}
			for {
				// The sleep never overshoots the deadline, so --timeout is
				// honored even when --interval is longer or probes are slow.
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return timeoutErr()
				}
				sleep := interval
				if remaining < sleep {
					sleep = remaining
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(sleep):
				}

				observation := probes.observe(ctx)
				events, terminal := machine.observe(observation)
				drainLogs()
				for _, event := range events {
					emit(event)
				}
				switch terminal {
				case watchOutcomeVerified:
					stopLogs()
					return nil
				case watchOutcomeFailed:
					stopLogs()
					return deployWatchFailedError{reason: machine.failureReason}
				}

				if time.Now().After(deadline) {
					return timeoutErr()
				}
				if heartbeat > 0 && time.Since(lastHeartbeat) >= heartbeat {
					logStatus := ""
					if logs != nil {
						logStatus = fmt.Sprintf(", logs: %d read, %d emitted", logs.seen, logs.total())
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "%s still watching — phase %s, elapsed %s%s\n", time.Now().UTC().Format(time.RFC3339), machine.phase, time.Since(started).Round(time.Second), logStatus)
					lastHeartbeat = time.Now()
				}
			}
		},
	}

	cmd.Flags().StringArrayVar(&healthPaths, "health-path", nil, "Public path that must return 2xx for the serving/verified phases (repeatable; default /). A redirect to a login page counts as unhealthy: on a basic-auth protected environment (Umbraco Cloud non-live) set basicAuthSharedSecret in the profile")
	cmd.Flags().StringVar(&publicURL, "public-url", "", "Public host for health paths when it differs from the management base URL")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "Poll interval")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Give up after this long without verification (exit 6, status unknown)")
	cmd.Flags().DurationVar(&escalation, "escalation", 10*time.Minute, "Treat sustained downtime or post-landing health failure longer than this as failed (exit 5)")
	cmd.Flags().DurationVar(&settle, "settle", 90*time.Second, "How long the environment must stay healthy after everything first looks good before verified is emitted; 0 disables (single-sample verification)")
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", time.Minute, "Interval for still-alive lines on stderr; 0 disables")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit phase transitions as NDJSON")
	cmd.Flags().BoolVar(&skipIndexVerify, "skip-index-verify", false, "Do not require Examine indexes to be healthy for the verified phase")
	addWatchLogFlags(cmd, &logFlags)
	return cmd
}

// watchEvent is one emitted phase transition, type "phase".
type watchEvent struct {
	Timestamp string         `json:"timestamp"`
	Phase     string         `json:"phase"`
	Type      string         `json:"type"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// watchEmitter writes phase transitions, log entries and log-monitor events
// to one stream: NDJSON with a "type" on every line, or one text line each.
func watchEmitter(out io.Writer, jsonOut bool) func(any) {
	encoder := json.NewEncoder(out)
	return func(value any) {
		if event, ok := value.(watchEvent); ok && event.Type == "" {
			event.Type = "phase"
			value = event
		}
		if jsonOut {
			_ = encoder.Encode(value)
			return
		}
		switch event := value.(type) {
		case watchEvent:
			fmt.Fprintf(out, "%s %s%s\n", event.Timestamp, event.Phase, formatWatchDetail(event.Detail))
		case watchLogEvent:
			source := ""
			if event.SourceContext != "" {
				source = " " + event.SourceContext + ":"
			}
			exception := ""
			if event.Exception != "" {
				exception = " — " + event.Exception
			}
			fmt.Fprintf(out, "%s log [%s] %s%s %s%s\n", event.Timestamp, event.Category, event.Level, source, api.SanitizeTerminalText(event.Message), api.SanitizeTerminalText(exception))
		case watchLogMonitorEvent:
			fmt.Fprintf(out, "%s log-monitor %s%s\n", event.Timestamp, event.Status, formatWatchDetail(event.Detail))
		}
	}
}

func formatWatchDetail(detail map[string]any) string {
	if len(detail) == 0 {
		return ""
	}
	keys := make([]string, 0, len(detail))
	for key := range detail {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(detail))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, detail[key]))
	}
	return " — " + strings.Join(parts, " ")
}
