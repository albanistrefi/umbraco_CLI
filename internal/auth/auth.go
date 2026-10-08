package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"umbraco-cli/internal/config"
	"umbraco-cli/internal/version"
)

// Error marks authentication and credential failures so the CLI can exit
// with a distinct code (3) — scripts can tell "fix your credentials" apart
// from API or usage errors.
type Error struct {
	Err error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// ExitCode implements the CLI's documented exit-code contract.
func (e *Error) ExitCode() int { return 3 }

func authErr(err error) error {
	if err == nil {
		return nil
	}
	return &Error{Err: err}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type Provider struct {
	cfg        config.Config
	httpClient *http.Client

	mu        sync.Mutex
	cached    string
	expiresAt time.Time
	inflight  *tokenFetch
}

// tokenFetch is one token request in flight. Callers that need a token
// while it runs wait for it instead of sending their own.
type tokenFetch struct {
	done  chan struct{}
	token string
	err   error
}

// tokenFetchTimeout bounds a token request whose HTTP client sets no
// timeout of its own.
const tokenFetchTimeout = time.Minute

func New(cfg config.Config, client *http.Client) *Provider {
	return &Provider{cfg: cfg, httpClient: client}
}

func (p *Provider) Invalidate() {
	p.mu.Lock()
	p.cached = ""
	p.expiresAt = time.Time{}
	p.mu.Unlock()
}

// InvalidateIfCurrent drops the cached token only while it is still token,
// the one a rejected request was sent with. A 401 for a request that went
// out with an older token must not discard the token a parallel request has
// just fetched, which would start another (slow) token request.
func (p *Provider) InvalidateIfCurrent(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached == token {
		p.cached = ""
		p.expiresAt = time.Time{}
	}
}

// AccessToken returns the cached token, or waits for a fresh one as long as
// ctx allows. The request for it runs detached from the caller that
// started it: a caller that stops waiting (a poll's request timeout) does
// not cancel it, so the next caller finds the token cached instead of
// starting over against a token endpoint slower than its timeout, as
// Umbraco's is while it starts up (9s on the 2026-10-08 live deploy).
func (p *Provider) AccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.cached != "" && time.Now().Before(p.expiresAt) {
		token := p.cached
		p.mu.Unlock()
		return token, nil
	}
	if err := p.cfg.ValidateAuth(); err != nil {
		p.mu.Unlock()
		return "", authErr(err)
	}
	fetch := p.inflight
	if fetch == nil {
		fetch = &tokenFetch{done: make(chan struct{})}
		p.inflight = fetch
		go p.fetch(context.WithoutCancel(ctx), fetch)
	}
	p.mu.Unlock()

	select {
	case <-fetch.done:
		return fetch.token, fetch.err
	case <-ctx.Done():
		return "", authErr(fmt.Errorf("auth request to %s failed (resolved base URL %s): %w", p.endpoint(), p.cfg.BaseURL, ctx.Err()))
	}
}

func (p *Provider) endpoint() string {
	return fmt.Sprintf("%s/umbraco/management/api/v1/security/back-office/token", p.cfg.BaseURL)
}

// fetch requests a token, caches it, and hands the result to every caller
// waiting on fetch.
func (p *Provider) fetch(ctx context.Context, fetch *tokenFetch) {
	ctx, cancel := context.WithTimeout(ctx, tokenFetchTimeout)
	defer cancel()
	token, expiresAt, err := p.request(ctx)
	p.mu.Lock()
	if err == nil {
		p.cached = token
		p.expiresAt = expiresAt
	}
	p.inflight = nil
	p.mu.Unlock()
	fetch.token, fetch.err = token, err
	close(fetch.done)
}

func (p *Provider) request(ctx context.Context) (string, time.Time, error) {
	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("client_id", p.cfg.ClientID)
	values.Set("client_secret", p.cfg.ClientSecret)

	endpoint := p.endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, authErr(fmt.Errorf("auth request to %s failed (resolved base URL %s): %w", endpoint, p.cfg.BaseURL, err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, authErr(fmt.Errorf("auth failed for %s (resolved base URL %s): %d %s", endpoint, p.cfg.BaseURL, resp.StatusCode, string(body)))
	}

	var payload tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", time.Time{}, authErr(fmt.Errorf("auth token response from %s is not valid JSON: %w", endpoint, err))
	}
	if payload.AccessToken == "" || payload.ExpiresIn <= 0 {
		return "", time.Time{}, authErr(fmt.Errorf("auth failed: token response missing required fields"))
	}

	return payload.AccessToken, time.Now().Add(time.Duration(payload.ExpiresIn)*time.Second - time.Minute), nil
}
