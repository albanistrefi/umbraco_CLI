package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// formsAPIPrefix is the mount point for the Umbraco Forms Management API.
// It is distinct from the core CMS prefix and is passed per-request via
// api.RequestOptions.APIPrefix so existing commands are unaffected.
const (
	formsAPIPrefix = "/umbraco/forms/management/api/v1"
	// formsRecordsDefaultTake caps records pulls when the caller does not pass
	// --take, so agents don't accidentally pull thousands of submissions in
	// one go. Overridden by an explicit --take (including --take=0 for "no
	// limit") or by --params.take.
	formsRecordsDefaultTake = 100
)

func formsRequestOpts(fields string, params map[string]any) api.RequestOptions {
	return api.RequestOptions{APIPrefix: formsAPIPrefix, Fields: fields, Params: params}
}

// findFormsRecord locates a record inside the GET /form/{id}/record response
// by matching either the GUID-shaped 'uniqueId' or the numeric 'id'
// (stringified). The response shape is {"results": [...], "schema": [...]}
// per the Forms Management API.
func findFormsRecord(payload any, recordID string) map[string]any {
	envelope, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	results, ok := envelope["results"].([]any)
	if !ok {
		return nil
	}
	for _, item := range results {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if asString(entry["uniqueId"]) == recordID || asString(entry["id"]) == recordID {
			return entry
		}
	}
	return nil
}

// formsRecordCount returns how many records the API actually returned in
// the envelope, regardless of what was requested via take.
func formsRecordCount(payload any) int {
	envelope, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	results, ok := envelope["results"].([]any)
	if !ok {
		return 0
	}
	return len(results)
}

// formsRecordScanWindowExhausted reports whether the API filled the scan
// window — i.e. the returned page is as large as requested, meaning more
// records likely exist beyond it.
func formsRecordScanWindowExhausted(payload any, scan int) bool {
	return formsRecordCount(payload) >= scan
}

