package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"umbraco-cli/internal/dotnet"
)

type OutputFormat string

const defaultBaseURL = "https://localhost:44391"

const (
	OutputJSON  OutputFormat = "json"
	OutputTable OutputFormat = "table"
	OutputPlain OutputFormat = "plain"
)

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	OutputFormat OutputFormat
}

type LoadOptions struct {
	Profile    string
	ConfigPath string
	// BaseURL, when set, replaces the resolved base URL from any source —
	// the global --base-url flag. Credentials are untouched, so a profile
	// can be pointed at another host (or an unreachable one) without
	// copying its secret.
	BaseURL string
}

type ConfigFileNotFoundError struct {
	Path string
}

func (e *ConfigFileNotFoundError) Error() string {
	return fmt.Sprintf("config file not found: %s", e.Path)
}

func IsConfigFileNotFound(err error) bool {
	var notFound *ConfigFileNotFoundError
	return errors.As(err, &notFound)
}

func Load() (Config, error) {
	return LoadWithOptions(LoadOptions{})
}

func LoadWithOptions(opts LoadOptions) (Config, error) {
	cfg, err := loadWithOptionsNoOverride(opts)
	if err != nil {
		return cfg, err
	}
	if override := strings.TrimSpace(opts.BaseURL); override != "" {
		cfg.BaseURL = strings.TrimRight(override, "/")
	}
	return cfg, nil
}

func loadWithOptionsNoOverride(opts LoadOptions) (Config, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	env := currentEnv()
	return loadResolvedConfigWithOptions(workingDir, homeDir, env, opts)
}

func UserConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".umbraco", "config.json"), nil
}

func loadResolvedConfigWithOptions(workingDir string, homeDir string, env map[string]string, opts LoadOptions) (Config, error) {
	if strings.TrimSpace(opts.ConfigPath) != "" && strings.TrimSpace(opts.Profile) != "" {
		return Config{}, fmt.Errorf("--profile and --config are mutually exclusive")
	}

	if selected, ok, err := selectedConfigPath(workingDir, homeDir, opts); err != nil {
		return Config{}, err
	} else if ok {
		raw, present, err := loadJSONConfig(selected.Path)
		if err != nil {
			return Config{}, err
		}
		if !present && (!selected.IsDefault() || selected.Explicit) {
			return Config{}, &ConfigFileNotFoundError{Path: selected.Path}
		}
		// Output format is not an environment identity, so keep the existing
		// UMBRACO_OUTPUT_FORMAT override even when credentials come from a
		// selected profile/config file.
		if outputFormat := strings.TrimSpace(env["UMBRACO_OUTPUT_FORMAT"]); outputFormat != "" {
			raw.OutputFormat = outputFormat
		}
		return finalizeRawConfig(raw)
	}

	return loadResolvedConfig(workingDir, homeDir, env)
}

// loadResolvedConfig merges configuration sources lowest precedence first,
// so later sources override earlier ones. The order (low to high) is the
// README's documented precedence, inverted:
//
//  1. user config ~/.umbraco/config.json
//  2. project .env
//  3. project .umbraco-cli.env
//  4. project .umbracorc.json / .umbracorc
//  5. process env (UMBRACO_*)
//
// A .NET host-project scan supplies the base URL only when no source above
// set one.
func loadResolvedConfig(workingDir string, homeDir string, env map[string]string) (Config, error) {
	sources := []func() (rawConfig, error){
		func() (rawConfig, error) { return userConfigSource(homeDir) },
		func() (rawConfig, error) { return dotEnvSource(workingDir, ".env") },
		func() (rawConfig, error) { return dotEnvSource(workingDir, ".umbraco-cli.env") },
		func() (rawConfig, error) { return projectRCSource(workingDir) },
		func() (rawConfig, error) { return rawConfigFromEnv(env), nil },
	}

	resolved := rawConfig{}
	for _, source := range sources {
		loaded, err := source()
		if err != nil {
			return Config{}, err
		}
		mergeRawConfig(&resolved, loaded)
	}

	// Crawling the working tree for a .NET host project is expensive, so it
	// only runs when no explicit source supplied a base URL. Discovery is
	// best-effort: an unreadable or malformed file nearby must never make
	// the CLI unusable.
	if strings.TrimSpace(resolved.BaseURL) == "" {
		if discoveredBaseURL, ok := dotnet.DiscoverBaseURL(workingDir, NormalizeBaseURL); ok {
			resolved.BaseURL = discoveredBaseURL
		}
	}

	return finalizeRawConfig(resolved)
}

func finalizeRawConfig(raw rawConfig) (Config, error) {
	cfg := Config{
		BaseURL:      normalizeBaseURL(raw.BaseURL),
		ClientID:     strings.TrimSpace(raw.ClientID),
		ClientSecret: strings.TrimSpace(raw.ClientSecret),
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	if output := strings.TrimSpace(raw.OutputFormat); output != "" {
		format, err := ParseOutputFormat(output)
		if err != nil {
			return Config{}, err
		}
		cfg.OutputFormat = format
	}

	return cfg, nil
}

func LoadUserConfig() (Config, bool, error) {
	path, err := UserConfigPath()
	if err != nil {
		return Config{}, false, err
	}
	return loadUserConfigAtPath(path)
}

func LoadUserConfigWithOptions(opts LoadOptions) (Config, bool, UserConfigSelection, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, false, UserConfigSelection{}, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, false, UserConfigSelection{}, err
	}
	selection, selected, err := selectedConfigPath(workingDir, homeDir, opts)
	if err != nil {
		return Config{}, false, UserConfigSelection{}, err
	}
	if !selected {
		path, err := UserConfigPath()
		if err != nil {
			return Config{}, false, UserConfigSelection{}, err
		}
		selection = UserConfigSelection{
			Profile: defaultProfileName,
			Path:    path,
		}
	}
	cfg, ok, err := loadUserConfigAtPath(selection.Path)
	return cfg, ok, selection, err
}

func loadUserConfigAtPath(path string) (Config, bool, error) {
	raw, ok, err := loadJSONConfig(path)
	if err != nil {
		return Config{}, false, err
	}
	if !ok {
		return Config{}, false, nil
	}

	cfg, err := finalizeRawConfig(raw)
	if err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, "/")
	lowerValue := strings.ToLower(value)
	if strings.HasSuffix(lowerValue, "/umbraco") {
		value = value[:len(value)-len("/umbraco")]
		value = strings.TrimRight(value, "/")
	}
	return value
}

func normalizeBaseURL(raw string) string {
	return NormalizeBaseURL(raw)
}

func ParseOutputFormat(raw string) (OutputFormat, error) {
	switch OutputFormat(strings.ToLower(strings.TrimSpace(raw))) {
	case OutputJSON:
		return OutputJSON, nil
	case OutputTable:
		return OutputTable, nil
	case OutputPlain:
		return OutputPlain, nil
	default:
		return "", fmt.Errorf("invalid output format %q (expected json|table|plain)", raw)
	}
}

func (c Config) ValidateAuth() error {
	if c.ClientID == "" || c.ClientSecret == "" {
		return fmt.Errorf("missing UMBRACO_CLIENT_ID or UMBRACO_CLIENT_SECRET; set process env or use project .umbraco-cli.env, .env, or .umbracorc(.json)")
	}
	return nil
}
