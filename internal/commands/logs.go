package commands

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	var level string
	var filterExpression string
	var from string
	var to string
	var skip int
	var take int

	cmd := &cobra.Command{Use: "list", Short: "List log entries", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := logParamsFromFlags(paramsRaw, logQueryFlags{
			level:            level,
			filterExpression: filterExpression,
			from:             from,
			to:               to,
			skip:             skip,
			take:             take,
		})
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
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Filter params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	cmd.Flags().StringVar(&level, "level", "", "Log level")
	cmd.Flags().StringVar(&filterExpression, "filter-expression", "", "Serilog filter expression")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
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
	var filterExpression string
	var level string
	var from string
	var to string
	var skip int
	var take int
	cmd := &cobra.Command{Use: "search", Short: "Search logs", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := logParamsFromFlags(paramsRaw, logQueryFlags{
			level:            level,
			filterExpression: filterExpression,
			from:             from,
			to:               to,
			skip:             skip,
			take:             take,
		})
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
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	cmd.Flags().StringVar(&filterExpression, "filter-expression", "", "Serilog filter expression")
	cmd.Flags().StringVar(&level, "level", "", "Log level")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
}

type logQueryFlags struct {
	level            string
	filterExpression string
	from             string
	to               string
	skip             int
	take             int
}

func logParamsFromFlags(raw string, flags logQueryFlags) (map[string]any, error) {
	params, err := parseParams(raw)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{}
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
		return params, nil
	}

	return normalizeLogParams(params), nil
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
