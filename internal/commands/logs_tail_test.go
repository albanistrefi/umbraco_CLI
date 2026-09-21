package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

func buildLogsRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	if deps.OutputFlag != nil {
		root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
	}
	RegisterLogs(root, deps)
	return root
}

func logsTailDeps(handler func(poll int64, req *http.Request) (*http.Response, error)) Dependencies {
	var polls int64
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return handler(atomic.AddInt64(&polls, 1), req)
	})
}

func TestLogsTailPrintsEachEntryOnceAsNDJSON(t *testing.T) {
	var lastStartDate atomic.Value
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		lastStartDate.Store(req.URL.Query().Get("startDate"))
		if poll == 1 {
			return endpointJSONResponse(http.StatusOK, `{"items":[
				{"timestamp":"2026-07-03T10:00:01Z","level":"Information","renderedMessage":"first"},
				{"timestamp":"2026-07-03T10:00:02Z","level":"Information","renderedMessage":"second"}
			],"total":2}`), nil
		}
		// Later polls replay the boundary entry plus one new one.
		return endpointJSONResponse(http.StatusOK, `{"items":[
			{"timestamp":"2026-07-03T10:00:02Z","level":"Information","renderedMessage":"second"},
			{"timestamp":"2026-07-03T10:00:03Z","level":"Information","renderedMessage":"third"}
		],"total":2}`), nil
	})

	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--for", "40ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 entries printed once each, got %d lines:\n%s", len(lines), out)
	}
	for i, expected := range []string{"first", "second", "third"} {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &decoded); err != nil {
			t.Fatalf("line %d is not valid NDJSON: %v\n%s", i, err, lines[i])
		}
		if decoded["renderedMessage"] != expected {
			t.Fatalf("line %d: expected message %q, got %v", i, expected, decoded["renderedMessage"])
		}
	}
	if got := lastStartDate.Load().(string); got != "2026-07-03T10:00:03Z" {
		t.Fatalf("expected cursor to advance to the newest entry, got startDate %q", got)
	}
}

func TestLogsTailFiltersByLevelClientSide(t *testing.T) {
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[
			{"timestamp":"2026-07-03T10:00:01Z","level":"Information","renderedMessage":"noise"},
			{"timestamp":"2026-07-03T10:00:02Z","level":"Error","renderedMessage":"boom"}
		],"total":2}`), nil
	})

	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "2026-07-03T10:00:00Z", "--level", "Error", "--interval", "1ms", "--for", "20ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	if strings.Contains(out, "noise") {
		t.Fatalf("expected Information entry filtered out, got %s", out)
	}
	if strings.Count(out, "boom") != 1 {
		t.Fatalf("expected the Error entry exactly once, got %s", out)
	}
}

func TestLogsTailPlainOutputFormatsLines(t *testing.T) {
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[
			{"timestamp":"2026-07-03T10:00:01Z","level":"Warning","renderedMessage":"careful"}
		],"total":1}`), nil
	})

	out, err := execute(buildLogsRoot(deps), "logs", "tail", "-o", "plain", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--for", "20ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	if !strings.Contains(out, "2026-07-03T10:00:01Z [Warning] careful") {
		t.Fatalf("expected formatted plain line, got %s", out)
	}
}

func TestLogsTailRedactsSensitiveValues(t *testing.T) {
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[
			{"timestamp":"2026-07-03T10:00:01Z","level":"Information","renderedMessage":"login by user@example.test"}
		],"total":1}`), nil
	})

	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--redact-default", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--for", "20ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	if strings.Contains(out, "user@example.test") {
		t.Fatalf("expected email redacted, got %s", out)
	}
	if !strings.Contains(out, "login by") {
		t.Fatalf("expected entry still printed, got %s", out)
	}
}

func TestLogsTailRejectsInvalidSince(t *testing.T) {
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		t.Fatalf("no HTTP request expected for invalid --since")
		return nil, nil
	})

	if _, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "not-a-time", "--for", "10ms"); err == nil || !strings.Contains(err.Error(), "invalid --since") {
		t.Fatalf("expected invalid --since error, got %v", err)
	}
}

// logPage renders count entries stamped one second apart starting at
// 10:00:<start>, newest first when descending is set.
func logPage(start, count int, descending bool) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < count; i++ {
		n := start + i
		if descending {
			n = start + count - 1 - i
		}
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"timestamp":"2026-07-03T%02d:%02d:%02dZ","level":"Information","renderedMessage":"entry-%d"}`, 10+n/3600, (n/60)%60, n%60, n)
	}
	fmt.Fprintf(&b, `],"total":%d}`, count)
	return b.String()
}

