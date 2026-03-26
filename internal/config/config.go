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

func Load() (Config, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	env := currentEnv()
	return loadResolvedConfig(workingDir, homeDir, env)
}

type rawConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	OutputFormat string
}

type jsonConfigFile struct {
	BaseURL       string `json:"baseUrl"`
	BaseURLLegacy string `json:"baseURL"`
	ClientID      string `json:"clientId"`
	ClientSecret  string `json:"clientSecret"`
	OutputFormat  string `json:"outputFormat"`
}

func loadResolvedConfig(workingDir string, homeDir string, env map[string]string) (Config, error) {
	resolved := rawConfig{BaseURL: defaultBaseURL}

	if discoveredBaseURL, ok, err := discoverBaseURL(workingDir); err != nil {
		return Config{}, err
	} else if ok {
		resolved.BaseURL = discoveredBaseURL
	}

	if homeDir != "" {
		userConfig, ok, err := loadJSONConfig(filepath.Join(homeDir, ".umbraco", "config.json"))
		if err != nil {
			return Config{}, err
		}
		if ok {
			mergeRawConfig(&resolved, userConfig)
		}
	}

	if dotEnvPath, ok := findNearestFile(workingDir, ".env"); ok {
		dotEnvConfig, err := loadDotEnvConfig(dotEnvPath)
		if err != nil {
			return Config{}, err
		}
		mergeRawConfig(&resolved, dotEnvConfig)
	}

	if cliDotEnvPath, ok := findNearestFile(workingDir, ".umbraco-cli.env"); ok {
		cliDotEnvConfig, err := loadDotEnvConfig(cliDotEnvPath)
		if err != nil {
			return Config{}, err
		}
		mergeRawConfig(&resolved, cliDotEnvConfig)
	}

	if projectConfigPath, ok := findNearestFileFromCandidates(workingDir, ".umbracorc.json", ".umbracorc"); ok {
		projectConfig, _, err := loadJSONConfig(projectConfigPath)
		if err != nil {
			return Config{}, err
		}
		mergeRawConfig(&resolved, projectConfig)
	}

	mergeRawConfig(&resolved, rawConfigFromEnv(env))
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

func currentEnv() map[string]string {
	return map[string]string{
		"UMBRACO_BASE_URL":      os.Getenv("UMBRACO_BASE_URL"),
		"UMBRACO_CLIENT_ID":     os.Getenv("UMBRACO_CLIENT_ID"),
		"UMBRACO_CLIENT_SECRET": os.Getenv("UMBRACO_CLIENT_SECRET"),
		"UMBRACO_OUTPUT_FORMAT": os.Getenv("UMBRACO_OUTPUT_FORMAT"),
	}
}

func rawConfigFromEnv(env map[string]string) rawConfig {
	return rawConfig{
		BaseURL:      strings.TrimSpace(env["UMBRACO_BASE_URL"]),
		ClientID:     strings.TrimSpace(env["UMBRACO_CLIENT_ID"]),
		ClientSecret: strings.TrimSpace(env["UMBRACO_CLIENT_SECRET"]),
		OutputFormat: strings.TrimSpace(env["UMBRACO_OUTPUT_FORMAT"]),
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
	}, true, nil
}

func loadDotEnvConfig(path string) (rawConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return rawConfig{}, err
	}
	defer file.Close()

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

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line %q", line)
		}

		key = strings.TrimSpace(key)
		if !strings.HasPrefix(key, "UMBRACO_") {
			continue
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

func findNearestFileFromCandidates(workingDir string, candidates ...string) (string, bool) {
	for _, candidate := range candidates {
		if path, ok := findNearestFile(workingDir, candidate); ok {
			return path, true
		}
	}
	return "", false
}

func discoverBaseURL(workingDir string) (string, bool, error) {
	if chosen, ok, err := discoverBaseURLFromSearchRoots(searchRootsForCurrentTree(workingDir)); err != nil {
		return "", false, err
	} else if ok {
		return chosen, true, nil
	}

	if chosen, ok, err := discoverBaseURLFromSearchRoots(searchRootsForSiblingProjects(workingDir)); err != nil {
		return "", false, err
	} else if ok {
		return chosen, true, nil
	}

	return "", false, nil
}

func searchRootsForCurrentTree(workingDir string) []string {
	roots := make([]string, 0)
	if strings.TrimSpace(workingDir) == "" {
		return roots
	}

	dir := workingDir
	for {
		roots = append(roots, dir)

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return roots
}

func searchRootsForSiblingProjects(workingDir string) []string {
	if strings.TrimSpace(workingDir) == "" {
		return nil
	}

	parent := filepath.Dir(workingDir)
	if parent == workingDir || strings.TrimSpace(parent) == "" {
		return nil
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}

	currentBase := filepath.Clean(workingDir)
	roots := make([]string, 0)
	seen := map[string]struct{}{}

	appendRoot := func(path string) {
		cleaned := filepath.Clean(path)
		if cleaned == currentBase {
			return
		}
		if !isLikelyUmbracoHostProject(cleaned) {
			return
		}
		if _, exists := seen[cleaned]; exists {
			return
		}
		seen[cleaned] = struct{}{}
		roots = append(roots, cleaned)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		siblingRoot := filepath.Join(parent, entry.Name())
		appendRoot(siblingRoot)

		nestedEntries, err := os.ReadDir(siblingRoot)
		if err != nil {
			continue
		}
		for _, nested := range nestedEntries {
			if nested.IsDir() {
				appendRoot(filepath.Join(siblingRoot, nested.Name()))
			}
		}
	}

	return roots
}

func isLikelyUmbracoHostProject(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}

	hostProjects := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".csproj" {
			continue
		}

		payload, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}

		contents := string(payload)
		if strings.Contains(contents, "Microsoft.NET.Sdk.Web") && strings.Contains(contents, "Umbraco.Cms") {
			hostProjects++
		}
	}

	return hostProjects == 1
}

