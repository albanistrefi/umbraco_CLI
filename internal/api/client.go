package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

type RequestOptions struct {
	// Fields is kept on request options for command-level projection metadata.
	// It is intentionally not sent as a query parameter because some
	// Management API endpoints reject otherwise valid requests when fields is
	// present.
	Fields string
	Params map[string]any
	DryRun bool
	// APIPrefix overrides the default "/umbraco/management/api/v1" base path
	// when non-empty. Used by command surfaces that target a different
	// Management API mount (e.g. Umbraco Forms at
	// "/umbraco/forms/management/api/v1").
	APIPrefix string
}

const defaultAPIPrefix = "/umbraco/management/api/v1"

// JoinPath builds a request path from a format string and arguments, escaping
// each argument so user-supplied values cannot introduce path segments or
// traversal ("../id" becomes a single literal segment, not a route rewrite).
func JoinPath(format string, args ...string) string {
	escaped := make([]any, len(args))
	for i, arg := range args {
		escaped[i] = escapePathSegment(arg)
	}
	return fmt.Sprintf(format, escaped...)
}

// escapePathSegment escapes one path segment. url.PathEscape leaves dots
// untouched (they are unreserved), so a segment that is entirely dots —
// "." or ".." — would survive as a relative-path segment that proxies and
// servers normalize into a route rewrite; those are percent-encoded
// explicitly.
func escapePathSegment(segment string) string {
	if strings.Trim(segment, ".") == "" && segment != "" {
		return strings.Repeat("%2E", len(segment))
	}
	return url.PathEscape(segment)
}

