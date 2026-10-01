package commands

import (
	"os"
	"strings"

	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
)

// resolveOutputFormat picks the format for commands that render table or
// plain output themselves: --output, else the configured default, else JSON
// when stdout is not a terminal.
func resolveOutputFormat(deps cmdkit.Dependencies) (config.OutputFormat, error) {
	if requested := strings.TrimSpace(deps.RequestedOutput()); requested != "" {
		return config.ParseOutputFormat(requested)
	}
	if envOutput := deps.CurrentEnvOutput(); envOutput != "" {
		return envOutput, nil
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return config.OutputJSON, nil
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return config.OutputJSON, nil
	}
	return config.OutputPlain, nil
}
