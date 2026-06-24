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

	root := &cobra.Command{
		Use:           "umbraco",
		Short:         "Umbraco CLI - Agent-first wrapper around the Umbraco Management API",
		Version:       commands.CLIVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			opts := config.LoadOptions{Profile: profile, ConfigPath: configPath}
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

	deps := commands.Dependencies{
		Client:     runtime.Client,
		Config:     runtime.Config,
		HTTPClient: runtime.HTTPClient,
		EnvOutputProvider: func() config.OutputFormat {
			return runtime.Config.OutputFormat
		},
		OutputFlag: &outputFormat,
		ConfigOptionsProvider: func() config.LoadOptions {
			return config.LoadOptions{Profile: profile, ConfigPath: configPath}
		},
	}

	commands.RegisterDocument(root, deps)
	commands.RegisterDictionary(root, deps)
	commands.RegisterMedia(root, deps)
	commands.RegisterDoctype(root, deps)
	commands.RegisterDatatype(root, deps)
	commands.RegisterTemplate(root, deps)
	commands.RegisterForms(root, deps)
	commands.RegisterModelsBuilder(root, deps)
	commands.RegisterMember(root, deps)
	commands.RegisterMemberGroup(root, deps)
	commands.RegisterWebhook(root, deps)
	commands.RegisterLanguage(root, deps)
	commands.RegisterUser(root, deps)
	commands.RegisterUserGroup(root, deps)
	commands.RegisterLogs(root, deps)
	commands.RegisterServer(root, deps)
	commands.RegisterHealth(root, deps)
	commands.RegisterTree(root, deps)
	commands.RegisterAPI(root, deps)
	commands.RegisterAuth(root, deps)
	commands.RegisterAutomate(root, deps)
	commands.RegisterSchema(root, deps)
	commands.RegisterGenerateSkills(root, deps)

	return root
}

func allowsMissingSelectedConfig(cmd *cobra.Command) bool {
	return cmd.CommandPath() == "umbraco auth login"
}
