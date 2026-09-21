package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Umbraco Deploy ships its own management API next to the CMS one. The
// routes and request shapes below were read from the Deploy 18 backoffice
// bundle (App_Plugins/Deploy) — Deploy publishes no OpenAPI document for
// them — and exercised against a local 18.1 instance with Deploy 18.0.1:
//
//	GET  /queue                    → [{udi:{uriValue,entityType}, culture, name, includeDescendants, releaseDate}]
//	POST /queue/add                {id, entityType, culture:"*", includeDescendants, releaseDate} → the queued item
//	POST /queue/remove             {udi, culture}
//	POST /queue/clear
//	GET  /entity/name?id&entityType → "Name"
//	GET  /configuration/client     → {clientConfiguration:{target:{name,type,deployUrl,url}, workspaces, allowDeployIgnoreDependencies, …}}
//	POST /deploy                   {targetUrl, ignoreDependencies, enableLogging} → {sessionId}  (transfers the queue)
//	POST /deploy/instant           {targetUrl, ignoreDependencies, enableLogging, items:[queue items]} → {sessionId}
//	POST /status/status            {sessionId} → {sessionId, status, percent, comment, log, exceptionJson, mismatchList, serverTimeStamp}
//
// targetUrl is the configured target workspace's deployUrl (the next
// environment in the Cloud chain, e.g. Local → Development).
const deployAPIPrefix = "/umbraco/deploy/management/api/v1"

func deployRequestOpts(params map[string]any) api.RequestOptions {
	return api.RequestOptions{APIPrefix: deployAPIPrefix, Params: params}
}

// deployTransferFailedError: the transfer session ended Failed, Cancelled
// or Mismatch. Exit 5 is the deployment-failed code shared with deploy watch.
type deployTransferFailedError struct{ reason string }

func (e deployTransferFailedError) Error() string { return "deploy transfer failed: " + e.reason }
func (deployTransferFailedError) ExitCode() int   { return 5 }

// deployTransferTimeoutError: --timeout elapsed while the session was still
// running. Exit 6 is "status unknown", as for deploy watch.
type deployTransferTimeoutError struct{ reason string }

func (e deployTransferTimeoutError) Error() string { return "deploy transfer timeout: " + e.reason }
func (deployTransferTimeoutError) ExitCode() int   { return 6 }

// deployWorkStatuses is Deploy's WorkStatus enum in declaration order; the
// status endpoint returns either the index or the name.
var deployWorkStatuses = []string{"Unknown", "New", "Executing", "Completed", "Failed", "Cancelled", "TimedOut", "Mismatch"}

func deployWorkStatusName(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		if index := int(value); index >= 0 && index < len(deployWorkStatuses) {
			return deployWorkStatuses[index]
		}
	}
	return fmt.Sprint(raw)
}

// deployTarget is the configured transfer target read from the client
// configuration.
type deployTarget struct {
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	DeployURL               string `json:"deployUrl"`
	URL                     string `json:"url,omitempty"`
	CurrentWorkspace        string `json:"currentWorkspace,omitempty"`
	AllowIgnoreDependencies bool   `json:"allowIgnoreDependencies"`
}

func fetchDeployTarget(ctx context.Context, client *api.Client) (deployTarget, error) {
	result, err := fetchObject(ctx, client, "/configuration/client", deployRequestOpts(nil))
	if err != nil {
		return deployTarget{}, friendlyDeployAPIError(err)
	}
	config, _ := result["clientConfiguration"].(map[string]any)
	target, _ := config["target"].(map[string]any)
	out := deployTarget{
		Name:             stringValue(target["name"]),
		Type:             stringValue(target["type"]),
		DeployURL:        stringValue(target["deployUrl"]),
		URL:              stringValue(target["url"]),
		CurrentWorkspace: stringValue(config["currentWorkspace"]),
	}
	out.AllowIgnoreDependencies, _ = config["allowDeployIgnoreDependencies"].(bool)
	if out.DeployURL == "" {
		return out, fmt.Errorf("this environment has no Deploy transfer target (workspace %q is the last in its chain, or Deploy is not connected to a project); nothing to transfer to", out.CurrentWorkspace)
	}
	return out, nil
}

