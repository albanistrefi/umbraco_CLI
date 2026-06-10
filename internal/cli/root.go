package cli

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands"
)

func NewRootCommand() *cobra.Command {
	runtime := NewRuntime()

	var outputFormat string

	root := &cobra.Command{
		Use:           "umbraco",
		Short:         "Umbraco CLI - Agent-first wrapper around the Umbraco Management API",
		Version:       commands.CLIVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("umbraco-cli {{.Version}}\n")

	root.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: json, table, plain")

	deps := commands.Dependencies{
		Client:     runtime.Client,
		Config:     runtime.Config,
		HTTPClient: runtime.HTTPClient,
		EnvOutput:  runtime.Config.OutputFormat,
		OutputFlag: &outputFormat,
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
	commands.RegisterAuth(root, deps)
	commands.RegisterSchema(root, deps)
	commands.RegisterGenerateSkills(root, deps)

	return root
}
