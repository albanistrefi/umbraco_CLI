package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Umbraco Automate serves its own Management API mount. The command group
// is gated behind an environment variable while the product's API surface
// settles post-launch; the gate also keeps the group out of --help and
// generated skills until it is officially supported.
const (
	automateEnableEnv = "UMBRACO_CLI_ENABLE_AUTOMATE"
	automateAPIPrefix = "/umbraco/automate/management/api/v1"
)

func RegisterAutomate(root *cobra.Command, deps Dependencies) {
	if !automateEnabled() {
		return
	}

	automate := &cobra.Command{
		Use:    "automate",
		Short:  "Umbraco Automate operations (event-driven workflow automation)",
		Long:   "Operate Umbraco Automate: discover the step catalogue, inspect and trigger automations, manage runs, and decide approvals. Targets the Automate Management API mount (" + automateAPIPrefix + ").",
		Hidden: true,
	}
	automate.AddCommand(automateCatalogue(deps))
	automate.AddCommand(automateAutomation(deps))
	automate.AddCommand(automateRun(deps))
	automate.AddCommand(automateApprovals(deps))
	automate.AddCommand(automateMetrics(deps))
	root.AddCommand(automate)
}

func automateEnabled() bool {
	return os.Getenv(automateEnableEnv) == "1"
}

func automateOpts(params map[string]any, dryRun bool) api.RequestOptions {
	return api.RequestOptions{APIPrefix: automateAPIPrefix, Params: params, DryRun: dryRun}
}

// automateArrayRead builds a read command for the catalogue-style endpoints
// that return bare arrays (no {items,total} envelope, no pagination).
func automateArrayRead(deps Dependencies, use string, short string, path string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(cmd.Context(), path, automateOpts(nil, false))
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyFieldsProjection(result, fields))
	}}
	addFieldsFlag(cmd, &fields)
	return cmd
}

func automateCatalogue(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalogue",
		Short: "Discover the building blocks automations are made of",
		Long:  "Catalogue reads return the step types available for building automations: triggers start a flow, actions do work, control-flows branch or loop, connection types describe external-service credentials. Step settings and output schemas are embedded, so prefer --fields (e.g. --fields alias,name,description) unless schema detail is needed.",
	}
	cmd.AddCommand(automateCatalogueScoped(deps, "actions", "List action step types", "/catalogue/actions"))
	cmd.AddCommand(automateCatalogueScoped(deps, "triggers", "List trigger step types", "/catalogue/triggers"))
	cmd.AddCommand(automateArrayRead(deps, "connection-types", "List connection types", "/catalogue/connection-types"))
	cmd.AddCommand(automateArrayRead(deps, "control-flows", "List control-flow step types", "/catalogue/control-flows"))
	cmd.AddCommand(automateArrayRead(deps, "notification-channels", "List notification channels", "/catalogue/notification-channels"))
	cmd.AddCommand(automateArrayRead(deps, "webhook-authenticators", "List webhook authenticators", "/catalogue/webhook-authenticators"))
	cmd.AddCommand(automateCatalogueStepTypes(deps))
	cmd.AddCommand(automateCatalogueOutputSchema(deps))
	return cmd
}

// automateCatalogueScoped builds the catalogue reads that accept an
// optional workspace scope (actions and triggers vary per workspace).
func automateCatalogueScoped(deps Dependencies, use string, short string, path string) *cobra.Command {
	var fields string
	var workspaceID string
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		var params map[string]any
		if workspaceID != "" {
			params = map[string]any{"workspaceId": workspaceID}
		}
		result, err := deps.Client.Get(cmd.Context(), path, automateOpts(params, false))
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyFieldsProjection(result, fields))
	}}
	addFieldsFlag(cmd, &fields)
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Scope to one workspace")
	return cmd
}

func automateCatalogueStepTypes(deps Dependencies) *cobra.Command {
	var fields string
	var stepType string
	cmd := &cobra.Command{Use: "step-types", Short: "List step types, optionally filtered by kind", RunE: func(cmd *cobra.Command, args []string) error {
		var params map[string]any
		if stepType != "" {
			params = map[string]any{"type": stepType}
		}
		result, err := deps.Client.Get(cmd.Context(), "/catalogue/step-types", automateOpts(params, false))
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyFieldsProjection(result, fields))
	}}
	addFieldsFlag(cmd, &fields)
	cmd.Flags().StringVar(&stepType, "type", "", "Step type filter")
	return cmd
}

