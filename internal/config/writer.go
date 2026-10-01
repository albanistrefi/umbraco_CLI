package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func WriteUserConfig(cfg Config) error {
	path, err := UserConfigPath()
	if err != nil {
		return err
	}
	return writeUserConfigAtPath(path, cfg)
}

func WriteUserConfigWithOptions(opts LoadOptions, cfg Config) error {
	_, _, selection, err := LoadUserConfigWithOptions(opts)
	if err != nil {
		return err
	}
	return writeUserConfigAtPath(selection.Path, cfg)
}

func writeUserConfigAtPath(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload := jsonConfigFile{
		BaseURL:      strings.TrimSpace(cfg.BaseURL),
		ClientID:     strings.TrimSpace(cfg.ClientID),
		ClientSecret: strings.TrimSpace(cfg.ClientSecret),
		OutputFormat: strings.TrimSpace(string(cfg.OutputFormat)),

		BasicAuthSharedSecret:       strings.TrimSpace(cfg.BasicAuthSharedSecret),
		BasicAuthSharedSecretHeader: strings.TrimSpace(cfg.BasicAuthSharedSecretHeader),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return err
	}
	// WriteFile's mode only applies to newly created files; tighten
	// pre-existing config files that may have looser permissions.
	return os.Chmod(path, 0o600)
}

func ClearUserAuth() error {
	cfg, ok, err := LoadUserConfig()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	cfg.ClientID = ""
	cfg.ClientSecret = ""
	cfg.BasicAuthSharedSecret = ""
	return WriteUserConfig(cfg)
}

func ClearUserAuthWithOptions(opts LoadOptions) error {
	cfg, ok, selection, err := LoadUserConfigWithOptions(opts)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	cfg.ClientID = ""
	cfg.ClientSecret = ""
	cfg.BasicAuthSharedSecret = ""
	return writeUserConfigAtPath(selection.Path, cfg)
}
