package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileBasicAuthSharedSecretLoadsAndSurvivesRewrite(t *testing.T) {
	homeDir := t.TempDir()
	dir := filepath.Join(homeDir, ".umbraco")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dev.config.json")
	if err := os.WriteFile(path, []byte(`{
  "baseUrl": "https://dev.example.test",
  "clientId": "dev-client",
  "clientSecret": "dev-secret",
  "basicAuthSharedSecret": "shh",
  "basicAuthSharedSecretHeader": "X-Cloud-Secret"
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadResolvedConfigWithOptions(t.TempDir(), homeDir, map[string]string{
		"UMBRACO_BASIC_AUTH_SHARED_SECRET": "from-env",
	}, LoadOptions{Profile: "dev"})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	name, value, ok := cfg.BasicAuthHeader()
	if !ok || name != "X-Cloud-Secret" || value != "shh" {
		t.Fatalf("expected the profile's shared secret (a selected profile ignores env credentials), got %q=%q ok=%v", name, value, ok)
	}

	// auth login rewrites the whole file; the shared secret must survive it.
	cfg.ClientSecret = "rotated"
	if err := writeUserConfigAtPath(path, cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, _, err := loadUserConfigAtPath(path)
	if err != nil || reloaded.BasicAuthSharedSecret != "shh" || reloaded.BasicAuthSharedSecretHeader != "X-Cloud-Secret" || reloaded.ClientSecret != "rotated" {
		t.Fatalf("rewrite dropped fields: err=%v cfg=%+v", err, reloaded)
	}
}

func TestBasicAuthSharedSecretFromEnvAndDefaultHeader(t *testing.T) {
	cfg, err := loadResolvedConfigWithOptions(t.TempDir(), t.TempDir(), map[string]string{
		"UMBRACO_BASE_URL":                 "https://dev.example.test",
		"UMBRACO_BASIC_AUTH_SHARED_SECRET": "from-env",
	}, LoadOptions{})
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	name, value, ok := cfg.BasicAuthHeader()
	if !ok || name != DefaultBasicAuthSharedSecretHeader || value != "from-env" {
		t.Fatalf("got %q=%q ok=%v", name, value, ok)
	}

	if _, _, ok := (Config{}).BasicAuthHeader(); ok {
		t.Fatalf("no secret configured must mean no header")
	}
}

func TestProfileWithoutSharedSecretWritesNoBasicAuthKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := writeUserConfigAtPath(path, Config{BaseURL: "https://x.test", ClientID: "a", ClientSecret: "b"}); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "basicAuth"; strings.Contains(string(payload), want) {
		t.Fatalf("unexpected %s keys in %s", want, payload)
	}
}
