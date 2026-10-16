package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

// The runtime's token provider uses the file cache: a second invocation
// reuses the first one's token without a token request.
func TestTheRuntimeReusesTheTokenAcrossInvocations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(auth.NoTokenCacheEnv, "")
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-123","expires_in":3600}`)
	}))
	t.Cleanup(server.Close)
	cfg := config.Config{BaseURL: server.URL, ClientID: "client", ClientSecret: "secret"}
	opts := config.LoadOptions{Profile: "dev"}
	for i := 0; i < 2; i++ {
		if _, err := newTokenProvider(cfg, opts, server.Client()).AccessToken(context.Background()); err != nil {
			t.Fatalf("invocation %d: %v", i, err)
		}
	}
	if requests != 1 {
		t.Fatalf("the second invocation must reuse the cached token, got %d token requests", requests)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".umbraco", "dev.token.json")); err != nil {
		t.Fatalf("the token file is written next to the profile's config: %v", err)
	}
}

// 'auth logout --token-only' deletes the profile's token even when the
// profile's config does not parse.
func TestTokenOnlyLogoutWorksWithAnUnreadableConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".umbraco")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(dir, "broken.token.json")
	auth.FileTokenCache(tokenFile, "https://example.test", "client", "profile:broken").Store("k", "t", time.Now().Add(time.Minute))

	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--profile", "broken", "--output", "json", "auth", "logout", "--token-only"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout --token-only with a broken config failed: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(out.Bytes(), &payload)
	if payload["cachedTokenDeleted"] != true || payload["tokenFile"] != tokenFile {
		t.Fatalf("unexpected payload %v", payload)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("the token file must be gone: %v", err)
	}

	// Plain logout still needs a readable config.
	root = NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--profile", "broken", "auth", "logout"})
	if err := root.Execute(); err == nil {
		t.Fatalf("plain logout with a broken config must still fail")
	}
}
