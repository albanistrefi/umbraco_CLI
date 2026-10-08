package deploy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/version"
)

// watchProbes gathers one observation per poll. Probe failures during a
// restart window are expected signals, not command errors. Every request
// gives up after timeout, so a slow or hanging endpoint costs one poll that
// signal, never the whole watch.
type watchProbes struct {
	client      *api.Client
	cfg         config.Config
	httpClient  *http.Client
	timeout     time.Duration
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

// observe probes the management endpoint first, then the health paths,
// the newest log entry and the indexes side by side, with alongside (when
// set) running next to them. A poll therefore takes about two request
// timeouts at most, however many paths are probed: on 08-10 the probes ran
// one after another and one poll took 1m48s while the app warmed up.
func (p *watchProbes) observe(ctx context.Context, alongside func()) watchObservation {
	obs := watchObservation{At: time.Now()}
	obs.MgmtAlive, obs.MgmtStatus = p.probeManagement(ctx)
	var wg sync.WaitGroup
	start := func(probe func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			probe()
		}()
	}
	start(func() { obs.Health = p.probeHealth(ctx) })
	if obs.MgmtAlive {
		start(func() { obs.ProcessID, obs.MachineName, obs.NewestLogAt, obs.LogErr = p.newestProcess(ctx) })
		if !p.skipIndexes {
			start(func() { obs.BadIndexes = p.badIndexes(ctx) })
		}
	}
	if alongside != nil {
		start(alongside)
	}
	wg.Wait()
	return obs
}

// probeManagement POSTs an empty unauthenticated request to the token
// endpoint. 5xx or unreachable means the app is down; any 4xx means the
// app is alive and rejecting the probe — the earliest all-clear during a
// restart window.
func (p *watchProbes) probeManagement(ctx context.Context) (bool, int) {
	requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
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
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, path := range p.healthPaths {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			healthy := p.probePath(ctx, path)
			mu.Lock()
			health[path] = healthy
			mu.Unlock()
		}()
	}
	wg.Wait()
	return health
}

// probePath reports whether path answers 2xx within the request timeout.
func (p *watchProbes) probePath(ctx context.Context, path string) bool {
	requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.publicURL+path, nil)
	if err != nil {
		return false
	}
	request.Header.Set("User-Agent", version.UserAgent())
	secretHeader, secretSent := api.SetBasicAuthSecret(request, p.cfg)
	response, err := p.healthClient(secretHeader, secretSent).Do(request)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
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
	requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	result, err := p.client.Get(requestCtx, cmdkit.LogViewerLogPath, api.RequestOptions{Params: map[string]any{
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
	requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	result, err := p.client.Get(requestCtx, "/indexer", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 100}})
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
