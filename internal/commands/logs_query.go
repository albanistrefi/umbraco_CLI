package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdkit"
)

// Query-side helpers for the logs command group: flag structs, --params
// normalization, date-range paging, and runtime filter resolution.

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
	parsed, err := cmdkit.ParseParams(raw)
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