// friendlyDeployAPIError turns the 404 a CMS without Umbraco Deploy answers
// into a statement instead of a route hint.
func friendlyDeployAPIError(err error) error {
	if isAPIStatus(err, http.StatusNotFound) {
		return fmt.Errorf("the Umbraco Deploy management API is not available on this environment (%w); deploy transfer needs the Umbraco.Deploy package installed on the source environment", err)
	}
	return err
}

// deployQueueItem is the shape both the queue and the instant transfer take.
type deployQueueItem struct {
	ID                 string  `json:"id"`
	EntityType         string  `json:"entityType"`
	Culture            string  `json:"culture"`
	IncludeDescendants bool    `json:"includeDescendants"`
	ReleaseDate        *string `json:"releaseDate"`
}

func (item deployQueueItem) body() map[string]any {
	body := map[string]any{
		"id":                 item.ID,
		"entityType":         item.EntityType,
		"culture":            item.Culture,
		"includeDescendants": item.IncludeDescendants,
		"releaseDate":        nil,
	}
	if item.ReleaseDate != nil {
		body["releaseDate"] = *item.ReleaseDate
	}
	return body
}

// deployEntityTypes lists what the transfer commands accept for --type.
var deployEntityTypes = map[string]bool{"document": true, "media": true, "member": true, "dictionary-item": true, "form": true}

func normalizeDeployEntityType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "document", "content", "doc":
		return "document", nil
	case "media":
		return "media", nil
	case "dictionary", "dictionary-item", "dictionaryitem":
		return "dictionary-item", nil
	}
	if deployEntityTypes[value] {
		return value, nil
	}
	return "", fmt.Errorf("--type must be document, media, member, dictionary-item or form, got %q", raw)
}

// resolveDeployEntityName asks Deploy for the entity's name. Deploy answers
// an unknown document or media id with a 500 from its connector (verified on
// 18.0.1), so those types are checked against the CMS first to turn a typo
// into a plain "does not exist".
func resolveDeployEntityName(ctx context.Context, client *api.Client, entityType string, id string) (string, error) {
	if entityType == "document" || entityType == "media" {
		if _, err := client.Get(ctx, api.JoinPath("/"+entityType+"/%s", id), api.RequestOptions{}); err != nil {
			if isAPIStatus(err, http.StatusNotFound) {
				return "", fmt.Errorf("%s %s does not exist in this environment", entityType, id)
			}
			return "", err
		}
	}
	result, err := client.Get(ctx, "/entity/name", deployRequestOpts(map[string]any{"id": id, "entityType": entityType}))
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			return "", fmt.Errorf("%s %s does not exist in this environment", entityType, id)
		}
		return "", friendlyDeployAPIError(err)
	}
	name, _ := result.(string)
	return name, nil
}

// countDescendants walks the CMS tree under id and returns how many nodes
// --descendants would carry along (capped so a dry-run on a huge tree
// stays cheap; the cap is reported as "at least").
const deployDescendantCountCap = 2000

func countDescendants(ctx context.Context, client *api.Client, entityType string, id string) (int, bool, error) {
	if entityType != "document" && entityType != "media" {
		return 0, false, nil
	}
	count := 0
	pending := []string{id}
	for len(pending) > 0 {
		parent := pending[0]
		pending = pending[1:]
		result, err := getAllPagesWithFallback(ctx, client, 0, 0, 0,
			getRequestCandidate{path: "/tree/" + entityType + "/children", opts: api.RequestOptions{Params: map[string]any{"parentId": parent}}},
		)
		if err != nil {
			return count, false, err
		}
		for _, item := range resultItems(result) {
			count++
			if count >= deployDescendantCountCap {
				return count, true, nil
			}
			entry, _ := item.(map[string]any)
			if hasChildren, _ := entry["hasChildren"].(bool); hasChildren {
				if childID := itemID(entry); childID != "" {
					pending = append(pending, childID)
				}
			}
		}
	}
	return count, false, nil
}

