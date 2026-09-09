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
	var headerFlags []string
	var formFlags []string
	var rawPath bool

	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Call an authenticated raw Umbraco Management API endpoint",
		Long: "Call a core Umbraco Management API endpoint that does not have a curated CLI command yet.\n\n" +
			"Pass paths relative to /umbraco/management/api/v1, for example /item/document/ancestors?id=a&id=b.\n" +
			"Full Management API paths are also accepted and normalized to the core API root.\n\n" +
			"--raw-path sends the path relative to the host root instead (any endpoint on the same host, e.g. /umbraco/automate/management/api/v1/automations or /media/abc/logo.svg).\n" +
			"--form field=value / field=@path sends multipart/form-data instead of JSON (e.g. POST /temporary-file with --form id=<uuid> --form file=@./logo.svg).\n" +
			"--header 'Key: Value' adds or overrides request headers. Every request already carries User-Agent umbraco-cli/<version>.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method, err := normalizeAPIMethod(args[0])
			if err != nil {
				return err
			}
			path, params, err := parseAPIRequestPathMode(args[1], rawPath)
			if err != nil {
				return err
			}
			body, err := parseAPIBody(bodyRaw)
			if err != nil {
				return err
			}
			headers, err := parseAPIHeaders(headerFlags)
			if err != nil {
				return err
			}
			fields, files, err := parseAPIForm(formFlags)
			if err != nil {
				return err
			}
			opts := managementapi.RequestOptions{
				Params:  params,
				DryRun:  dryRun,
				RawPath: rawPath,
				Headers: headers,
			}

			var result managementapi.ResponseResult
			if len(formFlags) > 0 {
				if body != nil {
					return fmt.Errorf("--form and --body are mutually exclusive")
				}
				if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
					return fmt.Errorf("--form requires POST, PUT, or PATCH")
				}
				result, err = deps.Client.MultipartResult(cmd.Context(), method, path, fields, files, opts)
			} else {
				result, err = deps.Client.RequestResult(cmd.Context(), method, path, body, opts)
			}
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
	cmd.Flags().StringArrayVar(&headerFlags, "header", nil, "Extra request header as 'Key: Value' (repeatable)")
	cmd.Flags().StringArrayVar(&formFlags, "form", nil, "Multipart form field as field=value or field=@path for a file (repeatable; replaces the JSON body)")
	cmd.Flags().BoolVar(&rawPath, "raw-path", false, "Send the path relative to the host root instead of /umbraco/management/api/v1")
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

// parseAPIHeaders turns repeated "Key: Value" flags into a header map.
func parseAPIHeaders(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	headers := make(map[string]string, len(raw))
	for _, entry := range raw {
		key, value, ok := strings.Cut(entry, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --header %q; expected 'Key: Value'", entry)
		}
		headers[key] = strings.TrimSpace(value)
	}
	return headers, nil
}

// parseAPIForm splits repeated field=value / field=@path flags into text
// fields and file fields.
func parseAPIForm(raw []string) (map[string]string, map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	fields := map[string]string{}
	files := map[string]string{}
	for _, entry := range raw {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, nil, fmt.Errorf("invalid --form %q; expected field=value or field=@path", entry)
		}
		if strings.HasPrefix(value, "@") {
			path := strings.TrimSpace(strings.TrimPrefix(value, "@"))
			if path == "" {
				return nil, nil, fmt.Errorf("invalid --form %q; file path after @ cannot be empty", entry)
			}
			if _, err := os.Stat(path); err != nil {
				return nil, nil, fmt.Errorf("--form %s: %w", key, err)
			}
			files[key] = path
			continue
		}
		fields[key] = value
	}
	return fields, files, nil
}

// parseAPIRequestPathMode parses the path argument. In raw mode the path is
// kept host-relative (no Management API prefix stripping or re-rooting).
func parseAPIRequestPathMode(raw string, rawMode bool) (string, map[string]any, error) {
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
	if !rawMode {
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
