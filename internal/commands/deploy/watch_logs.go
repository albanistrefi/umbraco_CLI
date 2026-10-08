package deploy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/jsonvalue"
)

// The log monitor reports the log entries a deploy produces next to the
// phase transitions, so nobody runs a second poller beside the watch. It
// reads through cmdkit.LogTail (the 'logs tail' poller) and only emits; it
// never feeds the phase machine and never fails the command.

// watchLogFlags are the --logs* options of deploy watch.
type watchLogFlags struct {
	enabled  bool
	matches  []string
	excludes []string
	keep     []string
	level    string
}

func addWatchLogFlags(cmd *cobra.Command, flags *watchLogFlags) {
	cmd.Flags().BoolVar(&flags.enabled, "logs", false, "Also emit the log entries a deploy produces (type \"log\"): app start/stop, migrations and upgrades, Examine indexer suspend/resume/rebuild, Umbraco Deploy entries, and anything at --logs-level or above; known noise is excluded (see --logs-keep). They never change phases or the exit code, and stop with the watch")
	cmd.Flags().StringArrayVar(&flags.matches, "logs-match", nil, "Also emit entries whose message, source context or exception contains this text, case-insensitive (repeatable; category \"match\")")
	cmd.Flags().StringArrayVar(&flags.excludes, "logs-exclude", nil, "Drop entries whose message, source context or exception contains this text, case-insensitive (repeatable), on top of the built-in exclusions")
	cmd.Flags().StringArrayVar(&flags.keep, "logs-keep", nil, "Turn off a built-in exclusion so its entries are emitted: "+strings.Join(builtinLogExclusionNames(), ", ")+", or all (repeatable)")
	cmd.Flags().StringVar(&flags.level, "logs-level", "Error", "Lowest level emitted whatever the category (Verbose, Debug, Information, Warning, Error, Fatal)")
}

// validate checks the --logs* flags before the watch arms.
func (f watchLogFlags) validate(cmd *cobra.Command) error {
	if !f.enabled {
		for _, name := range []string{"logs-match", "logs-exclude", "logs-keep", "logs-level"} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("--%s requires --logs", name)
			}
		}
		return nil
	}
	if _, ok := logLevelRank(f.level); !ok {
		return fmt.Errorf("invalid --logs-level %q (expected Verbose, Debug, Information, Warning, Error or Fatal)", f.level)
	}
	known := map[string]bool{"all": true}
	for _, name := range builtinLogExclusionNames() {
		known[name] = true
	}
	for _, name := range f.keep {
		if !known[strings.ToLower(strings.TrimSpace(name))] {
			return fmt.Errorf("unknown --logs-keep %q (expected %s, or all)", name, strings.Join(builtinLogExclusionNames(), ", "))
		}
	}
	return nil
}

// logEntryView is the text of one entry that matchers read. Message comes
// from cmdkit.LogEntryMessage, so it is the same whichever field the server
// put the text in.
type logEntryView struct {
	level     string
	source    string
	message   string
	exception string
}

func (v logEntryView) contains(needle string) bool {
	needle = strings.ToLower(needle)
	return strings.Contains(strings.ToLower(v.message), needle) ||
		strings.Contains(strings.ToLower(v.source), needle) ||
		strings.Contains(strings.ToLower(v.exception), needle)
}

func newLogEntryView(entry map[string]any) logEntryView {
	view := logEntryView{
		level:     jsonvalue.String(entry["level"]),
		message:   cmdkit.LogEntryMessage(entry),
		exception: jsonvalue.String(entry["exception"]),
	}
	if properties, ok := entry["properties"].([]any); ok {
		for _, item := range properties {
			property, ok := item.(map[string]any)
			if ok && jsonvalue.String(property["name"]) == "SourceContext" {
				view.source = jsonvalue.String(property["value"])
			}
		}
	}
	return view
}

