package engage

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/uuid"
)

// engageMetrics and engageDimensions are the MetricModel and DimensionModel
// enums of the Engage 18.1.0 Management API OpenAPI document
// (/umbraco/openapi/engage-management.json), minus the UNDEFINED sentinel.
// The analytics commands canonicalise names case-insensitively against them
// so a typo fails locally instead of as a server-side 400; '--json' on
// 'analytics query' sends a body verbatim for names a newer Engage adds.
var engageMetrics = []string{
	"users", "newVisitors", "percentNewSessions", "sessions", "bounceRate", "sessionDuration", "avgSessionDuration",
	"goalCompletionsAll", "goalValueAll", "pageviews", "pageviewsWithSubpages", "pageviewsPerSession", "uniquePageviews",
	"avgTimeOnPage", "totalEvents", "eventValue", "avgEventValue", "abTestVariantVisitors", "abTestVariantCompletionsAll",
	"videoPlays", "videoPlaytime", "avgVideoPlaytime", "avgRelativeVideoPlaytime", "videoEvents",
	"umbracoFormsViews", "umbracoFormsSubmissions", "umbracoFormsStarts", "umbracoFormsAbandons", "umbracoFormsErrors",
	"umbracoFormsFieldFocusses", "umbracoFormsFieldAbandons", "umbracoFormsFieldErrors", "avgEngagedTimeOnPage",
	"pageviewsBot", "pageSessions", "pageVisitors", "pageSessionsWithSubpages", "pageVisitorsWithSubpages",
	"activeSegmentPageviews", "inactiveSegmentPageviews", "activeSegmentSessions", "inactiveSegmentSessions",
	"activeSegmentProfiles", "inactiveSegmentProfiles", "activePersonalizedSegmentPageviews", "inactivePersonalizedSegmentPageviews",
	"activePersonalizedSegmentSessions", "inactivePersonalizedSegmentSessions", "activePersonalizedSegmentProfiles",
	"inactivePersonalizedSegmentProfiles", "searchVolume", "searchVolumeWithSubpages", "visitors", "campaignEntries",
	"campaignVisitors", "campaignGoals", "campaignGoalValue", "campaignFormSubmissions", "campaignCommerceOrders", "entrySessions",
}

var engageDimensions = []string{
	"usertype", "referralPath", "fullReferrer", "campaign", "source", "medium", "sourceMedium", "browser", "browserVersion",
	"operatingSystem", "operatingSystemVersion", "deviceCategory", "country", "city", "hostname", "pageUrl", "pageId",
	"NodeId", "NodeCulture", "NodeSegment", "NodeContentType", "pagePath", "eventCategory", "eventAction", "eventLabel",
	"date", "year", "month", "week", "day", "hour", "minute", "nthMonth", "nthWeek", "nthDay", "nthMinute", "nthHour",
	"abTest", "abTestVariant", "goal", "goalId", "goalType", "isMainGoal", "visitorType", "bot", "botId", "botVersion",
	"botVersionId", "referralHostname", "referralIsInternalPage", "referralPathAndQuery", "browserId", "datetime",
	"pageviewSegmentId", "pageviewSegmentName", "pageviewAppliedSegmentId", "pageviewAppliedSegmentName",
	"pageviewAppliedPersonalizationId", "pageviewAppliedPersonalizationName", "videoName", "videoUrl", "videoEventType",
	"videoEventTypeName", "umbracoFormsName", "umbracoFormsId", "umbracoFormsFieldName", "umbracoFormsFieldId",
	"umbracoFormsFieldError", "cityId", "countryId", "province", "provinceId", "county", "countyId", "sourcePlatform",
	"creativeFormat", "marketingTactic", "segmentId", "segmentName", "searchTerm",
}

// engageAnalyticsDefaultDays is the length of the default query range,
// today included.
const engageAnalyticsDefaultDays = 30

const engageDayLayout = "2006-01-02"

// canonicalEngageNames maps a comma-separated list onto the canonical enum
// spelling, case-insensitively, and reports unknown names together.
func canonicalEngageNames(flag string, raw string, known []string, catalogue string) ([]string, error) {
	index := make(map[string]string, len(known))
	for _, name := range known {
		index[strings.ToLower(name)] = name
	}
	names := []string{}
	unknown := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		canonical, ok := index[strings.ToLower(part)]
		if !ok {
			unknown = append(unknown, part)
			continue
		}
		names = append(names, canonical)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("%s: unknown name(s) %s; list the valid ones with '%s' (or send a raw body with --json)", flag, strings.Join(unknown, ", "), catalogue)
	}
	return names, nil
}