func TestLogsTailPrintsNewEntriesWhenServerIgnoresStartDate(t *testing.T) {
	// Field report (0.4.17): the log-viewer's startDate selects daily log
	// files, not entries, so a page always includes the day's older entries.
	// The mock ignores startDate entirely and serves a day with 700 entries,
	// of which only the newest 5 are after --since. Newest-first paging must
	// print exactly those 5, stop at the first older entry, and not spin.
	var requests []string
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		requests = append(requests, q.Get("orderDirection")+"/skip="+q.Get("skip"))
		if q.Get("orderDirection") != "Descending" {
			return endpointJSONResponse(http.StatusOK, logPage(0, tailPageSize, false)), nil // the day's oldest 500: the bug
		}
		return endpointJSONResponse(http.StatusOK, logPage(200, tailPageSize, true)), nil // newest 500 of 700
	})

	// --since 10:11:35 = second 695; entries 695..699 are new.
	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "2026-07-03T10:11:35Z", "--interval", "1h", "--for", "50ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected the 5 entries after --since, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "entry-695") || !strings.Contains(lines[4], "entry-699") {
		t.Fatalf("expected entries 695..699 oldest first, got:\n%s", out)
	}
	if len(requests) != 1 || requests[0] != "Descending/skip=0" {
		t.Fatalf("expected one newest-first request (no re-poll loop despite the full page), got %v", requests)
	}
}

func TestLogsTailPagesBackThroughBurstsWithSkip(t *testing.T) {
	// 1200 new entries since the cursor: three descending pages (500, 500,
	// 200); the short third page ends the drain. Every entry prints once.
	var skips []string
	const burst = 1200
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		skips = append(skips, q.Get("skip"))
		skip, _ := strconv.Atoi(q.Get("skip"))
		remaining := burst - skip
		if remaining <= 0 {
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		}
		count := tailPageSize
		if remaining < count {
			count = remaining
		}
		// Entries 1..1200; page at skip covers the newest-first slice.
		start := burst - skip - count + 1
		return endpointJSONResponse(http.StatusOK, logPage(start, count, true)), nil
	})

	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "2026-07-03T10:00:01Z", "--interval", "1h", "--for", "50ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	if strings.Join(skips, ",") != "0,500,1000" {
		t.Fatalf("expected skip paging 0,500,1000 within one poll, got %v", skips)
	}
	if printed := strings.Count(out, "entry-"); printed != burst {
		t.Fatalf("expected %d unique entries printed, got %d", burst, printed)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], `"entry-1"`) || !strings.Contains(lines[burst-1], `"entry-1200"`) {
		t.Fatalf("expected oldest-first output across pages, got first=%s last=%s", lines[0], lines[burst-1])
	}
}

func TestLogsTailJSONFlagAndHeartbeat(t *testing.T) {
	jsonDeps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[{"timestamp":"2026-07-03T10:00:01Z","level":"Information","renderedMessage":"fresh"}],"total":1}`), nil
	})
	out, err := execute(buildLogsRoot(jsonDeps), "logs", "tail", "-o", "plain", "--json", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--for", "20ms")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(out), "{") || !strings.Contains(out, `"renderedMessage":"fresh"`) {
		t.Fatalf("expected --json to force NDJSON like deploy watch --json, got err=%v out=%s", err, out)
	}

	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusOK, `{"items":[{"timestamp":"2026-07-03T09:00:00Z","level":"Information","renderedMessage":"old"}],"total":1}`), nil
	})
	out, status, err := executeWithErr(buildLogsRoot(deps), "logs", "tail", "-o", "plain", "--json", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--heartbeat", "5ms", "--for", "60ms")
	if err != nil {
		t.Fatalf("logs tail failed: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected nothing on stdout (only an old entry exists), got %s", out)
	}
	if !strings.Contains(status, "tailing log entries after 2026-07-03T10:00:00Z") {
		t.Fatalf("expected a startup line on stderr, got %q", status)
	}
	if !strings.Contains(status, "no new entries since 2026-07-03T10:00:00Z (0 printed so far; newest entry on the server: 2026-07-03T09:00:00Z)") {
		t.Fatalf("expected a heartbeat naming the newest server entry, got %q", status)
	}
}

func TestLogsTailStopsInsteadOfSkippingWhenBacklogExceedsPageCap(t *testing.T) {
	// Every page is full and never reaches the cursor: advancing would skip
	// the unread remainder, so tail must stop with a clear error, print
	// nothing partial, and never make a second poll.
	polls := 0
	deps := logsTailDeps(func(poll int64, req *http.Request) (*http.Response, error) {
		polls++
		skip, _ := strconv.Atoi(req.URL.Query().Get("skip"))
		return endpointJSONResponse(http.StatusOK, logPage(20000-skip-tailPageSize+1, tailPageSize, true)), nil
	})
	out, err := execute(buildLogsRoot(deps), "logs", "tail", "--since", "2026-07-03T10:00:00Z", "--interval", "1ms", "--for", "1s")
	if err == nil || !strings.Contains(err.Error(), "more than 10000 entries have arrived since 2026-07-03T10:00:00Z") || !strings.Contains(err.Error(), "logs search --from") {
		t.Fatalf("expected a backlog error naming the remedy, got %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected no partial output, got %d bytes", len(out))
	}
	if polls != tailMaxPagesPerPoll {
		t.Fatalf("expected exactly one poll of %d pages, got %d requests", tailMaxPagesPerPoll, polls)
	}
}
