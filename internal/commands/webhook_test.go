package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/cmdtest"
)

func TestNormalizeWebhookEventsMapsObjectsToAliases(t *testing.T) {
	body := map[string]any{"events": []any{
		map[string]any{"eventName": "Content Deleted", "eventType": "Content", "alias": "Umbraco.ContentDelete"},
		"Umbraco.ContentPublish",
	}}
	if err := normalizeWebhookEvents(body); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	events := body["events"].([]any)
	if events[0] != "Umbraco.ContentDelete" || events[1] != "Umbraco.ContentPublish" {
		t.Fatalf("expected alias strings, got %+v", events)
	}
	// Idempotent: a second pass must not change anything.
	if err := normalizeWebhookEvents(body); err != nil {
		t.Fatalf("second normalize failed: %v", err)
	}
	if body["events"].([]any)[0] != "Umbraco.ContentDelete" {
		t.Fatalf("expected idempotent normalization, got %+v", body["events"])
	}
}

func TestNormalizeWebhookEventsRejectsAliaslessObjects(t *testing.T) {
	body := map[string]any{"events": []any{map[string]any{"eventName": "X"}}}
	if err := normalizeWebhookEvents(body); err == nil || !strings.Contains(err.Error(), "without an alias") {
		t.Fatalf("expected aliasless rejection, got %v", err)
	}
}

func TestNormalizeWebhookEventsLeavesAbsentFieldAlone(t *testing.T) {
	body := map[string]any{"enabled": true}
	if err := normalizeWebhookEvents(body); err != nil {
		t.Fatalf("expected no-op on absent events, got %v", err)
	}
}

func TestWebhookUpdateEventsPatchReplacesInBothForms(t *testing.T) {
	for name, patch := range map[string]string{
		"strings": `{"events":["Umbraco.ContentDelete"]}`,
		"objects": `{"events":[{"eventName":"Content Deleted","eventType":"Content","alias":"Umbraco.ContentDelete"}]}`,
	} {
		var putBody map[string]any
		deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/umbraco/management/api/v1/security/back-office/token":
				return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
			default:
				if req.Method == http.MethodGet {
					return cmdtest.JSONResponse(http.StatusOK, `{"id":"wh-1","url":"https://localhost/x","enabled":true,"contentTypeKeys":[],"headers":{},"events":[{"eventName":"Content Published","eventType":"Content","alias":"Umbraco.ContentPublish"},{"eventName":"Content Deleted","eventType":"Content","alias":"Umbraco.ContentDelete"}]}`), nil
				}
				if err := json.NewDecoder(req.Body).Decode(&putBody); err != nil {
					t.Fatalf("decode put body: %v", err)
				}
				return cmdtest.JSONResponse(http.StatusOK, `null`), nil
			}
		})

		root := &cobra.Command{Use: "umbraco", SilenceErrors: true, SilenceUsage: true}
		root.SetErr(io.Discard)
		if deps.OutputFlag != nil {
			root.PersistentFlags().StringVarP(deps.OutputFlag, "output", "o", *deps.OutputFlag, "Output format: json, table, plain")
		}
		RegisterWebhook(root, deps)
		if _, err := cmdtest.Execute(root, "webhook", "update", "wh-1", "--merge-json", patch); err != nil {
			t.Fatalf("%s: update failed: %v", name, err)
		}
		events, _ := putBody["events"].([]any)
		if len(events) != 1 || events[0] != "Umbraco.ContentDelete" {
			t.Fatalf("%s: expected events patch to REPLACE the set with one alias string, got %+v", name, putBody["events"])
		}
		if putBody["url"] != "https://localhost/x" {
			t.Fatalf("%s: expected unrelated fields preserved by the merge, got %+v", name, putBody)
		}
	}
}
