package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/schema"
)

func TestAutomateCommandRegistersAndSchemaResolves(t *testing.T) {
	root := buildRootWithCollections(t, makeDeps())
	automate := findChildCommand(root, "automate")
	if automate == nil {
		t.Fatal("missing automate command")
	}
	if automate.Hidden {
		t.Fatal("automate command should be visible now that Automate is publicly launched")
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

func TestAutomateCatalogueOperatorsIsLocalAndDocumentsUDAIntegerMapping(t *testing.T) {
	requests := 0
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		requests++
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"automate", "catalogue", "operators")
	if err != nil {
		t.Fatalf("automate catalogue operators failed: %v", err)
	}
	if requests != 0 {
		t.Fatalf("operators are local metadata and should not perform HTTP requests, got %d", requests)
	}

	var operators []map[string]any
	if err := json.Unmarshal([]byte(output), &operators); err != nil {
		t.Fatalf("failed to decode operators: %v", err)
	}
	if len(operators) != 12 {
		t.Fatalf("expected 12 operators, got %d: %+v", len(operators), operators)
	}
	if operators[1]["operator"] != "NotEquals" || operators[1]["deployUdaOperator"] != float64(1) {
		t.Fatalf("expected NotEquals to map to Deploy .uda operator 1, got %+v", operators[1])
	}
}

func TestAutomateValidateHelpPointsExistingEditsToImportUpdateDryRun(t *testing.T) {
	output, err := execute(buildRootWithCollections(t, makeDeps()),
		"automate", "automation", "validate", "--help")
	if err != nil {
		t.Fatalf("validate help failed: %v", err)
	}
	if !strings.Contains(output, "does not validate overwriting an existing automation") ||
		!strings.Contains(output, "automation import-update <id> --dry-run") {
		t.Fatalf("validate help should point update edits to import-update --dry-run, got:\n%s", output)
	}
}

func TestAutomateAutomationTriggerDryRunUsesAutomatePathWithoutRequest(t *testing.T) {
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

func TestAutomateWorkspaceGroupCommandsHitNestedRoutes(t *testing.T) {
	var observed []string
	var observedBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			if req.Body != nil && (req.Method == http.MethodPost || req.Method == http.MethodPut) {
				_ = json.NewDecoder(req.Body).Decode(&observedBody)
			}
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "workspace", "group", "add", "ws-1", "--name", "Content flows", "--parent-id", "parent-1"); err != nil {
		t.Fatalf("workspace group add failed: %v", err)
	}
	if observed[0] != "POST /umbraco/automate/management/api/v1/workspaces/ws-1/groups" {
		t.Fatalf("unexpected group add request: %v", observed)
	}
	if observedBody["name"] != "Content flows" || observedBody["parentId"] != "parent-1" {
		t.Fatalf("unexpected group body: %+v", observedBody)
	}

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "workspace", "group", "remove", "ws-1", "g-1", "--force"); err != nil {
		t.Fatalf("workspace group remove failed: %v", err)
	}
	if observed[len(observed)-1] != "DELETE /umbraco/automate/management/api/v1/workspaces/ws-1/groups/g-1" {
		t.Fatalf("unexpected group remove request: %v", observed)
	}
}

func TestAutomateWorkspaceDeleteRequiresForce(t *testing.T) {
	_, err := execute(buildRootWithCollections(t, makeDeps()), "automate", "workspace", "delete", "ws-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected workspace delete to require --force, got %v", err)
	}
}