// deploySessionStatus is one poll of a transfer session.
type deploySessionStatus struct {
	SessionID  string  `json:"sessionId"`
	Status     string  `json:"status"`
	Percent    float64 `json:"percent"`
	Comment    string  `json:"comment,omitempty"`
	Log        string  `json:"log,omitempty"`
	Exception  string  `json:"exception,omitempty"`
	Mismatches []any   `json:"mismatches,omitempty"`
	ServerTime string  `json:"serverTime,omitempty"`
	Raw        any     `json:"-"`
}

func pollDeploySession(ctx context.Context, client *api.Client, sessionID string) (deploySessionStatus, error) {
	result, err := client.Post(ctx, "/status/status", map[string]any{"sessionId": sessionID}, deployRequestOpts(nil))
	if err != nil {
		return deploySessionStatus{}, err
	}
	entry, _ := result.(map[string]any)
	status := deploySessionStatus{
		SessionID:  stringValue(entry["sessionId"]),
		Status:     deployWorkStatusName(entry["status"]),
		Comment:    stringValue(entry["comment"]),
		Log:        stringValue(entry["log"]),
		Exception:  stringValue(entry["exceptionJson"]),
		ServerTime: stringValue(entry["serverTimeStamp"]),
		Raw:        result,
	}
	if percent, ok := entry["percent"].(float64); ok {
		status.Percent = percent
	}
	if mismatches, ok := entry["mismatchList"].([]any); ok {
		status.Mismatches = mismatches
	}
	if status.SessionID == "" {
		status.SessionID = sessionID
	}
	return status, nil
}

func deployTerminalStatus(status string) bool {
	switch status {
	case "Completed", "Failed", "Cancelled", "TimedOut", "Mismatch":
		return true
	}
	return false
}