type DryRunResult struct {
	DryRun bool   `json:"dryRun"`
	Valid  bool   `json:"valid"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   any    `json:"body"`
}

type Client struct {
	cfg           config.Config
	httpClient    *http.Client
	tokenProvider *auth.Provider
	// initErr, when non-nil, fails every request with the startup problem
	// (e.g. config resolution). Carrying it here keeps informational
	// commands (--help, --version, schema) working on a broken setup while
	// any command that actually needs the API surfaces the real cause.
	initErr error
}

// NewUnavailableClient returns a client whose every request fails with err.
func NewUnavailableClient(err error) *Client {
	return &Client{initErr: err}
}

type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Payload    any
	Hint       string
}

func (e *APIError) Error() string {
	encoded, _ := json.Marshal(e.Payload)
	if e.Method != "" || e.Path != "" {
		if e.Hint != "" {
			return fmt.Sprintf("API %d %s %s: %s. Hint: %s", e.StatusCode, e.Method, e.Path, encoded, e.Hint)
		}
		return fmt.Sprintf("API %d %s %s: %s", e.StatusCode, e.Method, e.Path, encoded)
	}
	if e.Hint != "" {
		return fmt.Sprintf("API %d: %s. Hint: %s", e.StatusCode, encoded, e.Hint)
	}
	return fmt.Sprintf("API %d: %s", e.StatusCode, encoded)
}

func NewClient(cfg config.Config, httpClient *http.Client, tokenProvider *auth.Provider) *Client {
	return &Client{cfg: cfg, httpClient: httpClient, tokenProvider: tokenProvider}
}

func (c *Client) buildURL(path string, opts RequestOptions) (string, error) {
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}

	base, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return "", err
	}
	prefix := opts.APIPrefix
	if prefix == "" {
		prefix = defaultAPIPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")

	// Track the escaped form alongside the decoded form so percent-escapes
	// produced by JoinPath survive into the request URI instead of being
	// re-encoded or collapsed back into path separators.
	rawPath := strings.TrimRight(base.EscapedPath(), "/") + prefix + normalizedPath
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid request path %q: %w", rawPath, err)
	}
	base.Path = decodedPath
	base.RawPath = rawPath

	query := base.Query()
	if opts.Params != nil {
		for key, raw := range opts.Params {
			if raw == nil {
				continue
			}
			switch value := raw.(type) {
			case []any:
				for _, item := range value {
					query.Add(key, fmt.Sprint(item))
				}
			default:
				query.Add(key, fmt.Sprint(value))
			}
		}
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func parseResponse(resp *http.Response) (any, error) {
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}

	if strings.Contains(contentType, "application/json") {
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err == nil {
		return payload, nil
	}
	return string(body), nil
}

func (c *Client) Request(ctx context.Context, method string, path string, body any, opts RequestOptions) (any, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	fullURL, err := c.buildURL(path, opts)
	if err != nil {
		return nil, err
	}
	relativePath := c.relativeAPIPath(fullURL)

	if opts.DryRun {
		return DryRunResult{
			DryRun: true,
			Valid:  true,
			Method: method,
			Path:   relativePath,
			Body:   body,
		}, nil
	}

	var encodedBody []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		encodedBody = encoded
	}

	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 4; attempt++ {
		var reqBody io.Reader
		if encodedBody != nil {
			reqBody = bytes.NewReader(encodedBody)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			retryDelay := retryAfterDelay(resp.Header.Get("Retry-After"), attempt)
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt < 3 && c.tokenProvider != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			c.tokenProvider.Invalidate()
			token, err = c.tokenProvider.AccessToken(ctx)
			if err != nil {
				return nil, err
			}
			continue
		}

		result, err := parseResponse(resp)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Method:     method,
				Path:       relativePath,
				Payload:    result,
				Hint:       buildAPIErrorHint(resp.StatusCode, method, relativePath),
			}
		}

		result = mergeLocationID(result, resp.Header.Get("Location"))
		return result, nil
	}

	return nil, fmt.Errorf("request retry budget exhausted")
}

func (c *Client) relativeAPIPath(fullURL string) string {
	return strings.TrimPrefix(fullURL, strings.TrimRight(c.cfg.BaseURL, "/"))
}

func buildAPIErrorHint(statusCode int, method string, path string) string {
	if statusCode != http.StatusNotFound {
		return ""
	}
	if !strings.Contains(path, "/management/api/v") {
		return ""
	}

	return fmt.Sprintf("endpoint %s %s was not found; this may not be supported in your Umbraco version or may require a different route", method, path)
}

func (c *Client) Get(ctx context.Context, path string, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodGet, path, nil, opts)
}

func mergeLocationID(result any, location string) any {
	id := idFromLocation(location)
	if id == "" {
		return result
	}
	if result == nil {
		return map[string]any{"id": id}
	}
	payload, ok := result.(map[string]any)
	if !ok {
		return result
	}
	if existing, ok := payload["id"].(string); ok && strings.TrimSpace(existing) != "" {
		return result
	}
	next := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		next[key] = value
	}
	next["id"] = id
	return next
}

func idFromLocation(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	if parsed, err := url.Parse(location); err == nil {
		location = parsed.Path
	}
	location = strings.TrimRight(location, "/")
	if location == "" {
		return ""
	}
	segments := strings.Split(location, "/")
	return segments[len(segments)-1]
}

func (c *Client) Post(ctx context.Context, path string, body any, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodPost, path, body, opts)
}

func (c *Client) MultipartPost(ctx context.Context, path string, fields map[string]string, fileField string, filePath string, opts RequestOptions) (any, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	fullURL, err := c.buildURL(path, opts)
	if err != nil {
		return nil, err
	}
	relativePath := c.relativeAPIPath(fullURL)

	if opts.DryRun {
		return DryRunResult{
			DryRun: true,
			Valid:  true,
			Method: http.MethodPost,
			Path:   relativePath,
			Body: map[string]any{
				"fields": fields,
				"file":   filePath,
			},
		}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	result, err := parseResponse(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Method:     http.MethodPost,
			Path:       relativePath,
			Payload:    result,
			Hint:       buildAPIErrorHint(resp.StatusCode, http.MethodPost, relativePath),
		}
	}
	return result, nil
}

func (c *Client) Put(ctx context.Context, path string, body any, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodPut, path, body, opts)
}

func (c *Client) Delete(ctx context.Context, path string, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodDelete, path, nil, opts)
}

func retryAfterDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(header); err == nil {
		delay := time.Until(retryAt)
		if delay > 0 {
			return delay
		}
	}

	delay := 200 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
