package commands

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

type documentURLResult struct {
	ID       string            `json:"id"`
	URLInfos []documentURLInfo `json:"urlInfos"`
}

type documentURLInfo struct {
	Culture  any    `json:"culture"`
	URL      any    `json:"url"`
	Provider string `json:"provider"`
	Message  any    `json:"message"`
}

type documentURLRow struct {
	ID       string
	Culture  string
	URL      string
	Provider string
	Message  string
}

type documentURLsMissingError struct {
	IDs []string
}

func (e documentURLsMissingError) Error() string {
	if len(e.IDs) == 1 {
		return fmt.Sprintf("document %s has no published URL", e.IDs[0])
	}
	return fmt.Sprintf("documents have no published URL: %s", strings.Join(e.IDs, ", "))
}

func documentURLs(deps Dependencies) *cobra.Command {
	var culture string
	var absolute bool
	cmd := &cobra.Command{
		Use:   "urls <id> [<id>...]",
		Short: "Get published document URLs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := fetchDocumentURLs(cmd.Context(), deps, args, documentURLOptions{
				Culture:  culture,
				Absolute: absolute,
			})
			if err != nil {
				return err
			}

			format, err := resolveOutputFormat(deps)
			if err != nil {
				return err
			}

			missing := documentURLMissingIDs(args, results)
			switch format {
			case config.OutputPlain:
				err = printDocumentURLPlain(cmd, results)
			case config.OutputTable:
				err = printDocumentURLTable(cmd, flattenDocumentURLRows(results))
			case config.OutputJSON:
				err = printResult(cmd, deps, results)
			default:
				err = fmt.Errorf("unsupported output format: %s", format)
			}
			if err != nil {
				return err
			}
			if len(missing) > 0 {
				return documentURLsMissingError{IDs: missing}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&culture, "culture", "", "Only include URL info for the given culture; defaults to all cultures")
	cmd.Flags().BoolVar(&absolute, "absolute", false, "Resolve returned URLs against the configured site host")
	return cmd
}

type documentURLOptions struct {
	Culture  string
	Absolute bool
}

func fetchDocumentURLs(ctx context.Context, deps Dependencies, ids []string, opts documentURLOptions) ([]documentURLResult, error) {
	params := map[string]any{"id": stringSliceAsAny(ids)}
	raw, err := deps.Client.Get(ctx, "/document/urls", api.RequestOptions{Params: params})
	if err != nil {
		return nil, err
	}
	results, err := decodeResult[[]documentURLResult](raw)
	if err != nil {
		return nil, err
	}
	results = filterDocumentURLsByCulture(results, opts.Culture)
	if opts.Absolute {
		baseURL, err := configuredBaseURL(deps)
		if err != nil {
			return nil, err
		}
		results = absolutizeDocumentURLs(results, baseURL)
	}
	return results, nil
}

func attachDocumentURLs(ctx context.Context, deps Dependencies, id string, result any) (any, error) {
	results, err := fetchDocumentURLs(ctx, deps, []string{id}, documentURLOptions{})
	if err != nil {
		return nil, err
	}
	entry, ok := result.(map[string]any)
	if !ok {
		return result, nil
	}
	next := cloneAnyMap(entry)
	if len(results) == 0 {
		next["urls"] = []any{}
		return next, nil
	}
	urlInfos := make([]any, 0, len(results[0].URLInfos))
	for _, info := range results[0].URLInfos {
		urlInfos = append(urlInfos, map[string]any{
			"culture":  info.Culture,
			"url":      info.URL,
			"provider": info.Provider,
			"message":  info.Message,
		})
	}
	next["urls"] = urlInfos
	return next, nil
}

func stringSliceAsAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func filterDocumentURLsByCulture(results []documentURLResult, culture string) []documentURLResult {
	culture = strings.TrimSpace(culture)
	if culture == "" {
		return results
	}
	filtered := make([]documentURLResult, 0, len(results))
	for _, result := range results {
		next := result
		next.URLInfos = make([]documentURLInfo, 0, len(result.URLInfos))
		for _, info := range result.URLInfos {
			if cultureValue(info.Culture) == culture {
				next.URLInfos = append(next.URLInfos, info)
			}
		}
		filtered = append(filtered, next)
	}
	return filtered
}

func absolutizeDocumentURLs(results []documentURLResult, baseURL string) []documentURLResult {
	out := make([]documentURLResult, 0, len(results))
	for _, result := range results {
		next := result
		next.URLInfos = make([]documentURLInfo, 0, len(result.URLInfos))
		for _, info := range result.URLInfos {
			if rawURL, ok := info.URL.(string); ok {
				info.URL = absolutizeDocumentURL(baseURL, rawURL)
			}
			next.URLInfos = append(next.URLInfos, info)
		}
		out = append(out, next)
	}
	return out
}

func configuredBaseURL(deps Dependencies) (string, error) {
	if deps.ConfigOptionsProvider != nil {
		cfg, err := config.LoadWithOptions(deps.configOptions())
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(cfg.BaseURL) != "" {
			return cfg.BaseURL, nil
		}
	}
	if strings.TrimSpace(deps.Config.BaseURL) != "" {
		return deps.Config.BaseURL, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.BaseURL, nil
}

func absolutizeDocumentURL(baseURL string, rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.IsAbs() {
		return rawURL
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "//") {
		return base.Scheme + ":" + rawURL
	}
	if strings.HasPrefix(rawURL, "/") {
		return (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: rawURL}).String()
	}
	return (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/" + rawURL}).String()
}

func flattenDocumentURLRows(results []documentURLResult) []documentURLRow {
	rows := make([]documentURLRow, 0)
	for _, result := range results {
		for _, info := range result.URLInfos {
			rows = append(rows, documentURLRow{
				ID:       result.ID,
				Culture:  cultureValue(info.Culture),
				URL:      urlValue(info.URL),
				Provider: info.Provider,
				Message:  messageValue(info.Message),
			})
		}
	}
	return rows
}

func documentURLMissingIDs(requestedIDs []string, results []documentURLResult) []string {
	missing := make([]string, 0)
	byID := make(map[string]documentURLResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	for _, requestedID := range requestedIDs {
		result, ok := byID[requestedID]
		if !ok {
			missing = append(missing, requestedID)
			continue
		}
		hasURL := false
		for _, info := range result.URLInfos {
			if strings.TrimSpace(urlValue(info.URL)) != "" {
				hasURL = true
				break
			}
		}
		if !hasURL {
			missing = append(missing, result.ID)
		}
	}
	return missing
}

func printDocumentURLPlain(cmd *cobra.Command, results []documentURLResult) error {
	for _, result := range results {
		for _, info := range result.URLInfos {
			if printedURL := strings.TrimSpace(urlValue(info.URL)); printedURL != "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), printedURL); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func printDocumentURLTable(cmd *cobra.Command, rows []documentURLRow) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "id\tculture\turl\tprovider\tmessage")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.ID, row.Culture, row.URL, row.Provider, row.Message)
	}
	return tw.Flush()
}

func cultureValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func messageValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func urlValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func isDocumentURLsMissing(err error) bool {
	var missing documentURLsMissingError
	return errors.As(err, &missing)
}
