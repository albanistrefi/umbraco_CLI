package deploy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/version"
)

// watchProbes gathers one observation per poll. Probe failures during a
// restart window are expected signals, not command errors.
type watchProbes struct {
	deps        cmdkit.Dependencies
	cfg         config.Config
	httpClient  *http.Client
	tokenURL    string
	publicURL   string
	healthPaths []string
	skipIndexes bool
}

func watchHTTPClient(deps cmdkit.Dependencies) *http.Client {
	if deps.HTTPClient != nil {
		return deps.HTTPClient
	}
	return http.DefaultClient
}

func (p *watchProbes) observe(ctx context.Context) watchObservation {
	obs := watchObservation{At: time.Now()}
	obs.MgmtAlive, obs.MgmtStatus = p.probeManagement(ctx)
	obs.Health = p.probeHealth(ctx)
	if obs.MgmtAlive {
		obs.ProcessID, obs.MachineName, obs.NewestLogAt, obs.LogErr = p.newestProcess(ctx)
		if !p.skipIndexes {
			obs.BadIndexes = p.badIndexes(ctx)
		}
	}
	return obs
}

// probeManagement POSTs an empty unauthenticated request to the token
// endpoint. 5xx or unreachable means the app is down; any 4xx means the
// app is alive and rejecting the probe — the earliest all-clear during a
// restart window.
func (p *watchProbes) probeManagement(ctx context.Context) (bool, int) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, p.tokenURL, strings.NewReader(""))
	if err != nil {
		return false, 0
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", version.UserAgent())
	response, err := p.httpClient.Do(request)
	if err != nil {
		return false, 0
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode < 500, response.StatusCode
}

func (p *watchProbes) probeHealth(ctx context.Context) map[string]bool {
	health := make(map[string]bool, len(p.healthPaths))
	for _, path := range p.healthPaths {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.publicURL+path, nil)
		if err != nil {
			cancel()
			health[path] = false
			continue
		}
		request.Header.Set("User-Agent", version.UserAgent())
		secretHeader, secretSent := api.SetBasicAuthSecret(request, p.cfg)
		response, err := p.healthClient(secretHeader, secretSent).Do(request)
		if err != nil {
			cancel()
			health[path] = false
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		cancel()
		health[path] = response.StatusCode >= 200 && response.StatusCode < 300
	}
	return health
}

// healthClient follows redirects as before, except to a login page: a
// basic-auth protected environment (Umbraco Cloud non-live) answers a
// public path with a redirect to its login form, which is a 200 and was
// counted as the site serving. Stopping there leaves the 3xx as the status.
func (p *watchProbes) healthClient(secretHeader string, secretSent bool) *http.Client {
	client := *p.httpClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if api.IsLoginRedirect(next.URL.String()) {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if secretSent && !strings.EqualFold(next.URL.Host, via[0].URL.Host) {
			next.Header.Del(secretHeader)
		}
		return nil
	}
	return &client
}

func (p *watchProbes) newestProcess(ctx context.Context) (string, string, time.Time, error) {
	result, err := p.deps.Client.Get(ctx, cmdkit.LogViewerLogPath, api.RequestOptions{Params: map[string]any{
		"take": 1, "skip": 0, "orderDirection": "Descending",
	}})
	if err != nil {
		return "", "", time.Time{}, err
	}
	envelope, ok := result.(map[string]any)
	if !ok {
		return "", "", time.Time{}, nil
	}
	items, _ := envelope["items"].([]any)
	if len(items) == 0 {
		return "", "", time.Time{}, nil
	}
	entry, ok := items[0].(map[string]any)
	if !ok {
		return "", "", time.Time{}, nil
	}
	newestAt, _ := cmdkit.LogEntryTimestamp(entry)
	var processID, machineName string
	if properties, ok := entry["properties"].([]any); ok {
		for _, item := range properties {
			property, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name, _ := property["name"].(string)
			value, _ := property["value"].(string)
			switch name {
			case "ProcessId":
				processID = value
			case "MachineName":
				machineName = value
			}
		}
	}
	return processID, machineName, newestAt, nil
}

func (p *watchProbes) badIndexes(ctx context.Context) []string {
	result, err := p.deps.Client.Get(ctx, "/indexer", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 100}})
	if err != nil {
		return nil
	}
	envelope, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	items, _ := envelope["items"].([]any)
	bad := make([]string, 0)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status := cmdkit.IndexerHealthStatus(entry)
		if status != "" && !strings.EqualFold(status, "Healthy") {
			if name, _ := entry["name"].(string); name != "" {
				bad = append(bad, name)
			}
		}
	}
	sort.Strings(bad)
	return bad
}
