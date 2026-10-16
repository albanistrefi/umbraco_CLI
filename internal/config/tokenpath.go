package config

import (
	"os"
	"path/filepath"
	"strings"
)

// TokenCacheSelection names where the credentials of opts come from, for
// the access-token cache, and the token file that goes with them:
//
//   - --config <file>: selection "config:<file>", token "<file>.token.json"
//   - --profile <name>, or the active profile: selection "profile:<name>",
//     token ~/.umbraco/<name>.token.json
//   - neither (environment variables, project files, ~/.umbraco/config.json):
//     selection "" and path "" — the caller keys the file by a hash under
//     ~/.umbraco/tokens/ (see TokenCacheDir).
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
		return "config:" + selection.Path, selection.Path + ".token.json", nil
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
