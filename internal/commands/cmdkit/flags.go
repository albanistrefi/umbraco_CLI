package cmdkit

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// AddFieldsFlag registers --fields, projected client-side by the builders.
func AddFieldsFlag(cmd *cobra.Command, fields *string) {
	cmd.Flags().StringVar(fields, "fields", "", "Limit response fields (comma-separated top-level keys)")
}

// AddDryRunFlag registers --dry-run; every mutation takes it.
func AddDryRunFlag(cmd *cobra.Command, dryRun *bool) {
	cmd.Flags().BoolVar(dryRun, "dry-run", false, "Print the planned request without executing")
}

// AddPaginationFlags registers --skip/--take with the same -1-sentinel
// convention used across the collection reads. Sentinel rather than Changed()
// because the helper has no easy way to access cmd.Flags() at apply time
// without coupling, and -1 is a value the API itself rejects so collision
// is impossible.
func AddPaginationFlags(cmd *cobra.Command, skip *int, take *int) {
	cmd.Flags().IntVar(skip, "skip", -1, "Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections)")
	cmd.Flags().IntVar(take, "take", -1, "Take count (passes through as ?take=N; combine with --skip to page)")
}

// AddAutoPaginationFlag registers --all on collection commands that support
// auto-paging. Separate from AddPaginationFlags so commands that genuinely
// need only a single page (e.g. previews) can register --skip/--take without
// gaining --all by accident.
func AddAutoPaginationFlag(cmd *cobra.Command, all *bool) {
	cmd.Flags().BoolVar(all, "all", false, "Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling.")
}

// AddReadTriageFlags registers --summarize/--ids-only/--first-n for item
// collections (see ApplyReadTriage).
func AddReadTriageFlags(cmd *cobra.Command, opts *ReadTriageOptions) {
	cmd.Flags().BoolVar(&opts.Summarize, "summarize", false, "Return only id/name/alias fields for item collections")
	cmd.Flags().BoolVar(&opts.IDsOnly, "ids-only", false, "Return only item IDs for item collections")
	cmd.Flags().IntVar(&opts.FirstN, "first-n", 0, "Return only the first N items from item collections")
}

// AddDocumentOutputTrimFlags registers the document-shaped output flags
// --fields/--summary/--no-empty/--full (see ApplyDocumentOutputTrim).
func AddDocumentOutputTrimFlags(cmd *cobra.Command, opts *OutputTrimOptions) {
	cmd.Flags().StringVar(&opts.Fields, "fields", "", "Project response fields client-side; supports comma-separated dotted paths such as id,name,documentType.alias,values.bodyText")
	cmd.Flags().BoolVar(&opts.Summary, "summary", false, "Return a compact document shape with id, name, documentType, route/url, and state/date fields when present")
	cmd.Flags().BoolVar(&opts.NoEmpty, "no-empty", false, "Omit null, empty string, empty array, and empty object values from trimmed output")
	cmd.Flags().BoolVar(&opts.Full, "full", false, "Return the full payload explicitly; cannot be combined with --fields, --summary, or --no-empty")
}

// RequireValue rejects a missing or blank required option.
func RequireValue(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("missing required option: %s", name)
	}
	return nil
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

// UniqueCSV splits a comma-separated flag value into trimmed, non-empty,
// de-duplicated entries in their original order.
func UniqueCSV(s string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range strings.Split(s, ",") {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