func discoverBaseURLFromSearchRoots(roots []string) (string, bool, error) {
	urls := make([]string, 0)

	for _, root := range roots {
		rootURLs, err := collectBaseURLsFromRoot(root)
		if err != nil {
			return "", false, err
		}
		urls = append(urls, rootURLs...)
	}

	if chosen, ok := choosePreferredURL(urls); ok {
		return chosen, true, nil
	}

	return "", false, nil
}

func collectBaseURLsFromRoot(root string) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}

	urls := make([]string, 0)
	for _, candidate := range []string{
		filepath.Join(root, "Properties", "launchSettings.json"),
		filepath.Join(root, "appsettings.Development.json"),
		filepath.Join(root, "appsettings.json"),
	} {
		collected, ok, err := collectJSONConfigURLs(candidate)
		if err != nil {
			return nil, err
		}
		if ok {
			urls = append(urls, collected...)
		}
	}

	return urls, nil
}

func collectJSONConfigURLs(path string) ([]string, bool, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	switch filepath.Base(path) {
	case "launchSettings.json":
		urls, err := collectLaunchSettingsURLs(payload)
		if err != nil {
			return nil, false, fmt.Errorf("invalid config file %s: %w", path, err)
		}
		return urls, true, nil
	case "appsettings.Development.json", "appsettings.json":
		urls, err := collectAppSettingsURLs(payload)
		if err != nil {
			return nil, false, fmt.Errorf("invalid config file %s: %w", path, err)
		}
		return urls, true, nil
	default:
		return nil, true, nil
	}
}

func collectLaunchSettingsURLs(payload []byte) ([]string, error) {
	var decoded struct {
		Profiles map[string]struct {
			ApplicationURL string `json:"applicationUrl"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	results := make([]string, 0)
	for _, profile := range decoded.Profiles {
		for _, candidate := range splitURLCandidates(profile.ApplicationURL) {
			if normalized := normalizeBaseURL(candidate); normalized != "" {
				results = append(results, normalized)
			}
		}
	}
	return results, nil
}

func collectAppSettingsURLs(payload []byte) ([]string, error) {
	var decoded struct {
		Kestrel struct {
			Endpoints map[string]struct {
				URL string `json:"Url"`
			} `json:"Endpoints"`
		} `json:"Kestrel"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	results := make([]string, 0)
	for _, endpoint := range decoded.Kestrel.Endpoints {
		for _, candidate := range splitURLCandidates(endpoint.URL) {
			if normalized := normalizeBaseURL(candidate); normalized != "" {
				results = append(results, normalized)
			}
		}
	}
	return results, nil
}

func splitURLCandidates(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ','
	})

	results := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			results = append(results, trimmed)
		}
	}
	return results
}

func choosePreferredURL(candidates []string) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}

	unique := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}

	if len(unique) == 1 {
		return unique[0], true
	}

	httpsOnly := make([]string, 0, len(unique))
	for _, candidate := range unique {
		if strings.HasPrefix(strings.ToLower(candidate), "https://") {
			httpsOnly = append(httpsOnly, candidate)
		}
	}
	if len(httpsOnly) == 1 {
		return httpsOnly[0], true
	}

	return "", false
}

func normalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, "/")
	lowerValue := strings.ToLower(value)
	if strings.HasSuffix(lowerValue, "/umbraco") {
		value = value[:len(value)-len("/umbraco")]
		value = strings.TrimRight(value, "/")
	}
	return value
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
