package cli

import (
	"os"
	"testing"

	"umbraco-cli/internal/auth"
)

// TestMain keeps the root command's tests off the token cache. The keychain
// package already refuses test binaries; this is the second guard.
func TestMain(m *testing.M) {
	_ = os.Setenv(auth.NoTokenCacheEnv, "1")
	os.Exit(m.Run())
}
