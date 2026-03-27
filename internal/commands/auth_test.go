package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"umbraco-cli/internal/config"
)

func TestAuthLoginPersistsVerifiedCredentials(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	originalVerify := verifyStoredAuth
	t.Cleanup(func() { verifyStoredAuth = originalVerify })
	verifyStoredAuth = func(cfg config.Config, httpClient *http.Client) error {
		if cfg.BaseURL != "https://localhost:44314" || cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" {
			t.Fatalf("unexpected credentials passed for verification: %+v", cfg)
		}
		return nil
	}

	output, err := execute(
		buildRootWithCollections(t, makeDeps()),
		"auth", "login",
		"--base-url", "https://localhost:44314",
		"--client-id", "client-id",
		"--client-secret", "client-secret",
	)
	if err != nil {
		t.Fatalf("auth login failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth login payload: %v", err)
	}
	if payload["loggedIn"] != true {
		t.Fatalf("unexpected auth login payload: %+v", payload)
	}

	cfg, ok, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored user config after login")
	}
	if cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" || cfg.BaseURL != "https://localhost:44314" {
		t.Fatalf("unexpected persisted user config: %+v", cfg)
	}
}

func TestAuthStatusReportsStoredConfigAndVerification(t *testing.T) {
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

	if err := config.WriteUserConfig(config.Config{
		BaseURL:      "https://localhost:44314",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}

	originalVerify := verifyStoredAuth
	t.Cleanup(func() { verifyStoredAuth = originalVerify })
	verifyStoredAuth = func(cfg config.Config, httpClient *http.Client) error { return nil }

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "status")
	if err != nil {
		t.Fatalf("auth status failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth status payload: %v", err)
	}
	if payload["authenticated"] != true || payload["verified"] != true || payload["source"] != "user-config" {
		t.Fatalf("unexpected auth status payload: %+v", payload)
	}
}

func TestAuthLogoutClearsStoredCredentials(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", homeDir)

	if err := config.WriteUserConfig(config.Config{
		BaseURL:      "https://localhost:44314",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "logout")
	if err != nil {
		t.Fatalf("auth logout failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth logout payload: %v", err)
	}
	if payload["loggedOut"] != true {
		t.Fatalf("unexpected auth logout payload: %+v", payload)
	}

	cfg, ok, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig after logout failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected config file to remain after logout")
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" {
		t.Fatalf("expected logout to clear stored auth, got %+v", cfg)
	}
	if cfg.BaseURL != "https://localhost:44314" {
		t.Fatalf("expected logout to preserve base URL, got %+v", cfg)
	}
}
