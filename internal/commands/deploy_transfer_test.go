package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// deployTransferServer mocks the Deploy management API: a configured target,
// entity names, a queue, an instant transfer that returns a session, and a
// status endpoint that walks the given statuses one poll at a time.
func deployTransferServer(t *testing.T, statuses []string, opts map[string]any) (Dependencies, *[]string, *[]map[string]any) {
	t.Helper()
	var requests []string
	var bodies []map[string]any
	var polls int64
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		path := strings.TrimPrefix(req.URL.Path, "/umbraco/deploy/management/api/v1")
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		if !strings.HasPrefix(req.URL.Path, "/umbraco/deploy/") {
			if strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
				if strings.HasSuffix(req.URL.Path, "dead") {
					return endpointJSONResponse(http.StatusNotFound, `{"title":"Not found","status":404}`), nil
				}
				return endpointJSONResponse(http.StatusOK, `{"id":"x"}`), nil
			}
			if strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/tree/document/children") {
				if req.URL.Query().Get("parentId") == "aaaaaaaa-0000-4000-8000-000000000001" {
					return endpointJSONResponse(http.StatusOK, `{"total":2,"items":[{"id":"aaaaaaaa-0000-4000-8000-000000000002","hasChildren":true},{"id":"aaaaaaaa-0000-4000-8000-000000000003","hasChildren":false}]}`), nil
				}
				return endpointJSONResponse(http.StatusOK, `{"total":1,"items":[{"id":"aaaaaaaa-0000-4000-8000-000000000004","hasChildren":false}]}`), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		}
		requests = append(requests, req.Method+" "+path)
		body := map[string]any{}
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &body)
		}
		bodies = append(bodies, body)
		switch path {
		case "/configuration/client":
			allow, _ := opts["allowIgnore"].(bool)
			return endpointJSONResponse(http.StatusOK, `{"clientConfiguration":{"currentWorkspace":"Local","allowDeployIgnoreDependencies":`+map[bool]string{true: "true", false: "false"}[allow]+`,"target":{"name":"Development","type":"development","deployUrl":"https://dev.example.test/umbraco/backoffice/deploy/environment","url":"https://dev.example.test"}}}`), nil
		case "/entity/name":
			if req.URL.Query().Get("id") == "aaaaaaaa-0000-4000-8000-00000000dead" {
				return endpointJSONResponse(http.StatusNotFound, `{"title":"Not found","status":404}`), nil
			}
			return endpointJSONResponse(http.StatusOK, `"Sector A"`), nil
		case "/queue":
			return endpointJSONResponse(http.StatusOK, `[{"udi":{"uriValue":"umb://document/aaaaaaaa00004000800000000000000a","entityType":"document"},"culture":"*","name":"Queued","includeDescendants":true,"releaseDate":null}]`), nil
		case "/queue/add":
			return endpointJSONResponse(http.StatusOK, `{"udi":{"uriValue":"umb://document/aaaaaaaa00004000800000000000000a","entityType":"document"},"culture":"*","name":"Queued","includeDescendants":false,"releaseDate":null}`), nil
		case "/queue/remove", "/queue/clear":
			return endpointNoContent(), nil
		case "/deploy/instant", "/deploy":
			return endpointJSONResponse(http.StatusOK, `{"sessionId":"11111111-2222-4333-8444-555555555555"}`), nil
		case "/status/status":
			index := int(atomic.AddInt64(&polls, 1)) - 1
			if index >= len(statuses) {
				index = len(statuses) - 1
			}
			status := statuses[index]
			extra := ""
			if status == "Failed" {
				extra = `,"comment":"Dependency missing","log":"line1\nline2","exceptionJson":"{\"Message\":\"boom\"}"`
			}
			return endpointJSONResponse(http.StatusOK, `{"sessionId":"11111111-2222-4333-8444-555555555555","status":"`+status+`","percent":`+map[bool]string{true: "100", false: "40"}[deployTerminalStatus(status)]+`,"mismatchList":[]`+extra+`}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	return deps, &requests, &bodies
}

func TestDeployTransferDryRunResolvesTargetNamesAndDescendantsWithoutSending(t *testing.T) {
	deps, requests, _ := deployTransferServer(t, []string{"Completed"}, nil)
	out, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--descendants", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	target := plan["target"].(map[string]any)
	if target["name"] != "Development" || target["deployUrl"] != "https://dev.example.test/umbraco/backoffice/deploy/environment" {
		t.Fatalf("expected the configured target, got %+v", target)
	}
	item := plan["items"].([]any)[0].(map[string]any)
	if item["name"] != "Sector A" || item["descendants"] != float64(3) || item["includeDescendants"] != true {
		t.Fatalf("expected name and a descendant count of 3 (2 children + 1 grandchild), got %+v", item)
	}
	request := plan["request"].(map[string]any)
	body := request["body"].(map[string]any)
	if !strings.HasSuffix(request["path"].(string), "/deploy/instant") || body["targetUrl"] != target["deployUrl"] || body["ignoreDependencies"] != false {
		t.Fatalf("expected the instant-deploy request planned against the target, got %+v", request)
	}
	requestItem := body["items"].([]any)[0].(map[string]any)
	if requestItem["id"] != "aaaaaaaa-0000-4000-8000-000000000001" || requestItem["entityType"] != "document" || requestItem["culture"] != "*" || requestItem["includeDescendants"] != true {
		t.Fatalf("unexpected request item: %+v", requestItem)
	}
	for _, r := range *requests {
		if strings.HasPrefix(r, "POST /deploy") || strings.Contains(r, "/status/status") {
			t.Fatalf("dry-run must not send the transfer, saw %v", *requests)
		}
	}
}

func TestDeployTransferRequiresForceAndValidatesInput(t *testing.T) {
	deps, requests, _ := deployTransferServer(t, []string{"Completed"}, nil)
	root := buildRootWithCollections(t, deps)
	if _, err := execute(root, "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the force/dry-run gate, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--dry-run"); err == nil || !strings.Contains(err.Error(), "--node") {
		t.Fatalf("expected a missing-source error, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "not-a-guid", "--dry-run"); err == nil || !strings.Contains(err.Error(), "is not a GUID") {
		t.Fatalf("expected GUID validation, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-00000000dead", "--dry-run"); err == nil || !strings.Contains(err.Error(), "does not exist in this environment") {
		t.Fatalf("expected an unknown-id error from the name lookup, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--ignore-dependencies", "--dry-run"); err == nil || !strings.Contains(err.Error(), "allowDeployIgnoreDependencies=false") {
		t.Fatalf("expected the ignore-dependencies gate, got %v", err)
	}
	if len(*requests) == 0 {
		t.Fatalf("expected configuration lookups")
	}
}

func TestDeployTransferWaitsForSessionAndReportsCompletion(t *testing.T) {
	deps, requests, bodies := deployTransferServer(t, []string{"New", "Executing", "Completed"}, nil)
	out, stderr, err := executeWithErr(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--force", "--interval", "1ms")
	if err != nil {
		t.Fatalf("transfer failed: %v (%s)", err, out)
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(out), &result)
	if result["status"] != "Completed" || result["sessionId"] != "11111111-2222-4333-8444-555555555555" || result["percent"] != float64(100) {
		t.Fatalf("unexpected result: %s", out)
	}
	if !strings.Contains(stderr, "transfer session 11111111-2222-4333-8444-555555555555 started → Development") || !strings.Contains(stderr, "Executing 40%") || !strings.Contains(stderr, "Completed 100%") {
		t.Fatalf("expected progress on stderr, got %q", stderr)
	}
	sent := false
	for i, r := range *requests {
		if r == "POST /deploy/instant" {
			sent = true
			if (*bodies)[i]["enableLogging"] != true || (*bodies)[i]["targetUrl"] != "https://dev.example.test/umbraco/backoffice/deploy/environment" {
				t.Fatalf("unexpected deploy body: %+v", (*bodies)[i])
			}
		}
	}
	if !sent {
		t.Fatalf("expected the instant deploy to be sent, got %v", *requests)
	}
}

func TestDeployTransferFailureExits5WithLogAndTimeoutExits6(t *testing.T) {
	deps, _, _ := deployTransferServer(t, []string{"Executing", "Failed"}, nil)
	out, _, err := executeWithErr(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--force", "--interval", "1ms")
	if err == nil || batchExitCode(err) != 5 || !strings.Contains(err.Error(), "Failed: Dependency missing") {
		t.Fatalf("expected exit 5 with the server comment, got %v", err)
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(out), &result)
	if result["status"] != "Failed" || result["log"] != "line1\nline2" || result["exception"].(map[string]any)["Message"] != "boom" {
		t.Fatalf("expected the failure detail in the result, got %s", out)
	}

	slow, _, _ := deployTransferServer(t, []string{"Executing"}, nil)
	_, _, err = executeWithErr(buildRootWithCollections(t, slow), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--force", "--interval", "1ms", "--timeout", "20ms")
	if err == nil || batchExitCode(err) != 6 || !strings.Contains(err.Error(), "still Executing") {
		t.Fatalf("expected exit 6 on timeout, got %v", err)
	}
}

func TestDeployTransferQueueModeAndNoWait(t *testing.T) {
	deps, requests, bodies := deployTransferServer(t, []string{"Completed"}, nil)
	out, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--queue", "--force", "--wait=false")
	if err != nil {
		t.Fatalf("queue transfer failed: %v", err)
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(out), &result)
	if result["status"] != "started" || result["source"] != "queue" || result["sessionId"] == "" {
		t.Fatalf("unexpected result: %s", out)
	}
	found := false
	for i, r := range *requests {
		if r == "POST /deploy" {
			found = true
			if _, hasItems := (*bodies)[i]["items"]; hasItems {
				t.Fatalf("queue transfer must not carry items, got %+v", (*bodies)[i])
			}
		}
		if strings.Contains(r, "/status/status") {
			t.Fatalf("--wait=false must not poll, got %v", *requests)
		}
	}
	if !found {
		t.Fatalf("expected POST /deploy for the queue, got %v", *requests)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--queue", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--dry-run"); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected --queue and --node to be exclusive, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--queue", "--culture", "en-US", "--dry-run"); err == nil || !strings.Contains(err.Error(), "--culture applies to --node items") {
		t.Fatalf("expected node-only flags rejected in queue mode, got %v", err)
	}
}

func TestDeployQueueCommands(t *testing.T) {
	deps, requests, bodies := deployTransferServer(t, nil, nil)
	root := buildRootWithCollections(t, deps)
	out, err := execute(root, "deploy", "queue", "add", "aaaaaaaa-0000-4000-8000-00000000000a", "--descendants", "--type", "media")
	if err != nil || !strings.Contains(out, `"count": 1`) {
		t.Fatalf("queue add failed: %v %s", err, out)
	}
	added := (*bodies)[len(*bodies)-1]
	if added["entityType"] != "media" || added["includeDescendants"] != true || added["culture"] != "*" {
		t.Fatalf("unexpected add body: %+v", added)
	}
	before := len(*requests)
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "queue", "add", "aaaaaaaa-0000-4000-8000-00000000000a", "not-a-guid"); err == nil || !strings.Contains(err.Error(), "nothing was queued") {
		t.Fatalf("expected batch validation before any write, got %v", err)
	}
	if len(*requests) != before {
		t.Fatalf("expected no queue/add request for an invalid batch, got %v", (*requests)[before:])
	}
	out, err = execute(buildRootWithCollections(t, deps), "deploy", "queue", "list")
	if err != nil || !strings.Contains(out, `"total": 1`) || !strings.Contains(out, "Queued") {
		t.Fatalf("queue list failed: %v %s", err, out)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "queue", "remove", "AAAAAAAA-0000-4000-8000-00000000000A"); err != nil {
		t.Fatalf("queue remove failed: %v", err)
	}
	removed := (*bodies)[len(*bodies)-1]
	if removed["udi"] != "umb://document/aaaaaaaa00004000800000000000000a" || removed["culture"] != "*" {
		t.Fatalf("expected a GUID turned into a document UDI, got %+v", removed)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "deploy", "queue", "clear"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the clear gate, got %v", err)
	}
	if out, err := execute(buildRootWithCollections(t, deps), "deploy", "queue", "clear", "--force"); err != nil || !strings.Contains(out, `"cleared": true`) {
		t.Fatalf("queue clear failed: %v %s", err, out)
	}
	if (*requests)[len(*requests)-1] != "POST /queue/clear" {
		t.Fatalf("expected the clear route last, got %v", *requests)
	}
}

func TestDeployTransferExplainsMissingDeployPackage(t *testing.T) {
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	_, err := execute(buildRootWithCollections(t, deps), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "Umbraco Deploy management API is not available") {
		t.Fatalf("expected a missing-package explanation, got %v", err)
	}
}

func TestDeployTransferAcceptsDoubleEncodedClientConfiguration(t *testing.T) {
	// Field report: against a Cloud environment the dry-run failed with
	// "json: cannot unmarshal string into Go value of type map" while the
	// same command worked locally and 'deploy queue list' worked on both.
	// The client configuration came back as a JSON string holding the
	// object; it must be unwrapped, and a genuinely wrong shape must name
	// the request instead of leaking a bare decoder error.
	config := `{"clientConfiguration":{"currentWorkspace":"Development","allowDeployIgnoreDependencies":false,"target":{"name":"Live","type":"live","deployUrl":"https://live.example.test/umbraco/backoffice/deploy/environment"}}}`
	encoded, _ := json.Marshal(config)
	serve := func(configBody string) Dependencies {
		return endpointDeps(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
				return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
			case req.URL.Path == "/umbraco/deploy/management/api/v1/configuration/client":
				return endpointJSONResponse(http.StatusOK, configBody), nil
			case req.URL.Path == "/umbraco/deploy/management/api/v1/entity/name":
				return endpointJSONResponse(http.StatusOK, `"Page"`), nil
			case strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/"):
				return endpointJSONResponse(http.StatusOK, `{"id":"x"}`), nil
			}
			return endpointJSONResponse(http.StatusNotFound, `null`), nil
		})
	}
	out, err := execute(buildRootWithCollections(t, serve(string(encoded))), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run with a double-encoded configuration failed: %v", err)
	}
	if !strings.Contains(out, `"name": "Live"`) {
		t.Fatalf("expected the unwrapped target, got %s", out)
	}
	_, err = execute(buildRootWithCollections(t, serve(`["not","an","object"]`)), "deploy", "transfer", "--node", "aaaaaaaa-0000-4000-8000-000000000001", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "GET /configuration/client returned an array, not a JSON object") {
		t.Fatalf("expected a shape error naming the request, got %v", err)
	}
}