// parseEngageDate accepts YYYY-MM-DD or RFC 3339 and returns the value as
// given. The annotation routes compare it with timestamps as an instant.
func parseEngageDate(flag string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if _, err := time.Parse(engageDayLayout, value); err == nil {
		return value, nil
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value, nil
	}
	return "", fmt.Errorf("%s must be an ISO 8601 date (YYYY-MM-DD) or RFC 3339 date-time, got %q", flag, value)
}

// parseEngageDay reads one end of an analytics query range. Engage counts
// whole days and drops the time of day, after converting an offset to its
// UTC date, so only a value that names a day exactly is accepted: YYYY-MM-DD,
// or RFC 3339 at midnight UTC (sent as its date). Anything else would query
// a different range than the one written.
func parseEngageDay(flag string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if _, err := time.Parse(engageDayLayout, value); err == nil {
		return value, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("%s must be a day as YYYY-MM-DD, got %q", flag, value)
	}
	_, offset := parsed.Zone()
	if offset == 0 && parsed.Equal(parsed.Truncate(24*time.Hour)) {
		return parsed.Format(engageDayLayout), nil
	}
	moved := ""
	if offset != 0 {
		moved = fmt.Sprintf(", after converting it to its UTC date (%s)", parsed.UTC().Format(engageDayLayout))
	}
	return "", fmt.Errorf("%s %q is not a whole day: Engage counts whole days, both ends inclusive, and ignores the time of day%s. Pass the day as YYYY-MM-DD", flag, value, moved)
}

func engageAnalytics(deps cmdkit.Dependencies) *cobra.Command {
	group := &cobra.Command{
		Use:   "analytics",
		Short: "Query Engage analytics (metrics by dimensions over a date range)",
		Long:  "Engage's analytics query endpoint powers every chart in the back-office Analytics section: pick metrics (pageviews, sessions, goal completions, ...), break them down by dimensions (date, pagePath, country, source, ...), and filter. 'analytics metrics' and 'analytics dimensions' list the valid names.",
	}
	group.AddCommand(engageAnalyticsQuery(deps))
	group.AddCommand(engageAnalyticsDistinct(deps))
	group.AddCommand(engageNameCatalogue(deps, "metrics", "List the metric names 'analytics query --metrics' accepts", engageMetrics))
	group.AddCommand(engageNameCatalogue(deps, "dimensions", "List the dimension names 'analytics query --dimensions' and 'analytics distinct' accept", engageDimensions))
	return group
}

func engageNameCatalogue(deps cmdkit.Dependencies, use string, short string, names []string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  short + ". Taken from the Engage 18.1.0 OpenAPI document; names are matched case-insensitively.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdkit.PrintResult(cmd, deps, names)
		},
	}
}

