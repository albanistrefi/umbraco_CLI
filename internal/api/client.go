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
	"umbraco-cli/internal/validate"
)

type RequestOptions struct {
	Fields         string
	Params         map[string]any
	DryRun         bool
	SkipValidation bool
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
	base.Path = strings.TrimRight(base.Path, "/") + "/umbraco/management/api/v1" + normalizedPath

	query := base.Query()
	if opts.Fields != "" {
		if err := validate.String(opts.Fields); err != nil {
			return "", err
		}
		query.Set("fields", opts.Fields)
	}
	if opts.Params != nil {
		if err := validate.Input(opts.Params); err != nil {
			return "", err
		}
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
	if raw, ok := body.(map[string]any); ok && !opts.SkipValidation {
		if err := validate.Input(raw); err != nil {
			return nil, err
		}
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
	if !strings.HasPrefix(path, "/umbraco/management/api/v1/") {
		return ""
	}

	return fmt.Sprintf("endpoint %s %s was not found; this may not be supported in your Umbraco version or may require a different route", method, path)
}

func (c *Client) Get(ctx context.Context, path string, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodGet, path, nil, opts)
}

func (c *Client) Post(ctx context.Context, path string, body any, opts RequestOptions) (any, error) {
	return c.Request(ctx, http.MethodPost, path, body, opts)
}

func (c *Client) MultipartPost(ctx context.Context, path string, fields map[string]string, fileField string, filePath string, opts RequestOptions) (any, error) {
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