// logExclusion drops known chronic noise.
type logExclusion struct {
	name    string
	matches func(logEntryView) bool
}

var builtinLogExclusions = []logExclusion{
	// Logged at Information about every 6s on live before 01-10 by
	// DeliveryApiContentIndexPopulator; 135 times on 01-10 alone.
	{name: "delivery-api-disabled", matches: func(v logEntryView) bool {
		return strings.Contains(v.message, "The Delivery API is not enabled, no indexing will performed for the Delivery API content index")
	}},
	// The readiness probe failing while migrations run, logged at Error by
	// DefaultHealthCheckService: Health check "umbraco-ready" with status
	// "Unhealthy" ... "Umbraco is not yet ready. Level: Upgrading".
	{name: "ready-probe-upgrading", matches: func(v logEntryView) bool {
		lower := strings.ToLower(v.message)
		return strings.Contains(lower, "umbraco-ready") && strings.Contains(lower, "unhealthy") && strings.Contains(lower, "level: upgrading")
	}},
	// Automate's workflow lock racing on startup: SQL 2627, "Violation of
	// PRIMARY KEY constraint 'PK_umbracoAutomateWorkflowLock'", which may be
	// in the message or only in the exception.
	{name: "automate-workflow-lock-pk", matches: func(v logEntryView) bool {
		return v.contains("PK_umbracoAutomateWorkflowLock")
	}},
}

func builtinLogExclusionNames() []string {
	names := make([]string, 0, len(builtinLogExclusions))
	for _, exclusion := range builtinLogExclusions {
		names = append(names, exclusion.name)
	}
	return names
}

var logLevels = []string{"verbose", "debug", "information", "warning", "error", "fatal"}

func logLevelRank(level string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(level))
	switch normalized {
	case "info":
		normalized = "information"
	case "critical":
		normalized = "fatal"
	}
	for rank, name := range logLevels {
		if name == normalized {
			return rank, true
		}
	}
	return 0, false
}

// Log categories, in the order they are tried.
const (
	logCategoryLifecycle = "lifecycle"
	logCategoryMigration = "migration"
	logCategoryIndexer   = "indexer"
	logCategoryDeploy    = "deploy"
	logCategoryMatch     = "match"
	logCategoryLevel     = "level"
)

func hasPrefixFold(value string, prefix string) bool {
	return len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)
}

// categorize names the category an entry is emitted under, or "" to skip
// it. The rules follow what Umbraco 18.2 logged during the 01-10 live
// deploy.
func (m *logMonitor) categorize(v logEntryView) string {
	lower := strings.ToLower(v.message)
	switch {
	// MainDom: Acquiring/Acquired MainDom, Stopping/Released; the generic
	// host's "Application started"/"Application is shutting down".
	case hasPrefixFold(v.source, "Umbraco.Cms.Core.Runtime."), strings.EqualFold(v.source, "Microsoft.Hosting.Lifetime"):
		return logCategoryLifecycle
	// MigrationPlanExecutor, MigrationContext, UnattendedUpgrader,
	// MigrationCoordinator, and ProfilingLogger's "package migration" timings.
	case hasPrefixFold(v.source, "Umbraco.Cms.Infrastructure.Migrations."), hasPrefixFold(v.source, "Umbraco.Cms.Infrastructure.Install."),
		strings.Contains(lower, "package migration"):
		return logCategoryMigration
	// Deploy's "Suspend indexers." / "Resume indexers (rebuild:True)." and
	// Examine's own suspend, resume and rebuild entries.
	case (strings.Contains(lower, "index") || strings.Contains(strings.ToLower(v.source), "examine")) &&
		(strings.Contains(lower, "suspend") || strings.Contains(lower, "resum") || strings.Contains(lower, "rebuild")):
		return logCategoryIndexer
	// Umbraco.Deploy.* sources, plus the suspend/resume steps Deploy's work
	// items log under a bare "object" source context ("Resume document
	// cache", "Resume scheduled publishing").
	case hasPrefixFold(v.source, "Umbraco.Deploy."),
		strings.EqualFold(v.source, "object") && (strings.HasPrefix(v.message, "Suspend ") || strings.HasPrefix(v.message, "Resume ")):
		return logCategoryDeploy
	}
	for _, needle := range m.matches {
		if v.contains(needle) {
			return logCategoryMatch
		}
	}
	if rank, ok := logLevelRank(v.level); ok && rank >= m.minLevel {
		return logCategoryLevel
	}
	return ""
}

