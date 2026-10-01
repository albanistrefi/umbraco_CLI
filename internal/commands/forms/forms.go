// Package forms holds the Umbraco Forms add-on commands, served from the
// Forms Management API mount rather than the core one.
package forms

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
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
		if cmdkit.AsString(entry["uniqueId"]) == recordID || cmdkit.AsString(entry["id"]) == recordID {
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

// Register attaches the forms command group to root.
func Register(root *cobra.Command, deps cmdkit.Dependencies) {
	forms := &cobra.Command{
		Use:   "forms",
		Short: "Umbraco Forms operations: forms, folders, records, prevalue sources",
		Long: "Commands for the Umbraco Forms Management API (" + formsAPIPrefix + "): browse and author forms and folders, act on submitted records (approve, reject, delete, edit, retry workflows), and manage prevalue sources. " +
			"What the CLI may read or change is governed by the API user's Forms permissions (manage forms, manage workflows, view/edit/delete entries, ...); a refused operation comes back as the server's 403. " +
			"Every mutation takes --dry-run; deletes and destructive record actions also need --force. Useful for resolving form and field GUIDs when composing Umbraco.Forms.Automate flows.",
	}
	forms.AddCommand(formsList(deps))
	forms.AddCommand(formsChildren(deps))
	forms.AddCommand(formsGet(deps))
	forms.AddCommand(formsCreate(deps))
	forms.AddCommand(formsUpdate(deps))
	forms.AddCommand(formsDelete(deps))
	forms.AddCommand(formsCopy(deps))
	forms.AddCommand(formsMove(deps))
	forms.AddCommand(formsCopyWorkflows(deps))
	forms.AddCommand(formsCreateFolder(deps))
	forms.AddCommand(formsUpdateFolder(deps))
	forms.AddCommand(formsMoveFolder(deps))
	forms.AddCommand(formsDeleteFolder(deps))
	forms.AddCommand(formsRecords(deps))
	forms.AddCommand(formsRecord(deps))
	forms.AddCommand(formsRecordWorkflowLog(deps))
	forms.AddCommand(formsRecordActions(deps))
	forms.AddCommand(formsRecordAction(deps))
	forms.AddCommand(formsRecordUpdate(deps))
	forms.AddCommand(formsRecordWorkflowRetry(deps))
	forms.AddCommand(formsPrevalueSource(deps))
	root.AddCommand(forms)
}

func formsList(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	var triage cmdkit.ReadTriageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List forms (tree root: returns folders and root-level forms)",
		Long:  "Returns the Forms tree root. On real installs this is mostly folders. Every item carries isFolder and type (\"folder\" or \"form\"); use 'forms children <folderId>' to drill into a folder and 'forms get <formId>' only on forms.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := cmdkit.GetWithFallback(
				cmd.Context(),
				deps.Client,
				cmdkit.GetRequestCandidate{Path: "/tree/form/root", Opts: formsRequestOpts(fields, nil)},
				cmdkit.GetRequestCandidate{Path: "/form", Opts: formsRequestOpts(fields, nil)},
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyReadTriage(cmdkit.ApplyFieldsProjection(annotateFormsItems(result), fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmdkit.AddReadTriageFlags(cmd, &triage)
	return cmd
}

// annotateFormsItems gives every tree/list item an explicit isFolder and a
// type of "folder" or "form". Field report: a folder named like a form
// ("Contact sales") was passed to 'forms get', which 404s; the tree root
// flags folders but form rows carry nothing, so absence read as ambiguity.
func annotateFormsItems(result any) any {
	for _, item := range cmdkit.ResultItems(result) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		folder, _ := entry["isFolder"].(bool)
		entry["isFolder"] = folder
		if folder {
			entry["type"] = "folder"
		} else {
			entry["type"] = "form"
		}
	}
	return result
}

func formsNotAFolderError(id string) error {
	return fmt.Errorf("%s is not a Forms folder id (a form id, perhaps — try 'umbraco forms get %s'); pick a folder with isFolder=true from 'umbraco forms list' or 'umbraco forms children <folderId>'", id, id)
}

// isFormsFolderID reports whether the Forms API knows the id as a folder.
// Only a 404 means "not a folder"; any other failure (403, 500, network) is
// returned so the caller keeps the API error and its exit code instead of
// misreporting the id.
func isFormsFolderID(ctx context.Context, client *api.Client, id string) (bool, error) {
	result, err := client.Get(ctx, api.JoinPath("/folder/%s", id), formsRequestOpts("", nil))
	if err != nil {
		if cmdkit.IsAPIStatus(err, http.StatusNotFound) {
			return false, nil
		}
		return false, err
	}
	folder, ok := result.(map[string]any)
	return ok && folder["id"] != nil, nil
}

func formsChildren(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	var triage cmdkit.ReadTriageOptions
	cmd := &cobra.Command{
		Use:   "children <folderId>",
		Short: "List the forms and sub-folders inside a folder",
		Long: "GET /tree/form/children/{folderId}. Forms in Umbraco are organized into folders. 'forms list' returns root-level items (mostly folders); use 'forms children <folderId>' to drill into a folder returned with isFolder=true. " +
			"Every item carries isFolder and type (\"folder\" or \"form\"), so nested folders can be walked. " +
			"Note: GET /form?folderId=… is not used — the server ignores the filter and returns every form (verified on Forms 17/18). The tree route is not paged either: it returns the whole folder and ignores skip/take (verified: take=2 still returned all 30 items).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(
				cmd.Context(),
				api.JoinPath("/tree/form/children/%s", args[0]),
				formsRequestOpts(fields, nil),
			)
			if err != nil {
				if cmdkit.IsAPIStatus(err, http.StatusNotFound) {
					folder, probeErr := isFormsFolderID(cmd.Context(), deps.Client, args[0])
					if probeErr != nil {
						return probeErr
					}
					if !folder {
						return formsNotAFolderError(args[0])
					}
				}
				return err
			}
			// The tree answers 200 with an empty page for any id (a form id,
			// a typo), which would read as "empty folder"; only an actual
			// folder record may be reported as empty.
			if len(cmdkit.ResultItems(result)) == 0 {
				folder, probeErr := isFormsFolderID(cmd.Context(), deps.Client, args[0])
				if probeErr != nil {
					return probeErr
				}
				if !folder {
					return formsNotAFolderError(args[0])
				}
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyReadTriage(cmdkit.ApplyFieldsProjection(annotateFormsItems(result), fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmdkit.AddReadTriageFlags(cmd, &triage)
	return cmd
}

func formsGet(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get form definition by ID (includes fields, pages, workflows)",
		Long:  "GET /form/{id}. Folders are not forms: a folder id (isFolder=true in 'forms list'/'forms children') is refused with a pointer to 'forms children <folderId>' instead of a bare 404.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/form/%s", args[0]), formsRequestOpts(fields, nil))
			if err != nil {
				if cmdkit.IsAPIStatus(err, http.StatusNotFound) {
					folder, probeErr := isFormsFolderID(cmd.Context(), deps.Client, args[0])
					if probeErr != nil {
						return probeErr
					}
					if folder {
						return fmt.Errorf("%s is a Forms folder, not a form; use 'umbraco forms children %s' to list the forms inside it", args[0], args[0])
					}
				}
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func formsRecords(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	var triage cmdkit.ReadTriageOptions
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
			params, err := cmdkit.ParseParams(paramsRaw)
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
				cmd.Context(),
				api.JoinPath("/form/%s/record", args[0]),
				formsRequestOpts(fields, params),
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyReadTriage(cmdkit.ApplyFieldsProjection(result, fields), triage))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().StringVar(&state, "state", "", "Filter by record state (e.g. submitted, approved, pending). Pass-through; see your Umbraco Forms version for supported values")
	cmd.Flags().StringVar(&from, "from", "", "Filter records created on or after this ISO 8601 date/time")
	cmd.Flags().StringVar(&to, "to", "", "Filter records created on or before this ISO 8601 date/time")
	cmd.Flags().IntVar(&skip, "skip", 0, "Number of records to skip")
	cmd.Flags().IntVar(&take, "take", 0, "Maximum number of records to return (defaults to 100 if not set; pass --take 0 explicitly for no limit)")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Additional query parameters as JSON; merged with --state/--from/--to/--skip/--take, with --params taking precedence on key collisions")
	cmdkit.AddReadTriageFlags(cmd, &triage)
	return cmd
}

func formsRecord(deps cmdkit.Dependencies) *cobra.Command {
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
				cmd.Context(),
				api.JoinPath("/form/%s/record", formID),
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
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(match, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	cmd.Flags().IntVar(&scan, "scan", 500, "Maximum number of records to scan when looking up the record (the Forms API has no direct GET-by-id, so we filter client-side). Must be positive.")
	return cmd
}

func formsRecordWorkflowLog(deps cmdkit.Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "record-workflow-log <formId> <recordId>",
		Short: "Get the workflow execution audit trail for a record",
		Long:  "Returns the per-workflow execution log for a single record. Useful when debugging why an Umbraco.Forms.Automate flow did or did not fire for a given submission.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(
				cmd.Context(),
				api.JoinPath("/form/%s/record/%s/workflow-audit-trail", args[0], args[1]),
				formsRequestOpts(fields, nil),
			)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.ApplyFieldsProjection(result, fields))
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}
