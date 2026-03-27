package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/output"
)

type Dependencies struct {
	Client     *api.Client
	Config     config.Config
	HTTPClient *http.Client
	EnvOutput  config.OutputFormat
	OutputFlag *string
}

func (d Dependencies) requestedOutput() string {
	if d.OutputFlag == nil {
		return ""
	}
	return *d.OutputFlag
}

func printResult(cmd *cobra.Command, deps Dependencies, data any) error {
	return output.Print(data, deps.requestedOutput(), deps.EnvOutput, cmd.OutOrStdout())
}

func resolveOutputFormat(deps Dependencies) (config.OutputFormat, error) {
	if requested := strings.TrimSpace(deps.requestedOutput()); requested != "" {
		return config.ParseOutputFormat(requested)
	}
	if deps.EnvOutput != "" {
		return deps.EnvOutput, nil
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

func parseJSONObject(raw string, label string) (map[string]any, error) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", label, err)
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return obj, nil
}

func parseParams(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return parseJSONObject(raw, "--params")
}

func parsePayload(raw string) (map[string]any, error) {
	return parseJSONObject(raw, "--json")
}

func requireValue(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("missing required option: %s", name)
	}
	return nil
}

func optionalBody(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	return parsePayload(raw)
}

func decodeResult[T any](raw any) (T, error) {
	var result T
	encoded, err := json.Marshal(raw)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return result, err
	}
	return result, nil
}
