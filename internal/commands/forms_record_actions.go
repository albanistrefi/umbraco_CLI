package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Record writes: state changes (approve/reject/delete via record-set
// actions), field edits, and workflow retries on submitted records. Records
// hold what site visitors submitted, so these commands echo ids and counts,
// never record contents.

func formsRecordActions(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "record-actions",
		Short: "List the record actions (approve, reject, delete, ...) the API user may run",
		Long: "GET /record-set-actions. The list is filtered by the API user's Forms permissions: delete appears only when the user may delete entries. " +
			"Run one with 'forms record-action <formId> <alias> --record-ids …'.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/record-set-actions", formsRequestOpts(fields, nil))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

// resolveFormsRecordAction finds a record-set action by alias, id or name.
// The server answers an unknown action id with a bare 500, so the lookup
// happens client-side first and an unknown action is reported with the
// aliases that are available.
func resolveFormsRecordAction(ctx context.Context, client *api.Client, action string) (map[string]any, error) {
	result, err := client.Get(ctx, "/record-set-actions", formsRequestOpts("", nil))
	if err != nil {
		return nil, err
	}
	entries, _ := result.([]any)
	available := []string{}
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		alias := asString(entry["alias"])
		available = append(available, alias)
		for _, key := range []string{"alias", "id", "name"} {
			if strings.EqualFold(asString(entry[key]), strings.TrimSpace(action)) {
				return entry, nil
			}
		}
	}
	sort.Strings(available)
	return nil, fmt.Errorf("no record action %q is available to the API user (available: %s); delete is listed only when the user has the Forms delete-entries permission — see 'umbraco forms record-actions'", action, strings.Join(available, ", "))
}

// formsRecordActionDestructive reports whether running the action needs
// --force: delete-type actions, and any action the backoffice itself would
// confirm first.
func formsRecordActionDestructive(entry map[string]any) bool {
	if confirm, _ := entry["needsConfirm"].(bool); confirm {
		return true
	}
	for _, key := range []string{"alias", "name"} {
		if strings.Contains(strings.ToLower(asString(entry[key])), "delete") {
			return true
		}
	}
	return false
}

func formsRecordAction(deps Dependencies) *cobra.Command {
	var recordIDs string
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "record-action <formId> <action>",
		Short: "Run a record action (approve, reject, delete, ...) on records of a form",
		Long: "POST /form/{formId}/record/actions/{actionId}/execute. <action> is an alias, id or name from 'forms record-actions' (e.g. approve, reject). --record-ids takes record uniqueIds (GUIDs) from 'forms records <formId> --fields uniqueId,state'. " +
			"Delete actions, and any action the backoffice asks to confirm, permanently change records and need --force (or --dry-run to rehearse). " +
			"The server ignores record ids it does not know and still answers 200 (verified on Forms 18.1), so read the records back with 'forms records' to confirm the new state.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := uniqueCSV(recordIDs)
			if len(ids) == 0 {
				return fmt.Errorf("record-action requires --record-ids <comma-separated record uniqueIds>")
			}
			for _, id := range ids {
				if !isUUIDLike(id) {
					return fmt.Errorf("--record-ids must be record uniqueIds (GUIDs), got %q; the numeric record id is not accepted here", id)
				}
			}
			ctx := cmd.Context()
			action, err := resolveFormsRecordAction(ctx, deps.Client, args[1])
			if err != nil {
				return err
			}
			alias := asString(action["alias"])
			if formsRecordActionDestructive(action) {
				if err := requireForceOrDryRun(cmd, fmt.Sprintf("runs the %q record action, which permanently changes the records", alias), force, dryRun); err != nil {
					return err
				}
			}
			actionID := asString(action["id"])
			body := map[string]any{"recordKeys": stringsToAny(ids)}
			result, err := deps.Client.Post(ctx, api.JoinPath("/form/%s/record/actions/%s/execute", args[0], actionID), body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun || result != nil {
				return printResult(cmd, deps, result)
			}
			return printResult(cmd, deps, map[string]any{"executed": true, "action": alias, "actionId": actionID, "formId": args[0], "recordIds": stringsToAny(ids)})
		},
	}
	cmd.Flags().StringVar(&recordIDs, "record-ids", "", "Comma-separated record uniqueIds (GUIDs) to run the action on (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm a destructive action (delete)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// parseFormsRecordFields accepts the record update body: a JSON array of
// {"fieldId", "values": [...]} entries, one per field being changed.
func parseFormsRecordFields(raw string) ([]any, error) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("invalid --json JSON: %w", err)
	}
	entries, ok := payload.([]any)
	if !ok || len(entries) == 0 {
		return nil, fmt.Errorf("--json must be a non-empty JSON array of {\"fieldId\": \"<field GUID>\", \"values\": [...]}")
	}
	for i, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("--json entry %d must be an object {\"fieldId\", \"values\"}", i)
		}
		if !isUUIDLike(asString(entry["fieldId"])) {
			return nil, fmt.Errorf("--json entry %d needs \"fieldId\": the field GUID from 'forms get <formId>' (pages → fieldSets → containers → fields)", i)
		}
		if _, ok := entry["values"].([]any); !ok {
			return nil, fmt.Errorf("--json entry %d needs \"values\" as an array, e.g. [\"new value\"]", i)
		}
	}
	return entries, nil
}

func formsRecordUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "record-update <formId> <recordId>",
		Short: "Change field values on a submitted record",
		Long: "PUT /form/{formId}/record/{recordId}. --json is an array with one entry per field to change: [{\"fieldId\": \"<field GUID>\", \"values\": [\"new value\"]}]. " +
			"recordId is the record's uniqueId (GUID). Needs the Forms edit-entries permission. Record contents are not echoed back; read the record with 'forms record <formId> <recordId>'.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			if !isUUIDLike(args[1]) {
				return fmt.Errorf("recordId must be the record's uniqueId (GUID), got %q", args[1])
			}
			body, err := parseFormsRecordFields(jsonPayload)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/form/%s/record/%s", args[0], args[1]), body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Field changes as a JSON array: [{\"fieldId\", \"values\": [...]}] (required)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func formsRecordWorkflowRetry(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "record-workflow-retry <formId> <recordId> <workflowId>",
		Short: "Re-run one workflow for a submitted record",
		Long: "POST /form/{formId}/record/{recordId}/workflow/{workflowId}/retry. Use after 'forms record-workflow-log <formId> <recordId>' shows a failed workflow. " +
			"The workflow really runs again, with its side effects (emails are re-sent, data is re-posted), so rehearse with --dry-run first.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/form/%s/record/%s/workflow/%s/retry", args[0], args[1], args[2]), nil, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "retried", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
