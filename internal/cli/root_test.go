package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"umbraco-cli/internal/config"
)

func TestRootProfileAuthLoginCreatesMissingSelectedProfile(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalBaseURL := os.Getenv("UMBRACO_BASE_URL")
	originalClientID := os.Getenv("UMBRACO_CLIENT_ID")
	originalClientSecret := os.Getenv("UMBRACO_CLIENT_SECRET")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
		_ = os.Setenv("UMBRACO_BASE_URL", originalBaseURL)
		_ = os.Setenv("UMBRACO_CLIENT_ID", originalClientID)
		_ = os.Setenv("UMBRACO_CLIENT_SECRET", originalClientSecret)
	})
	_ = os.Setenv("HOME", homeDir)
	_ = os.Unsetenv("UMBRACO_BASE_URL")
	_ = os.Unsetenv("UMBRACO_CLIENT_ID")
	_ = os.Unsetenv("UMBRACO_CLIENT_SECRET")

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/umbraco/management/api/v1/security/back-office/token" {
			t.Errorf("unexpected token path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse form: %v", err)
		}
		if r.Form.Get("client_id") != "dev-client" || r.Form.Get("client_secret") != "dev-secret" {
			t.Errorf("unexpected credentials: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-123","expires_in":3600}`)
	}))
	t.Cleanup(tokenServer.Close)

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--profile", "dev",
		"--output", "json",
		"auth", "login",
		"--base-url", tokenServer.URL,
		"--client-id", "dev-client",
		"--client-secret", "dev-secret",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("profile auth login failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode auth login output: %v", err)
	}
	if payload["loggedIn"] != true || payload["source"] != "profile:dev" {
		t.Fatalf("unexpected auth login output: %+v", payload)
	}

	cfg, ok, _, err := config.LoadUserConfigWithOptions(config.LoadOptions{Profile: "dev"})
	if err != nil {
		t.Fatalf("LoadUserConfigWithOptions failed: %v", err)
	}
	if !ok || cfg.BaseURL != tokenServer.URL || cfg.ClientID != "dev-client" || cfg.ClientSecret != "dev-secret" {
		t.Fatalf("unexpected profile config: ok=%v cfg=%+v", ok, cfg)
	}
}
