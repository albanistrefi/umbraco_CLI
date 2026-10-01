package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// logsErrors answers "is anything broken, and is any of it NEW?" after an
// incident or deployment. Without --distinct it is a flat Error+Fatal list;
// with --distinct entries collapse into fingerprint-grouped error classes so
// one chronic error repeating every few seconds reads as one class, not a
// wall of entries. Known-chronic classes are suppressed per invocation via
// --suppress/--suppress-contains — deliberately configuration, not code,
// because chronic noise differs per site and changes as bugs are fixed.
func logsErrors(deps cmdkit.Dependencies) *cobra.Command {
	var since string
	var until string
	var distinct bool
	var suppress []string
	var suppressContains []string
	var maxEntries int

	cmd := &cobra.Command{
		Use:   "errors",
		Short: "List Error/Fatal log entries, optionally grouped into distinct error classes",
		Long:  "GET /log-viewer/log filtered to Error and Fatal levels. --distinct groups entries into error classes by fingerprint (message template + normalized exception head, so two SQL violations with different constraint names are different classes) and reports count, first/last seen, and an example per class, sorted newest-first-seen so new breakage tops the list. --suppress drops known-chronic classes by fingerprint; --suppress-contains drops classes whose template or exception matches a substring. Suppressed classes are counted in the summary so they never vanish silently.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxEntries <= 0 {
				return fmt.Errorf("--max-entries must be greater than zero")
			}
			start := time.Now().UTC().Add(-24 * time.Hour)
			if strings.TrimSpace(since) != "" {
				parsed, err := cmdkit.ParseLogTime(since)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				start = parsed
			}
			// The end of the window is always pinned before pagination: this
			// endpoint returns newest-first and pages by skip offset, so
			// errors arriving between page requests would shift the
			// collection and produce duplicates and gaps against an open end.
			end := time.Now().UTC()
			if strings.TrimSpace(until) != "" {
				parsed, err := cmdkit.ParseLogTime(until)
				if err != nil {
					return fmt.Errorf("invalid --until: %w", err)
				}
				end = parsed
			}
			params := map[string]any{
				"logLevel":  []any{"Error", "Fatal"},
				"startDate": start.Format(time.RFC3339Nano),
				"endDate":   end.Format(time.RFC3339Nano),
			}

			result, err := cmdkit.GetAllPagesWithFallback(
				cmd.Context(),
				deps.Client,
				0, 0, maxEntries,
				cmdkit.GetRequestCandidate{Path: cmdkit.LogViewerLogPath, Opts: api.RequestOptions{Params: params}},
				cmdkit.GetRequestCandidate{Path: cmdkit.LogViewerLegacyListPath, Opts: api.RequestOptions{Params: params}},
			)
			if err != nil {
				return friendlyLogViewerError(err)
			}
			envelope, ok := result.(map[string]any)
			if !ok {
				return cmdkit.PrintResult(cmd, deps, result)
			}
			items, _ := envelope["items"].([]any)
			// The window is enforced client-side as well, matching logs
			// list/search: the legacy route ignores the date parameters.
			items = filterEntriesToWindow(items, start, end)
			if serverTotal, ok := envelope["total"].(float64); ok && int(serverTotal) > len(items) && len(items) >= maxEntries {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: hit --max-entries cap of %d with %d entries in the window; older entries were not scanned\n", maxEntries, int(serverTotal))
			}

			if !distinct {
				if len(suppress) > 0 || len(suppressContains) > 0 {
					return fmt.Errorf("--suppress and --suppress-contains require --distinct")
				}
				return cmdkit.PrintResult(cmd, deps, map[string]any{"items": items, "total": len(items)})
			}

			groups, suppressed := groupErrorClasses(items, suppress, suppressContains)
			return cmdkit.PrintResult(cmd, deps, map[string]any{
				"classes":          groups,
				"totalEntries":     len(items),
				"suppressedGroups": suppressed,
				"since":            start.Format(time.RFC3339),
				"until":            end.Format(time.RFC3339),
			})
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Start of the window (ISO/RFC3339); default: 24 hours ago")
	cmd.Flags().StringVar(&until, "until", "", "End of the window (ISO/RFC3339); default: now")
	cmd.Flags().BoolVar(&distinct, "distinct", false, "Group entries into fingerprinted error classes")
	cmd.Flags().StringArrayVar(&suppress, "suppress", nil, "Fingerprint of a known-chronic error class to drop (repeatable)")
	cmd.Flags().StringArrayVar(&suppressContains, "suppress-contains", nil, "Drop classes whose template or exception contains this substring (repeatable)")
	cmd.Flags().IntVar(&maxEntries, "max-entries", 10000, "Maximum entries to scan in the window")
	return cmd
}

// filterEntriesToWindow drops entries outside [start, end]. The modern
// route honours startDate/endDate server-side, but the legacy fallback
// route ignores them, so the window is enforced here as well — matching
// what logs list/search do via shapeLogResult.
func filterEntriesToWindow(items []any, start time.Time, end time.Time) []any {
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw, _ := entry["timestamp"].(string)
		timestamp, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			// An unparsable timestamp is kept: dropping it could hide an
			// error, and the window exists to exclude entries known to be
			// outside it, not to require perfect data.
			filtered = append(filtered, item)
			continue
		}
		if timestamp.Before(start) || timestamp.After(end) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// errorClass is one fingerprinted group of Error/Fatal entries.
