package commands

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/schema"
)

func TestAutomateCommandIsGatedAndSchemaHiddenByDefault(t *testing.T) {
	t.Setenv(automateEnableEnv, "")

	root := buildRootWithCollections(t, makeDeps())
	if findChildCommand(root, "automate") != nil {
		t.Fatal("automate command should not be registered without the feature gate")
	}

	output, err := execute(root, "schema", "--list")
	if err != nil {
		t.Fatalf("schema --list failed: %v", err)
	}
	if strings.Contains(output, "automate.") {
		t.Fatalf("Automate schema endpoints should be hidden without the feature gate: %s", output)
	}

	_, err = execute(buildRootWithCollections(t, makeDeps()), "schema", "automate.automation.list")
	if err == nil {
		t.Fatal("expected gated Automate schema lookup to fail")
	}
}

func TestAutomateCommandRegistersHiddenWhenFeatureGateEnabled(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")

	root := buildRootWithCollections(t, makeDeps())
	automate := findChildCommand(root, "automate")
	if automate == nil {
		t.Fatal("missing automate command with feature gate enabled")
	}
	if !automate.Hidden {
		t.Fatal("automate command should stay hidden while gated")
	}

	output, err := execute(root, "schema", "automate.automation.list")
	if err != nil {
		t.Fatalf("Automate schema lookup failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode schema output: %v", err)
	}
	if payload["apiRoot"] != automateAPIPrefix || payload["path"] != "/automations" {
		t.Fatalf("unexpected Automate schema payload: %+v", payload)
	}
}

func TestAutomateAutomationListUsesAutomateMountAndQueryFlags(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	var observedPath string
	observedQuery := map[string][]string{}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/automate/management/api/v1/automations" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			observedPath = req.URL.Path
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"auto-1","name":"Publish alert"}]}`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"automate", "automation", "list",
		"--filter", "publish",
		"--workspace-id", "workspace-1",
		"--group-id", "group-1",
		"--skip", "5",
		"--take", "10",
	)
	if err != nil {
		t.Fatalf("automate automation list failed: %v", err)
	}

	if observedPath != "/umbraco/automate/management/api/v1/automations" {
		t.Fatalf("unexpected request path: %s", observedPath)
	}
	assertQueryValue(t, observedQuery, "filter", "publish")
	assertQueryValue(t, observedQuery, "workspaceId", "workspace-1")
	assertQueryValue(t, observedQuery, "groupId", "group-1")
	assertQueryValue(t, observedQuery, "skip", "5")
	assertQueryValue(t, observedQuery, "take", "10")

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode automation list output: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected items payload, got %+v", payload)
	}
}

func TestAutomateCatalogueProjectsFieldsOnArrayResponses(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/automate/management/api/v1/catalogue/triggers" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			return endpointJSONResponse(http.StatusOK, `[{"alias":"content.published","name":"Content Published","outputSchema":{"properties":{"big":"schema"}}}]`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"automate", "catalogue", "triggers", "--fields", "alias,name")
	if err != nil {
		t.Fatalf("automate catalogue triggers failed: %v", err)
	}
	if strings.Contains(output, "outputSchema") {
		t.Fatalf("expected --fields to drop the embedded schema, got %s", output)
	}
	if !strings.Contains(output, "content.published") {
		t.Fatalf("expected projected alias in output, got %s", output)
	}
}

func TestAutomateCatalogueOutputSchemaSendsSettingsBody(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	var body map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/automate/management/api/v1/catalogue/step-types/Umbraco.Automate.Http/output-schema" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"status":{"type":"string"}}`), nil
		})
	})

	_, err := execute(buildRootWithCollections(t, deps),
		"automate", "catalogue", "output-schema", "Umbraco.Automate.Http",
		"--json", `{"settings":{"url":"https://example.test"}}`,
	)
	if err != nil {
		t.Fatalf("automate output-schema failed: %v", err)
	}

	settings, ok := body["settings"].(map[string]any)
	if !ok || settings["url"] != "https://example.test" {
		t.Fatalf("unexpected output-schema body: %+v", body)
	}
}