func automateCatalogueOutputSchema(deps Dependencies) *cobra.Command {
	var jsonRaw string
	cmd := &cobra.Command{
		Use:   "output-schema <alias>",
		Short: "Resolve a step type's dynamic output schema",
		Long:  "POST /catalogue/step-types/{alias}/output-schema. Steps with hasDynamicOutputSchema=true shape their output by their settings; pass the intended settings via --json to see the fields available for ${...} bindings in later steps.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := optionalBody(jsonRaw)
			if err != nil {
				return err
			}
			if _, ok := body["settings"]; !ok {
				body["settings"] = map[string]any{}
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/catalogue/step-types/%s/output-schema", args[0]), body, automateOpts(nil, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&jsonRaw, "json", "", "JSON body; defaults to {\"settings\":{}}")
	return cmd
}

func automateAutomation(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "automation", Short: "Automation operations"}
	cmd.AddCommand(automateAutomationList(deps))
	cmd.AddCommand(automateAutomationGet(deps))
	cmd.AddCommand(automateAutomationRuns(deps))
	cmd.AddCommand(automateAutomationTrigger(deps))
	cmd.AddCommand(automateAutomationExport(deps))
	return cmd
}

func automateAutomationList(deps Dependencies) *cobra.Command {
	var filter string
	var workspaceID string
	var groupID string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List automations (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			for key, value := range map[string]string{"filter": filter, "workspaceId": workspaceID, "groupId": groupID} {
				if value != "" {
					params = withParam(params, key, value)
				}
			}
			return []getRequestCandidate{
				{path: "/automations", opts: automateOpts(params, false)},
			}
		},
	})
	cmd.Flags().StringVar(&filter, "filter", "", "Text filter")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	cmd.Flags().StringVar(&groupID, "group-id", "", "Group ID")
	return cmd
}

func automateAutomationGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:       "get <id>",
		Short:     "Get an automation by ID (trigger, steps, connections, state)",
		Path:      func(args []string) string { return api.JoinPath("/automations/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateAutomationRuns(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "runs <id>",
		Short: "List runs for an automation (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: api.JoinPath("/automations/%s/runs", args[0]), opts: automateOpts(params, false)},
			}
		},
	})
}

func automateAutomationTrigger(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "trigger <id>",
		Short: "Trigger a published automation manually",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/automations/%s/trigger", args[0]), nil, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "triggered", result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateAutomationExport(deps Dependencies) *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export an automation as a portable definition",
		Long:  "GET /automations/{id}/export. The export model is the template format for 'automation validate' and 'automation import' -- export a working automation, adjust the JSON, validate, and import it elsewhere.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var params map[string]any
			if include != "" {
				params = map[string]any{"include": include}
			}
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/automations/%s/export", args[0]), automateOpts(params, false))
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Export include option")
	return cmd
}

func automateRun(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Automation run operations (inspect, replay, suspend, resume, terminate)",
	}
	cmd.AddCommand(automateRunGet(deps))
	cmd.AddCommand(automateRunAction(deps, "replay", "Replay a run", "replayed"))
	cmd.AddCommand(automateRunAction(deps, "resume", "Resume a suspended run", "resumed"))
	cmd.AddCommand(automateRunAction(deps, "suspend", "Suspend a run", "suspended"))
	cmd.AddCommand(automateRunAction(deps, "terminate", "Terminate a run", "terminated"))
	return cmd
}

func automateRunGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:       "get <id>",
		Short:     "Get a run by ID (per-step inputs, outputs, errors, timing)",
		Path:      func(args []string) string { return api.JoinPath("/runs/%s", args[0]) },
		APIPrefix: automateAPIPrefix,
	})
}

func automateRunAction(deps Dependencies, action string, short string, verb string) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   action + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/runs/%s/"+action, args[0]), nil, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, verb, result, dryRun)
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func automateApprovals(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "approvals", Short: "Approval-step operations"}
	cmd.AddCommand(automateArrayRead(deps, "pending", "List approvals waiting for a decision", "/approvals/pending"))
	cmd.AddCommand(automateApprovalsDecide(deps))
	return cmd
}

func automateApprovalsDecide(deps Dependencies) *cobra.Command {
	var outcome string
	var comment string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "decide <run-id> <step-id>",
		Short: "Submit an approval decision for a suspended run step",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outcome != "Approved" && outcome != "Rejected" {
				return fmt.Errorf("--outcome must be Approved or Rejected")
			}
			body := map[string]any{"outcome": outcome}
			if comment != "" {
				body["comment"] = comment
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/approvals/%s/steps/%s/decision", args[0], args[1]), body, automateOpts(nil, dryRun))
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "decided", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&outcome, "outcome", "", "Approval outcome: Approved or Rejected")
	cmd.Flags().StringVar(&comment, "comment", "", "Approval comment")
	addDryRunFlag(cmd, &dryRun)
	_ = cmd.MarkFlagRequired("outcome")
	return cmd
}

func automateMetrics(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "metrics", Short: "Run metrics (success rates, totals)"}
	cmd.AddCommand(automateMetricsRead(deps, "summary", "Get run summary metrics", "/metrics", false))
	cmd.AddCommand(automateMetricsRead(deps, "by-automation", "Get run metrics grouped by automation", "/metrics/by-automation", true))
	return cmd
}

func automateMetricsRead(deps Dependencies, use string, short string, path string, includeTake bool) *cobra.Command {
	var workspaceID string
	var from string
	var to string
	var take int
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]any{}
		for key, value := range map[string]string{"workspaceId": workspaceID, "from": from, "to": to} {
			if value != "" {
				params[key] = value
			}
		}
		if includeTake && take >= 0 {
			params["take"] = take
		}
		result, err := deps.Client.Get(cmd.Context(), path, automateOpts(params, false))
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace ID")
	cmd.Flags().StringVar(&from, "from", "", "Start date (ISO)")
	cmd.Flags().StringVar(&to, "to", "", "End date (ISO)")
	if includeTake {
		cmd.Flags().IntVar(&take, "take", -1, "Take count")
	}
	return cmd
}
