package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
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
	var requestTimeoutFlag time.Duration
	var restartMinDown time.Duration
	var jsonOut bool
	var skipIndexVerify bool
	var logFlags watchLogFlags
	var udaDir string

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch an environment for the effects of a deployment and report phase transitions",
		Long: `Observes the target environment for state deltas only a deployment can cause — no pipeline or portal API involved, so it works identically on Umbraco Cloud and on-prem, and it is strictly read-only.

Signals: the newest log entry's ProcessId/MachineName (an app recycle means the deploy landed; the "landed" line says "basis": "process-id"), the management token endpoint probed unauthenticated (503/unreachable = down; 401 = app alive and rejecting the probe — the earliest all-clear, typically ~15s before public pages return), configured health paths on the public host, and Examine index health (deploys can trigger full index rebuilds, during which search is empty — "deploy succeeded" and "the site works" are different questions).

Umbraco 18.2's log viewer refuses every read (HTTP 400, CancelledByLogsSizeValidation) once the log files of the days a read covers exceed 100 MB, which a busy site can reach during the day. When that refusal is why the process id is missing (at baseline, or on the poll in question), the watch infers landing from a restart cycle instead: one outage of the management endpoint lasting at least --restart-min-down, then the health paths serving again. The outage is measured from its first to its last failed probe, at least two, so it is a lower bound on the downtime whatever the poll spacing (the 15-10 live restart was down 47s, about 40s of it between failed probes at the 5s interval); the default is 20s, or twice --interval when that is longer, and it must be shorter than --escalation. Since the first and last failed probes can each fall up to one --interval inside the outage, a restart must last --restart-min-down plus up to two poll gaps to be measured: with the default, intervals above about 11s may miss a 47s restart. The "landed" line then says "basis": "restart-cycle" with "processId": "unavailable", the reason and the outage (downSince, downFor, failedProbes). Shorter outages count as blips. Any read of the baseline process after the outage means it was no restart, even an entry from before the outage; an unreadable poll neither disproves nor confirms. Any other failure to read the process id, or an entry without one, waits for a readable process id as before. Residual risk on this basis: a restart that is not a deploy (a platform recycle) and lasts long enough also lands it; a deploy whose restart is too short to measure never lands (timeout, exit 6); and a real restart that leaves the old process's last line newest for one poll never lands on this basis either (timeout, exit 6) and needs a manual check: the watch fails safe, preferring status unknown to verifying an outage the process survived. If the log viewer already refuses at baseline, the watch arms with "landingBasis": "restart-cycle"; any other failure to read the newest log entry at baseline still refuses to arm. With --uda-dir and no baseline process id, schema pass detection is off (log entries from before the deploy cannot be told apart): verified then waits for every tracked artifact to be in sync, so a deploy that leaves one drifted times out (exit 6) instead of exiting 7.

Phases: baseline → restarting → app-alive → serving → landed → settling → verified | failed | timeout. Everything is baselined before arming — a signal already true on the target is not a signal. Verified requires the environment to stay healthy for a full --settle window after everything first looks good: deployment pipelines can disturb the environment AFTER the app is already serving (observed in production: Umbraco Deploy wiped every Examine index 27 seconds after a single-sample check had passed, leaving search empty for 17 minutes), so a single passing sample is not verification. An interrupted settle (index rebuild, health flap) is emitted as settle-interrupted and the window restarts once the environment recovers. Transitions are emitted with timestamps as they are observed (fast recycles may skip phases); silence between transitions means "still in the current phase", and --heartbeat writes a periodic still-alive line so silence is never ambiguous: text on stderr, and with --json also a "heartbeat" line on stdout ({timestamp, type, currentPhase, elapsed, lastPoll, logs: {read, emitted, status} with --logs, status being reading, unavailable or refused}; lastPoll is when the latest completed poll started). Heartbeats run on their own clock and the log monitor reads beside the probes, so neither waits on a slow request; each poll probes the management endpoint, then the health paths, the newest log entry and the indexes side by side, and every request gives up after --request-timeout (a health path that does not answer in time counts as failing for that poll). Success is never inferred from silence: reaching --timeout without verification exits 6 (status unknown), and sustained downtime or post-landing health failure beyond --escalation exits 5.

Every --json line has a "type": "phase" for the transitions above. --logs adds the deploy's log entries to the same stream as they are read, with the 'logs tail' poller from the newest entry at baseline: "log" lines (timestamp, category, level, sourceContext, message, exception's first line; e-mail addresses, tokens and secrets masked) for app start/stop (lifecycle), migrations and upgrades (migration), Examine indexer suspend/resume/rebuild (indexer), Umbraco Deploy entries (deploy), --logs-match hits (match) and anything at --logs-level or above (level, default Error); and "log-monitor" lines when the log viewer is unavailable (expected during the restart, or a rejected read) or resumes, when it refuses reads for size (refused: these reads start at the baseline, so it usually lasts the rest of the run; other failures meanwhile leave it refused; it is retried every 5 minutes and as soon as a poll reads the newest entry again, resumed follows if a read succeeds, and heartbeats say "logs: unavailable (size)"), when a burst outran the poller (gap, with the unread window), and once when the watch ends (stopped, with counts of what was read, emitted and excluded). Known chronic noise is dropped before categories apply: delivery-api-disabled (the Delivery API index populator's "not enabled" line), ready-probe-upgrading (the umbraco-ready health check failing with "Level: Upgrading" while migrations run) and automate-workflow-lock-pk (SQL 2627 on PK_umbracoAutomateWorkflowLock); --logs-keep turns one off, --logs-exclude adds more. Log lines never change phases or the exit code, and the monitor stops with the watch.

--uda-dir checks schema tripwires, using the same comparison as 'deploy status'. At baseline every artifact is compared: the drifted and missing-remote ones are what this deploy should change and are tracked; in-sync ones are counted; unknown and error ones (Automate artifacts where the Automate API is unreachable, as on non-live environments behind basic auth) are listed as unverifiable and never counted as in sync or failed. After landing the tracked artifacts are re-checked each poll, with a "schema" line per status change. Umbraco Deploy applies schema in its own pass after the app is serving, logging its start and its end (Work Status "Completed"), so an artifact still at baseline before then is expected; "schema-pass" lines report the pass starting and ending as read from the log (entries of the new process only), and "waiting" once when only the schema is holding verified back. Verified waits until every tracked artifact is in sync, or the pass has ended and a re-check after it has a definitive answer for each (an unreadable one gets three tries, then counts as unknown). If the pass end never shows up, verified never comes and the watch times out (exit 6): success is not inferred from silence. A "schema-summary" line closes the stream with confirmed, unconfirmed (still drifted or missing), unknown and unverifiable artifacts. When the environment is verified but tracked artifacts are still drifted or missing after the pass ended, or the pass ended with another work status than Completed, the watch exits 7, the documented code for drifted or missing entities (Deploy can skip artifacts without failing the pass).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return fmt.Errorf("--interval must be greater than zero")
			}
			if err := logFlags.validate(cmd); err != nil {
				return err
			}
			if udaDir != "" {
				// Local checks first: a wrong directory is a usage error
				// before any request is sent.
				artifacts, err := loadUdaArtifacts(udaDir, nil)
				if err != nil {
					return err
				}
				if len(artifacts) == 0 {
					return fmt.Errorf("no .uda artifacts found in %s (pass --uda-dir pointing at the site repo's umbraco/Deploy/Revision)", udaDir)
				}
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
			resolvedMinDown, err := watchRestartMinDownFor(restartMinDown, cmd.Flags().Changed("restart-min-down"), interval, escalation)
			if err != nil {
				return err
			}
			restartMinDown = resolvedMinDown
			requestTimeout, err := watchRequestTimeout(requestTimeoutFlag, interval, heartbeat)
			if err != nil {
				return err
			}
			// Every call after baseline gives up after requestTimeout as a
			// whole, token and rate-limit waits included; a token request
			// itself carries on and serves the next poll. The baseline keeps the longer bounds:
			// nothing is waiting on it, and a timed-out baseline refuses to
			// arm.
			loopDeps := deps
			loopDeps.Client = deps.Client.WithTimeout(requestTimeout)
			probes := &watchProbes{
				client:      loopDeps.Client,
				cfg:         deps.CurrentConfig(),
				httpClient:  watchHTTPClient(deps),
				timeout:     requestTimeout,
				tokenURL:    base + "/umbraco/management/api/v1/security/back-office/token",
				publicURL:   public,
				healthPaths: healthPaths,
				skipIndexes: skipIndexVerify,
			}
			baselineProbes := *probes
			baselineProbes.client = deps.Client
			baselineProbes.timeout = max(requestTimeout, watchBaselineRequestTimeout)

			ctx := cmd.Context()
			baseline := baselineProbes.observe(ctx, nil)
			machine, err := newWatchMachine(baseline, escalation, settle, skipIndexVerify)
			if err != nil {
				return err
			}
			machine.restartMinDown = restartMinDown
			baselineDetail := map[string]any{
				"processId":      baseline.ProcessID,
				"machineName":    baseline.MachineName,
				"landingBasis":   "process-id",
				"healthyPaths":   machine.baselineHealthy,
				"unhealthyPaths": unhealthyPathNames(baseline.Health),
				"ignoredIndexes": cmdkit.SortedKeys(machine.baselineBadIndexes),
			}
			if machine.processIDUnavailable != "" {
				baselineDetail["processId"] = "unavailable"
				baselineDetail["machineName"] = "unavailable"
				baselineDetail["landingBasis"] = "restart-cycle"
				baselineDetail["processIdReason"] = machine.processIDUnavailable
			}
			var schema *schemaTracker
			if udaDir != "" {
				schema, err = newSchemaTracker(ctx, deps, udaDir, baseline)
				if err != nil {
					return err
				}
				schema.deps = loopDeps
				baselineDetail["schema"] = schema.baselineDetail()
			}
			var feed *watchLogFeed
			var logs *logMonitor
			if logFlags.enabled || schema != nil {
				feed = newWatchLogFeed(ctx, loopDeps.Client, baseline.NewestLogAt)
			}
			var feedEvents []any
			if feed != nil {
				feedEvents = feed.initial
			}
			if logFlags.enabled {
				logs = newLogMonitor(logFlags)
				baselineDetail["logs"] = map[string]any{"excluding": logs.exclusionNames(), "level": logFlags.level}
			}
			// Output goes through per-stream writers, flushed when the watch
			// returns (after the workers below have stopped).
			stdout, stderr := newWatchStream(cmd.OutOrStdout()), newWatchStream(cmd.ErrOrStderr())
			defer stderr.Close()
			defer stdout.Close()
			run := &watchRun{
				write:    watchEmitter(stdout, jsonOut),
				errOut:   stderr,
				jsonOut:  jsonOut,
				baseline: baseline,
				machine:  machine,
				feed:     feed,
				logs:     logs,
				schema:   schema,
				lastPoll: baseline.At,

				logRefusedRetry: watchLogRefusedRetry,
			}
			run.emit(watchEvent{
				Timestamp: baseline.At.UTC().Format(time.RFC3339),
				Phase:     "baseline",
				Detail:    baselineDetail,
			})
			if logs != nil {
				run.emit(feedEvents...)
			}

			// The heartbeat, the log follower and the schema rechecker run
			// beside the probe loop until the terminal phase; finish then reads the feed once more
			// and closes the stream.
			run.started = time.Now()
			workers, stopWorkers := context.WithCancel(ctx)
			var wg sync.WaitGroup
			stop := func() {
				stopWorkers()
				wg.Wait()
			}
			defer stop()
			if heartbeat > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					run.heartbeats(workers, heartbeat)
				}()
			}
			if feed != nil {
				run.logWake = make(chan struct{}, 1)
				wg.Add(1)
				go func() {
					defer wg.Done()
					run.followLogs(workers, interval)
				}()
			}
			if schema != nil {
				run.recheckKick = make(chan struct{}, 1)
				wg.Add(1)
				go func() {
					defer wg.Done()
					run.followSchema(workers)
				}()
			}
			end := func() {
				stop()
				run.finish(ctx, 2*requestTimeout)
			}

			deadline := run.started.Add(timeout)
			for {
				// The sleep never overshoots the deadline, so --timeout is
				// honored even when --interval is longer.
				remaining := time.Until(deadline)
				if remaining <= 0 {
					reason := run.timedOut(timeout)
					end()
					return deployWatchTimeoutError{reason: reason}
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

				switch run.poll(ctx, probes) {
				case watchOutcomeVerified:
					end()
					if schema != nil {
						return schema.exitError()
					}
					return nil
				case watchOutcomeFailed:
					end()
					return deployWatchFailedError{reason: machine.failureReason}
				}
				if time.Now().After(deadline) {
					reason := run.timedOut(timeout)
					end()
					return deployWatchTimeoutError{reason: reason}
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
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", time.Minute, "Interval for still-alive lines: text on stderr, and with --json also a \"heartbeat\" NDJSON line on stdout; on their own clock, so they keep coming while a request is slow; 0 disables")
	cmd.Flags().DurationVar(&requestTimeoutFlag, "request-timeout", 0, "Give up on any one request after this long; that signal counts as unknown (a health path as failing) for the poll. Must be shorter than --interval and --heartbeat; default half of the shorter of the two, between 250ms and 15s (2.5s with the defaults)")
	cmd.Flags().DurationVar(&restartMinDown, "restart-min-down", watchRestartMinDown, "When the log viewer refuses to read the process id, one outage of the management endpoint must last at least this long to count as the restart that lands the deploy, measured from its first to its last failed probe (at least two), so it is a lower bound on the downtime: a restart must last this plus up to two --interval gaps to be measured. Shorter outages are blips. Default 20s, or twice --interval when that is longer; a value must be longer than --interval (or 0 to count any outage) and shorter than --escalation")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit phase transitions, heartbeats and the other lines as NDJSON")
	cmd.Flags().BoolVar(&skipIndexVerify, "skip-index-verify", false, "Do not require Examine indexes to be healthy for the verified phase")
	addWatchLogFlags(cmd, &logFlags)
	cmd.Flags().StringVar(&udaDir, "uda-dir", "", "Check schema tripwires: the .uda artifacts here (the site repo's umbraco/Deploy/Revision) that are drifted or missing on the target at baseline are re-checked after landing; verified waits until they are in sync or Umbraco Deploy's schema pass has ended, and exits 7 if any is still drifted or missing then")
	return cmd
}

// watchRestartMinDownFor resolves --restart-min-down: unset, it is
// watchRestartMinDown or twice the interval when that is longer. A positive
// value must be longer than the interval, since one failed probe proves no
// downtime, and shorter than the escalation, since an outage that long has
// already failed the watch.
func watchRestartMinDownFor(value time.Duration, explicit bool, interval time.Duration, escalation time.Duration) (time.Duration, error) {
	if value < 0 {
		return 0, fmt.Errorf("--restart-min-down cannot be negative")
	}
	if !explicit {
		value = max(watchRestartMinDown, 2*interval)
	} else if value != 0 && value <= interval {
		return 0, fmt.Errorf("--restart-min-down %s must be longer than --interval %s: it is measured between failed probes, so one failed probe can never prove it (or pass 0 to count any outage)", value, interval)
	}
	if value != 0 && value >= escalation {
		source := "--restart-min-down " + value.String()
		if !explicit {
			basis := "its 20s floor"
			if value > watchRestartMinDown {
				basis = "twice --interval"
			}
			source = "the default --restart-min-down " + value.String() + " (" + basis + ")"
		}
		return 0, fmt.Errorf("%s must be shorter than --escalation %s: an outage that long fails the watch (exit 5) before it can count as a restart; set a shorter --restart-min-down or a longer --escalation", source, escalation)
	}
	return value, nil
}

// watchRestartMinDown is the default minimum provable outage for a
// restart-cycle landing. The 15-10 live restart was down 47s, about 40s of
// it between failed probes at the 5s interval; a blip is one or two polls.
const watchRestartMinDown = 20 * time.Second

// watchBaselineRequestTimeout bounds each baseline request: nothing waits
// on the baseline, and one that times out refuses to arm.
const watchBaselineRequestTimeout = 15 * time.Second

const (
	watchRequestTimeoutFloor = 250 * time.Millisecond
	watchRequestTimeoutCap   = 15 * time.Second
)

// watchRequestTimeout is the bound on every request after baseline: the
// --request-timeout value, which must be shorter than the poll and
// heartbeat intervals, or by default half of the shorter of the two,
// clamped to [250ms, 15s].
func watchRequestTimeout(explicit time.Duration, interval time.Duration, heartbeat time.Duration) (time.Duration, error) {
	if explicit < 0 {
		return 0, fmt.Errorf("--request-timeout cannot be negative")
	}
	if explicit > 0 {
		if explicit >= interval {
			return 0, fmt.Errorf("--request-timeout %s must be shorter than --interval %s, so a slow request never delays the next poll (raise --interval for a slow site)", explicit, interval)
		}
		if heartbeat > 0 && explicit >= heartbeat {
			return 0, fmt.Errorf("--request-timeout %s must be shorter than --heartbeat %s", explicit, heartbeat)
		}
		return explicit, nil
	}
	timeout := interval / 2
	if heartbeat > 0 && heartbeat/2 < timeout {
		timeout = heartbeat / 2
	}
	return min(max(timeout, watchRequestTimeoutFloor), watchRequestTimeoutCap), nil
}

// watchEvent is one emitted phase transition, type "phase".
type watchEvent struct {
	Timestamp string         `json:"timestamp"`
	Phase     string         `json:"phase"`
	Type      string         `json:"type"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// watchEmitter writes phase transitions, log entries, log-monitor events
// and (with --json) heartbeats to one stream: NDJSON with a "type" on every
// line, or one text line each. Each line is one write to a watchStream,
// which passes it on unbuffered as soon as it is emitted. Callers serialize
// calls (watchRun.mu).
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
		case watchSchemaEvent:
			fmt.Fprintf(out, "%s schema %s %s → %s%s\n", event.Timestamp, event.File, event.Previous, event.Status, formatWatchDetail(map[string]any{"kind": event.Kind, "name": event.Name}))
		case watchSchemaPassEvent:
			fmt.Fprintf(out, "%s %s %s%s\n", event.Timestamp, event.Type, event.Status, formatWatchDetail(event.Detail))
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