func engageAnalyticsQuery(deps cmdkit.Dependencies) *cobra.Command {
	var metricsRaw, dimensionsRaw, from, to, filter, node, culture, sortBy, jsonPayload string
	var page, pageSize int
	var ascending, realtime, includeSubpages, dryRun bool
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run an analytics query (POST /analytics/query; read-only)",
		Long: "POST /analytics/query. The request is a POST but reads only.\n\n" +
			"--metrics is required; --dimensions is optional (none gives one total row). Names are checked against 'analytics metrics' / 'analytics dimensions'. " +
			"--from and --to are whole days, both inclusive: Engage counts every day from --from through --to and ignores any time of day, so --from 2026-10-01 --to 2026-10-01 is that one day. " +
			"Pass days as YYYY-MM-DD. An RFC 3339 value is accepted only at midnight UTC (00:00:00Z) and is sent as its date; any other time or offset is refused, because Engage would drop the time and convert an offset to its UTC date. " +
			"Without them the range is the last 30 days: today (UTC) and the 29 days before.\n\n" +
			"--filter takes Engage's filter syntax: Dimension=='value' clauses joined with ';' (AND), e.g. \"country=='Denmark';deviceCategory=='mobile'\". " +
			"--node <documentGuid> narrows to one page the way the back office does (NodeId=='<guid>', with a '+' suffix under --include-subpages); --culture adds NodeCulture.\n\n" +
			"--sort defaults to the first dimension. --page is 1-based. The result carries `columns` and `rows` (one array per row, in column order) plus paging totals.\n\n" +
			"--json sends a full AnalyticsQueryGetModel body verbatim and cannot be combined with the builder flags.\n\n" +
			"Engage 18.1.0 answers some combinations with HTTP 500 and an empty body; the CLI keeps the error (exit code 4) and adds a hint. Measured on 18.1.0: " +
			"without --realtime, the year, month, week and day dimensions; the visitorType and usertype dimensions; totalEvents by pagePath; an eventCategory/eventAction filter with pageviews, sessions or goalCompletionsAll; and a goal filter with sessions or users. " +
			"With --realtime, the totalEvents metric.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				for _, name := range []string{"metrics", "dimensions", "from", "to", "filter", "node", "culture", "sort", "page", "page-size", "ascending", "realtime", "include-subpages"} {
					if cmd.Flags().Changed(name) {
						return fmt.Errorf("--json sends the body verbatim and cannot be combined with --%s", name)
					}
				}
				parsed, err := cmdkit.ParsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				built, err := buildEngageAnalyticsQuery(engageAnalyticsQueryInput{
					Metrics: metricsRaw, Dimensions: dimensionsRaw, From: from, To: to, Filter: filter, Node: node, Culture: culture,
					Sort: sortBy, Page: page, PageSize: pageSize, Ascending: ascending, Realtime: realtime, IncludeSubpages: includeSubpages,
				}, time.Now().UTC())
				if err != nil {
					return err
				}
				body = built
			}
			opts := engageOpts(nil)
			opts.DryRun = dryRun
			result, err := deps.Client.Post(cmd.Context(), "/analytics/query", body, opts)
			if err != nil {
				realtime, _ := body["realtime"].(bool)
				return engageAnalyticsQueryError(engageError(err), realtime)
			}
			return cmdkit.PrintResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&metricsRaw, "metrics", "", "Comma-separated metric names (required unless --json), e.g. pageviews,sessions")
	cmd.Flags().StringVar(&dimensionsRaw, "dimensions", "", "Comma-separated dimension names, e.g. date or pagePath,country")
	cmd.Flags().StringVar(&from, "from", "", "First day of the range, inclusive, as YYYY-MM-DD (default: 29 days before today, UTC)")
	cmd.Flags().StringVar(&to, "to", "", "Last day of the range, inclusive, as YYYY-MM-DD (default: today, UTC)")
	cmd.Flags().StringVar(&filter, "filter", "", "Engage filter expression: Dimension=='value' clauses joined with ';'")
	cmd.Flags().StringVar(&node, "node", "", "Restrict to one page by its document GUID")
	cmd.Flags().StringVar(&culture, "culture", "", "With --node: restrict to one culture, e.g. en-US")
	cmd.Flags().StringVar(&sortBy, "sort", "", "Metric or dimension to sort by (default: the first dimension)")
	cmd.Flags().IntVar(&page, "page", 1, "Result page, 1-based")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Rows per page")
	cmd.Flags().BoolVar(&ascending, "ascending", false, "Sort ascending instead of descending")
	cmd.Flags().BoolVar(&realtime, "realtime", false, "Query the realtime (unaggregated) data instead of the reporting tables")
	cmd.Flags().BoolVar(&includeSubpages, "include-subpages", false, "With --node: include the page's descendants")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full AnalyticsQueryGetModel body, sent verbatim")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

type engageAnalyticsQueryInput struct {
	Metrics, Dimensions, From, To, Filter, Node, Culture, Sort string
	Page, PageSize                                             int
	Ascending, Realtime, IncludeSubpages                       bool
}

