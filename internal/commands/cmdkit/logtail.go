package cmdkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/jsonvalue"
)

// LogViewerLegacyListPath is the log-viewer entries route before
// LogViewerLogPath; reads fall back to it.
const LogViewerLegacyListPath = "/log-viewer"

// LogTailPageSize bounds each request; LogTailMaxPagesPerPoll bounds how far
// one poll pages back through a burst before giving up on draining it.
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
	LogTailPageSize        = 500
	LogTailMaxPagesPerPoll = 20
)

// ErrLogTailBacklogTooLarge is returned when a poll pages through the cap
// without reaching the cursor: advancing past what was fetched would skip
// the unread remainder for good, so the caller must decide what to do.
var ErrLogTailBacklogTooLarge = errors.New("tail backlog exceeds the per-poll page cap")

// LogTail follows the log viewer from a timestamp cursor, returning each new
// entry exactly once. It is the one poller behind 'logs tail' and the
// 'deploy watch --logs' monitor.
type LogTail struct {
	client *api.Client
	params map[string]any
	cursor time.Time
	seen   map[string]struct{}
	newest time.Time
}

// NewLogTail starts a tail at since. params carries server-side filters
// (filterExpression, logLevel); paging and dates are set per request.
func NewLogTail(client *api.Client, params map[string]any, since time.Time) *LogTail {
	return &LogTail{client: client, params: params, cursor: since, seen: map[string]struct{}{}}
}

// Cursor is the timestamp the next poll reads from.
func (t *LogTail) Cursor() time.Time { return t.cursor }

// NewestSeen is the newest entry timestamp any poll has seen on the server.
func (t *LogTail) NewestSeen() time.Time { return t.newest }

// SkipTo moves the cursor forward to at, dropping anything before it. Used
// after ErrLogTailBacklogTooLarge by a caller that must keep going and
// reports the gap itself.
func (t *LogTail) SkipTo(at time.Time) {
	if at.After(t.cursor) {
		t.cursor = at
		t.seen = map[string]struct{}{}
	}
}

// Next returns the entries that arrived since the previous call, oldest
// first. Entries stamped exactly at the cursor are fetched again on the next
// poll (the cut is "not before cursor"), so their identities are remembered
// and each entry is returned once.
func (t *LogTail) Next(ctx context.Context) ([]map[string]any, error) {
	fresh, newest, err := t.poll(ctx)
	if newest.After(t.newest) {
		t.newest = newest
	}
	if err != nil {
		return nil, err
	}

	unseen := make([]map[string]any, 0, len(fresh))
	nextCursor := t.cursor
	for _, stamped := range fresh {
		if _, duplicate := t.seen[logEntryKey(stamped.entry)]; !duplicate {
			unseen = append(unseen, stamped.entry)
		}
		if stamped.ts.After(nextCursor) {
			nextCursor = stamped.ts
		}
	}
	if len(fresh) > 0 {
		nextSeen := map[string]struct{}{}
		for _, stamped := range fresh {
			if stamped.ts.Equal(nextCursor) {
				nextSeen[logEntryKey(stamped.entry)] = struct{}{}
			}
		}
		t.seen = nextSeen
		t.cursor = nextCursor
	}
	return unseen, nil
}

type tailEntry struct {
	entry map[string]any
	ts    time.Time
}

// poll fetches every entry stamped at or after the cursor, oldest first,
// paging newest-first through the log-viewer with skip until it meets an
// entry older than the cursor or an incomplete page. It also reports the
// newest timestamp it saw, so a heartbeat can say whether the server has
// anything at all.
func (t *LogTail) poll(ctx context.Context) ([]tailEntry, time.Time, error) {
	fresh := make([]tailEntry, 0)
	var newest time.Time
	for page := 0; page < LogTailMaxPagesPerPoll; page++ {
		params := maps.Clone(t.params)
		if params == nil {
			params = map[string]any{}
		}
		// startDate narrows the set of daily log files the server reads;
		// it does not cut entries, hence the client-side check below.
		params["startDate"] = t.cursor.Format(time.RFC3339)
		params["skip"] = page * LogTailPageSize
		params["take"] = LogTailPageSize
		params["orderDirection"] = "Descending"
		result, err := GetWithFallback(ctx, t.client,
			GetRequestCandidate{Path: LogViewerLogPath, Opts: api.RequestOptions{Params: params}},
			GetRequestCandidate{Path: LogViewerLegacyListPath, Opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return nil, newest, err
		}
		items := ResultItems(result)
		reachedCursor := false
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ts, ok := LogEntryTimestamp(entry)
			if !ok {
				continue
			}
			if ts.After(newest) {
				newest = ts
			}
			if ts.Before(t.cursor) {
				reachedCursor = true
				continue
			}
			fresh = append(fresh, tailEntry{entry: entry, ts: ts})
		}
		if reachedCursor || len(items) < LogTailPageSize {
			break
		}
		if page == LogTailMaxPagesPerPoll-1 {
			return nil, newest, ErrLogTailBacklogTooLarge
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].ts.Before(fresh[j].ts) })
	return fresh, newest, nil
}

func logEntryKey(entry map[string]any) string {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprint(entry)
	}
	return string(encoded)
}

// ParseLogTime parses a log timestamp or a --since/--from style value:
// RFC3339 (with or without fractional seconds) or YYYY-MM-DD.
func ParseLogTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("%q must be RFC3339 or YYYY-MM-DD", raw)
}

// LogEntryTimestamp reads a log-viewer entry's timestamp.
func LogEntryTimestamp(entry map[string]any) (time.Time, bool) {
	raw := jsonvalue.String(entry["timestamp"])
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := ParseLogTime(raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// LogEntryMessage is an entry's text as a reader sees it: the rendered
// message when the server sent one, else the template, else "message". The
// log-viewer's own entries carry renderedMessage while 'logs search --flat'
// output calls the same text "message"; matching on one field name broke a
// hand-built monitor, so every matcher reads through this.
func LogEntryMessage(entry map[string]any) string {
	for _, key := range []string{"renderedMessage", "messageTemplate", "message"} {
		if text := jsonvalue.String(entry[key]); text != "" {
			return text
		}
	}
	return ""
}

var (
	logEmailPattern            = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	logBearerTokenPattern      = regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/=-]+`)
	logSecretAssignmentPattern = regexp.MustCompile(`(?i)("?(?:access_token|refresh_token|id_token|client_secret|password|secret|api[_-]?key|authorization)"?\s*[:=]\s*"?)[^",}\s]+`)
)

// RedactLogText masks e-mail addresses, bearer tokens and secret
// assignments (password=…, "client_secret": …) in log text.
func RedactLogText(value string, emails, tokens, secrets bool) string {
	result := value
	if emails {
		result = logEmailPattern.ReplaceAllString(result, "[redacted-email]")
	}
	if tokens {
		result = logBearerTokenPattern.ReplaceAllString(result, "Bearer [redacted-token]")
	}
	if secrets || tokens {
		result = logSecretAssignmentPattern.ReplaceAllString(result, `${1}[redacted]`)
	}
	return result
}
