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

func addReadTriageFlags(cmd *cobra.Command, opts *readTriageOptions) {
	cmd.Flags().BoolVar(&opts.Summarize, "summarize", false, "Return only id/name/alias fields for item collections")
	cmd.Flags().BoolVar(&opts.IDsOnly, "ids-only", false, "Return only item IDs for item collections")
	cmd.Flags().IntVar(&opts.FirstN, "first-n", 0, "Return only the first N items from item collections")
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

// buildUpdatePropertiesPatch normalizes the three accepted input shapes for
// "<resource> update-properties --json" into a {"values":[...]} envelope ready
// to merge into the current resource via mergeAliasPayload. Used by both
// document update-properties and member update-properties — any future
// resource with the same values[] shape can reuse it.
//
// Returning a structured error here is what prevents the v0.3.15 footgun
// where object-shape input landed at the resource root instead of inside
// values[] and silently no-op'd against the Management API.
func buildUpdatePropertiesPatch(raw string) (map[string]any, error) {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("invalid --json: %w", err)
	}

	switch payload := parsed.(type) {
	case []any:
		for i, item := range payload {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("--json array entry %d must be an object with 'alias' and 'value' keys", i)
			}
			if _, hasAlias := entry["alias"]; !hasAlias {
				return nil, fmt.Errorf("--json array entry %d is missing required 'alias' key", i)
			}
		}
		return map[string]any{"values": payload}, nil

	case map[string]any:
		if values, isEnvelope := payload["values"].([]any); isEnvelope && len(payload) == 1 {
			return map[string]any{"values": values}, nil
		}
		values := make([]any, 0, len(payload))
		for alias, value := range payload {
			values = append(values, map[string]any{
				"alias":   alias,
				"value":   value,
				"culture": nil,
				"segment": nil,
			})
		}
		return map[string]any{"values": values}, nil

	default:
		return nil, fmt.Errorf("--json must be an object, an array of values entries, or an envelope {\"values\":[...]}; got %T", parsed)
	}
}

// coalescePutResult returns true for a real (non-dry-run) PUT whose response
// body was empty (Umbraco answers 204 No Content for successful update /
// publish calls on documents, members, etc.). The previous behaviour of
// returning the raw nil here surfaced as {"updated":null} or
// {"published":null} in command output, which scripts could not distinguish
// from failure.
func coalescePutResult(result any, dryRun bool) any {
	if dryRun {
		return result
	}
	if result == nil {
		return true
	}
	return result
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