// buildEngageAnalyticsQuery assembles the body the back office sends from
// analytics-context.js: every required field present, sort defaulting to
// the first dimension, and --node folded into the filter.
func buildEngageAnalyticsQuery(in engageAnalyticsQueryInput, now time.Time) (map[string]any, error) {
	metrics, err := canonicalEngageNames("--metrics", in.Metrics, engageMetrics, "umbraco engage analytics metrics")
	if err != nil {
		return nil, err
	}
	// Checked after canonicalisation: "--metrics ," names nothing and would
	// otherwise reach the API as an empty list.
	if len(metrics) == 0 {
		return nil, fmt.Errorf("missing required option: --metrics (see 'umbraco engage analytics metrics')")
	}
	dimensions, err := canonicalEngageNames("--dimensions", in.Dimensions, engageDimensions, "umbraco engage analytics dimensions")
	if err != nil {
		return nil, err
	}
	if in.Page < 1 {
		return nil, fmt.Errorf("--page is 1-based, got %d", in.Page)
	}
	if in.PageSize < 1 {
		return nil, fmt.Errorf("--page-size must be positive, got %d", in.PageSize)
	}
	today := now.UTC().Truncate(24 * time.Hour)
	start := today.AddDate(0, 0, 1-engageAnalyticsDefaultDays).Format(engageDayLayout)
	end := today.Format(engageDayLayout)
	if strings.TrimSpace(in.From) != "" {
		if start, err = parseEngageDay("--from", in.From); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(in.To) != "" {
		if end, err = parseEngageDay("--to", in.To); err != nil {
			return nil, err
		}
	}
	filter := strings.TrimSpace(in.Filter)
	if node := strings.TrimSpace(in.Node); node != "" {
		if !uuid.Valid(node) {
			return nil, fmt.Errorf("--node expects a document GUID, got %q", node)
		}
		clause := "NodeId=='" + node
		if in.IncludeSubpages {
			clause += "+"
		}
		clause += "'"
		if culture := strings.TrimSpace(in.Culture); culture != "" {
			clause += ";NodeCulture=='" + culture + "'"
		}
		if filter != "" {
			filter += ";" + clause
		} else {
			filter = clause
		}
	} else if strings.TrimSpace(in.Culture) != "" {
		return nil, fmt.Errorf("--culture requires --node")
	}
	body := map[string]any{
		"metrics":         metrics,
		"dimensions":      dimensions,
		"startDate":       start,
		"endDate":         end,
		"realtime":        in.Realtime,
		"ascending":       in.Ascending,
		"page":            in.Page,
		"pageSize":        in.PageSize,
		"includeSubpages": in.IncludeSubpages,
	}
	if filter != "" {
		body["filter"] = filter
	}
	switch {
	case strings.TrimSpace(in.Sort) != "":
		known := append(append([]string{}, engageMetrics...), engageDimensions...)
		sortNames, err := canonicalEngageNames("--sort", in.Sort, known, "umbraco engage analytics metrics' / 'umbraco engage analytics dimensions")
		if err != nil {
			return nil, err
		}
		if len(sortNames) != 1 {
			return nil, fmt.Errorf("--sort takes exactly one metric or dimension name")
		}
		body["sort"] = sortNames[0]
	case len(dimensions) > 0:
		body["sort"] = dimensions[0]
	}
	return body, nil
}

func engageAnalyticsDistinct(deps cmdkit.Dependencies) *cobra.Command {
	var dimension string
	cmd := &cobra.Command{
		Use:   "distinct",
		Short: "List the distinct values recorded for one dimension (GET /analytics/distinct)",
		Long: "GET /analytics/distinct?dimension=<name>. Useful for building --filter values, e.g. the countries or device categories Engage has seen.\n\n" +
			"Engage 18.1.0 answers HTTP 500 for the visitorType and usertype dimensions; the CLI keeps the error (exit code 4) and adds a hint.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireValue("--dimension", dimension); err != nil {
				return err
			}
			names, err := canonicalEngageNames("--dimension", dimension, engageDimensions, "umbraco engage analytics dimensions")
			if err != nil {
				return err
			}
			if len(names) != 1 {
				return fmt.Errorf("--dimension takes exactly one dimension name")
			}
			result, err := engageGet(cmd, deps, "/analytics/distinct", map[string]any{"dimension": names[0]})
			if err != nil {
				return engageServerErrorHint(err, engageDistinctFailedHint)
			}
			return cmdkit.PrintResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&dimension, "dimension", "", "Dimension name (see 'analytics dimensions')")
	return cmd
}

