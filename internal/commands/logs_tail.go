package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/jsonvalue"
)

func logsTail(deps cmdkit.Dependencies) *cobra.Command {
	var flags logQueryFlags
	flags.skip = -1
	flags.take = -1
	var since string
	var interval time.Duration
	var forDuration time.Duration
	var heartbeat time.Duration
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Follow new log entries as they arrive (client-side polling)",
		Long: `The Management API has no streaming endpoint, so tail polls the log-viewer
newest-first, keeps the entries after its timestamp cursor (the server's
startDate/endDate pick daily log files, not entries, so the cut is made
client-side), pages back through bursts, deduplicates boundary entries, and
prints each new entry exactly once: NDJSON (one JSON object per line) for
json output (-o json or --json), one formatted line per entry otherwise.
A startup line and optional --heartbeat lines go to stderr so a quiet
environment is distinguishable from a tail that sees nothing. Runs until
interrupted or --for elapses; exits 0 on both.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseParams, runtime, err := logParamsFromFlags("", flags)
			if err != nil {
				return err
			}
			format, err := resolveOutputFormat(deps)
			if err != nil {
				return err
			}
			if jsonOut {
				format = config.OutputJSON
			}

			cursor := time.Now().UTC()
			if strings.TrimSpace(since) != "" {
				parsed, err := cmdkit.ParseLogTime(since)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				cursor = parsed.UTC()
			}

			out := cmd.OutOrStdout()
			printEntry := func(entry map[string]any) error {
				shaped := any(entry)
				if runtime.flat {
					shaped = flattenLogEntry(entry)
				}
				if runtime.redaction.enabled() {
					shaped = redactLogValue(shaped, runtime.redaction)
				}
				if format == config.OutputJSON {
					encoded, err := json.Marshal(shaped)
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(out, string(encoded))
					return err
				}
				message := firstLogString(entry["renderedMessage"], entry["messageTemplate"], entry["message"])
				if runtime.redaction.enabled() {
					message = redactLogString(message, runtime.redaction)
				}
				_, err := fmt.Fprintf(out, "%s [%s] %s\n", jsonvalue.String(entry["timestamp"]), jsonvalue.String(entry["level"]), message)
				return err
			}

			ctx := cmd.Context()
			errOut := cmd.ErrOrStderr()
			tail := cmdkit.NewLogTail(deps.Client, baseParams, cursor)
			var deadline time.Time
			if forDuration > 0 {
				deadline = time.Now().Add(forDuration)
			}
			fmt.Fprintf(errOut, "tailing log entries after %s (polling every %s; entries print on stdout, status on stderr)\n", cursor.Format(time.RFC3339), interval)
			printed := 0
			lastHeartbeat := time.Now()

			for {
				if !deadline.IsZero() && time.Now().After(deadline) {
					return nil
				}
				fresh, err := tail.Next(ctx)
				if err != nil {
					// An interrupt mid-request is a clean stop, not a failure.
					if ctx.Err() != nil {
						return nil
					}
					if errors.Is(err, cmdkit.ErrLogTailBacklogTooLarge) {
						from := tail.Cursor().Format(time.RFC3339)
						return fmt.Errorf("more than %d entries have arrived since %s; tail replays at most that many per poll and will not skip silently — start from a later --since, or read the backlog with 'umbraco logs search --from %s'", cmdkit.LogTailPageSize*cmdkit.LogTailMaxPagesPerPoll, from, from)
					}
					return friendlyLogViewerError(err)
				}

				for _, entry := range fresh {
					if logEntryMatches(entry, runtime) {
						if err := printEntry(entry); err != nil {
							return err
						}
						printed++
					}
				}
				if len(fresh) > 0 {
					lastHeartbeat = time.Now()
				} else if heartbeat > 0 && time.Since(lastHeartbeat) >= heartbeat {
					newestText := "none in the current log file"
					if newest := tail.NewestSeen(); !newest.IsZero() {
						newestText = newest.Format(time.RFC3339)
					}
					fmt.Fprintf(errOut, "no new entries since %s (%d printed so far; newest entry on the server: %s)\n", tail.Cursor().Format(time.RFC3339), printed, newestText)
					lastHeartbeat = time.Now()
				}

				// Never sleep past the --for deadline.
				wait := interval
				if !deadline.IsZero() {
					if remaining := time.Until(deadline); remaining < wait {
						wait = remaining
					}
				}
				if wait <= 0 {
					continue
				}
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(wait):
				}
			}
		},
	}

	cmd.Flags().StringVar(&flags.level, "level", "", "Only entries at this level (Verbose, Debug, Information, Warning, Error, Fatal)")
	cmd.Flags().StringVar(&flags.filterExpression, "filter-expression", "", "Serilog filter expression (server-side)")
	cmd.Flags().StringVar(&flags.sourceContext, "source-context", "", "Only entries whose SourceContext contains this value")
	cmd.Flags().StringVar(&flags.path, "path", "", "Only entries whose RequestPath contains this value")
	cmd.Flags().StringVar(&flags.contains, "contains", "", "Only entries containing this substring anywhere")
	cmd.Flags().StringVar(&flags.correlationID, "correlation-id", "", "Only entries with this correlation/request id")
	cmd.Flags().BoolVar(&flags.flat, "flat", false, "Flatten entries (timestamp, level, message, sourceContext, ...)")
	cmd.Flags().StringVar(&flags.redact, "redact", "", "Redact matching value kinds: emails,secrets,tokens (comma-separated)")
	cmd.Flags().BoolVar(&flags.redactDefault, "redact-default", false, "Apply the default redaction set (emails, secrets, tokens)")
	cmd.Flags().StringVar(&since, "since", "", "Start from this timestamp (RFC3339); default: now (only new entries)")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "Poll interval")
	cmd.Flags().DurationVar(&forDuration, "for", 0, "Stop after this duration (0 = run until interrupted); exits 0")
	cmd.Flags().DurationVar(&heartbeat, "heartbeat", 0, "Print a still-alive line on stderr after this long without new entries (0 = off)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit entries as NDJSON (same as -o json; matches 'deploy watch --json')")
	return cmd
}
