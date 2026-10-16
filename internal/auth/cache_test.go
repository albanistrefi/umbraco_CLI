package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/config"
	"umbraco-cli/internal/keychain"
)

// fakeCache is an in-memory TokenCache that counts its calls.
type fakeCache struct {
	mu      sync.Mutex
	items   map[string]cachedToken
	deletes []string
}

func newFakeCache() *fakeCache { return &fakeCache{items: map[string]cachedToken{}} }

func (c *fakeCache) Load(key string) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	return item.Token, time.Unix(item.ExpiresAt, 0), ok
}

func (c *fakeCache) Store(key string, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cachedToken{Token: token, ExpiresAt: expiresAt.Unix()}
}

func (c *fakeCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	c.deletes = append(c.deletes, key)
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
	cache.Store(key, "from-keychain", time.Now().Add(4*time.Minute))
	token, err := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
	if err != nil || token != "from-keychain" || requests != 0 {
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
	cache.Store(key, "from-keychain", time.Now().Add(4*time.Minute))
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
	if SelectionName(config.LoadOptions{Profile: "Live"}) != "profile:live" || SelectionName(config.LoadOptions{ConfigPath: "/a.json"}) != "config:/a.json" || SelectionName(config.LoadOptions{}) != "" {
		t.Fatalf("unexpected selection names")
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

type fakeStore struct{ items map[string]string }

func (s *fakeStore) Get(service, account string) (string, error) {
	value, ok := s.items[service+"/"+account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return value, nil
}
func (s *fakeStore) Set(service, account, _ string, secret string) error {
	s.items[service+"/"+account] = secret
	return nil
}
func (s *fakeStore) Delete(service, account string) error {
	delete(s.items, service+"/"+account)
	return nil
}

func TestSystemTokenCacheHonoursTheOptOut(t *testing.T) {
	store := &fakeStore{items: map[string]string{}}
	defer func(previous func() keychain.Store) { systemStore = previous }(systemStore)
	systemStore = func() keychain.Store { return store }
	for value, wantCache := range map[string]bool{"": true, "0": true, "false": true, "1": false, "true": false, "yes": false} {
		t.Setenv(NoTokenCacheEnv, value)
		if got := SystemTokenCache() != nil; got != wantCache {
			t.Fatalf("%s=%q: cache %v, want %v", NoTokenCacheEnv, value, got, wantCache)
		}
	}
	systemStore = func() keychain.Store { return keychain.Unavailable{} }
	t.Setenv(NoTokenCacheEnv, "")
	if SystemTokenCache() != nil {
		t.Fatalf("an unavailable keychain means no cache")
	}
}

func TestKeychainTokenCacheRoundTripsAndToleratesGarbage(t *testing.T) {
	store := &fakeStore{items: map[string]string{}}
	cache := KeychainTokenCache(store)
	expires := time.Now().Add(4 * time.Minute).Truncate(time.Second)
	cache.Store("k", "eyJ.a-b_c", expires)
	for _, value := range store.items {
		if strings.Contains(value, "a-b_c") {
			t.Fatalf("the stored value is encoded, got %q", value)
		}
	}
	token, at, ok := cache.Load("k")
	if !ok || token != "eyJ.a-b_c" || !at.Equal(expires) {
		t.Fatalf("round trip: %q %s %v", token, at, ok)
	}
	store.items[keychainService+"/k"] = "not base64 !"
	if _, _, ok := cache.Load("k"); ok {
		t.Fatalf("an unreadable item is a miss")
	}
	cache.Delete("k")
	if len(store.items) != 0 {
		t.Fatalf("delete must remove the item")
	}
}

func TestAnUnavailableKeychainStillAuthenticates(t *testing.T) {
	requests := 0
	cache := KeychainTokenCache(keychain.Unavailable{})
	key := CacheKey("profile:live", testConfig)
	for i := 0; i < 2; i++ {
		token, err := NewCached(testConfig, tokenServer(&requests), cache, key).AccessToken(context.Background())
		if err != nil || token == "" {
			t.Fatalf("invocation %d: %q, %v", i, token, err)
		}
	}
	if requests != 2 {
		t.Fatalf("without a keychain each invocation fetches its own token, got %d requests", requests)
	}
}
