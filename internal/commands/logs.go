package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

const (
	logViewerMessageTemplatePath        = "/log-viewer/message-template"
	logViewerLegacyListPath             = "/log-viewer"
	logViewerLegacySearchPath           = "/log-viewer/search"
	logViewerLegacyMessageTemplatesPath = "/log-viewer/templates"
)

func RegisterLogs(root *cobra.Command, deps cmdkit.Dependencies) {
	logs := &cobra.Command{Use: "logs", Short: "Log and diagnostics operations"}
	logs.AddCommand(logsList(deps))
	logs.AddCommand(logsLevels(deps))
	logs.AddCommand(logsLevelCount(deps))
	logs.AddCommand(logsTemplates(deps))
	logs.AddCommand(logsSearch(deps))
	logs.AddCommand(logsTail(deps))
	logs.AddCommand(logsErrors(deps))
	root.AddCommand(logs)
}

func logsList(deps cmdkit.Dependencies) *cobra.Command {
	var paramsRaw string
	var flags logQueryFlags
	flags.skip = -1
	flags.take = -1
	flags.minutes = 5

	cmd := &cobra.Command{Use: "list", Short: "List log entries", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		params, runtime, err := logParamsFromFlags(paramsRaw, flags)
		if err != nil {
			return err
		}
		result, err := cmdkit.GetWithFallback(
			cmd.Context(),
			deps.Client,
			cmdkit.GetRequestCandidate{Path: cmdkit.LogViewerLogPath, Opts: api.RequestOptions{Params: params}},
			cmdkit.GetRequestCandidate{Path: logViewerLegacyListPath, Opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		result, err = shapeLogResult(result, runtime)
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Filter params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	addLogQueryFlags(cmd, &flags)
	return cmd
}

func logsLevels(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{Use: "levels", Short: "List log levels", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("logs levels is not available in the Umbraco v17 Management API; use logs list --level <level> or logs list --filter-expression <expression>")
	}}
}

func logsLevelCount(deps cmdkit.Dependencies) *cobra.Command {
	var paramsRaw string
	var from string
	var to string
	cmd := &cobra.Command{Use: "level-count", Short: "Get count per level", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		params, err := cmdkit.ParseParams(paramsRaw)
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
		return cmdkit.PrintResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Filter params as JSON")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	return cmd
}

func logsTemplates(deps cmdkit.Dependencies) *cobra.Command {
	var from string
	var to string
	var skip int
	var take int
	cmd := &cobra.Command{Use: "templates", Short: "List paginated log message templates", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		params := logDateRangePagingParams(from, to, skip, take)
		result, err := cmdkit.GetWithFallback(
			cmd.Context(),
			deps.Client,
			cmdkit.GetRequestCandidate{Path: logViewerMessageTemplatePath, Opts: api.RequestOptions{Params: params}},
			cmdkit.GetRequestCandidate{Path: logViewerLegacyMessageTemplatesPath, Opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
}

func logsSearch(deps cmdkit.Dependencies) *cobra.Command {
	var paramsRaw string
	var flags logQueryFlags
	flags.skip = -1
	flags.take = -1
	flags.minutes = 5
	cmd := &cobra.Command{Use: "search", Short: "Search logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		params, runtime, err := logParamsFromFlags(paramsRaw, flags)
		if err != nil {
			return err
		}
		result, err := cmdkit.GetWithFallback(
			cmd.Context(),
			deps.Client,
			cmdkit.GetRequestCandidate{Path: cmdkit.LogViewerLogPath, Opts: api.RequestOptions{Params: params}},
			cmdkit.GetRequestCandidate{Path: logViewerLegacySearchPath, Opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return friendlyLogViewerError(err)
		}
		result, err = shapeLogResult(result, runtime)
		if err != nil {
			return err
		}
		return cmdkit.PrintResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel)")
	addLogQueryFlags(cmd, &flags)
	return cmd
}