func TestAutomateAutomationTriggerDryRunUsesAutomatePathWithoutRequest(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	requests := 0
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		requests++
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"automate", "automation", "trigger", "auto-1", "--dry-run",
	)
	if err != nil {
		t.Fatalf("automate trigger dry-run failed: %v", err)
	}
	if requests != 0 {
		t.Fatalf("dry-run should not perform HTTP requests, got %d", requests)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode dry-run output: %v", err)
	}
	if payload["path"] != "/umbraco/automate/management/api/v1/automations/auto-1/trigger" {
		t.Fatalf("unexpected dry-run path: %+v", payload)
	}
}

func TestAutomateApprovalsDecideBuildsDecisionBody(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")

	output, err := execute(buildRootWithCollections(t, makeDeps()),
		"automate", "approvals", "decide", "run-1", "step-1",
		"--outcome", "Rejected",
		"--comment", "Needs changes",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("automate approval decision dry-run failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode dry-run output: %v", err)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok || body["outcome"] != "Rejected" || body["comment"] != "Needs changes" {
		t.Fatalf("unexpected decision body: %+v", payload)
	}

	_, err = execute(buildRootWithCollections(t, makeDeps()),
		"automate", "approvals", "decide", "run-1", "step-1", "--outcome", "Maybe", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "Approved or Rejected") {
		t.Fatalf("expected invalid outcome to fail, got %v", err)
	}
}

func TestAutomateMetricsByAutomationUsesQueryFlags(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	observedQuery := map[string][]string{}

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/automate/management/api/v1/metrics/by-automation" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			observedQuery = req.URL.Query()
			return endpointJSONResponse(http.StatusOK, `[{"automationId":"auto-1","totalRuns":3}]`), nil
		})
	})

	_, err := execute(buildRootWithCollections(t, deps),
		"automate", "metrics", "by-automation",
		"--workspace-id", "workspace-1",
		"--from", "2026-06-01T00:00:00Z",
		"--to", "2026-06-08T00:00:00Z",
		"--take", "5",
	)
	if err != nil {
		t.Fatalf("automate metrics by-automation failed: %v", err)
	}

	assertQueryValue(t, observedQuery, "workspaceId", "workspace-1")
	assertQueryValue(t, observedQuery, "from", "2026-06-01T00:00:00Z")
	assertQueryValue(t, observedQuery, "to", "2026-06-08T00:00:00Z")
	assertQueryValue(t, observedQuery, "take", "5")
}

func TestAutomateRunActionsHitRunRoutesAndCoalesce(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	var observed []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps), "automate", "run", "terminate", "run-1")
	if err != nil {
		t.Fatalf("automate run terminate failed: %v", err)
	}
	if len(observed) != 1 || observed[0] != "POST /umbraco/automate/management/api/v1/runs/run-1/terminate" {
		t.Fatalf("unexpected request: %v", observed)
	}
	if !strings.Contains(output, `"terminated": true`) {
		t.Fatalf("expected 204 coalescing, got %s", output)
	}
}

// TestAutomateCommandsHaveSchemas walks the automate tree recursively: every
// leaf command must have a schema entry keyed by its command path. The core
// schema coverage test only checks direct children of top-level collections,
// which for automate are all subgroups.
func TestAutomateCommandsHaveSchemas(t *testing.T) {
	t.Setenv(automateEnableEnv, "1")
	root := buildRootWithCollections(t, makeDeps())
	automate := findChildCommand(root, "automate")
	if automate == nil {
		t.Fatal("missing automate command")
	}

	missing := make([]string, 0)
	walkAutomateCommands(automate, []string{"automate"}, &missing)
	if len(missing) > 0 {
		t.Fatalf("Automate commands missing schemas: %v", missing)
	}
}

func walkAutomateCommands(cmd *cobra.Command, path []string, missing *[]string) {
	for _, child := range cmd.Commands() {
		next := append(append([]string{}, path...), child.Name())
		if len(child.Commands()) == 0 {
			key := strings.Join(next, ".")
			if _, ok := schema.Schemas[key]; !ok {
				*missing = append(*missing, key)
			}
			continue
		}
		walkAutomateCommands(child, next, missing)
	}
}
