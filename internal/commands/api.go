package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	managementapi "umbraco-cli/internal/api"
)

func RegisterAPI(root *cobra.Command, deps Dependencies) {
	var bodyRaw string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Call an authenticated raw Umbraco Management API endpoint",
		Long: "Call a core Umbraco Management API endpoint that does not have a curated CLI command yet.\n\n" +
			"Pass paths relative to /umbraco/management/api/v1, for example /item/document/ancestors?id=a&id=b.\n" +
			"Full Management API paths are also accepted and normalized to the core API root.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method, err := normalizeAPIMethod(args[0])
			if err != nil {
				return err
			}
			path, params, err := parseAPIRequestPath(args[1])
			if err != nil {
				return err
			}
			body, err := parseAPIBody(bodyRaw)
			if err != nil {
				return err
			}

			result, err := deps.Client.RequestResult(cmd.Context(), method, path, body, managementapi.RequestOptions{
				Params: params,
				DryRun: dryRun,
			})
			if err != nil {
				var apiErr *managementapi.APIError
				if !errors.As(err, &apiErr) {
					return err
				}
				return printResult(cmd, deps, map[string]any{
					"ok":         false,
					"statusCode": apiErr.StatusCode,
					"method":     method,
					"path":       path,
					"params":     params,
					"body":       apiErr.Payload,
					"error":      apiErr.Error(),
				})
			}

			return printResult(cmd, deps, map[string]any{
				"ok":         true,
				"statusCode": result.StatusCode,
				"method":     method,
				"path":       path,
				"params":     params,
				"body":       result.Body,
			})
		},
	}

	cmd.Flags().StringVar(&bodyRaw, "body", "", "JSON request body, or @path to read JSON from a file")
	addDryRunFlag(cmd, &dryRun)
	root.AddCommand(cmd)
}

func normalizeAPIMethod(raw string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(raw))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return method, nil
	default:
		return "", fmt.Errorf("unsupported API method %q; use GET, POST, PUT, PATCH, DELETE, or HEAD", raw)
	}
}

func parseAPIRequestPath(raw string) (string, map[string]any, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil, fmt.Errorf("api path cannot be empty")
	}

	var parsed *url.URL
	var err error
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		parsed, err = url.Parse(value)
	} else {
		if !strings.HasPrefix(value, "/") {
			return "", nil, fmt.Errorf("api path must start with /")
		}
		parsed, err = url.ParseRequestURI(value)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid api path %q: %w", raw, err)
	}

	path := parsed.Path
	const apiPrefix = "/umbraco/management/api/v1"
	if strings.HasPrefix(path, apiPrefix) {
		path = strings.TrimPrefix(path, apiPrefix)
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return "", nil, fmt.Errorf("api path cannot contain relative segment %q", segment)
		}
	}

	params, err := parseAPIQuery(parsed.RawQuery)
	if err != nil {
		return "", nil, err
	}
	return path, params, nil
}

func parseAPIQuery(rawQuery string) (map[string]any, error) {
	if strings.TrimSpace(rawQuery) == "" {
		return nil, nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("invalid api query: %w", err)
	}
	params := make(map[string]any, len(values))
	for key, rawValues := range values {
		if len(rawValues) == 1 {
			params[key] = rawValues[0]
			continue
		}
		items := make([]any, 0, len(rawValues))
		for _, value := range rawValues {
			items = append(items, value)
		}
		params[key] = items
	}
	return params, nil
}

func parseAPIBody(raw string) (any, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(value, "@"))
		if path == "" {
			return nil, fmt.Errorf("--body @path cannot be empty")
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		value = string(payload)
	}
	var body any
	if err := json.Unmarshal([]byte(value), &body); err != nil {
		return nil, fmt.Errorf("invalid --body JSON: %w", err)
	}
	return body, nil
}
