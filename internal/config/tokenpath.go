package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// TokenCacheSelection names where the credentials of opts come from, for
// the access-token cache, and the token file that goes with them:
//
//   - --profile <name>, or the active profile: selection "profile:<name>",
//     token ~/.umbraco/<name>.token.json
//   - --config <file>: selection "config:<file>", token
//     ~/.umbraco/tokens/config-<sha256 of the path>.token.json, never next to
//     the config file, so a token cannot end up in a project repository
//   - neither (environment variables, project files, ~/.umbraco/config.json):
//     selection "" and path "" — the caller keys the file by a hash under
//     ~/.umbraco/tokens/ (see TokenCacheDir).
//
// Profile and --config paths depend on the selection only, so the file can
// be found without a readable config.
func TokenCacheSelection(opts LoadOptions) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	selection, ok, err := selectedConfigPath(workingDir, homeDir, opts)
	if err != nil || !ok {
		return "", "", err
	}
	if selection.CustomPath {
		sum := sha256.Sum256([]byte(selection.Path))
		return "config:" + selection.Path, filepath.Join(homeDir, ".umbraco", "tokens", "config-"+hex.EncodeToString(sum[:])+".token.json"), nil
	}
	name := strings.TrimSpace(selection.Profile)
	if name == "" {
		name = defaultProfileName
	}
	return "profile:" + name, filepath.Join(homeDir, ".umbraco", name+".token.json"), nil
}

// TokenCacheDir is where tokens of unselected configurations are kept.
func TokenCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".umbraco", "tokens"), nil
}