// watchLogEvent is one emitted log entry. Its type is "log" and it has no
// "phase" key, so a consumer reading phases cannot mistake it for one.
type watchLogEvent struct {
	Timestamp     string `json:"timestamp"`
	Type          string `json:"type"`
	Category      string `json:"category"`
	Level         string `json:"level"`
	SourceContext string `json:"sourceContext,omitempty"`
	Message       string `json:"message"`
	Exception     string `json:"exception,omitempty"`
}

// watchLogMonitorEvent reports on the monitor itself: unavailable/resumed
// around a restart, a gap when a burst outran the poller, and stopped with
// the totals when the watch ends.
type watchLogMonitorEvent struct {
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// watchLogFeed reads the log viewer once per tick for every consumer (the
// --logs monitor, the --uda-dir schema tracker) and reports its own state.
type watchLogFeed struct {
	tail        *cmdkit.LogTail
	read        int
	unavailable bool
	gaps        int
}

// newWatchLogFeed starts reading after the newest entry at baseline, by the
// server's clock, so local clock skew cannot drop or replay entries. When
// the baseline had no timestamp it starts from now.
func newWatchLogFeed(ctx context.Context, client *api.Client, since time.Time) *watchLogFeed {
	if since.IsZero() {
		since = time.Now().UTC()
	}
	feed := &watchLogFeed{tail: cmdkit.NewLogTail(client, nil, since)}
	// The entries stamped at the baseline instant are already history;
	// reading them once marks them seen.
	_, _ = feed.tail.Next(ctx)
	return feed
}

// fetch reads the entries since the previous read. It touches only the
// tail, which one goroutine owns, so it runs without the watch's lock.
func (f *watchLogFeed) fetch(ctx context.Context) ([]map[string]any, error) {
	return f.tail.Next(ctx)
}

// record books one fetch, stamped when it started, and returns its entries
// and any monitor events about the feed itself. Failures are reported as
// events, never returned.
func (f *watchLogFeed) record(ctx context.Context, now string, entries []map[string]any, err error) ([]map[string]any, []any) {
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil
		}
		if errors.Is(err, cmdkit.ErrLogTailBacklogTooLarge) {
			from := f.tail.Cursor()
			to := f.tail.NewestSeen()
			f.tail.SkipTo(to)
			f.gaps++
			return nil, []any{watchLogMonitorEvent{Timestamp: now, Type: "log-monitor", Status: "gap", Detail: map[string]any{
				"reason": fmt.Sprintf("more than %d entries arrived in one poll; the entries between from and to were not read", cmdkit.LogTailPageSize*cmdkit.LogTailMaxPagesPerPoll),
				"from":   from.UTC().Format(time.RFC3339Nano),
				"to":     to.UTC().Format(time.RFC3339Nano),
			}}}
		}
		if f.unavailable {
			return nil, nil
		}
		f.unavailable = true
		return nil, []any{watchLogMonitorEvent{Timestamp: now, Type: "log-monitor", Status: "unavailable", Detail: map[string]any{
			"reason": api.SanitizeTerminalText(err.Error()),
			"note":   "expected while the app restarts; entries are read on recovery",
		}}}
	}
	f.read += len(entries)
	if f.unavailable {
		f.unavailable = false
		return entries, []any{watchLogMonitorEvent{Timestamp: now, Type: "log-monitor", Status: "resumed"}}
	}
	return entries, nil
}

