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
)

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
}

func New(cfg config.Config, client *http.Client) *Provider {
	return &Provider{cfg: cfg, httpClient: client}
}

func (p *Provider) Invalidate() {
	p.mu.Lock()
	p.cached = ""
	p.expiresAt = time.Time{}
	p.mu.Unlock()
}

func (p *Provider) AccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.cached != "" && time.Now().Before(p.expiresAt) {
		defer p.mu.Unlock()
		return p.cached, nil
	}
	p.mu.Unlock()

	if err := p.cfg.ValidateAuth(); err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("client_id", p.cfg.ClientID)
	values.Set("client_secret", p.cfg.ClientSecret)

	endpoint := fmt.Sprintf("%s/umbraco/management/api/v1/security/back-office/token", p.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request to %s failed (resolved base URL %s): %w", endpoint, p.cfg.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth failed for %s (resolved base URL %s): %d %s", endpoint, p.cfg.BaseURL, resp.StatusCode, string(body))
	}

	var payload tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" || payload.ExpiresIn <= 0 {
		return "", fmt.Errorf("auth failed: token response missing required fields")
	}

	expiresAt := time.Now().Add(time.Duration(payload.ExpiresIn)*time.Second - time.Minute)
	p.mu.Lock()
	p.cached = payload.AccessToken
	p.expiresAt = expiresAt
	p.mu.Unlock()

	return payload.AccessToken, nil
}