type errorClass struct {
	Fingerprint     string   `json:"fingerprint"`
	Count           int      `json:"count"`
	Levels          []string `json:"levels"`
	FirstSeen       string   `json:"firstSeen"`
	LastSeen        string   `json:"lastSeen"`
	MessageTemplate string   `json:"messageTemplate"`
	ExceptionHead   string   `json:"exceptionHead,omitempty"`
	ExampleMessage  string   `json:"exampleMessage,omitempty"`
	SourceContexts  []string `json:"sourceContexts,omitempty"`
}

func groupErrorClasses(items []any, suppress []string, suppressContains []string) ([]errorClass, int) {
	suppressed := map[string]struct{}{}
	for _, fp := range suppress {
		suppressed[strings.TrimSpace(fp)] = struct{}{}
	}

	byFingerprint := map[string]*errorClass{}
	sourceSets := map[string]map[string]struct{}{}
	levelSets := map[string]map[string]struct{}{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		template, _ := entry["messageTemplate"].(string)
		exception, _ := entry["exception"].(string)
		head := normalizeExceptionHead(exception)
		if template == "" && head == "" {
			// Both fingerprint fields are nullable on the API; without this
			// fallback every such entry would collapse into one class.
			rendered, _ := entry["renderedMessage"].(string)
			template = normalizeExceptionHead(rendered)
		}
		fingerprint := errorFingerprint(template, head)

		class, exists := byFingerprint[fingerprint]
		if !exists {
			class = &errorClass{
				Fingerprint:     fingerprint,
				MessageTemplate: template,
				ExceptionHead:   head,
			}
			if rendered, _ := entry["renderedMessage"].(string); rendered != "" {
				class.ExampleMessage = truncateForDisplay(rendered, 300)
			}
			byFingerprint[fingerprint] = class
			sourceSets[fingerprint] = map[string]struct{}{}
			levelSets[fingerprint] = map[string]struct{}{}
		}
		class.Count++
		timestamp, _ := entry["timestamp"].(string)
		if class.FirstSeen == "" || timestamp < class.FirstSeen {
			class.FirstSeen = timestamp
		}
		if timestamp > class.LastSeen {
			class.LastSeen = timestamp
		}
		if level, _ := entry["level"].(string); level != "" {
			levelSets[fingerprint][level] = struct{}{}
		}
		if properties, ok := entry["properties"].([]any); ok {
			for _, p := range properties {
				prop, ok := p.(map[string]any)
				if !ok {
					continue
				}
				if name, _ := prop["name"].(string); name == "SourceContext" {
					if value, _ := prop["value"].(string); value != "" {
						sourceSets[fingerprint][value] = struct{}{}
					}
				}
			}
		}
	}

	suppressedCount := 0
	classes := make([]errorClass, 0, len(byFingerprint))
	for fingerprint, class := range byFingerprint {
		if isSuppressedClass(class, suppressed, suppressContains) {
			suppressedCount++
			continue
		}
		class.Levels = cmdkit.SortedKeys(levelSets[fingerprint])
		class.SourceContexts = cmdkit.SortedKeys(sourceSets[fingerprint])
		classes = append(classes, *class)
	}
	// Newest-first-seen so brand-new breakage tops the list; chronic classes
	// that predate the window sink to the bottom.
	sort.Slice(classes, func(i, j int) bool {
		if classes[i].FirstSeen != classes[j].FirstSeen {
			return classes[i].FirstSeen > classes[j].FirstSeen
		}
		return classes[i].Fingerprint < classes[j].Fingerprint
	})
	return classes, suppressedCount
}

func isSuppressedClass(class *errorClass, suppressed map[string]struct{}, suppressContains []string) bool {
	if _, ok := suppressed[class.Fingerprint]; ok {
		return true
	}
	for _, needle := range suppressContains {
		needle = strings.TrimSpace(needle)
		if needle == "" {
			continue
		}
		if strings.Contains(class.MessageTemplate, needle) || strings.Contains(class.ExceptionHead, needle) || strings.Contains(class.ExampleMessage, needle) {
			return true
		}
	}
	return false
}

var (
	errorHexPattern  = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	errorGUIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	errorNumPattern  = regexp.MustCompile(`\d+`)
)

// normalizeExceptionHead reduces an exception blob to its stable first line:
// hex codes, GUIDs, and numbers become placeholders so occurrences
// fingerprint identically, while quoted identifiers (constraint names,
// aliases) are kept — they are what distinguishes one SQL violation class
// from another.
func normalizeExceptionHead(exception string) string {
	head := strings.TrimSpace(exception)
	if head == "" {
		return ""
	}
	if index := strings.IndexAny(head, "\r\n"); index >= 0 {
		head = head[:index]
	}
	head = errorGUIDPattern.ReplaceAllString(head, "<guid>")
	head = errorHexPattern.ReplaceAllString(head, "0x#")
	head = errorNumPattern.ReplaceAllString(head, "#")
	return truncateForDisplay(head, 300)
}

func errorFingerprint(template string, exceptionHead string) string {
	sum := sha256.Sum256([]byte(template + "\n" + exceptionHead))
	return hex.EncodeToString(sum[:])[:12]
}

func truncateForDisplay(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
