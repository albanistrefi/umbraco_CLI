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
	Client                *api.Client
	Config                config.Config
	HTTPClient            *http.Client
	EnvOutput             config.OutputFormat
	OutputFlag            *string
	EnvOutputProvider     func() config.OutputFormat
	ConfigOptionsProvider func() config.LoadOptions
}

func (d Dependencies) requestedOutput() string {
	if d.OutputFlag == nil {
		return ""
	}
	return *d.OutputFlag
}

func (d Dependencies) envOutput() config.OutputFormat {
	if d.EnvOutputProvider != nil {
		return d.EnvOutputProvider()
	}
	return d.EnvOutput
}

func (d Dependencies) configOptions() config.LoadOptions {
	if d.ConfigOptionsProvider != nil {
		return d.ConfigOptionsProvider()
	}
	return config.LoadOptions{}
}

func printResult(cmd *cobra.Command, deps Dependencies, data any) error {
	return output.Print(data, deps.requestedOutput(), deps.envOutput(), cmd.OutOrStdout())
}

// addPaginationFlags registers --skip/--take with the same -1-sentinel
// convention already used by documentSearch. Sentinel rather than Changed()
// because the helper has no easy way to access cmd.Flags() at apply time
// without coupling, and -1 is a value the API itself rejects so collision
// is impossible.
func addPaginationFlags(cmd *cobra.Command, skip *int, take *int) {
	cmd.Flags().IntVar(skip, "skip", -1, "Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections)")
	cmd.Flags().IntVar(take, "take", -1, "Take count (passes through as ?take=N; combine with --skip to page)")
}

// addAutoPaginationFlag registers --all on collection commands that support
// auto-paging. Separate from addPaginationFlags so commands that genuinely
// need only a single page (e.g. previews) can register --skip/--take without
// gaining --all by accident.
func addAutoPaginationFlag(cmd *cobra.Command, all *bool) {
	cmd.Flags().BoolVar(all, "all", false, "Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling.")
}

// applyPaginationParams folds skip/take into an existing params map, leaving
// it nil-safe so callers don't need to pre-allocate when no other params
// are present.
func applyPaginationParams(params map[string]any, skip int, take int) map[string]any {
	if skip < 0 && take < 0 {
		return params
	}
	if params == nil {
		params = map[string]any{}
	}
	if skip >= 0 {
		params["skip"] = skip
	}
	if take >= 0 {
		params["take"] = take
	}
	return params
}

func addReadTriageFlags(cmd *cobra.Command, opts *readTriageOptions) {
	cmd.Flags().BoolVar(&opts.Summarize, "summarize", false, "Return only id/name/alias fields for item collections")
	cmd.Flags().BoolVar(&opts.IDsOnly, "ids-only", false, "Return only item IDs for item collections")
	cmd.Flags().IntVar(&opts.FirstN, "first-n", 0, "Return only the first N items from item collections")
}

func addDocumentOutputTrimFlags(cmd *cobra.Command, opts *outputTrimOptions) {
	cmd.Flags().StringVar(&opts.Fields, "fields", "", "Project response fields client-side; supports comma-separated dotted paths such as id,name,documentType.alias,values.bodyText")
	cmd.Flags().BoolVar(&opts.Summary, "summary", false, "Return a compact document shape with id, name, documentType, route/url, and state/date fields when present")
	cmd.Flags().BoolVar(&opts.NoEmpty, "no-empty", false, "Omit null, empty string, empty array, and empty object values from trimmed output")
	cmd.Flags().BoolVar(&opts.Full, "full", false, "Return the full payload explicitly; cannot be combined with --fields, --summary, or --no-empty")
}

func resolveOutputFormat(deps Dependencies) (config.OutputFormat, error) {
	if requested := strings.TrimSpace(deps.requestedOutput()); requested != "" {
		return config.ParseOutputFormat(requested)
	}
	if envOutput := deps.envOutput(); envOutput != "" {
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
		if err := validateValuesEntries(payload); err != nil {
			return nil, err
		}
		return map[string]any{"values": payload}, nil

	case map[string]any:
		if values, isEnvelope := payload["values"].([]any); isEnvelope && len(payload) == 1 {
			if err := validateValuesEntries(values); err != nil {
				return nil, err
			}
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

// validateValuesEntries enforces that every entry in a values[]-shaped array
// carries both 'alias' and 'value'. An explicit "value": null is fine (Umbraco
// treats it as "clear the value"), but a missing value key is rejected —
// otherwise the merge preserves the existing value and the PUT silently
// no-op's on that property, recreating the exact footgun this surface was
// built to prevent.
func validateValuesEntries(items []any) error {
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("values entry %d must be an object with 'alias' and 'value' keys", i)
		}
		alias, hasAlias := entry["alias"]
		if !hasAlias {
			return fmt.Errorf("values entry %d is missing required 'alias' key", i)
		}
		if _, hasValue := entry["value"]; !hasValue {
			return fmt.Errorf("values entry %d (alias %q) is missing required 'value' key; pass \"value\":null to clear", i, alias)
		}
	}
	return nil
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
