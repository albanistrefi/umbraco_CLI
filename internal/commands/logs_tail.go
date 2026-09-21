package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

// tailPageSize bounds each request; tailMaxPagesPerPoll bounds how far one
// poll pages back through a burst before giving up on draining it.
//
// Field report (0.4.17): tail printed nothing, forever, on a busy site. The
// log-viewer's startDate/endDate select which daily log *files* are read;
// they do not filter entries by timestamp (verified on 18.1: startDate one
// hour in the future still returns today's entries, and an ascending query
// with startDate=now returns the day's oldest entries). So an ascending page
// was always the first 500 entries of the day, every one older than the
// cursor, and a full page triggered an immediate re-poll of the same page:
// zero output and a tight request loop. Polls therefore descend from the
// newest entry and page back with skip until an entry at or before the
// cursor appears; the timestamp filter is client-side.
const (
	tailPageSize        = 500
	tailMaxPagesPerPoll = 20
)

func logsTail(deps Dependencies) *cobra.Command {
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
				parsed, err := parseLogTime(since)
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
				_, err := fmt.Fprintf(out, "%s [%s] %s\n", stringValue(entry["timestamp"]), stringValue(entry["level"]), message)
				return err
			}

			ctx := cmd.Context()
			errOut := cmd.ErrOrStderr()
			seen := map[string]struct{}{}
			var deadline time.Time
			if forDuration > 0 {
				deadline = time.Now().Add(forDuration)
			}
			fmt.Fprintf(errOut, "tailing log entries after %s (polling every %s; entries print on stdout, status on stderr)\n", cursor.Format(time.RFC3339), interval)
			printed := 0
			lastHeartbeat := time.Now()
			var newestSeen time.Time

			for {
				if !deadline.IsZero() && time.Now().After(deadline) {
					return nil
				}
				fresh, newest, err := tailPoll(ctx, deps.Client, baseParams, cursor)
				if err != nil {
					// An interrupt mid-request is a clean stop, not a failure.
					if ctx.Err() != nil {
						return nil
					}
					if errors.Is(err, errTailBacklogTooLarge) {
						return fmt.Errorf("more than %d entries have arrived since %s; tail replays at most that many per poll and will not skip silently — start from a later --since, or read the backlog with 'umbraco logs search --from %s'", tailPageSize*tailMaxPagesPerPoll, cursor.Format(time.RFC3339), cursor.Format(time.RFC3339))
					}
					return friendlyLogViewerError(err)
				}
				if newest.After(newestSeen) {
					newestSeen = newest
				}

				nextCursor := cursor
				unseen := 0
				for _, stamped := range fresh {
					key := stableJSON(stamped.entry)
					if _, duplicate := seen[key]; !duplicate {
						unseen++
						if logEntryMatches(stamped.entry, runtime) {
							if err := printEntry(stamped.entry); err != nil {
								return err
							}
							printed++
						}
					}
					if stamped.ts.After(nextCursor) {
						nextCursor = stamped.ts
					}
				}

				// Entries stamped exactly at the cursor are fetched again on
				// the next poll (the cut is "not before cursor"); remember
				// their identities so each entry prints exactly once.
				if len(fresh) > 0 {
					nextSeen := map[string]struct{}{}
					for _, stamped := range fresh {
						if stamped.ts.Equal(nextCursor) {
							nextSeen[stableJSON(stamped.entry)] = struct{}{}
						}
					}
					seen = nextSeen
					cursor = nextCursor
				}
				if unseen > 0 {
					lastHeartbeat = time.Now()
				} else if heartbeat > 0 && time.Since(lastHeartbeat) >= heartbeat {
					newestText := "none in the current log file"
					if !newestSeen.IsZero() {
						newestText = newestSeen.Format(time.RFC3339)
					}
					fmt.Fprintf(errOut, "no new entries since %s (%d printed so far; newest entry on the server: %s)\n", cursor.Format(time.RFC3339), printed, newestText)
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

// tailEntry is a fetched log entry with its parsed timestamp.
type tailEntry struct {
	entry map[string]any
	ts    time.Time
}

// errTailBacklogTooLarge is returned when a poll pages through the cap
// without reaching the cursor: advancing past what was fetched would skip
// the unread remainder for good, so the run stops and says so instead.
var errTailBacklogTooLarge = errors.New("tail backlog exceeds the per-poll page cap")

// tailPoll fetches every entry stamped at or after cursor, oldest first,
// paging newest-first through the log-viewer with skip until it meets an
// entry older than the cursor or an incomplete page. Hitting the page cap
// first is errTailBacklogTooLarge. It also reports the newest timestamp it
// saw, so a heartbeat can say whether the server has anything at all.
func tailPoll(ctx context.Context, client *api.Client, baseParams map[string]any, cursor time.Time) ([]tailEntry, time.Time, error) {
	fresh := make([]tailEntry, 0)
	var newest time.Time
	for page := 0; page < tailMaxPagesPerPoll; page++ {
		params := copyAnyMap(baseParams)
		// startDate narrows the set of daily log files the server reads;
		// it does not cut entries, hence the client-side check below.
		params["startDate"] = cursor.Format(time.RFC3339)
		params["skip"] = page * tailPageSize
		params["take"] = tailPageSize
		params["orderDirection"] = "Descending"
		result, err := getWithFallback(ctx, client,
			getRequestCandidate{path: logViewerLogPath, opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: logViewerLegacyListPath, opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return nil, newest, err
		}
		items := resultItems(result)
		reachedCursor := false
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ts, ok := logEntryTimestamp(entry)
			if !ok {
				continue
			}
			if ts.After(newest) {
				newest = ts
			}
			if ts.Before(cursor) {
				reachedCursor = true
				continue
			}
			fresh = append(fresh, tailEntry{entry: entry, ts: ts})
		}
		if reachedCursor || len(items) < tailPageSize {
			break
		}
		if page == tailMaxPagesPerPoll-1 {
			return nil, newest, errTailBacklogTooLarge
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].ts.Before(fresh[j].ts) })
	return fresh, newest, nil
}
