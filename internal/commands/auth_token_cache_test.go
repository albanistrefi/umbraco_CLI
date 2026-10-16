package commands

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

type recordingTokenCache struct {
	items   map[string]string
	deleted []string
}

func (c *recordingTokenCache) Load(key string) (string, time.Time, bool) {
	token, ok := c.items[key]
	return token, time.Now().Add(time.Minute), ok
}
func (c *recordingTokenCache) Store(key string, token string, _ time.Time) { c.items[key] = token }
func (c *recordingTokenCache) Delete(key string) {
	delete(c.items, key)
	c.deleted = append(c.deleted, key)
}

func writeTestUserConfig(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{BaseURL: "https://localhost:44314", ClientID: "client-id", ClientSecret: "client-secret"}
	if err := config.WriteUserConfig(cfg); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}
	return cfg
}

func TestAuthLogoutTokenOnlyClearsTheCachedTokenAndKeepsTheConfig(t *testing.T) {
	cfg := writeTestUserConfig(t)
	before, _ := os.ReadFile(os.Getenv("HOME") + "/.umbraco/config.json")
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = func() config.Config { return cfg }
	key := auth.CacheKey("", cfg)
	cache := &recordingTokenCache{items: map[string]string{key: "cached", "other-profile": "keep"}}
	deps.TokenCache = cache

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "auth", "logout", "--token-only")
	if err != nil {
		t.Fatalf("logout --token-only failed: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(output), &payload)
	if payload["cachedTokenCleared"] != true {
		t.Fatalf("unexpected payload %v", payload)
	}
	if _, ok := cache.items[key]; ok || cache.items["other-profile"] != "keep" {
		t.Fatalf("only this profile's token is cleared: %v", cache.items)
	}
	after, _ := os.ReadFile(os.Getenv("HOME") + "/.umbraco/config.json")
	if string(before) != string(after) || len(before) == 0 {
		t.Fatalf("--token-only must leave the config untouched:\nbefore %s\nafter  %s", before, after)
	}
}

func TestAuthLogoutAlsoClearsTheCachedToken(t *testing.T) {
	cfg := writeTestUserConfig(t)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = func() config.Config { return cfg }
	key := auth.CacheKey("", cfg)
	cache := &recordingTokenCache{items: map[string]string{key: "cached"}}
	deps.TokenCache = cache
	if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), "auth", "logout"); err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != key {
		t.Fatalf("logout must clear the cached token: %v", cache.deleted)
	}
	stored, _, _ := config.LoadUserConfig()
	if stored.ClientSecret != "" {
		t.Fatalf("logout still clears the stored credentials as before")
	}
}

func TestAuthLogoutDryRunTouchesNeitherTheConfigNorTheCache(t *testing.T) {
	cfg := writeTestUserConfig(t)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = func() config.Config { return cfg }
	cache := &recordingTokenCache{items: map[string]string{auth.CacheKey("", cfg): "cached"}}
	deps.TokenCache = cache
	for _, args := range [][]string{{"auth", "logout", "--dry-run"}, {"auth", "logout", "--token-only", "--dry-run"}} {
		if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), args...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}
	if len(cache.deleted) != 0 {
		t.Fatalf("a dry run must not clear the cache: %v", cache.deleted)
	}
	if stored, _, _ := config.LoadUserConfig(); stored.ClientSecret != "client-secret" {
		t.Fatalf("a dry run must not touch the config")
	}
}
