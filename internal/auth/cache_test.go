package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/config"
)

// fakeCache is an in-memory TokenCache that counts its calls.
type fakeItem struct {
	Token     string
	ExpiresAt int64
}

type fakeCache struct {
	mu      sync.Mutex
	items   map[string]fakeItem
	deletes []string
}

func newFakeCache() *fakeCache { return &fakeCache{items: map[string]fakeItem{}} }

func (c *fakeCache) Load(key string) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	return item.Token, time.Unix(item.ExpiresAt, 0), ok
}

func (c *fakeCache) Store(key string, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = fakeItem{Token: token, ExpiresAt: expiresAt.Unix()}
}

func (c *fakeCache) Delete(key string, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token != "" && c.items[key].Token != token {
		return nil
	}
	delete(c.items, key)
	c.deletes = append(c.deletes, key)
	return nil
}

type roundTripper func(*http.Request) (*http.Response, error)

func (fn roundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

// tokenServer answers the token endpoint with fresh-1, fresh-2, ... and
// counts the requests.
func tokenServer(requests *int) *http.Client {
	var mu sync.Mutex
	return &http.Client{Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		*requests++
		n := *requests
		mu.Unlock()
		body := `{"access_token":"fresh-` + string(rune('0'+n)) + `","expires_in":300}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
}

var testConfig = config.Config{BaseURL: "https://live.example.test", ClientID: "client-a", ClientSecret: "secret"}

func TestCachedTokenIsReusedWithoutATokenRequest(t *testing.T) {
	requests := 0
	cache := newFakeCache()
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "from-cache", time.Now().Add(4*time.Minute))
	token, err := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
	if err != nil || token != "from-cache" || requests != 0 {
		t.Fatalf("expected the cached token without a request, got %q (%v), %d requests", token, err, requests)
	}
}

func TestAnExpiredCachedTokenIsRefetchedAndStored(t *testing.T) {
	requests := 0
	cache := newFakeCache()
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "stale", time.Now().Add(-time.Second))
	token, err := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
	if err != nil || token != "fresh-1" || requests != 1 {
		t.Fatalf("expected one fetch, got %q (%v), %d requests", token, err, requests)
	}
	stored, expiresAt, ok := cache.Load(key)
	if !ok || stored != "fresh-1" {
		t.Fatalf("the fetched token must be cached, got %q", stored)
	}
	// Kept only until shortly before expires_in (300s): a minute early.
	if remaining := time.Until(expiresAt); remaining > 4*time.Minute+time.Second || remaining < 4*time.Minute-5*time.Second {
		t.Fatalf("expected about 4m of validity, got %s", remaining)
	}
	// A second invocation reuses it.
	again, _ := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
	if again != "fresh-1" || requests != 1 {
		t.Fatalf("the next invocation must reuse the cached token, got %q after %d requests", again, requests)
	}
}

func TestInvalidatingTheRejectedTokenDropsItFromTheCache(t *testing.T) {
	requests := 0
	cache := newFakeCache()
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "from-cache", time.Now().Add(4*time.Minute))
	provider := NewCached(testConfig, tokenServer(&requests), cache, key)
	token, _ := provider.AccessToken(context.Background())
	provider.InvalidateIfCurrent("an-older-token")
	if _, _, ok := cache.Load(key); !ok {
		t.Fatalf("a 401 for another token must not drop the cached one")
	}
	provider.InvalidateIfCurrent(token)
	if _, _, ok := cache.Load(key); ok || len(cache.deletes) != 1 {
		t.Fatalf("the rejected token must be dropped from the cache: %+v", cache.deletes)
	}
	next, _ := provider.AccessToken(context.Background())
	if next != "fresh-1" || requests != 1 {
		t.Fatalf("expected one refetch, got %q after %d requests", next, requests)
	}
}

func TestCacheKeysIsolateProfilesServersAndClients(t *testing.T) {
	other := testConfig
	other.BaseURL = "https://dev.example.test"
	otherClient := testConfig
	otherClient.ClientID = "client-b"
	trailing := testConfig
	trailing.BaseURL = "https://live.example.test/"
	keys := map[string]string{
		"live":            CacheKey("profile:live", testConfig),
		"dev profile":     CacheKey("profile:dev", testConfig),
		"dev server":      CacheKey("profile:live", other),
		"other client":    CacheKey("profile:live", otherClient),
		"default":         CacheKey("", testConfig),
		"explicit config": CacheKey("config:/tmp/x.json", testConfig),
	}
	seen := map[string]string{}
	for name, key := range keys {
		if prev, dup := seen[key]; dup {
			t.Fatalf("%s and %s share a cache key", name, prev)
		}
		seen[key] = name
		if strings.Contains(key, "example") || strings.Contains(key, "client") {
			t.Fatalf("the key must not name the host or client: %s", key)
		}
	}
	if CacheKey("profile:live", trailing) != keys["live"] {
		t.Fatalf("a trailing slash must not change the key")
	}

	// Two profiles sharing one store never see each other's token.
	requests := 0
	cache := newFakeCache()
	a, _ := NewCached(testConfig, tokenServer(&requests), cache, keys["live"]).AccessToken(context.Background())
	b, _ := NewCached(other, tokenServer(&requests), cache, keys["dev server"]).AccessToken(context.Background())
	if a == b || requests != 2 {
		t.Fatalf("profiles must not share a token: %q %q after %d requests", a, b, requests)
	}
}

func TestTheOptOutDisablesTheCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for value, disabled := range map[string]bool{"": false, "0": false, "false": false, "1": true, "true": true, "yes": true} {
		t.Setenv(NoTokenCacheEnv, value)
		cache, _ := SystemTokenCache(config.LoadOptions{Profile: "live"}, testConfig)
		if (cache == nil) != disabled {
			t.Fatalf("%s=%q: cache %v, want disabled %v", NoTokenCacheEnv, value, cache != nil, disabled)
		}
	}
}

// stubbornCache never deletes: a cache whose delete reports success but
// leaves the refused token in place, as a file a parallel invocation wrote
// back would.
type stubbornCache struct{ fakeCache }

func (c *stubbornCache) Delete(string, string) error { return nil }

func TestARefusedTokenTheCacheKeepsIsNotReused(t *testing.T) {
	requests := 0
	cache := &stubbornCache{fakeCache{items: map[string]fakeItem{}}}
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "revoked", time.Now().Add(time.Hour))
	provider := NewCached(testConfig, tokenServer(&requests), cache, key)
	token, _ := provider.AccessToken(context.Background())
	provider.InvalidateIfCurrent(token)
	next, err := provider.AccessToken(context.Background())
	if err != nil || next == "revoked" || requests != 1 {
		t.Fatalf("a refused token the cache still holds must not be reused: %q %v after %d requests", next, err, requests)
	}
}

// failingCache cannot delete and counts its loads.
type failingCache struct {
	fakeCache
	loads int
}

func (c *failingCache) Load(key string) (string, time.Time, bool) {
	c.loads++
	return c.fakeCache.Load(key)
}

func (c *failingCache) Delete(string, string) error { return errors.New("read-only") }

func TestACacheThatCannotDeleteIsBypassedUntilTheNextFetch(t *testing.T) {
	requests := 0
	cache := &failingCache{fakeCache: fakeCache{items: map[string]fakeItem{}}}
	key := CacheKey("profile:live", testConfig)
	cache.Store(key, "revoked", time.Now().Add(time.Hour))
	provider := NewCached(testConfig, tokenServer(&requests), cache, key)
	token, _ := provider.AccessToken(context.Background())
	provider.InvalidateIfCurrent(token)
	loadsBefore := cache.loads
	if _, err := provider.AccessToken(context.Background()); err != nil || requests != 1 {
		t.Fatalf("expected one refetch: %v after %d requests", err, requests)
	}
	if cache.loads != loadsBefore {
		t.Fatalf("a cache that could not delete the refused token must be skipped until a fetch: %d loads", cache.loads-loadsBefore)
	}
}
