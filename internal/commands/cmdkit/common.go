package cmdkit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/output"
)

func PrintResult(cmd *cobra.Command, deps Dependencies, data any) error {
	return output.Print(data, deps.RequestedOutput(), deps.CurrentEnvOutput(), cmd.OutOrStdout())
}

// AddPaginationFlags registers --skip/--take with the same -1-sentinel
// convention already used by documentSearch. Sentinel rather than Changed()
// because the helper has no easy way to access cmd.Flags() at apply time
// without coupling, and -1 is a value the API itself rejects so collision
// is impossible.
func AddPaginationFlags(cmd *cobra.Command, skip *int, take *int) {
	cmd.Flags().IntVar(skip, "skip", -1, "Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections)")
	cmd.Flags().IntVar(take, "take", -1, "Take count (passes through as ?take=N; combine with --skip to page)")
}

// AddAutoPaginationFlag registers --all on collection commands that support
// auto-paging. Separate from addPaginationFlags so commands that genuinely
// need only a single page (e.g. previews) can register --skip/--take without
// gaining --all by accident.
func AddAutoPaginationFlag(cmd *cobra.Command, all *bool) {
	cmd.Flags().BoolVar(all, "all", false, "Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling.")
}

// ApplyPaginationParams folds skip/take into an existing params map, leaving
// it nil-safe so callers don't need to pre-allocate when no other params
// are present.
func ApplyPaginationParams(params map[string]any, skip int, take int) map[string]any {
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

func AddReadTriageFlags(cmd *cobra.Command, opts *ReadTriageOptions) {
	cmd.Flags().BoolVar(&opts.Summarize, "summarize", false, "Return only id/name/alias fields for item collections")
	cmd.Flags().BoolVar(&opts.IDsOnly, "ids-only", false, "Return only item IDs for item collections")
	cmd.Flags().IntVar(&opts.FirstN, "first-n", 0, "Return only the first N items from item collections")
}

func AddDocumentOutputTrimFlags(cmd *cobra.Command, opts *OutputTrimOptions) {
	cmd.Flags().StringVar(&opts.Fields, "fields", "", "Project response fields client-side; supports comma-separated dotted paths such as id,name,documentType.alias,values.bodyText")
	cmd.Flags().BoolVar(&opts.Summary, "summary", false, "Return a compact document shape with id, name, documentType, route/url, and state/date fields when present")
	cmd.Flags().BoolVar(&opts.NoEmpty, "no-empty", false, "Omit null, empty string, empty array, and empty object values from trimmed output")
	cmd.Flags().BoolVar(&opts.Full, "full", false, "Return the full payload explicitly; cannot be combined with --fields, --summary, or --no-empty")
}

func ParseJSONObject(raw string, label string) (map[string]any, error) {
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

func ParseParams(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return ParseJSONObject(raw, "--params")
}

func ParsePayload(raw string) (map[string]any, error) {
	return ParseJSONObject(raw, "--json")
}

func RequireValue(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("missing required option: %s", name)
	}
	return nil
}

func OptionalBody(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	return ParsePayload(raw)
}

// RequireForceOrDryRun gates destructive commands behind explicit intent:
// the caller must rehearse with --dry-run or confirm with --force. reason
// names the consequence (e.g. "permanently deletes").
func RequireForceOrDryRun(cmd *cobra.Command, reason string, force bool, dryRun bool) error {
	if force || dryRun {
		return nil
	}
	return fmt.Errorf("%s %s; pass --force to confirm or --dry-run to rehearse", cmd.CommandPath(), reason)
}