func engageAnnotation(deps cmdkit.Dependencies) *cobra.Command {
	group := &cobra.Command{Use: "annotation", Short: "Analytics annotations (notes pinned to dates on the analytics charts)"}
	var fields, from, to, node, culture string
	var global bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List annotations, optionally only global ones or those on one page",
		Long: "GET /annotations/all by default; --global uses /annotations/global (annotations not tied to a page); --node <documentGuid> uses /annotations/page, with --culture for one culture.\n\n" +
			"--from and --to (YYYY-MM-DD or RFC 3339) are instants, unlike the whole days of 'analytics query': Engage returns the annotations timestamped between them, honouring the time and any offset. " +
			"A YYYY-MM-DD value is midnight at the start of that day, so --to 2026-09-30 leaves out annotations made on the 30th; pass --to 2026-10-01 to include them. " +
			"Engage 18.1.0 answers HTTP 500 unless both are given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if global && strings.TrimSpace(node) != "" {
				return fmt.Errorf("--global and --node are mutually exclusive")
			}
			if strings.TrimSpace(culture) != "" && strings.TrimSpace(node) == "" {
				return fmt.Errorf("--culture requires --node")
			}
			params := map[string]any{}
			for flag, value := range map[string]string{"from": from, "to": to} {
				if strings.TrimSpace(value) == "" {
					continue
				}
				parsed, err := parseEngageDate("--"+flag, value)
				if err != nil {
					return err
				}
				params[flag] = parsed
			}
			path := "/annotations/all"
			switch {
			case global:
				path = "/annotations/global"
			case strings.TrimSpace(node) != "":
				if !uuid.Valid(node) {
					return fmt.Errorf("--node expects a document GUID, got %q", node)
				}
				path = "/annotations/page"
				params["unique"] = strings.TrimSpace(node)
				if strings.TrimSpace(culture) != "" {
					params["culture"] = strings.TrimSpace(culture)
				}
			}
			if len(params) == 0 {
				params = nil
			}
			result, err := engageGet(cmd, deps, path, params)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(result, fields))
		},
	}
	cmdkit.AddFieldsFlag(list, &fields)
	list.Flags().StringVar(&from, "from", "", "Start of the range: YYYY-MM-DD or RFC 3339")
	list.Flags().StringVar(&to, "to", "", "End of the range: YYYY-MM-DD or RFC 3339")
	list.Flags().BoolVar(&global, "global", false, "Only annotations not tied to a page")
	list.Flags().StringVar(&node, "node", "", "Only annotations on this page (document GUID)")
	list.Flags().StringVar(&culture, "culture", "", "With --node: only this culture")
	group.AddCommand(list)
	group.AddCommand(engageAnnotationCreate(deps))
	group.AddCommand(engageAnnotationDelete(deps))
	return group
}

func engageStats(deps cmdkit.Dependencies) *cobra.Command {
	group := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate counts (pageviews, visitors, identified profiles); no per-visitor data",
	}
	group.AddCommand(engageRead(deps, engageReadSpec{Use: "overview", Short: "Totals of pageviews, person and bot visitors, events, and segment settings (GET /statistics)", Path: "/statistics"}))
	group.AddCommand(engageRead(deps, engageReadSpec{Use: "profiles", Short: "Count identified and unknown profiles (GET /profile/statistics/total)", Path: "/profile/statistics/total"}))
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "profile-growth", Short: "Monthly counts of identified and unknown profiles (GET /profile/statistics/growth)", Path: "/profile/statistics/growth",
		Flags: []engageFlag{{Name: "months", Param: "numberOfMonths", Kind: engageFlagInt, Usage: "Number of months to return (server default when unset)"}},
	}))
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "identification", Short: "Newly identified vs. still-unknown profiles over a window (GET /profile/statistics/identification)", Path: "/profile/statistics/identification",
		Flags: []engageFlag{{Name: "days", Param: "numberOfDays", Kind: engageFlagInt, Usage: "Window in days (server default when unset)"}},
	}))
	return group
}

func engageReporting(deps cmdkit.Dependencies) *cobra.Command {
	group := &cobra.Command{Use: "reporting", Short: "Engage's aggregated reporting tables"}
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "status", Short: "Whether the reporting tables exist, are being generated, and when they were last generated (GET /reporting/generation/status)",
		Long: "GET /reporting/generation/status. Non-realtime analytics queries read the reporting tables, so a missing or stale generation explains empty or old 'analytics query' results.",
		Path: "/reporting/generation/status",
	}))
	group.AddCommand(engageReportingGenerate(deps))
	return group
}
