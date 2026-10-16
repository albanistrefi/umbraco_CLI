package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

type memoryTokenCache struct {
	mu      sync.Mutex
	token   string
	expires time.Time
	deletes int
}

func (c *memoryTokenCache) Load(string) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.expires, c.token != ""
}

func (c *memoryTokenCache) Store(_ string, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token, c.expires = token, expiresAt
}

func (c *memoryTokenCache) Delete(_ string, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token != "" && c.token != token {
		return nil
	}
	c.token, c.expires = "", time.Time{}
	c.deletes++
	return nil
}

// A cached token the server rejects (401) is dropped from the cache and
// replaced by one refetch; the request is retried once with the new token.
func TestARejectedCachedTokenIsDroppedAndRefetchedOnce(t *testing.T) {
	tokenRequests := 0
	var seen []string
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			tokenRequests++
			return jsonResponse(http.StatusOK, `{"access_token":"fresh","expires_in":3600}`, nil), nil
		case "/umbraco/management/api/v1/document/root":
			seen = append(seen, r.Header.Get("Authorization"))
			if r.Header.Get("Authorization") != "Bearer fresh" {
				return jsonResponse(http.StatusUnauthorized, `{}`, nil), nil
			}
			return jsonResponse(http.StatusOK, `{"items":[]}`, nil), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`, nil), nil
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	cache := &memoryTokenCache{token: "cached-but-revoked", expires: time.Now().Add(4 * time.Minute)}
	client := NewClient(cfg, httpClient, auth.NewCached(cfg, httpClient, cache, "key"))

	if _, err := client.Get(context.Background(), "/document/root", RequestOptions{}); err != nil {
		t.Fatalf("request should succeed after one refetch: %v", err)
	}
	if tokenRequests != 1 || len(seen) != 2 || seen[0] != "Bearer cached-but-revoked" || seen[1] != "Bearer fresh" {
		t.Fatalf("expected the cached token, a 401, one refetch and one retry: %d token requests, %v", tokenRequests, seen)
	}
	if cache.deletes != 1 || cache.token != "fresh" {
		t.Fatalf("the rejected token must be dropped and the fresh one cached: deletes %d, cached %q", cache.deletes, cache.token)
	}
}

// A revoked token in a token file that cannot be deleted (its directory is
// read-only) must not be sent again and again: the provider remembers the
// refused token, so the request refetches once and succeeds.
func TestAnUndeletableRevokedTokenIsRefetchedOnce(t *testing.T) {
	tokenRequests, apiCalls := 0, 0
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			tokenRequests++
			return jsonResponse(http.StatusOK, `{"access_token":"fresh","expires_in":3600}`, nil), nil
		}
		apiCalls++
		if r.Header.Get("Authorization") != "Bearer fresh" {
			return jsonResponse(http.StatusUnauthorized, `{}`, nil), nil
		}
		return jsonResponse(http.StatusOK, `{"items":[]}`, nil), nil
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	dir := filepath.Join(t.TempDir(), "ro")
	path := filepath.Join(dir, "p.token.json")
	key := auth.CacheKey("profile:p", cfg)
	auth.FileTokenCache(path, cfg.BaseURL, cfg.ClientID, "profile:p").Store(key, "revoked", time.Now().Add(30*time.Minute))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	cache := auth.FileTokenCache(path, cfg.BaseURL, cfg.ClientID, "profile:p")
	client := NewClient(cfg, httpClient, auth.NewCached(cfg, httpClient, cache, key))
	for i := 0; i < 3; i++ {
		if _, err := client.Get(context.Background(), "/document/root", RequestOptions{}); err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	if tokenRequests != 1 || apiCalls != 4 {
		t.Fatalf("expected one 401, one refetch, then the fresh token: %d token requests, %d API calls", tokenRequests, apiCalls)
	}
}

// A token the server keeps refusing is refetched within the request's
// attempt budget: up to three refetches, then the 401 is the answer.
func TestAPersistent401RefetchesWithinTheAttemptBudget(t *testing.T) {
	tokenRequests, apiCalls := 0, 0
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			tokenRequests++
			return jsonResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"t%d","expires_in":3600}`, tokenRequests), nil), nil
		}
		apiCalls++
		return jsonResponse(http.StatusUnauthorized, `{}`, nil), nil
	})
	cfg := config.Config{BaseURL: "https://example.test", ClientID: "client-id", ClientSecret: "client-secret"}
	path := filepath.Join(t.TempDir(), "p.token.json")
	key := auth.CacheKey("profile:p", cfg)
	cache := auth.FileTokenCache(path, cfg.BaseURL, cfg.ClientID, "profile:p")
	client := NewClient(cfg, httpClient, auth.NewCached(cfg, httpClient, cache, key))
	if _, err := client.Get(context.Background(), "/document/root", RequestOptions{}); err == nil {
		t.Fatalf("a persistent 401 must fail")
	}
	if tokenRequests != maxRequestAttempts || apiCalls != maxRequestAttempts {
		t.Fatalf("expected %d token requests (one fetch, %d refetches) and %d calls, got %d and %d", maxRequestAttempts, maxRequestAttempts-1, maxRequestAttempts, tokenRequests, apiCalls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the last refused token must not stay cached for the next invocation: %v", err)
	}
}
