package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OutputFormat string

const defaultBaseURL = "https://localhost:44391"
const defaultProfileName = "default"

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
}

type UserConfigSelection struct {
	Profile    string
	Path       string
	CustomPath bool
	Active     bool
	Explicit   bool
}

type UserProfile struct {
	Name      string
	Path      string
	Config    Config
	Active    bool
	IsDefault bool
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

func ProfileConfigPath(profile string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return profileConfigPath(homeDir, profile)
}

func ActiveProfile() (string, bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	return readActiveProfile(homeDir)
}

func SetActiveProfile(profile string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	normalized, err := normalizeProfileName(profile)
	if err != nil {
		return err
	}
	path := activeProfilePath(homeDir)
	if normalized == "" || normalized == defaultProfileName {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(normalized+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
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

func loadResolvedConfig(workingDir string, homeDir string, env map[string]string) (Config, error) {
	resolved := rawConfig{}

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

	// Crawling the working tree for a .NET host project is expensive, so it
	// only runs when no explicit source supplied a base URL. Discovery is
	// best-effort: an unreadable or malformed file nearby must never make
	// the CLI unusable.
	if strings.TrimSpace(resolved.BaseURL) == "" {
		if discoveredBaseURL, ok := discoverBaseURL(workingDir); ok {
			resolved.BaseURL = discoveredBaseURL
		}
	}

	return finalizeRawConfig(resolved)
}

func (s UserConfigSelection) IsDefault() bool {
	return s.Profile == "" || s.Profile == defaultProfileName
}

func selectedConfigPath(workingDir string, homeDir string, opts LoadOptions) (UserConfigSelection, bool, error) {
	if strings.TrimSpace(opts.ConfigPath) != "" {
		path, err := expandConfigPath(opts.ConfigPath, workingDir, homeDir)
		if err != nil {
			return UserConfigSelection{}, false, err
		}
		return UserConfigSelection{
			Profile:    "",
			Path:       path,
			CustomPath: true,
			Explicit:   true,
		}, true, nil
	}

	if strings.TrimSpace(opts.Profile) != "" {
		path, err := profileConfigPath(homeDir, opts.Profile)
		if err != nil {
			return UserConfigSelection{}, false, err
		}
		profile, _ := normalizeProfileName(opts.Profile)
		return UserConfigSelection{
			Profile:  profile,
			Path:     path,
			Explicit: true,
		}, true, nil
	}

	active, ok, err := readActiveProfile(homeDir)
	if err != nil {
		return UserConfigSelection{}, false, err
	}
	if ok {
		path, err := profileConfigPath(homeDir, active)
		if err != nil {
			return UserConfigSelection{}, false, err
		}
		return UserConfigSelection{
			Profile: active,
			Path:    path,
			Active:  true,
		}, true, nil
	}

	return UserConfigSelection{}, false, nil
}

func expandConfigPath(rawPath string, workingDir string, homeDir string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("config path cannot be empty")
	}
	if strings.HasPrefix(path, "~/") {
		if homeDir == "" {
			return "", fmt.Errorf("cannot expand %q without a home directory", rawPath)
		}
		path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(path) {
		base := workingDir
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path), nil
}

func profileConfigPath(homeDir string, profile string) (string, error) {
	normalized, err := normalizeProfileName(profile)
	if err != nil {
		return "", err
	}
	if homeDir == "" {
		return "", fmt.Errorf("cannot resolve profile %q without a home directory", normalized)
	}
	if normalized == "" || normalized == defaultProfileName {
		return filepath.Join(homeDir, ".umbraco", "config.json"), nil
	}
	return filepath.Join(homeDir, ".umbraco", normalized+".config.json"), nil
}

func normalizeProfileName(profile string) (string, error) {
	value := strings.TrimSpace(profile)
	if value == "" {
		return "", nil
	}
	if value == defaultProfileName {
		return defaultProfileName, nil
	}
	if strings.Contains(value, "..") {
		return "", fmt.Errorf("invalid profile name %q: profile names cannot contain '..'", profile)
	}
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_' ||
			r == '.'
		if !valid {
			return "", fmt.Errorf("invalid profile name %q: use letters, numbers, '.', '-' or '_'", profile)
		}
	}
	return value, nil
}

func activeProfilePath(homeDir string) string {
	return filepath.Join(homeDir, ".umbraco", "active-profile")
}

func readActiveProfile(homeDir string) (string, bool, error) {
	if homeDir == "" {
		return "", false, nil
	}
	payload, err := os.ReadFile(activeProfilePath(homeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	profile, err := normalizeProfileName(string(payload))
	if err != nil {
		return "", false, err
	}
	if profile == "" || profile == defaultProfileName {
		return "", false, nil
	}
	return profile, true, nil
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
	return writeUserConfigAtPath(selection.Path, cfg)
}

func ListUserProfiles() ([]UserProfile, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(homeDir, ".umbraco")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	active, hasActive, err := readActiveProfile(homeDir)
	if err != nil {
		return nil, err
	}

	profiles := make([]UserProfile, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		profileName := ""
		isDefault := false
		switch {
		case name == "config.json":
			profileName = defaultProfileName
			isDefault = true
		case strings.HasSuffix(name, ".config.json"):
			profileName = strings.TrimSuffix(name, ".config.json")
		default:
			continue
		}
		if _, err := normalizeProfileName(profileName); err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		cfg, ok, err := loadUserConfigAtPath(path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		profiles = append(profiles, UserProfile{
			Name:      profileName,
			Path:      path,
			Config:    cfg,
			Active:    (hasActive && active == profileName) || (!hasActive && isDefault),
			IsDefault: isDefault,
		})
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].IsDefault != profiles[j].IsDefault {
			return profiles[i].IsDefault
		}
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
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

func discoverBaseURL(workingDir string) (string, bool) {
	if chosen, ok := discoverBaseURLFromSearchRoots(searchRootsForCurrentTree(workingDir)); ok {
		return chosen, true
	}

	if chosen, ok := discoverBaseURLFromSearchRoots(searchRootsForSiblingProjects(workingDir)); ok {
		return chosen, true
	}

	return "", false
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

func discoverBaseURLFromSearchRoots(roots []string) (string, bool) {
	urls := make([]string, 0)

	for _, root := range roots {
		urls = append(urls, collectBaseURLsFromRoot(root)...)
	}

	return choosePreferredURL(urls)
}

func collectBaseURLsFromRoot(root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}

	urls := make([]string, 0)
	for _, candidate := range []string{
		filepath.Join(root, "Properties", "launchSettings.json"),
		filepath.Join(root, "appsettings.Development.json"),
		filepath.Join(root, "appsettings.json"),
	} {
		urls = append(urls, collectJSONConfigURLs(candidate)...)
	}

	return urls
}

func collectJSONConfigURLs(path string) []string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var urls []string
	switch filepath.Base(path) {
	case "launchSettings.json":
		urls, err = collectLaunchSettingsURLs(payload)
	case "appsettings.Development.json", "appsettings.json":
		urls, err = collectAppSettingsURLs(payload)
	}
	if err != nil {
		return nil
	}
	return urls
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
