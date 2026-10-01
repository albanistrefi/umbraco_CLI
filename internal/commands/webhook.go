package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/schema"
)

func RegisterWebhook(root *cobra.Command, deps cmdkit.Dependencies) {
	webhook := &cobra.Command{
		Use:   "webhook",
		Short: "Webhook management (the Management API's outbound event notifications)",
		Long:  "Create, inspect, and audit webhooks that fire on content events. 'webhook events' lists the event aliases a webhook can subscribe to; 'webhook logs' shows delivery attempts with status codes for debugging integrations.",
	}
	webhook.AddCommand(webhookList(deps))
	webhook.AddCommand(webhookGet(deps))
	webhook.AddCommand(webhookCreate(deps))
	webhook.AddCommand(webhookUpdate(deps))
	webhook.AddCommand(webhookDelete(deps))
	webhook.AddCommand(webhookEvents(deps))
	webhook.AddCommand(webhookLogs(deps))
	root.AddCommand(webhook)
}

func webhookList(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List webhooks (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/webhook", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func webhookGet(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:   "get <id>",
		Short: "Get a webhook by ID",
		Path:  func(args []string) string { return api.JoinPath("/webhook/%s", args[0]) },
	})
}

func webhookCreate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook",
		Long:  "POST /webhook. Required fields: url, events (aliases from 'webhook events'), enabled, contentTypeKeys (empty array = all content types), headers (empty object = none). Use --print-template for the payload shape.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				return cmdkit.PrintResult(cmd, deps, schema.Templates["webhook.create"])
			}
			if err := cmdkit.RequireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := cmdkit.ParsePayload(jsonPayload)
			if err != nil {
				return err
			}
			if err := normalizeWebhookEvents(body); err != nil {
				return err
			}
			if _, err := cmdkit.EnsurePayloadID(body); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/webhook", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, cmdkit.CreateResult(result, body, "url"))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func webhookUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update <id>",
		Short: "Update a webhook",
		Long:  "PUT /webhook/{id}. An events array in the patch REPLACES the whole subscription set (identical for alias strings and object-form entries) — events are pure identifiers, so an entry-wise merge could only ever add subscriptions and never remove one. Omit events from --merge-json to keep the current set.",
		Path:  func(args []string) string { return api.JoinPath("/webhook/%s", args[0]) },
		// Normalize runs on the patch BEFORE the merge deliberately: mapping
		// object-form events to alias strings there keeps them out of the
		// alias-aware array merge, giving events consistent replace
		// semantics in both entry forms.
		Normalize:       normalizeWebhookEvents,
		NormalizeMerged: normalizeWebhookEvents,
	})
}

// normalizeWebhookEvents maps response-shaped events entries
// ({eventName, eventType, alias}) down to the alias strings the webhook
// request models require. GET returns events as objects while PUT/POST
// take string arrays, so without this every merge-based update — even one
// not touching events — failed server validation. Idempotent: string
// entries pass through untouched.
func normalizeWebhookEvents(body map[string]any) error {
	events, ok := body["events"].([]any)
	if !ok {
		return nil
	}
	for i, item := range events {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		alias, _ := entry["alias"].(string)
		if alias == "" {
			return fmt.Errorf("events[%d] is an object without an alias; pass event aliases as strings (see 'umbraco api GET /webhook/events')", i)
		}
		events[i] = alias
	}
	return nil
}

func webhookDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:   "delete <id>",
		Short: "Permanently delete a webhook",
		Path: func(args []string) string {
			return api.JoinPath("/webhook/%s", args[0])
		},
	})
}

func webhookEvents(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "events",
		Short: "List the event aliases webhooks can subscribe to",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/webhook/events", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func webhookLogs(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "logs [webhook-id]",
		Short: "List webhook delivery logs, optionally scoped to one webhook",
		Long:  "GET /webhook/logs, or /webhook/{id}/logs when a webhook ID is given. Each entry carries the event alias, target URL, response status, and retry count — the audit trail for 'did my integration fire'.",
		Args:  cobra.MaximumNArgs(1),
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			if len(args) == 1 {
				return []cmdkit.GetRequestCandidate{
					{Path: api.JoinPath("/webhook/%s/logs", args[0]), Opts: api.RequestOptions{Params: params}},
				}
			}
			return []cmdkit.GetRequestCandidate{
				{Path: "/webhook/logs", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}
