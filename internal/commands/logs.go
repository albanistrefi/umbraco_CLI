package commands

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

const (
	logViewerLogPath                    = "/log-viewer/log"
	logViewerMessageTemplatePath        = "/log-viewer/message-template"
	logViewerLegacyListPath             = "/log-viewer"
	logViewerLegacySearchPath           = "/log-viewer/search"
	logViewerLegacyMessageTemplatesPath = "/log-viewer/templates"
)

func RegisterLogs(root *cobra.Command, deps Dependencies) {
	logs := &cobra.Command{Use: "logs", Short: "Log and diagnostics operations"}
	logs.AddCommand(logsList(deps))
	logs.AddCommand(logsLevels(deps))
	logs.AddCommand(logsLevelCount(deps))
	logs.AddCommand(logsTemplates(deps))
	logs.AddCommand(logsSearch(deps))
	root.AddCommand(logs)
}

func logsList(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var flags logQueryFlags
	flags.skip = -1
	flags.take = -1
	flags.minutes = 5

	cmd := &cobra.Command{Use: "list", Short: "List log entries", RunE: func(cmd *cobra.Command, args []string) error {
		params, runtime, err := logParamsFromFlags(paramsRaw, flags)
		if err != nil {
			return err
		}
		result, err := getWithFallback(
			cmd.Context(),
			deps.Client,
			getRequestCandidate{path: logViewerLogPath, opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: logViewerLegacyListPath, opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		result, err = shapeLogResult(result, runtime)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Filter params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	addLogQueryFlags(cmd, &flags)
	return cmd
}

func logsLevels(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "levels", Short: "List log levels", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("logs levels is not available in the Umbraco v17 Management API; use logs list --level <level> or logs list --filter-expression <expression>")
	}}
}

func logsLevelCount(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var from string
	var to string
	cmd := &cobra.Command{Use: "level-count", Short: "Get count per level", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			params = map[string]any{}
			if from != "" {
				params["startDate"] = from
			}
			if to != "" {
				params["endDate"] = to
			}
		}
		result, err := deps.Client.Get(cmd.Context(), "/log-viewer/level-count", api.RequestOptions{Params: params})
		if err != nil {
			return friendlyLogViewerError(err)
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Filter params as JSON")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	return cmd
}

func logsTemplates(deps Dependencies) *cobra.Command {
	var from string
	var to string
	var skip int
	var take int
	cmd := &cobra.Command{Use: "templates", Short: "List paginated log message templates", RunE: func(cmd *cobra.Command, args []string) error {
		params := logDateRangePagingParams(from, to, skip, take)
		result, err := getWithFallback(
			cmd.Context(),
			deps.Client,
			getRequestCandidate{path: logViewerMessageTemplatePath, opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: logViewerLegacyMessageTemplatesPath, opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
}

func logsSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var flags logQueryFlags
	flags.skip = -1
	flags.take = -1
	flags.minutes = 5
	cmd := &cobra.Command{Use: "search", Short: "Search logs", RunE: func(cmd *cobra.Command, args []string) error {
		params, runtime, err := logParamsFromFlags(paramsRaw, flags)
		if err != nil {
			return err
		}
		result, err := getWithFallback(
			cmd.Context(),
			deps.Client,
			getRequestCandidate{path: logViewerLogPath, opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: logViewerLegacySearchPath, opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		result, err = shapeLogResult(result, runtime)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	addLogQueryFlags(cmd, &flags)
	return cmd
}

type logQueryFlags struct {
	level            string
	filterExpression string
	from             string
	to               string
	skip             int
	take             int
	sourceContext    string
	path             string
	contains         string
	correlationID    string
	around           string
	minutes          int
	flat             bool
	redact           string
	redactDefault    bool
	countBy          string
	cursor           string
}

type logRuntimeOptions struct {
	from           *time.Time
	to             *time.Time
	levels         []string
	sourceContext  string
	path           string
	contains       string
	correlationID  string
	flat           bool
	countBy        string
	redaction      logRedactionOptions
	skip           int
	take           int
	hasPagination  bool
	hasPostFilters bool
}

type logRedactionOptions struct {
	emails  bool
	secrets bool
	tokens  bool
}

func addLogQueryFlags(cmd *cobra.Command, flags *logQueryFlags) {
	cmd.Flags().StringVar(&flags.level, "level", "", "Log level")
	cmd.Flags().StringVar(&flags.filterExpression, "filter-expression", "", "Serilog filter expression")
	cmd.Flags().StringVar(&flags.from, "from", "", "Start date/time (ISO/RFC3339); enforced client-side")
	cmd.Flags().StringVar(&flags.to, "to", "", "End date/time (ISO/RFC3339); enforced client-side")
	cmd.Flags().IntVar(&flags.skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&flags.take, "take", -1, "Take count")
	cmd.Flags().StringVar(&flags.cursor, "cursor", "", "Pagination cursor returned as nextCursor")
	cmd.Flags().StringVar(&flags.sourceContext, "source-context", "", "Client-side SourceContext contains filter")
	cmd.Flags().StringVar(&flags.path, "path", "", "Client-side RequestPath contains filter")
	cmd.Flags().StringVar(&flags.contains, "contains", "", "Client-side text contains filter across message, exception, and properties")
	cmd.Flags().StringVar(&flags.correlationID, "correlation-id", "", "Client-side correlation/request ID contains filter")
	cmd.Flags().StringVar(&flags.around, "around", "", "Center timestamp for a strict time window (ISO/RFC3339)")
	cmd.Flags().IntVar(&flags.minutes, "minutes", 5, "Minutes before and after --around")
	cmd.Flags().BoolVar(&flags.flat, "flat", false, "Return stable flat JSON entries with properties as an object")
	cmd.Flags().StringVar(&flags.redact, "redact", "", "Comma-separated redaction modes: emails,secrets,tokens,all")
	cmd.Flags().BoolVar(&flags.redactDefault, "redact-default", false, "Redact emails, secrets, and tokens from output")
	cmd.Flags().StringVar(&flags.countBy, "count-by", "", "Return counts grouped by level, source, or path")
}

func logParamsFromFlags(raw string, flags logQueryFlags) (map[string]any, logRuntimeOptions, error) {
	parsed, err := parseParams(raw)
	if err != nil {
		return nil, logRuntimeOptions{}, err
	}

	// Start from the raw --params blob (if any), then layer the named flags
	// on top so the two sources combine. Previously a non-nil --params short-
	// circuited every flag, so `--level Error --params '{"take":30}'` silently
	// dropped the level filter and returned newest-N unfiltered.
	params := map[string]any{}
	if parsed != nil {
		params = normalizeLogParams(parsed)
	}
	if flags.around != "" {
		if flags.from != "" || flags.to != "" || params["startDate"] != nil || params["endDate"] != nil {
			return nil, logRuntimeOptions{}, fmt.Errorf("--around cannot be combined with --from, --to, or startDate/endDate in --params")
		}
		if flags.minutes <= 0 {
			return nil, logRuntimeOptions{}, fmt.Errorf("--minutes must be greater than zero")
		}
		center, err := parseLogTime(flags.around)
		if err != nil {
			return nil, logRuntimeOptions{}, fmt.Errorf("invalid --around: %w", err)
		}
		window := time.Duration(flags.minutes) * time.Minute
		flags.from = center.Add(-window).Format(time.RFC3339Nano)
		flags.to = center.Add(window).Format(time.RFC3339Nano)
	}
	if flags.cursor != "" {
		if flags.skip >= 0 || params["skip"] != nil {
			return nil, logRuntimeOptions{}, fmt.Errorf("--cursor cannot be combined with --skip or skip in --params")
		}
		cursor, err := strconv.Atoi(flags.cursor)
		if err != nil || cursor < 0 {
			return nil, logRuntimeOptions{}, fmt.Errorf("--cursor must be a non-negative integer")
		}
		flags.skip = cursor
	}
	if flags.level != "" {
		params["logLevel"] = []any{flags.level}
	}
	if flags.filterExpression != "" {
		params["filterExpression"] = flags.filterExpression
	}
	if flags.from != "" {
		params["startDate"] = flags.from
	}
	if flags.to != "" {
		params["endDate"] = flags.to
	}
	if flags.skip >= 0 {
		params["skip"] = flags.skip
	}
	if flags.take >= 0 {
		params["take"] = flags.take
	}

	runtime, err := logRuntimeFromParams(params, flags)
	if err != nil {
		return nil, logRuntimeOptions{}, err
	}
	return params, runtime, nil
}

func normalizeLogParams(params map[string]any) map[string]any {
	normalized := make(map[string]any, len(params))
	for key, value := range params {
		switch key {
		case "from":
			normalized["startDate"] = value
		case "to":
			normalized["endDate"] = value
		case "level":
			normalized["logLevel"] = []any{value}
		case "logLevels":
			normalized["logLevel"] = value
		case "filter":
			normalized["filterExpression"] = value
		default:
			normalized[key] = value
		}
	}
	return normalized
}

func logDateRangePagingParams(from string, to string, skip int, take int) map[string]any {
	params := map[string]any{}
	if from != "" {
		params["startDate"] = from
	}
	if to != "" {
		params["endDate"] = to
	}
	if skip >= 0 {
		params["skip"] = skip
	}
	if take >= 0 {
		params["take"] = take
	}
	return params
}

func logRuntimeFromParams(params map[string]any, flags logQueryFlags) (logRuntimeOptions, error) {
	runtime := logRuntimeOptions{
		levels:        logStringListParam(params["logLevel"]),
		sourceContext: flags.sourceContext,
		path:          flags.path,
		contains:      flags.contains,
		correlationID: flags.correlationID,
		flat:          flags.flat,
		skip:          logIntParam(params["skip"], 0),
		take:          logIntParam(params["take"], 100),
		hasPagination: params["skip"] != nil || params["take"] != nil || flags.cursor != "",
	}

	if raw := strings.TrimSpace(fmt.Sprint(params["startDate"])); raw != "" && raw != "<nil>" {
		parsed, err := parseLogTime(raw)
		if err != nil {
			return runtime, fmt.Errorf("invalid --from/startDate: %w", err)
		}
		runtime.from = &parsed
	}
	if raw := strings.TrimSpace(fmt.Sprint(params["endDate"])); raw != "" && raw != "<nil>" {
		parsed, err := parseLogTime(raw)
		if err != nil {
			return runtime, fmt.Errorf("invalid --to/endDate: %w", err)
		}
		runtime.to = &parsed
	}
	if runtime.from != nil && runtime.to != nil && runtime.from.After(*runtime.to) {
		return runtime, fmt.Errorf("--from/startDate must be before --to/endDate")
	}

	countBy := strings.ToLower(strings.TrimSpace(flags.countBy))
	switch countBy {
	case "", "level", "source", "source-context", "sourcecontext", "path":
	case "request-path", "requestpath":
		countBy = "path"
	default:
		return runtime, fmt.Errorf("--count-by must be one of: level, source, path")
	}
	if countBy == "source-context" || countBy == "sourcecontext" {
		countBy = "source"
	}
	runtime.countBy = countBy

	redaction, err := parseLogRedaction(flags.redact, flags.redactDefault)
	if err != nil {
		return runtime, err
	}
	runtime.redaction = redaction

	runtime.hasPostFilters = runtime.from != nil ||
		runtime.to != nil ||
		len(runtime.levels) > 0 ||
		runtime.sourceContext != "" ||
		runtime.path != "" ||
		runtime.contains != "" ||
		runtime.correlationID != ""
	return runtime, nil
}

func shapeLogResult(result any, opts logRuntimeOptions) (any, error) {
	if !opts.needsShaping() {
		return result, nil
	}

	envelope, ok := result.(map[string]any)
	if !ok {
		return redactLogValue(result, opts.redaction), nil
	}
	items, ok := envelope["items"].([]any)
	if !ok {
		return redactLogValue(result, opts.redaction), nil
	}

	serverReturned := len(items)
	filtered := make([]any, 0, serverReturned)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			if !opts.hasPostFilters {
				filtered = append(filtered, item)
			}
			continue
		}
		if !logEntryMatches(entry, opts) {
			continue
		}
		if opts.flat && opts.countBy == "" {
			filtered = append(filtered, flattenLogEntry(entry))
			continue
		}
		filtered = append(filtered, item)
	}

	if opts.countBy != "" {
		return redactLogValue(logCountResult(filtered, envelope, opts, serverReturned), opts.redaction), nil
	}

	shaped := copyAnyMap(envelope)
	shaped["items"] = filtered
	addLogPaginationMetadata(shaped, envelope, opts, serverReturned, len(filtered))
	return redactLogValue(shaped, opts.redaction), nil
}

func (opts logRuntimeOptions) needsShaping() bool {
	return opts.hasPostFilters ||
		opts.flat ||
		opts.countBy != "" ||
		opts.redaction.enabled() ||
		opts.hasPagination
}

func (opts logRedactionOptions) enabled() bool {
	return opts.emails || opts.secrets || opts.tokens
}

func logEntryMatches(entry map[string]any, opts logRuntimeOptions) bool {
	if opts.from != nil || opts.to != nil {
		timestamp, ok := logEntryTimestamp(entry)
		if !ok {
			return false
		}
		if opts.from != nil && timestamp.Before(*opts.from) {
			return false
		}
		if opts.to != nil && timestamp.After(*opts.to) {
			return false
		}
	}

	if len(opts.levels) > 0 && !matchesAnyFold(stringValue(entry["level"]), opts.levels) {
		return false
	}

	properties := logPropertiesMap(entry)
	if opts.sourceContext != "" && !containsFold(stringValue(properties["SourceContext"]), opts.sourceContext) {
		return false
	}
	if opts.path != "" && !containsFold(stringValue(properties["RequestPath"]), opts.path) {
		return false
	}
	if opts.correlationID != "" && !logCorrelationMatches(properties, opts.correlationID) {
		return false
	}
	if opts.contains != "" && !logEntryContains(entry, properties, opts.contains) {
		return false
	}
	return true
}

func logEntryTimestamp(entry map[string]any) (time.Time, bool) {
	raw := stringValue(entry["timestamp"])
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := parseLogTime(raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func parseLogTime(raw string) (time.Time, error) {
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

func logStringListParam(raw any) []string {
	switch value := raw.(type) {
	case nil:
		return nil
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	case []string:
		return value
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{text}
	}
}

func logIntParam(raw any, fallback int) int {
	switch value := raw.(type) {
	case nil:
		return fallback
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func logPropertiesMap(entry map[string]any) map[string]any {
	result := map[string]any{}
	properties, ok := entry["properties"].([]any)
	if !ok {
		return result
	}
	for _, property := range properties {
		propertyMap, ok := property.(map[string]any)
		if !ok {
			continue
		}
		name := stringValue(propertyMap["name"])
		if name == "" {
			continue
		}
		result[name] = propertyMap["value"]
	}
	return result
}

func flattenLogEntry(entry map[string]any) map[string]any {
	properties := logPropertiesMap(entry)
	flat := map[string]any{
		"timestamp":     entry["timestamp"],
		"level":         entry["level"],
		"message":       firstLogString(entry["renderedMessage"], entry["messageTemplate"], entry["message"]),
		"sourceContext": properties["SourceContext"],
		"requestPath":   properties["RequestPath"],
		"exception":     entry["exception"],
		"properties":    properties,
	}
	if correlation := firstLogString(properties["CorrelationId"], properties["CorrelationID"], properties["RequestId"], properties["HttpRequestId"]); correlation != "" {
		flat["correlationId"] = correlation
	}
	return flat
}

func firstLogString(values ...any) string {
	for _, value := range values {
		text := stringValue(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func logCorrelationMatches(properties map[string]any, needle string) bool {
	for _, key := range []string{"CorrelationId", "CorrelationID", "RequestId", "HttpRequestId", "TraceId", "SpanId"} {
		if containsFold(stringValue(properties[key]), needle) {
			return true
		}
	}
	return false
}

func logEntryContains(entry map[string]any, properties map[string]any, needle string) bool {
	for _, key := range []string{"timestamp", "level", "message", "renderedMessage", "messageTemplate", "exception"} {
		if containsFold(stringValue(entry[key]), needle) {
			return true
		}
	}
	for key, value := range properties {
		if containsFold(key, needle) || containsFold(stringValue(value), needle) {
			return true
		}
	}
	return false
}

func logCountResult(filtered []any, envelope map[string]any, opts logRuntimeOptions, serverReturned int) map[string]any {
	counts := map[string]int{}
	for _, item := range filtered {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := logCountKey(entry, opts.countBy)
		if key == "" {
			key = "(empty)"
		}
		counts[key]++
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([]any, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, map[string]any{"key": key, "count": counts[key]})
	}

	result := map[string]any{
		"countBy":  opts.countBy,
		"counts":   rows,
		"returned": len(filtered),
	}
	addLogPaginationMetadata(result, envelope, opts, serverReturned, len(filtered))
	return result
}

func logCountKey(entry map[string]any, countBy string) string {
	switch countBy {
	case "level":
		return stringValue(entry["level"])
	case "source":
		return stringValue(logPropertiesMap(entry)["SourceContext"])
	case "path":
		return stringValue(logPropertiesMap(entry)["RequestPath"])
	default:
		return ""
	}
}

func addLogPaginationMetadata(target map[string]any, envelope map[string]any, opts logRuntimeOptions, serverReturned int, filteredReturned int) {
	if !opts.hasPagination && !opts.hasPostFilters && !opts.flat && opts.countBy == "" {
		return
	}
	target["returned"] = filteredReturned
	target["serverReturned"] = serverReturned
	target["cursor"] = opts.skip
	target["take"] = opts.take
	if total, ok := numericAny(envelope["total"]); ok {
		target["serverTotal"] = total
		target["hasMore"] = opts.skip+serverReturned < total
		if opts.skip+serverReturned < total {
			target["nextCursor"] = opts.skip + serverReturned
		} else {
			target["nextCursor"] = nil
		}
	} else {
		hasMore := opts.take > 0 && serverReturned >= opts.take
		target["hasMore"] = hasMore
		if hasMore {
			target["nextCursor"] = opts.skip + serverReturned
		} else {
			target["nextCursor"] = nil
		}
	}
	if opts.hasPostFilters {
		target["filteredOut"] = serverReturned - filteredReturned
	}
	if opts.from != nil || opts.to != nil {
		window := map[string]any{}
		if opts.from != nil {
			window["from"] = opts.from.Format(time.RFC3339Nano)
		}
		if opts.to != nil {
			window["to"] = opts.to.Format(time.RFC3339Nano)
		}
		target["window"] = window
	}
}

func numericAny(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		parsed, err := strconv.Atoi(value)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func copyAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	text, ok := raw.(string)
	if ok {
		return text
	}
	return fmt.Sprint(raw)
}

func matchesAnyFold(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func containsFold(value string, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func parseLogRedaction(raw string, useDefault bool) (logRedactionOptions, error) {
	var opts logRedactionOptions
	if useDefault {
		opts = logRedactionOptions{emails: true, secrets: true, tokens: true}
	}
	for _, part := range strings.Split(raw, ",") {
		mode := strings.ToLower(strings.TrimSpace(part))
		if mode == "" {
			continue
		}
		switch mode {
		case "all", "default":
			opts = logRedactionOptions{emails: true, secrets: true, tokens: true}
		case "email", "emails":
			opts.emails = true
		case "secret", "secrets":
			opts.secrets = true
		case "token", "tokens":
			opts.tokens = true
		default:
			return opts, fmt.Errorf("--redact mode %q is not supported; use emails,secrets,tokens,all", part)
		}
	}
	return opts, nil
}

var (
	logEmailPattern            = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	logBearerTokenPattern      = regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/=-]+`)
	logSecretAssignmentPattern = regexp.MustCompile(`(?i)("?(?:access_token|refresh_token|id_token|client_secret|password|secret|api[_-]?key|authorization)"?\s*[:=]\s*"?)[^",}\s]+`)
)

func redactLogValue(value any, opts logRedactionOptions) any {
	if !opts.enabled() {
		return value
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		propertyName := stringValue(typed["name"])
		for key, item := range typed {
			if (logSensitiveKey(key) || (key == "value" && logSensitiveKey(propertyName))) && (opts.secrets || opts.tokens) {
				result[key] = "[redacted]"
				continue
			}
			result[key] = redactLogValue(item, opts)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = redactLogValue(item, opts)
		}
		return result
	case string:
		return redactLogString(typed, opts)
	default:
		return value
	}
}

func redactLogString(value string, opts logRedactionOptions) string {
	result := value
	if opts.emails {
		result = logEmailPattern.ReplaceAllString(result, "[redacted-email]")
	}
	if opts.tokens {
		result = logBearerTokenPattern.ReplaceAllString(result, "Bearer [redacted-token]")
	}
	if opts.secrets || opts.tokens {
		result = logSecretAssignmentPattern.ReplaceAllString(result, `${1}[redacted]`)
	}
	return result
}

func logSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	if normalized == "token_type" {
		return false
	}
	return normalized == "token" ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "access_token") ||
		strings.Contains(normalized, "refresh_token") ||
		strings.Contains(normalized, "id_token") ||
		strings.Contains(normalized, "api_key") ||
		strings.Contains(normalized, "apikey")
}

func friendlyLogViewerError(err error) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		return err
	}
	if !strings.Contains(fmt.Sprint(apiErr.Payload), "CancelledByLogsSizeValidation") {
		return err
	}
	return fmt.Errorf("log query time range is too large for Umbraco's log size guard (CancelledByLogsSizeValidation); try a narrower --from/--to window")
}
