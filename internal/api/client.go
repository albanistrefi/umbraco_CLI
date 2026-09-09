package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/version"
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
	// RawPath, when true, sends the path relative to the host root instead of
	// the Management API mount. Used by `api --raw-path` and by media
	// downloads, which fetch /media/... assets outside the API.
	RawPath bool
	// Headers are extra request headers applied after the defaults, so a
	// caller can override Content-Type or add e.g. a custom User-Agent.
	Headers map[string]string
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

type ResponseResult struct {
	StatusCode int
	Body       any
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

// maxErrorPayloadBytes caps the response payload rendered into error
// messages. The full payload stays available on APIError.Payload; the cap
// only keeps pathological error bodies (HTML error pages, huge validation
// payloads) from flooding terminals and agent context windows.
const maxErrorPayloadBytes = 500

func (e *APIError) Error() string {
	encoded, _ := json.Marshal(e.Payload)
	encoded = truncatePayload(encoded, maxErrorPayloadBytes)
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

func truncatePayload(encoded []byte, limit int) []byte {
	if len(encoded) <= limit {
		return encoded
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(encoded[cut]) {
		cut--
	}
	return append(encoded[:cut:cut], []byte("…(truncated)")...)
}

// ExitCode marks API error responses for the CLI's documented exit-code
// contract (4 = the Management API answered with an error status).
func (e *APIError) ExitCode() int { return 4 }

func NewClient(cfg config.Config, httpClient *http.Client, tokenProvider *auth.Provider) *Client {
	return &Client{cfg: cfg, httpClient: httpClient, tokenProvider: tokenProvider}
}

func (c *Client) ReplaceWith(next *Client) {
	if c == nil || next == nil {
		return
	}
	*c = *next
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
	if opts.RawPath {
		prefix = ""
	}

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
	defer func() { _ = resp.Body.Close() }()
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
	result, err := c.RequestResult(ctx, method, path, body, opts)
	return result.Body, err
}

func (c *Client) RequestResult(ctx context.Context, method string, path string, body any, opts RequestOptions) (ResponseResult, error) {
	if c.initErr != nil {
		return ResponseResult{}, c.initErr
	}

	fullURL, err := c.buildURL(path, opts)
	if err != nil {
		return ResponseResult{}, err
	}
	relativePath := c.relativeAPIPath(fullURL)

	if opts.DryRun {
		return ResponseResult{Body: DryRunResult{
			DryRun: true,
			Valid:  true,
			Method: method,
			Path:   relativePath,
			Body:   body,
		}}, nil
	}

	var encodedBody []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return ResponseResult{}, err
		}
		encodedBody = encoded
	}

	resp, err := c.send(ctx, method, fullURL, "application/json", opts.Headers, func() io.Reader {
		if encodedBody == nil {
			return nil
		}
		return bytes.NewReader(encodedBody)
	})
	if err != nil {
		return ResponseResult{}, err
	}

	result, err := parseResponse(resp)
	if err != nil {
		return ResponseResult{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResponseResult{StatusCode: resp.StatusCode, Body: result}, &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       relativePath,
			Payload:    result,
			Hint:       buildAPIErrorHint(resp.StatusCode, method, relativePath),
		}
	}

	result = mergeLocationID(result, resp.Header.Get("Location"))
	return ResponseResult{StatusCode: resp.StatusCode, Body: result}, nil
}

const maxRequestAttempts = 4

// send executes an authenticated request, retrying rate limits (429) with
// Retry-After/backoff and refreshing the token once per 401, within a fixed
// attempt budget. makeBody must return a fresh reader per call so retries
// never replay a consumed reader.
func (c *Client) send(ctx context.Context, method string, fullURL string, contentType string, headers map[string]string, makeBody func() io.Reader) (*http.Response, error) {
	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < maxRequestAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, makeBody())
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("User-Agent", version.UserAgent())
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRequestAttempts-1 {
			retryDelay := retryAfterDelay(resp.Header.Get("Retry-After"), attempt)
			drainAndClose(resp)
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt < maxRequestAttempts-1 && c.tokenProvider != nil {
			drainAndClose(resp)
			c.tokenProvider.Invalidate()
			token, err = c.tokenProvider.AccessToken(ctx)
			if err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request retry budget exhausted")
}

func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
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

// MultipartPost uploads a single file field plus text fields with POST. It is
// a thin wrapper over MultipartRequest kept for the media upload workflow.
func (c *Client) MultipartPost(ctx context.Context, path string, fields map[string]string, fileField string, filePath string, opts RequestOptions) (any, error) {
	return c.MultipartRequest(ctx, http.MethodPost, path, fields, map[string]string{fileField: filePath}, opts)
}

// MultipartRequest sends a multipart/form-data body with the given text
// fields and file fields (form field name → local path). The form is fully
// buffered so retries can replay it from a fresh reader per attempt.
func (c *Client) MultipartRequest(ctx context.Context, method string, path string, fields map[string]string, files map[string]string, opts RequestOptions) (any, error) {
	result, err := c.MultipartResult(ctx, method, path, fields, files, opts)
	return result.Body, err
}

// MultipartResult is MultipartRequest with the HTTP status code preserved.
func (c *Client) MultipartResult(ctx context.Context, method string, path string, fields map[string]string, files map[string]string, opts RequestOptions) (ResponseResult, error) {
	if c.initErr != nil {
		return ResponseResult{}, c.initErr
	}

	fullURL, err := c.buildURL(path, opts)
	if err != nil {
		return ResponseResult{}, err
	}
	relativePath := c.relativeAPIPath(fullURL)

	if opts.DryRun {
		return ResponseResult{Body: DryRunResult{
			DryRun: true,
			Valid:  true,
			Method: method,
			Path:   relativePath,
			Body: map[string]any{
				"fields": fields,
				"files":  files,
			},
		}}, nil
	}

	var buffered bytes.Buffer
	writer := multipart.NewWriter(&buffered)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return ResponseResult{}, err
		}
	}
	for field, filePath := range files {
		if err := writeMultipartFile(writer, field, filePath); err != nil {
			return ResponseResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return ResponseResult{}, err
	}

	encodedBody := buffered.Bytes()
	resp, err := c.send(ctx, method, fullURL, writer.FormDataContentType(), opts.Headers, func() io.Reader {
		return bytes.NewReader(encodedBody)
	})
	if err != nil {
		return ResponseResult{}, err
	}
	result, err := parseResponse(resp)
	if err != nil {
		return ResponseResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResponseResult{StatusCode: resp.StatusCode, Body: result}, &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       relativePath,
			Payload:    result,
			Hint:       buildAPIErrorHint(resp.StatusCode, method, relativePath),
		}
	}
	return ResponseResult{StatusCode: resp.StatusCode, Body: mergeLocationID(result, resp.Header.Get("Location"))}, nil
}

