package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
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

func TestAuthLoginNormalizesBaseURLBeforeVerificationAndPersistence(t *testing.T) {
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
		if cfg.BaseURL != "https://localhost:44314" {
			t.Fatalf("expected normalized base URL for verification, got %q", cfg.BaseURL)
		}
		return nil
	}

	output, err := execute(
		buildRootWithCollections(t, makeDeps()),
		"auth", "login",
		"--base-url", "https://localhost:44314/umbraco/",
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
	if payload["baseUrl"] != "https://localhost:44314" {
		t.Fatalf("expected normalized baseUrl in output, got %+v", payload)
	}

	cfg, ok, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored user config after login")
	}
	if cfg.BaseURL != "https://localhost:44314" {
		t.Fatalf("expected normalized persisted base URL, got %+v", cfg)
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

func TestAuthStatusReportsFailedVerificationAsUnauthenticated(t *testing.T) {
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
		ClientSecret: "bad-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}

	originalVerify := verifyStoredAuth
	t.Cleanup(func() { verifyStoredAuth = originalVerify })
	verifyStoredAuth = func(cfg config.Config, httpClient *http.Client) error {
		return fmt.Errorf("token request failed: 401")
	}

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "status")
	if err != nil {
		t.Fatalf("auth status failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth status payload: %v", err)
	}
	if payload["hasCredentials"] != true || payload["authenticated"] != false || payload["verified"] != false {
		t.Fatalf("expected credentials present but not authenticated, got %+v", payload)
	}
	if authError, _ := payload["authError"].(string); !strings.Contains(authError, "401") {
		t.Fatalf("expected authError to preserve verification failure, got %+v", payload)
	}
}

func TestAuthLoginPersistsToSelectedProfile(t *testing.T) {
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
	verifyStoredAuth = func(cfg config.Config, httpClient *http.Client) error { return nil }

	deps := makeDeps()
	deps.ConfigOptionsProvider = func() config.LoadOptions {
		return config.LoadOptions{Profile: "dev"}
	}

	output, err := execute(
		buildRootWithCollections(t, deps),
		"auth", "login",
		"--base-url", "https://dev.example.test",
		"--client-id", "dev-client",
		"--client-secret", "dev-secret",
	)
	if err != nil {
		t.Fatalf("auth login --profile failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth login payload: %v", err)
	}
	if payload["source"] != "profile:dev" {
		t.Fatalf("expected profile source, got %+v", payload)
	}

	cfg, ok, _, err := config.LoadUserConfigWithOptions(config.LoadOptions{Profile: "dev"})
	if err != nil {
		t.Fatalf("LoadUserConfigWithOptions failed: %v", err)
	}
	if !ok || cfg.ClientID != "dev-client" || cfg.ClientSecret != "dev-secret" || cfg.BaseURL != "https://dev.example.test" {
		t.Fatalf("unexpected selected profile config: ok=%v cfg=%+v", ok, cfg)
	}
}

func TestAuthListRedactsStoredProfiles(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	if err := config.WriteUserConfig(config.Config{
		BaseURL:      "https://default.example.test",
		ClientID:     "default-client",
		ClientSecret: "default-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfig failed: %v", err)
	}
	if err := config.WriteUserConfigWithOptions(config.LoadOptions{Profile: "dev"}, config.Config{
		BaseURL:      "https://dev.example.test",
		ClientID:     "dev-client",
		ClientSecret: "dev-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfigWithOptions failed: %v", err)
	}
	if err := config.SetActiveProfile("dev"); err != nil {
		t.Fatalf("SetActiveProfile failed: %v", err)
	}

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "list")
	if err != nil {
		t.Fatalf("auth list failed: %v", err)
	}
	if strings.Contains(output, "default-secret") || strings.Contains(output, "dev-secret") {
		t.Fatalf("auth list leaked a client secret: %s", output)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth list payload: %v", err)
	}
	if payload["activeProfile"] != "dev" || payload["count"] != float64(2) {
		t.Fatalf("unexpected auth list payload: %+v", payload)
	}
}

func TestAuthUseSetsActiveProfile(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	if err := config.WriteUserConfigWithOptions(config.LoadOptions{Profile: "dev"}, config.Config{
		BaseURL:      "https://dev.example.test",
		ClientID:     "dev-client",
		ClientSecret: "dev-secret",
	}); err != nil {
		t.Fatalf("WriteUserConfigWithOptions failed: %v", err)
	}

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "use", "dev")
	if err != nil {
		t.Fatalf("auth use failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth use payload: %v", err)
	}
	if payload["activeProfile"] != "dev" {
		t.Fatalf("unexpected auth use payload: %+v", payload)
	}
	active, ok, err := config.ActiveProfile()
	if err != nil {
		t.Fatalf("ActiveProfile failed: %v", err)
	}
	if !ok || active != "dev" {
		t.Fatalf("expected active profile dev, got ok=%v active=%q", ok, active)
	}
}

func TestAuthStatusCheckReportsCommandRequirements(t *testing.T) {
	homeDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", homeDir)

	output, err := execute(buildRootWithCollections(t, makeDeps()), "auth", "status", "--check")
	if err != nil {
		t.Fatalf("auth status --check failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode auth status payload: %v", err)
	}
	check, ok := payload["permissionCheck"].(map[string]any)
	if !ok {
		t.Fatalf("expected permissionCheck output, got %+v", payload)
	}
	commands, ok := check["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("expected command requirement entries, got %+v", check)
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
