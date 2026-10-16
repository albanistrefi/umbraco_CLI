package api

import (
	"context"
	"net/http"
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

func (c *memoryTokenCache) Delete(string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token, c.expires = "", time.Time{}
	c.deletes++
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