func writeMultipartFile(writer *multipart.Writer, field string, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	part, err := writer.CreateFormFile(field, filepath.Base(filePath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
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
	// Up to half the base delay of jitter so concurrent commands rate-limited
	// together do not retry in lockstep.
	return delay + rand.N(delay/2)
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

// GetBytes fetches a resource verbatim (no JSON decoding) and returns the
// body with its Content-Type. Used for media asset downloads, which live
// outside the Management API (pair with RequestOptions{RawPath: true}).
// For large assets prefer GetStream, which does not buffer the body.
func (c *Client) GetBytes(ctx context.Context, path string, opts RequestOptions) ([]byte, string, error) {
	var buffered bytes.Buffer
	result, err := c.GetStream(ctx, path, &buffered, opts)
	if err != nil {
		return nil, "", err
	}
	return buffered.Bytes(), result.ContentType, nil
}

// StreamResult describes a completed GetStream.
type StreamResult struct {
	StatusCode  int
	ContentType string
	Bytes       int64
}

// GetStream fetches a resource verbatim and copies the body straight into w,
// so a large binary never has to fit in memory. Non-2xx responses are read
// (bounded) into an APIError instead of being written to w.
func (c *Client) GetStream(ctx context.Context, path string, w io.Writer, opts RequestOptions) (StreamResult, error) {
	if c.initErr != nil {
		return StreamResult{}, c.initErr
	}
	fullURL, err := c.buildURL(path, opts)
	if err != nil {
		return StreamResult{}, err
	}
	relativePath := c.relativeAPIPath(fullURL)
	resp, err := c.send(ctx, http.MethodGet, fullURL, "application/json", opts.Headers, func() io.Reader { return nil })
	if err != nil {
		return StreamResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return StreamResult{StatusCode: resp.StatusCode}, &APIError{
			StatusCode: resp.StatusCode,
			Method:     http.MethodGet,
			Path:       relativePath,
			Payload:    strings.TrimSpace(string(body)),
			Hint:       buildAPIErrorHint(resp.StatusCode, http.MethodGet, relativePath),
		}
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return StreamResult{}, err
	}
	return StreamResult{StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Bytes: n}, nil
}
