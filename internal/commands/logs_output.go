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

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// Output-side helpers for the logs command group: client-side entry
// filtering, flattening, counting, pagination metadata, and redaction.

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

	if len(opts.levels) > 0 && !matchesAnyFold(cmdkit.StringValue(entry["level"]), opts.levels) {
		return false
	}

	properties := logPropertiesMap(entry)
	if opts.sourceContext != "" && !containsFold(cmdkit.StringValue(properties["SourceContext"]), opts.sourceContext) {
		return false
	}
	if opts.path != "" && !containsFold(cmdkit.StringValue(properties["RequestPath"]), opts.path) {
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
	raw := cmdkit.StringValue(entry["timestamp"])
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := parseLogTime(raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
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
		name := cmdkit.StringValue(propertyMap["name"])
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
		text := cmdkit.StringValue(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func logCorrelationMatches(properties map[string]any, needle string) bool {
	for _, key := range []string{"CorrelationId", "CorrelationID", "RequestId", "HttpRequestId", "TraceId", "SpanId"} {
		if containsFold(cmdkit.StringValue(properties[key]), needle) {
			return true
		}
	}
	return false
}

func logEntryContains(entry map[string]any, properties map[string]any, needle string) bool {
	for _, key := range []string{"timestamp", "level", "message", "renderedMessage", "messageTemplate", "exception"} {
		if containsFold(cmdkit.StringValue(entry[key]), needle) {
			return true
		}
	}
	for key, value := range properties {
		if containsFold(key, needle) || containsFold(cmdkit.StringValue(value), needle) {
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
		return cmdkit.StringValue(entry["level"])
	case "source":
		return cmdkit.StringValue(logPropertiesMap(entry)["SourceContext"])
	case "path":
		return cmdkit.StringValue(logPropertiesMap(entry)["RequestPath"])
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
		propertyName := cmdkit.StringValue(typed["name"])
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
