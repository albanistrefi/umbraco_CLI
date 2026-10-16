package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

// tokenFileFixture writes a user config and a cached token for it under a
// temporary HOME, and returns the deps, the token file and the config.
func tokenFileFixture(t *testing.T) (string, config.Config) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{BaseURL: "https://localhost:44314", ClientID: "client-id", ClientSecret: "client-secret"}
	if err := config.WriteUserConfig(cfg); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}
	path, key, err := auth.TokenCacheFile(config.LoadOptions{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	auth.FileTokenCache(path, cfg.BaseURL, cfg.ClientID, "").Store(key, "cached-token", time.Now().Add(time.Minute))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture token file missing: %v", err)
	}
	return path, cfg
}

func depsFor(cfg config.Config) func() config.Config {
	return func() config.Config { return cfg }
}

func TestAuthLogoutTokenOnlyDeletesTheTokenFileAndKeepsTheConfig(t *testing.T) {
	path, cfg := tokenFileFixture(t)
	configPath := filepath.Join(os.Getenv("HOME"), ".umbraco", "config.json")
	before, _ := os.ReadFile(configPath)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = depsFor(cfg)

	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "auth", "logout", "--token-only")
	if err != nil {
		t.Fatalf("logout --token-only failed: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(output), &payload)
	if payload["cachedTokenDeleted"] != true || payload["tokenFile"] != path {
		t.Fatalf("unexpected payload %v", payload)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the token file must be gone: %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if string(before) != string(after) || len(before) == 0 {
		t.Fatalf("--token-only must leave the config untouched:\nbefore %s\nafter  %s", before, after)
	}
}

func TestAuthLogoutAlsoDeletesTheTokenFile(t *testing.T) {
	path, cfg := tokenFileFixture(t)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = depsFor(cfg)
	if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), "auth", "logout"); err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("logout must delete the token file: %v", err)
	}
	if stored, _, _ := config.LoadUserConfig(); stored.ClientSecret != "" {
		t.Fatalf("logout still clears the stored credentials as before")
	}
}

func TestAuthLogoutDryRunTouchesNeitherTheConfigNorTheTokenFile(t *testing.T) {
	path, cfg := tokenFileFixture(t)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = depsFor(cfg)
	for _, args := range [][]string{{"auth", "logout", "--dry-run"}, {"auth", "logout", "--token-only", "--dry-run"}} {
		if _, err := cmdtest.Execute(buildRootWithCollections(t, deps), args...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a dry run must not delete the token file: %v", err)
	}
	if stored, _, _ := config.LoadUserConfig(); stored.ClientSecret != "client-secret" {
		t.Fatalf("a dry run must not touch the config")
	}
}

func TestAuthLogoutTokenOnlyWithoutAFileReportsNothingDeleted(t *testing.T) {
	path, cfg := tokenFileFixture(t)
	_ = os.Remove(path)
	deps := cmdtest.MakeDeps()
	deps.ConfigProvider = depsFor(cfg)
	output, err := cmdtest.Execute(buildRootWithCollections(t, deps), "auth", "logout", "--token-only")
	if err != nil {
		t.Fatalf("logout --token-only failed: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(output), &payload)
	if payload["cachedTokenDeleted"] != false {
		t.Fatalf("unexpected payload %v", payload)
	}
}