// waitForDeploySession polls until the session is terminal or the timeout
// elapses, writing progress changes to progress (stderr).
func waitForDeploySession(ctx context.Context, client *api.Client, sessionID string, interval time.Duration, timeout time.Duration, progress func(deploySessionStatus)) (deploySessionStatus, error) {
	deadline := time.Now().Add(timeout)
	var last deploySessionStatus
	lastKey := ""
	for {
		status, err := pollDeploySession(ctx, client, sessionID)
		if err != nil {
			if ctx.Err() != nil {
				return last, ctx.Err()
			}
			return last, fmt.Errorf("polling transfer session %s failed: %w", sessionID, err)
		}
		last = status
		if key := fmt.Sprintf("%s|%.0f|%s", status.Status, status.Percent, status.Comment); key != lastKey {
			lastKey = key
			if progress != nil {
				progress(status)
			}
		}
		if deployTerminalStatus(status.Status) {
			return status, nil
		}
		if time.Now().After(deadline) {
			return status, deployTransferTimeoutError{reason: fmt.Sprintf("session %s still %s (%.0f%%) after %s; it keeps running on the server — check the Deploy dashboard or poll 'umbraco api POST /status/status' with the session id", sessionID, status.Status, status.Percent, timeout)}
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func deployTransfer(deps Dependencies) *cobra.Command {
	var nodes []string
	var entityType string
	var descendants bool
	var culture string
	var useQueue bool
	var ignoreDependencies bool
	var wait bool
	var interval time.Duration
	var timeout time.Duration
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "transfer --node <id> [--node <id>…] [--descendants] | --queue",
		Short: "Transfer content to the next environment with Umbraco Deploy (the backoffice's \"Transfer now\" / \"Transfer queue\")",
		Long: `Moves content (documents, media, members, dictionary items, forms) to the
environment Umbraco Deploy is configured to transfer to, keeping GUIDs
identical — so pickers and start nodes that store a content id resolve on
the target. Schema travels in .uda files; content does not, hence this
command.

Two sources: --node <id> (repeatable; --descendants carries the subtree)
transfers those items directly (POST /deploy/instant); --queue transfers
whatever 'deploy queue add' has accumulated (POST /deploy). Deploy resolves
dependencies at transfer time (picked content, media, members and their
schema) and includes them automatically; --ignore-dependencies turns that
off where the environment allows it.

--dry-run resolves the target and each item's name, counts descendants, and
shows the exact request; nothing is sent. Dependencies are computed
server-side during the transfer, so a dry-run cannot list them — the
completed session reports what actually moved. Without --dry-run the
command requires --force: it writes to another environment.

By default the command waits for the transfer session, prints progress on
stderr, and exits 0 on Completed, 5 on Failed/Cancelled/Mismatch (with the
server's log), and 6 when --timeout elapses first (status unknown; the
transfer keeps running). --no-wait returns the session id immediately.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if useQueue && len(nodes) > 0 {
				return fmt.Errorf("pass either --node (transfer these items now) or --queue (transfer the accumulated queue), not both")
			}
			if !useQueue && len(nodes) == 0 {
				return fmt.Errorf("deploy transfer needs --node <id> (repeatable) or --queue")
			}
			if interval <= 0 {
				return fmt.Errorf("--interval must be greater than zero")
			}
			resolvedType, err := normalizeDeployEntityType(entityType)
			if err != nil {
				return err
			}
			if err := requireForceOrDryRun(cmd, "transfers content to another environment", force, dryRun); err != nil {
				return err
			}

			target, err := fetchDeployTarget(ctx, deps.Client)
			if err != nil {
				return err
			}
			if ignoreDependencies && !target.AllowIgnoreDependencies {
				return fmt.Errorf("--ignore-dependencies is disabled by this environment's Deploy configuration (allowDeployIgnoreDependencies=false)")
			}

			cultureValue := strings.TrimSpace(culture)
			if cultureValue == "" {
				cultureValue = "*"
			}

			items := []map[string]any{}
			if useQueue {
				queued, err := deps.Client.Get(ctx, "/queue", deployRequestOpts(nil))
				if err != nil {
					return friendlyDeployAPIError(err)
				}
				for _, raw := range resultItems(queued) {
					if entry, ok := raw.(map[string]any); ok {
						items = append(items, entry)
					}
				}
				if len(items) == 0 {
					return fmt.Errorf("the Deploy transfer queue is empty; add items with 'umbraco deploy queue add <id>' or pass --node")
				}
			} else {
				for _, raw := range nodes {
					for _, id := range uniqueCSV(raw) {
						if !isUUIDLike(id) {
							return fmt.Errorf("--node %q is not a GUID", id)
						}
						name, err := resolveDeployEntityName(ctx, deps.Client, resolvedType, id)
						if err != nil {
							return err
						}
						entry := deployQueueItem{ID: id, EntityType: resolvedType, Culture: cultureValue, IncludeDescendants: descendants}.body()
						entry["name"] = name
						if descendants {
							count, capped, err := countDescendants(ctx, deps.Client, resolvedType, id)
							if err != nil {
								return fmt.Errorf("counting descendants of %s failed: %w", id, err)
							}
							entry["descendants"] = count
							if capped {
								entry["descendantsAtLeast"] = true
							}
						}
						items = append(items, entry)
					}
				}
			}

			plan := map[string]any{
				"target":             target,
				"items":              items,
				"ignoreDependencies": ignoreDependencies,
				"dependencies":       "resolved by Deploy at transfer time and included unless ignoreDependencies",
			}

			var path string
			var body map[string]any
			if useQueue {
				path = "/deploy"
				body = map[string]any{"targetUrl": target.DeployURL, "ignoreDependencies": ignoreDependencies, "enableLogging": true}
				plan["source"] = "queue"
			} else {
				path = "/deploy/instant"
				requestItems := make([]any, 0, len(items))
				for _, entry := range items {
					requestItems = append(requestItems, map[string]any{
						"id":                 entry["id"],
						"entityType":         entry["entityType"],
						"culture":            entry["culture"],
						"includeDescendants": entry["includeDescendants"],
						"releaseDate":        entry["releaseDate"],
					})
				}
				body = map[string]any{"targetUrl": target.DeployURL, "ignoreDependencies": ignoreDependencies, "enableLogging": true, "items": requestItems}
				plan["source"] = "nodes"
			}

			result, err := deps.Client.Post(ctx, path, body, api.RequestOptions{APIPrefix: deployAPIPrefix, DryRun: dryRun})
			if err != nil {
				return friendlyDeployAPIError(err)
			}
			if dryRun {
				plan["dryRun"] = true
				plan["request"] = result
				return printResult(cmd, deps, plan)
			}
			started, _ := result.(map[string]any)
			sessionID := stringValue(started["sessionId"])
			if sessionID == "" {
				return fmt.Errorf("Deploy accepted the transfer but returned no session id: %v", result)
			}
			plan["sessionId"] = sessionID
			if !wait {
				plan["status"] = "started"
				plan["hint"] = "poll with: umbraco api POST /umbraco/deploy/management/api/v1/status/status --raw-path --body '{\"sessionId\":\"" + sessionID + "\"}'"
				return printResult(cmd, deps, plan)
			}

			errOut := cmd.ErrOrStderr()
			fmt.Fprintf(errOut, "transfer session %s started → %s (%s); waiting up to %s\n", sessionID, target.Name, target.Type, timeout)
			final, waitErr := waitForDeploySession(ctx, deps.Client, sessionID, interval, timeout, func(status deploySessionStatus) {
				line := fmt.Sprintf("%s %.0f%%", status.Status, status.Percent)
				if status.Comment != "" {
					line += " — " + status.Comment
				}
				fmt.Fprintln(errOut, line)
			})
			plan["status"] = final.Status
			plan["percent"] = final.Percent
			if final.Comment != "" {
				plan["comment"] = final.Comment
			}
			if len(final.Mismatches) > 0 {
				plan["mismatches"] = final.Mismatches
			}
			if final.Log != "" {
				plan["log"] = final.Log
			}
			if final.Exception != "" {
				var decoded any
				if json.Unmarshal([]byte(final.Exception), &decoded) == nil {
					plan["exception"] = decoded
				} else {
					plan["exception"] = final.Exception
				}
			}
			if err := printResult(cmd, deps, plan); err != nil {
				return err
			}
			if waitErr != nil {
				return waitErr
			}
			switch final.Status {
			case "Completed":
				return nil
			default:
				reason := final.Status
				if final.Comment != "" {
					reason += ": " + final.Comment
				}
				return deployTransferFailedError{reason: reason}
			}
		},
	}

	cmd.Flags().StringArrayVar(&nodes, "node", nil, "Content id to transfer (GUID; repeatable, or comma-separated)")
	cmd.Flags().StringVar(&entityType, "type", "document", "Entity type of --node: document, media, member, dictionary-item, form")
	cmd.Flags().BoolVar(&descendants, "descendants", false, "Also transfer everything under each --node")
	cmd.Flags().StringVar(&culture, "culture", "", "Only this culture's variant (default: all cultures)")
	cmd.Flags().BoolVar(&useQueue, "queue", false, "Transfer the accumulated Deploy queue ('deploy queue list') instead of --node")
	cmd.Flags().BoolVar(&ignoreDependencies, "ignore-dependencies", false, "Do not include dependencies (only when the environment allows it)")
	cmd.Flags().BoolVar(&wait, "wait", true, "Wait for the transfer session to finish (--wait=false returns the session id immediately)")
	cmd.Flags().DurationVar(&interval, "interval", 3*time.Second, "Session poll interval while waiting")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Give up waiting after this long (exit 6; the transfer keeps running)")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the transfer when not using --dry-run")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// deployQueue is the "queue for transfer" side: accumulate items, review
// them, transfer them together with 'deploy transfer --queue'.
func deployQueue(deps Dependencies) *cobra.Command {
	group := &cobra.Command{
		Use:   "queue",
		Short: "Umbraco Deploy transfer queue (list, add, remove, clear)",
		Long:  "The queue is per source environment and lives on the server, shared with the backoffice's \"Queue for transfer\". 'deploy transfer --queue' sends it to the configured target.",
	}
	group.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the items queued for transfer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/queue", deployRequestOpts(nil))
			if err != nil {
				return friendlyDeployAPIError(err)
			}
			items := resultItems(result)
			return printResult(cmd, deps, map[string]any{"items": items, "total": len(items)})
		},
	})

	var addType string
	var addDescendants bool
	var addCulture string
	var addDryRun bool
	add := &cobra.Command{
		Use:   "add <id> [<id>…]",
		Short: "Queue content for transfer (the backoffice's \"Queue for transfer\")",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedType, err := normalizeDeployEntityType(addType)
			if err != nil {
				return err
			}
			cultureValue := strings.TrimSpace(addCulture)
			if cultureValue == "" {
				cultureValue = "*"
			}
			queued := make([]any, 0, len(args))
			for _, raw := range args {
				for _, id := range uniqueCSV(raw) {
					if !isUUIDLike(id) {
						return fmt.Errorf("%q is not a GUID", id)
					}
					item := deployQueueItem{ID: id, EntityType: resolvedType, Culture: cultureValue, IncludeDescendants: addDescendants}
					result, err := deps.Client.Post(cmd.Context(), "/queue/add", item.body(), api.RequestOptions{APIPrefix: deployAPIPrefix, DryRun: addDryRun})
					if err != nil {
						return friendlyDeployAPIError(err)
					}
					queued = append(queued, result)
				}
			}
			return printResult(cmd, deps, map[string]any{"queued": queued, "count": len(queued), "dryRun": addDryRun})
		},
	}
	add.Flags().StringVar(&addType, "type", "document", "Entity type: document, media, member, dictionary-item, form")
	add.Flags().BoolVar(&addDescendants, "descendants", false, "Queue the whole subtree under each id")
	add.Flags().StringVar(&addCulture, "culture", "", "Only this culture's variant (default: all cultures)")
	addDryRunFlag(add, &addDryRun)
	group.AddCommand(add)

	var removeCulture string
	var removeType string
	var removeDryRun bool
	remove := &cobra.Command{
		Use:   "remove <id-or-udi>",
		Short: "Remove one item from the transfer queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			udi := strings.TrimSpace(args[0])
			if !strings.HasPrefix(udi, "umb://") {
				resolvedType, err := normalizeDeployEntityType(removeType)
				if err != nil {
					return err
				}
				if !isUUIDLike(udi) {
					return fmt.Errorf("%q is neither a GUID nor a UDI (umb://document/<32 hex>)", args[0])
				}
				udi = "umb://" + resolvedType + "/" + strings.ToLower(strings.ReplaceAll(udi, "-", ""))
			}
			cultureValue := strings.TrimSpace(removeCulture)
			if cultureValue == "" {
				cultureValue = "*"
			}
			result, err := deps.Client.Post(cmd.Context(), "/queue/remove", map[string]any{"udi": udi, "culture": cultureValue}, api.RequestOptions{APIPrefix: deployAPIPrefix, DryRun: removeDryRun})
			if err != nil {
				return friendlyDeployAPIError(err)
			}
			if removeDryRun {
				return printResult(cmd, deps, result)
			}
			return printResult(cmd, deps, map[string]any{"removed": true, "udi": udi, "culture": cultureValue})
		},
	}
	remove.Flags().StringVar(&removeType, "type", "document", "Entity type when passing a GUID: document, media, member, dictionary-item, form")
	remove.Flags().StringVar(&removeCulture, "culture", "", "Culture the item was queued with (default: all cultures)")
	addDryRunFlag(remove, &removeDryRun)
	group.AddCommand(remove)

	var clearForce bool
	var clearDryRun bool
	clear := &cobra.Command{
		Use:   "clear",
		Short: "Empty the transfer queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "discards every queued transfer item", clearForce, clearDryRun); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/queue/clear", map[string]any{}, api.RequestOptions{APIPrefix: deployAPIPrefix, DryRun: clearDryRun})
			if err != nil {
				return friendlyDeployAPIError(err)
			}
			return printMutationResult(cmd, deps, "cleared", result, clearDryRun)
		},
	}
	clear.Flags().BoolVar(&clearForce, "force", false, "Confirm clearing the queue")
	addDryRunFlag(clear, &clearDryRun)
	group.AddCommand(clear)
	return group
}
