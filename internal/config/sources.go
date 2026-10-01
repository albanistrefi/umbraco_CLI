package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type rawConfig struct {
	BaseURL                     string
	ClientID                    string
	ClientSecret                string
	OutputFormat                string
	BasicAuthSharedSecret       string
	BasicAuthSharedSecretHeader string
}

type jsonConfigFile struct {
	BaseURL       string `json:"baseUrl"`
	BaseURLLegacy string `json:"baseURL"`
	ClientID      string `json:"clientId"`
	ClientSecret  string `json:"clientSecret"`
	OutputFormat  string `json:"outputFormat"`

	BasicAuthSharedSecret       string `json:"basicAuthSharedSecret,omitempty"`
	BasicAuthSharedSecretHeader string `json:"basicAuthSharedSecretHeader,omitempty"`
}

func currentEnv() map[string]string {
	return map[string]string{
		"UMBRACO_BASE_URL":      os.Getenv("UMBRACO_BASE_URL"),
		"UMBRACO_CLIENT_ID":     os.Getenv("UMBRACO_CLIENT_ID"),
		"UMBRACO_CLIENT_SECRET": os.Getenv("UMBRACO_CLIENT_SECRET"),
		"UMBRACO_OUTPUT_FORMAT": os.Getenv("UMBRACO_OUTPUT_FORMAT"),

		"UMBRACO_BASIC_AUTH_SHARED_SECRET":        os.Getenv("UMBRACO_BASIC_AUTH_SHARED_SECRET"),
		"UMBRACO_BASIC_AUTH_SHARED_SECRET_HEADER": os.Getenv("UMBRACO_BASIC_AUTH_SHARED_SECRET_HEADER"),
	}
}

func rawConfigFromEnv(env map[string]string) rawConfig {
	return rawConfig{
		BaseURL:      strings.TrimSpace(env["UMBRACO_BASE_URL"]),
		ClientID:     strings.TrimSpace(env["UMBRACO_CLIENT_ID"]),
		ClientSecret: strings.TrimSpace(env["UMBRACO_CLIENT_SECRET"]),
		OutputFormat: strings.TrimSpace(env["UMBRACO_OUTPUT_FORMAT"]),

		BasicAuthSharedSecret:       strings.TrimSpace(env["UMBRACO_BASIC_AUTH_SHARED_SECRET"]),
		BasicAuthSharedSecretHeader: strings.TrimSpace(env["UMBRACO_BASIC_AUTH_SHARED_SECRET_HEADER"]),
	}
}

func mergeRawConfig(target *rawConfig, source rawConfig) {
	if strings.TrimSpace(source.BaseURL) != "" {
		target.BaseURL = source.BaseURL
	}
	if strings.TrimSpace(source.ClientID) != "" {
		target.ClientID = source.ClientID
	}
	if strings.TrimSpace(source.ClientSecret) != "" {
		target.ClientSecret = source.ClientSecret
	}
	if strings.TrimSpace(source.OutputFormat) != "" {
		target.OutputFormat = source.OutputFormat
	}
	if strings.TrimSpace(source.BasicAuthSharedSecret) != "" {
		target.BasicAuthSharedSecret = source.BasicAuthSharedSecret
	}
	if strings.TrimSpace(source.BasicAuthSharedSecretHeader) != "" {
		target.BasicAuthSharedSecretHeader = source.BasicAuthSharedSecretHeader
	}
}

func loadJSONConfig(path string) (rawConfig, bool, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rawConfig{}, false, nil
		}
		return rawConfig{}, false, err
	}

	var file jsonConfigFile
	if err := json.Unmarshal(payload, &file); err != nil {
		return rawConfig{}, false, fmt.Errorf("invalid config file %s: %w", path, err)
	}

	baseURL := strings.TrimSpace(file.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(file.BaseURLLegacy)
	}

	return rawConfig{
		BaseURL:      baseURL,
		ClientID:     strings.TrimSpace(file.ClientID),
		ClientSecret: strings.TrimSpace(file.ClientSecret),
		OutputFormat: strings.TrimSpace(file.OutputFormat),

		BasicAuthSharedSecret:       strings.TrimSpace(file.BasicAuthSharedSecret),
		BasicAuthSharedSecretHeader: strings.TrimSpace(file.BasicAuthSharedSecretHeader),
	}, true, nil
}

func loadDotEnvConfig(path string) (rawConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return rawConfig{}, err
	}
	defer func() { _ = file.Close() }()

	values, err := parseDotEnv(file)
	if err != nil {
		return rawConfig{}, fmt.Errorf("invalid dotenv file %s: %w", path, err)
	}
	return rawConfigFromEnv(values), nil
}

func parseDotEnv(reader io.Reader) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		// Dotenv files are shared with other tools, so only UMBRACO_ lines
		// are ours to judge; anything else is skipped, malformed or not.
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !strings.HasPrefix(key, "UMBRACO_") {
			continue
		}
		if !ok {
			return nil, fmt.Errorf("invalid line %q", line)
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}

func findNearestFile(workingDir string, relativePath string) (string, bool) {
	if strings.TrimSpace(workingDir) == "" {
		return "", false
	}

	dir := workingDir
	for {
		candidate := filepath.Join(dir, relativePath)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// findNearestFileFromCandidates checks all candidates per directory before
// moving up, so a nearby .umbracorc is not shadowed by a distant .umbracorc.json.
func findNearestFileFromCandidates(workingDir string, candidates ...string) (string, bool) {
	if strings.TrimSpace(workingDir) == "" {
		return "", false
	}

	dir := workingDir
	for {
		for _, candidate := range candidates {
			path := filepath.Join(dir, candidate)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, true
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// userConfigSource loads ~/.umbraco/config.json; a missing file or home
// directory is not an error, just an empty source.
func userConfigSource(homeDir string) (rawConfig, error) {
	if homeDir == "" {
		return rawConfig{}, nil
	}
	raw, _, err := loadJSONConfig(filepath.Join(homeDir, ".umbraco", "config.json"))
	return raw, err
}

// dotEnvSource loads UMBRACO_* keys from the nearest dotenv file with the
// given name, walking upward from the working directory.
func dotEnvSource(workingDir string, name string) (rawConfig, error) {
	path, ok := findNearestFile(workingDir, name)
	if !ok {
		return rawConfig{}, nil
	}
	return loadDotEnvConfig(path)
}

// projectRCSource loads the nearest .umbracorc.json or .umbracorc.
func projectRCSource(workingDir string) (rawConfig, error) {
	path, ok := findNearestFileFromCandidates(workingDir, ".umbracorc.json", ".umbracorc")
	if !ok {
		return rawConfig{}, nil
	}
	raw, _, err := loadJSONConfig(path)
	return raw, err
}
