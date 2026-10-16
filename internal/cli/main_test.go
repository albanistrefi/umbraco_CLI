package cli

import (
	"os"
	"testing"

	"umbraco-cli/internal/auth"
)

// TestMain keeps the root command's tests from writing token files under
// the real home directory.
func TestMain(m *testing.M) {
	_ = os.Setenv(auth.NoTokenCacheEnv, "1")
	os.Exit(m.Run())
}
