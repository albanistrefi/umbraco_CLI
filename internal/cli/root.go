package cli

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands"
	"umbraco-cli/internal/config"
)

func NewRootCommand() *cobra.Command {
	runtime := NewRuntime()

	var outputFormat string
	var profile string
	var configPath string
	var baseURL string

	root := &cobra.Command{
		Use:           "umbraco",
		Short:         "Umbraco CLI - Agent-first wrapper around the Umbraco Management API",
		Version:       commands.CLIVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			opts := config.LoadOptions{Profile: profile, ConfigPath: configPath, BaseURL: baseURL}
			if err := runtime.Reload(opts); err != nil && (profile != "" || configPath != "") {
				if config.IsConfigFileNotFound(err) && allowsMissingSelectedConfig(cmd) {
					return nil
				}
				return err
			}
			return nil
		},
	}
	root.SetVersionTemplate("umbraco-cli {{.Version}}\n")

	root.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: json, table, plain")
	root.PersistentFlags().StringVar(&profile, "profile", "", "User config profile to load from ~/.umbraco/<profile>.config.json")
	root.PersistentFlags().StringVar(&configPath, "config", "", "Explicit Umbraco CLI config file path")
	root.PersistentFlags().StringVar(&baseURL, "base-url", "", "Override the Umbraco base URL from any config source, keeping the resolved credentials (e.g. point a profile at another host)")

	deps := commands.Dependencies{
		Client:     runtime.Client,
		Config:     runtime.Config,
		HTTPClient: runtime.HTTPClient,
		EnvOutputProvider: func() config.OutputFormat {
			return runtime.Config.OutputFormat
		},
		OutputFlag: &outputFormat,
		ConfigOptionsProvider: func() config.LoadOptions {
			return config.LoadOptions{Profile: profile, ConfigPath: configPath, BaseURL: baseURL}
		},
		ConfigProvider: func() config.Config {
			return runtime.Config
		},
	}

	commands.RegisterAll(root, deps)

	return root
}

func allowsMissingSelectedConfig(cmd *cobra.Command) bool {
	return cmd.CommandPath() == "umbraco auth login"
}