func TestAutomateConnectionTestHitsTestRoute(t *testing.T) {
	var observed []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"success":true}`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps), "automate", "connection", "test", "conn-1")
	if err != nil {
		t.Fatalf("connection test failed: %v", err)
	}
	if len(observed) != 1 || observed[0] != "POST /umbraco/automate/management/api/v1/connections/conn-1/test" {
		t.Fatalf("unexpected request: %v", observed)
	}
	if !strings.Contains(output, `"success": true`) {
		t.Fatalf("expected test result passthrough, got %s", output)
	}
}

func TestAutomateConnectionDeleteRequiresForce(t *testing.T) {
	_, err := execute(buildRootWithCollections(t, makeDeps()), "automate", "connection", "delete", "conn-1")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected connection delete to require --force, got %v", err)
	}
}

func TestAutomateAutomationValidateWrapsExportModel(t *testing.T) {
	var observedPath string
	var observedBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observedPath = req.URL.Path
			if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, `{"success":true,"errors":[],"warnings":[]}`), nil
		})
	})

	output, err := execute(buildRootWithCollections(t, deps),
		"automate", "automation", "validate",
		"--workspace-id", "ws-1",
		"--json", `{"formatVersion":1,"automation":{"alias":"a"}}`,
	)
	if err != nil {
		t.Fatalf("automation validate failed: %v", err)
	}
	if observedPath != "/umbraco/automate/management/api/v1/automations/import/validate" {
		t.Fatalf("unexpected path: %s", observedPath)
	}
	if observedBody["workspaceId"] != "ws-1" {
		t.Fatalf("expected workspace wrapper, got %+v", observedBody)
	}
	exportModel, ok := observedBody["exportModel"].(map[string]any)
	if !ok || exportModel["formatVersion"] != float64(1) {
		t.Fatalf("expected export model passthrough, got %+v", observedBody)
	}
	if !strings.Contains(output, `"success": true`) {
		t.Fatalf("expected validation result, got %s", output)
	}

	_, err = execute(buildRootWithCollections(t, deps),
		"automate", "automation", "validate", "--workspace-id", "ws-1")
	if err == nil || !strings.Contains(err.Error(), "exactly one of --file or --json") {
		t.Fatalf("expected missing input to fail, got %v", err)
	}
}

func TestAutomateAutomationImportUpdateSendsBareExportModel(t *testing.T) {
	var observed string
	var observedBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = req.Method + " " + req.URL.Path
			if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "automation", "import-update", "auto-1",
		"--json", `{"formatVersion":1,"automation":{"alias":"a"}}`,
	); err != nil {
		t.Fatalf("import-update failed: %v", err)
	}
	if observed != "PUT /umbraco/automate/management/api/v1/automations/auto-1/import" {
		t.Fatalf("unexpected request: %s", observed)
	}
	if _, wrapped := observedBody["exportModel"]; wrapped {
		t.Fatalf("import-update must send the bare export model, got wrapper: %+v", observedBody)
	}
	if observedBody["formatVersion"] != float64(1) {
		t.Fatalf("unexpected body: %+v", observedBody)
	}
}

func TestAutomateVersionHistoryRoutes(t *testing.T) {
	var observed []string

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			observed = append(observed, req.Method+" "+req.URL.Path)
			return endpointJSONResponse(http.StatusOK, `{"items":[],"total":0}`), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "version-history", "list", "Automation", "auto-1"); err != nil {
		t.Fatalf("version-history list failed: %v", err)
	}
	if observed[0] != "GET /umbraco/automate/management/api/v1/version-history/Automation/auto-1" {
		t.Fatalf("unexpected list request: %v", observed)
	}

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "version-history", "compare", "Automation", "auto-1", "2", "5"); err != nil {
		t.Fatalf("version-history compare failed: %v", err)
	}
	if observed[len(observed)-1] != "GET /umbraco/automate/management/api/v1/version-history/Automation/auto-1/2/compare/5" {
		t.Fatalf("unexpected compare request: %v", observed)
	}

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "version-history", "rollback", "Automation", "auto-1", "2"); err != nil {
		t.Fatalf("version-history rollback failed: %v", err)
	}
	if observed[len(observed)-1] != "POST /umbraco/automate/management/api/v1/version-history/Automation/auto-1/2/rollback" {
		t.Fatalf("unexpected rollback request: %v", observed)
	}
}

func TestGenerateSkillsIncludesAutomateByDefault(t *testing.T) {
	_, err := execute(buildRootWithCollections(t, makeDeps()), "generate-skills", "--include-hidden", "--output-dir", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "--include-hidden requires --filter") {
		t.Fatalf("expected --include-hidden without --filter to fail, got %v", err)
	}

	dir := t.TempDir()
	if _, err := execute(buildRootWithCollections(t, makeDeps()),
		"generate-skills", "--filter", "automate", "--output-dir", dir); err != nil {
		t.Fatalf("generate-skills failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "umbraco-automate", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected automate skill in default generation: %v", err)
	}
	for _, want := range []string{"automation list", "automation validate", "catalogue operators", "workspace group add", "version-history rollback"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated automate skill missing %q:\n%s", want, string(content)[:500])
		}
	}
}

func TestGeneratedSkillsFlattenNestedSubgroups(t *testing.T) {
	dir := t.TempDir()
	if _, err := execute(buildRootWithCollections(t, makeDeps()),
		"generate-skills", "--filter", "document", "--output-dir", dir); err != nil {
		t.Fatalf("generate-skills failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "umbraco-document", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generated skill: %v", err)
	}
	if !strings.Contains(string(content), "### version rollback") {
		t.Fatal("expected nested 'document version rollback' to document as a leaf command")
	}
	if strings.Contains(string(content), "```bash\numbraco document version\n```") {
		t.Fatal("subgroup must not render as an empty stub")
	}
}

func TestAutomateUpdateMergeStripsResponseOnlyFields(t *testing.T) {
	var observedBody map[string]any

	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return tokenOr404(t, req, func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/umbraco/automate/management/api/v1/automations/auto-1" {
				return endpointJSONResponse(http.StatusNotFound, `null`), nil
			}
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{
					"id":"auto-1","workspaceId":"ws-1","status":"Draft","health":"Healthy",
					"publishedVersion":null,"dateCreated":"2026-06-09T10:00:00Z","dateModified":"2026-06-09T10:00:00Z",
					"alias":"a","name":"Old","steps":[],"connections":[],"version":3
				}`), nil
			}
			if err := json.NewDecoder(req.Body).Decode(&observedBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return endpointJSONResponse(http.StatusOK, ``), nil
		})
	})

	if _, err := execute(buildRootWithCollections(t, deps),
		"automate", "automation", "update", "auto-1", "--merge-json", `{"name":"New"}`); err != nil {
		t.Fatalf("automation update failed: %v", err)
	}

	// UpdateAutomationRequestModel declares additionalProperties: false, so
	// fields the GET echoes but the PUT rejects must be stripped.
	for _, key := range []string{"id", "workspaceId", "status", "health", "publishedVersion", "dateCreated", "dateModified"} {
		if _, present := observedBody[key]; present {
			t.Fatalf("response-only field %q must be stripped from the update body, got %+v", key, observedBody)
		}
	}
	if observedBody["name"] != "New" || observedBody["version"] != float64(3) {
		t.Fatalf("expected merged name and concurrency version to survive, got %+v", observedBody)
	}
}