type logMonitor struct {
	exclusions []logExclusion
	matches    []string
	minLevel   int

	emitted  map[string]int
	excluded map[string]int
}

func newLogMonitor(flags watchLogFlags) *logMonitor {
	minLevel, _ := logLevelRank(flags.level)
	keepAll := false
	keep := map[string]bool{}
	for _, name := range flags.keep {
		name = strings.ToLower(strings.TrimSpace(name))
		keep[name] = true
		keepAll = keepAll || name == "all"
	}
	exclusions := make([]logExclusion, 0, len(builtinLogExclusions)+len(flags.excludes))
	for _, exclusion := range builtinLogExclusions {
		if !keepAll && !keep[exclusion.name] {
			exclusions = append(exclusions, exclusion)
		}
	}
	for _, text := range flags.excludes {
		needle := strings.TrimSpace(text)
		if needle == "" {
			continue
		}
		exclusions = append(exclusions, logExclusion{name: "custom:" + needle, matches: func(v logEntryView) bool { return v.contains(needle) }})
	}
	return &logMonitor{
		exclusions: exclusions,
		matches:    flags.matches,
		minLevel:   minLevel,
		emitted:    map[string]int{},
		excluded:   map[string]int{},
	}
}

func (m *logMonitor) exclusionNames() []string {
	names := make([]string, 0, len(m.exclusions))
	for _, exclusion := range m.exclusions {
		names = append(names, exclusion.name)
	}
	return names
}

// consume turns a tick's entries into log events.
func (m *logMonitor) consume(entries []map[string]any) []any {
	events := make([]any, 0)
	for _, entry := range entries {
		view := newLogEntryView(entry)
		if name := m.excludedBy(view); name != "" {
			m.excluded[name]++
			continue
		}
		category := m.categorize(view)
		if category == "" {
			continue
		}
		m.emitted[category]++
		events = append(events, m.event(entry, view, category))
	}
	return events
}

func (m *logMonitor) excludedBy(view logEntryView) string {
	for _, exclusion := range m.exclusions {
		if exclusion.matches(view) {
			return exclusion.name
		}
	}
	return ""
}

// event shapes an entry for output: the message and the exception's first
// line, with e-mail addresses, tokens and secrets masked.
func (m *logMonitor) event(entry map[string]any, view logEntryView, category string) watchLogEvent {
	exception, _, _ := strings.Cut(view.exception, "\n")
	return watchLogEvent{
		Timestamp:     jsonvalue.String(entry["timestamp"]),
		Type:          "log",
		Category:      category,
		Level:         view.level,
		SourceContext: view.source,
		Message:       cmdkit.RedactLogText(view.message, true, true, true),
		Exception:     strings.TrimSpace(cmdkit.RedactLogText(exception, true, true, true)),
	}
}

func (m *logMonitor) total() int {
	total := 0
	for _, count := range m.emitted {
		total += count
	}
	return total
}

// stopped is the final monitor event: what was read, emitted and excluded.
func (m *logMonitor) stopped(phase string, feed *watchLogFeed) watchLogMonitorEvent {
	excluded := map[string]any{}
	for _, name := range sortedCountKeys(m.excluded) {
		excluded[name] = m.excluded[name]
	}
	emitted := map[string]any{}
	for _, name := range sortedCountKeys(m.emitted) {
		emitted[name] = m.emitted[name]
	}
	return watchLogMonitorEvent{Timestamp: time.Now().UTC().Format(time.RFC3339), Type: "log-monitor", Status: "stopped", Detail: map[string]any{
		"phase":    phase,
		"read":     feed.read,
		"emitted":  emitted,
		"excluded": excluded,
		"gaps":     feed.gaps,
		"through":  feed.tail.Cursor().UTC().Format(time.RFC3339Nano),
	}}
}

func sortedCountKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
