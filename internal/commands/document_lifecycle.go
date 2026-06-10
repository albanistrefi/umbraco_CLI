package commands

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// This file holds the document lifecycle commands beyond single-node
// publish: descendant publishing, sibling ordering, culture domains, and
// public access (member-protected pages).

func documentPublishDescendants(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var cultures []string
	var includeUnpublished bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "publish-descendants <id>",
		Short: "Publish a document and its entire subtree",
		Long: `PUT /document/{id}/publish-with-descendants. Publishes the node and every published-state descendant; pass --include-unpublished to also publish drafts.

On variant content pass --culture per culture to publish; with no --culture the invariant default is used. The operation is asynchronous server-side — the response carries a taskId, and 'document publish-descendants-result <id> <task-id>' reports completion.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := parsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				body = map[string]any{
					"cultures":                      stringsToAny(cultures),
					"includeUnpublishedDescendants": includeUnpublished,
				}
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/publish-with-descendants", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "publishing", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Publish payload as JSON")
	cmd.Flags().StringArrayVar(&cultures, "culture", nil, "Culture to publish; repeat for multiple (omit for invariant content)")
	cmd.Flags().BoolVar(&includeUnpublished, "include-unpublished", false, "Also publish descendants that have never been published")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentPublishDescendantsResult(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "publish-descendants-result <id> <task-id>",
		Short: "Check the progress of an asynchronous publish-descendants run",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/document/%s/publish-with-descendants/result/%s", args[0], args[1]), api.RequestOptions{})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func documentSort(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var parent string
	var idsCSV string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sort",
		Short: "Reorder sibling documents",
		Long:  "PUT /document/sort. Pass --ids with the desired order (sortOrder is assigned from position) and --parent for the common parent; omit --parent when sorting root-level documents. IDs not listed keep their relative order after the sorted ones.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := parsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				ids := uniqueCSV(idsCSV)
				if len(ids) == 0 {
					return fmt.Errorf("document sort requires --ids <comma-separated guids in the desired order> or --json")
				}
				sorting := make([]any, len(ids))
				for i, id := range ids {
					sorting[i] = map[string]any{"id": id, "sortOrder": i}
				}
				body = map[string]any{"sorting": sorting}
				if strings.TrimSpace(parent) != "" {
					body["parent"] = map[string]any{"id": parent}
				}
			}
			result, err := deps.Client.Put(cmd.Context(), "/document/sort", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "sorted", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Sort payload as JSON")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent document ID (omit for root-level documents)")
	cmd.Flags().StringVar(&idsCSV, "ids", "", "Comma-separated document GUIDs in the desired order")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentDomains(deps Dependencies) *cobra.Command {
	domains := &cobra.Command{
		Use:   "domains",
		Short: "Culture domains (hostname → language routing) on a document",
	}
	domains.AddCommand(documentDomainsGet(deps))
	domains.AddCommand(documentDomainsSet(deps))
	return domains
}

func documentDomainsGet(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get the domains assigned to a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/document/%s/domains", args[0]), api.RequestOptions{})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func documentDomainsSet(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var defaultIso string
	var domainPairs []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Replace the domains assigned to a document",
		Long:  "PUT /document/{id}/domains. The PUT replaces the full set: pass every domain that should remain via repeated --domain host=isoCode flags (e.g. --domain example.dk=da-DK), or the raw payload via --json.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := parsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				if len(domainPairs) == 0 {
					return fmt.Errorf("document domains set requires --domain host=isoCode entries or --json")
				}
				domains := make([]any, 0, len(domainPairs))
				for _, pair := range domainPairs {
					host, iso, ok := strings.Cut(pair, "=")
					if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(iso) == "" {
						return fmt.Errorf("invalid --domain value %q, expected host=isoCode", pair)
					}
					domains = append(domains, map[string]any{"domainName": strings.TrimSpace(host), "isoCode": strings.TrimSpace(iso)})
				}
				body = map[string]any{"domains": domains}
				if strings.TrimSpace(defaultIso) != "" {
					body["defaultIsoCode"] = defaultIso
				}
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath("/document/%s/domains", args[0]), body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Domains payload as JSON")
	cmd.Flags().StringVar(&defaultIso, "default-iso-code", "", "Default culture for unmatched hosts")
	cmd.Flags().StringArrayVar(&domainPairs, "domain", nil, "Domain assignment as host=isoCode; repeat for multiple")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentPublicAccess(deps Dependencies) *cobra.Command {
	publicAccess := &cobra.Command{
		Use:   "public-access",
		Short: "Member protection (login-required access) on a document",
	}
	publicAccess.AddCommand(documentPublicAccessGet(deps))
	publicAccess.AddCommand(documentPublicAccessSet(deps))
	publicAccess.AddCommand(documentPublicAccessRemove(deps))
	return publicAccess
}

func documentPublicAccessGet(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get the public-access (member protection) rules on a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), api.JoinPath("/document/%s/public-access", args[0]), api.RequestOptions{})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
}

func documentPublicAccessSet(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Create or replace the public-access rules on a document",
		Long: `Sets member protection: which member groups (or named members) may view the document, plus the login and error pages. Payload shape:

  {"loginDocument":{"id":"<guid>"},"errorDocument":{"id":"<guid>"},"memberGroupNames":["Members"],"memberUserNames":[]}

The CLI checks whether rules already exist and issues POST (create) or PUT (replace) accordingly.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			path := api.JoinPath("/document/%s/public-access", args[0])

			// The API splits create (POST) and replace (PUT); resolve which
			// applies so the caller doesn't have to know whether rules exist.
			exists := true
			if _, err := deps.Client.Get(ctx, path, api.RequestOptions{}); err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
					exists = false
				} else {
					return err
				}
			}

			var result any
			if exists {
				result, err = deps.Client.Put(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			} else {
				result, err = deps.Client.Post(ctx, path, body, api.RequestOptions{DryRun: dryRun})
			}
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Public-access payload as JSON")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func documentPublicAccessRemove(deps Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove the public-access rules from a document (makes it publicly visible again)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !dryRun {
				return fmt.Errorf("document public-access remove drops member protection; pass --force to confirm or --dry-run to rehearse")
			}
			result, err := deps.Client.Delete(cmd.Context(), api.JoinPath("/document/%s/public-access", args[0]), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "removed", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm removing member protection")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