func asString(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		// JSON numbers decode as float64; render integers without trailing .0
		if value == float64(int64(value)) {
			return fmt.Sprintf("%d", int64(value))
		}
		return fmt.Sprintf("%v", value)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func RegisterForms(root *cobra.Command, deps Dependencies) {
	forms := &cobra.Command{
		Use:   "forms",
		Short: "Umbraco Forms operations (read-only)",
		Long:  "Read-focused commands for the Umbraco Forms Management API. Useful for resolving form and field GUIDs when composing Umbraco.Forms.Automate flows, and for inspecting submitted records.",
	}
	forms.AddCommand(formsList(deps))
	forms.AddCommand(formsChildren(deps))
	forms.AddCommand(formsGet(deps))
	forms.AddCommand(formsRecords(deps))
	forms.AddCommand(formsRecord(deps))
	forms.AddCommand(formsRecordWorkflowLog(deps))
	root.AddCommand(forms)
}

func formsList(deps Dependencies) *cobra.Command {
	var fields string
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List forms (tree root: returns folders and root-level forms)",
		Long:  "Returns the Forms tree root. On real installs this is mostly folders — use 'forms children <folderId>' to drill into a folder returned with isFolder=true.",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := getWithFallback(
				context.Background(),
				deps.Client,
				getRequestCandidate{path: "/tree/form/root", opts: formsRequestOpts(fields, nil)},
				getRequestCandidate{path: "/form", opts: formsRequestOpts(fields, nil)},
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func formsChildren(deps Dependencies) *cobra.Command {
	var fields string
	var triage readTriageOptions
	cmd := &cobra.Command{
		Use:   "children <folderId>",
		Short: "List forms inside a folder",
		Long:  "Forms in Umbraco are organized into folders. 'forms list' returns root-level items (mostly folders); use 'forms children <folderId>' to drill into a folder returned with isFolder=true.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(
				context.Background(),
				"/form",
				formsRequestOpts(fields, map[string]any{"folderId": args[0]}),
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func formsGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get form definition by ID (includes fields, pages, workflows)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/form/%s", args[0]), formsRequestOpts(fields, nil))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func formsRecords(deps Dependencies) *cobra.Command {
	var fields string
	var triage readTriageOptions
	var state string
	var from string
	var to string
	var skip int
	var take int
	var paramsRaw string
	cmd := &cobra.Command{
		Use:   "records <formId>",
		Short: "List form records (submissions)",
		Long:  "List records for a form. Filter flags (--state, --from, --to, --skip, --take) are passed through to the Management API verbatim; use --params for any other supported filter.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := parseParams(paramsRaw)
			if err != nil {
				return err
			}
			if params == nil {
				params = map[string]any{}
			}
			if strings.TrimSpace(state) != "" {
				if _, ok := params["state"]; !ok {
					params["state"] = state
				}
			}
			if strings.TrimSpace(from) != "" {
				if _, ok := params["from"]; !ok {
					params["from"] = from
				}
			}
			if strings.TrimSpace(to) != "" {
				if _, ok := params["to"]; !ok {
					params["to"] = to
				}
			}
			if cmd.Flags().Changed("skip") {
				if _, ok := params["skip"]; !ok {
					params["skip"] = skip
				}
			}
			if cmd.Flags().Changed("take") {
				if _, ok := params["take"]; !ok {
					params["take"] = take
				}
			} else if _, ok := params["take"]; !ok {
				params["take"] = formsRecordsDefaultTake
			}

			result, err := deps.Client.Get(
				context.Background(),
				fmt.Sprintf("/form/%s/record", args[0]),
				formsRequestOpts(fields, params),
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().StringVar(&state, "state", "", "Filter by record state (e.g. submitted, approved, pending). Pass-through; see your Umbraco Forms version for supported values")
	cmd.Flags().StringVar(&from, "from", "", "Filter records created on or after this ISO 8601 date/time")
	cmd.Flags().StringVar(&to, "to", "", "Filter records created on or before this ISO 8601 date/time")
	cmd.Flags().IntVar(&skip, "skip", 0, "Number of records to skip")
	cmd.Flags().IntVar(&take, "take", 0, "Maximum number of records to return (defaults to 100 if not set; pass --take 0 explicitly for no limit)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Additional query parameters as JSON; merged with --state/--from/--to/--skip/--take, with --params taking precedence on key collisions")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func formsRecord(deps Dependencies) *cobra.Command {
	var fields string
	var scan int
	cmd := &cobra.Command{
		Use:   "record <formId> <recordId>",
		Short: "Get a single form record by its uniqueId (GUID); scans the first --scan records (default 500)",
		Long: "Returns one record from a form. recordId is the record's uniqueId (GUID, e.g. 917a242d-d48c-44ac-ad99-9dcfaf2d3e7f), visible in 'forms records' output. The numeric 'id' field is also accepted.\n\n" +
			"Implementation note: the Forms Management API does not expose a GET endpoint on /form/{formId}/record/{recordId} — only PUT is registered. This subcommand therefore fetches the records list and filters client-side. Use --scan to control how many records are scanned (default 500); for forms with more records, narrow by date with 'forms records --from/--to' and pipe to jq.\n\n" +
			"Record ordering is controlled by the Forms API and is not part of its public contract. Observation against v17.3 suggests newest-first, but agents shouldn't rely on it — if a record isn't in the scan window, the error distinguishes 'definitely not present' from 'scan window exhausted' so you know whether to widen.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if scan <= 0 {
				return fmt.Errorf("--scan must be a positive integer, got %d", scan)
			}
			formID, recordID := args[0], args[1]
			result, err := deps.Client.Get(
				context.Background(),
				fmt.Sprintf("/form/%s/record", formID),
				formsRequestOpts("", map[string]any{"take": scan}),
			)
			if err != nil {
				return err
			}
			match := findFormsRecord(result, recordID)
			if match == nil {
				if formsRecordScanWindowExhausted(result, scan) {
					return fmt.Errorf("no record with id %q in the first %d records of form %s (scan window exhausted — the record may exist outside this window; widen with --scan or narrow with 'forms records --from/--to')", recordID, scan, formID)
				}
				return fmt.Errorf("no record with id %q on form %s (scanned all %d records the form returned)", recordID, formID, formsRecordCount(result))
			}
			return printResult(cmd, deps, applyFieldsProjection(match, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().IntVar(&scan, "scan", 500, "Maximum number of records to scan when looking up the record (the Forms API has no direct GET-by-id, so we filter client-side). Must be positive.")
	return cmd
}

func formsRecordWorkflowLog(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "record-workflow-log <formId> <recordId>",
		Short: "Get the workflow execution audit trail for a record",
		Long:  "Returns the per-workflow execution log for a single record. Useful when debugging why an Umbraco.Forms.Automate flow did or did not fire for a given submission.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(
				context.Background(),
				fmt.Sprintf("/form/%s/record/%s/workflow-audit-trail", args[0], args[1]),
				formsRequestOpts(fields, nil),
			)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}
