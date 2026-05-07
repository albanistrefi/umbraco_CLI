package commands

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

var verifyStoredAuth = func(cfg config.Config, httpClient *http.Client) error {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	provider := auth.New(cfg, httpClient)
	_, err := provider.AccessToken(context.Background())
	return err
}

func RegisterAuth(root *cobra.Command, deps Dependencies) {
	authCmd := &cobra.Command{Use: "auth", Short: "Persistent authentication helpers"}
	authCmd.AddCommand(authLogin(deps))
	authCmd.AddCommand(authStatus(deps))
	authCmd.AddCommand(authLogout(deps))
	root.AddCommand(authCmd)
}

func authLogin(deps Dependencies) *cobra.Command {
	var baseURL string
	var clientID string
	var clientSecret string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store Umbraco API credentials in the user config after verifying them",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			baseURL, err = promptIfEmpty(cmd, baseURL, "Base URL: ")
			if err != nil {
				return err
			}
			clientID, err = promptIfEmpty(cmd, clientID, "Client ID: ")
			if err != nil {
				return err
			}
			clientSecret, err = promptIfEmpty(cmd, clientSecret, "Client secret: ")
			if err != nil {
				return err
			}

			cfg := config.Config{
				BaseURL:      baseURL,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}
			cfg.BaseURL = config.NormalizeBaseURL(cfg.BaseURL)

			if err := verifyStoredAuth(cfg, deps.HTTPClient); err != nil {
				return err
			}

			if dryRun {
				return printResult(cmd, deps, map[string]any{
					"loggedIn": false,
					"dryRun":   true,
					"baseUrl":  cfg.BaseURL,
					"source":   "user-config",
					"message":  "credentials verified and would be saved",
				})
			}

			existing, ok, err := config.LoadUserConfig()
			if err != nil {
				return err
			}
			if !ok {
				existing = config.Config{}
			}
			existing.BaseURL = cfg.BaseURL
			existing.ClientID = cfg.ClientID
			existing.ClientSecret = cfg.ClientSecret
			if err := config.WriteUserConfig(existing); err != nil {
				return err
			}

			return printResult(cmd, deps, map[string]any{
				"loggedIn": true,
				"baseUrl":  cfg.BaseURL,
				"source":   "user-config",
			})
		},
	}

	cmd.Flags().StringVar(&baseURL, "base-url", "", "Umbraco base URL")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Management API client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "Management API client secret")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify credentials without persisting them")
	return cmd
}

func authStatus(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current auth/config status without exposing secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			env := currentAuthEnv()
			userConfig, hasUserConfig, err := config.LoadUserConfig()
			if err != nil {
				return err
			}
			resolved, err := config.Load()
			if err != nil {
				return err
			}

			source := "unconfigured"
			switch {
			case env["UMBRACO_CLIENT_ID"] != "" || env["UMBRACO_CLIENT_SECRET"] != "":
				source = "env"
			case hasUserConfig && (userConfig.ClientID != "" || userConfig.ClientSecret != ""):
				source = "user-config"
			}

			verified := false
			var authError string
			if resolved.ClientID != "" && resolved.ClientSecret != "" {
				if err := verifyStoredAuth(resolved, deps.HTTPClient); err != nil {
					authError = err.Error()
				} else {
					verified = true
				}
			}

			return printResult(cmd, deps, map[string]any{
				"authenticated": resolved.ClientID != "" && resolved.ClientSecret != "",
				"verified":      verified,
				"baseUrl":       resolved.BaseURL,
				"source":        source,
				"authError":     authError,
				"userConfig": map[string]any{
					"present":         hasUserConfig,
					"hasClientID":     hasUserConfig && userConfig.ClientID != "",
					"hasClientSecret": hasUserConfig && userConfig.ClientSecret != "",
					"baseUrl":         userConfig.BaseURL,
				},
				"env": map[string]any{
					"hasBaseURL":      env["UMBRACO_BASE_URL"] != "",
					"hasClientID":     env["UMBRACO_CLIENT_ID"] != "",
					"hasClientSecret": env["UMBRACO_CLIENT_SECRET"] != "",
				},
			})
		},
	}
}

func authLogout(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials from the user config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				return printResult(cmd, deps, map[string]any{
					"loggedOut": false,
					"dryRun":    true,
					"source":    "user-config",
					"message":   "stored credentials would be removed",
				})
			}
			if err := config.ClearUserAuth(); err != nil {
				return err
			}
			return printResult(cmd, deps, map[string]any{
				"loggedOut": true,
				"source":    "user-config",
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview logout without modifying the user config")
	return cmd
}

func promptIfEmpty(cmd *cobra.Command, current string, prompt string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current), nil
	}

	if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
		return "", err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return "", fmt.Errorf("missing required input")
	}
	return value, nil
}

func currentAuthEnv() map[string]string {
	return map[string]string{
		"UMBRACO_BASE_URL":      strings.TrimSpace(os.Getenv("UMBRACO_BASE_URL")),
		"UMBRACO_CLIENT_ID":     strings.TrimSpace(os.Getenv("UMBRACO_CLIENT_ID")),
		"UMBRACO_CLIENT_SECRET": strings.TrimSpace(os.Getenv("UMBRACO_CLIENT_SECRET")),
	}
}
