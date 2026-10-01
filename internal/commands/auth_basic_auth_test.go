package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdtest"
	"umbraco-cli/internal/config"
)

func stubVerifiedLogin(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	originalVerify := verifyStoredAuth
	t.Cleanup(func() { verifyStoredAuth = originalVerify })
	verifyStoredAuth = func(config.Config, *http.Client) error { return nil }
}

func devProfileRoot(t *testing.T) *cobra.Command {
	t.Helper()
	deps := cmdtest.MakeDeps()
	deps.ConfigOptionsProvider = func() config.LoadOptions { return config.LoadOptions{Profile: "dev"} }
	return buildRootWithCollections(t, deps)
}

func login(t *testing.T, extra ...string) map[string]any {
	t.Helper()
	args := append([]string{"auth", "login", "--base-url", "https://dev.example.test", "--client-id", "id", "--client-secret", "secret"}, extra...)
	output, err := cmdtest.Execute(devProfileRoot(t), args...)
	if err != nil {
		t.Fatalf("auth login %v failed: %v", extra, err)
	}
	if strings.Contains(output, "shh") {
		t.Fatalf("auth login printed the shared secret: %s", output)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return payload
}

func storedDevProfile(t *testing.T) config.Config {
	t.Helper()
	cfg, ok, _, err := config.LoadUserConfigWithOptions(config.LoadOptions{Profile: "dev"})
	if err != nil || !ok {
		t.Fatalf("load dev profile: ok=%v err=%v", ok, err)
	}
	return cfg
}

func TestAuthLoginStoresAndKeepsBasicAuthSharedSecret(t *testing.T) {
	stubVerifiedLogin(t)

	payload := login(t, "--basic-auth-shared-secret", "shh", "--basic-auth-shared-secret-header", "X-Cloud-Secret")
	if payload["hasBasicAuthSharedSecret"] != true {
		t.Fatalf("unexpected payload %+v", payload)
	}
	if cfg := storedDevProfile(t); cfg.BasicAuthSharedSecret != "shh" || cfg.BasicAuthSharedSecretHeader != "X-Cloud-Secret" {
		t.Fatalf("shared secret not stored: %+v", cfg)
	}

	// Re-login without the flags keeps what the profile stores.
	login(t)
	if cfg := storedDevProfile(t); cfg.BasicAuthSharedSecret != "shh" || cfg.BasicAuthSharedSecretHeader != "X-Cloud-Secret" {
		t.Fatalf("re-login dropped the shared secret: %+v", cfg)
	}

	// An explicit empty value removes it.
	payload = login(t, "--basic-auth-shared-secret", "")
	if cfg := storedDevProfile(t); cfg.BasicAuthSharedSecret != "" || payload["hasBasicAuthSharedSecret"] != false {
		t.Fatalf("empty flag should remove the secret: %+v %+v", cfg, payload)
	}
}

func TestAuthStatusListAndLogoutTreatSharedSecretAsACredential(t *testing.T) {
	stubVerifiedLogin(t)
	login(t, "--basic-auth-shared-secret", "shh")

	for _, args := range [][]string{{"auth", "status"}, {"auth", "list"}} {
		output, err := cmdtest.Execute(devProfileRoot(t), args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(output, "shh") || !strings.Contains(output, `"hasBasicAuthSharedSecret": true`) {
			t.Fatalf("%v should report presence only:\n%s", args, output)
		}
	}

	if _, err := cmdtest.Execute(devProfileRoot(t), "auth", "logout"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if cfg := storedDevProfile(t); cfg.BasicAuthSharedSecret != "" {
		t.Fatalf("logout kept the shared secret: %+v", cfg)
	}
}
