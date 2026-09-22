package commands

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func batchExitCode(err error) int {
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

func documentBatchDeps(t *testing.T, puts *[]string, failPublishFor string) Dependencies {
	t.Helper()
	return endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/"):
			rest := strings.TrimPrefix(req.URL.Path, "/umbraco/management/api/v1/document/")
			id := strings.SplitN(rest, "/", 2)[0]
			if id == "missing" {
				return endpointJSONResponse(http.StatusNotFound, `{"title":"Not found","status":404}`), nil
			}
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{"id":"`+id+`","variants":[{"culture":null,"segment":null,"name":"Page `+id+`"}],"values":[{"alias":"title","culture":null,"segment":null,"value":"old"}]}`), nil
			}
			if req.Method == http.MethodPut {
				*puts = append(*puts, req.URL.Path)
				if strings.HasSuffix(rest, "/update-and-publish") {
					return endpointJSONResponse(http.StatusNotFound, `null`), nil // older server: two-step path
				}
				if strings.HasSuffix(rest, "/publish") && id == failPublishFor {
					return endpointJSONResponse(http.StatusBadRequest, `{"title":"Publish failed","status":400}`), nil
				}
				return endpointNoContent(), nil
			}
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
}

func TestDocumentPublishIDsRunsPerDocumentAndExits4OnFailure(t *testing.T) {
	var puts []string
	deps := documentBatchDeps(t, &puts, "b")

	if _, err := execute(buildRootWithCollections(t, deps), "document", "publish", "--ids", "a,b"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the force/dry-run gate on a multi-document publish, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "document", "publish", "a", "--ids", "b", "--force"); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected positional and --ids to be exclusive, got %v", err)
	}

	out, err := execute(buildRootWithCollections(t, deps), "document", "publish", "--ids", "a,b,missing", "--force")
	if err == nil || batchExitCode(err) != 4 || !strings.Contains(err.Error(), "2 of 3 documents failed") {
		t.Fatalf("expected exit 4 with a failure summary, got %v", err)
	}
	var result documentBatchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode: %v (%s)", err, out)
	}
	if result.Total != 3 || result.Published != 1 || result.Failed != 2 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	rows := map[string]documentBatchRow{}
	for _, row := range result.Items {
		rows[row.ID] = row
	}
	if rows["a"].Publish != "published" || rows["a"].Name != "Page a" || rows["a"].Error != "" {
		t.Fatalf("expected a published, got %+v", rows["a"])
	}
	if rows["b"].Publish != "failed" || !strings.Contains(rows["b"].Error, "400") {
		t.Fatalf("expected b failed with the API error, got %+v", rows["b"])
	}
	if rows["missing"].Publish != "failed" || !strings.Contains(rows["missing"].Error, "404") {
		t.Fatalf("expected the unknown id reported on its row, got %+v", rows["missing"])
	}
	if len(puts) != 2 { // a and b published; missing never reached the PUT
		t.Fatalf("expected one publish PUT per existing document, got %v", puts)
	}
}

func TestDocumentUpdateIDsMergesPerDocumentAndPublishes(t *testing.T) {
	var puts []string
	deps := documentBatchDeps(t, &puts, "")
	dir := t.TempDir()
	out, err := execute(buildRootWithCollections(t, deps), "document", "update", "--ids", "a,b", "--merge-json", `{"values":[{"alias":"title","value":"new"}]}`, "--save-and-publish", "--backup="+dir, "--force")
	if err != nil {
		t.Fatalf("document update --ids failed: %v (%s)", err, out)
	}
	var result documentBatchResult
	_ = json.Unmarshal([]byte(out), &result)
	if result.Updated != 2 || result.Published != 2 || result.Failed != 0 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	for _, row := range result.Items {
		if row.Update != "updated" || row.Publish != "published" || row.Backup == "" || !strings.HasPrefix(row.Backup, dir) {
			t.Fatalf("unexpected row: %+v", row)
		}
		if _, err := os.Stat(row.Backup); err != nil {
			t.Fatalf("expected the backup file to exist: %v", err)
		}
	}
	// atomic attempt (404) → update → publish, per document.
	want := []string{"/umbraco/management/api/v1/document/a/update-and-publish", "/umbraco/management/api/v1/document/a", "/umbraco/management/api/v1/document/a/publish", "/umbraco/management/api/v1/document/b/update-and-publish", "/umbraco/management/api/v1/document/b", "/umbraco/management/api/v1/document/b/publish"}
	if strings.Join(puts, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected request sequence: %v", puts)
	}

	// An unchanged merge is a skip, not a write.
	puts = nil
	out, err = execute(buildRootWithCollections(t, deps), "document", "update", "--ids", "a", "--merge-json", `{"values":[{"alias":"title","value":"old"}]}`, "--force")
	if err != nil || !strings.Contains(out, `"update": "skipped"`) || len(puts) != 0 {
		t.Fatalf("expected a skip without a PUT, got err=%v puts=%v out=%s", err, puts, out)
	}
}

// The per-document merge in the batch path runs through the same
// mergeAliasPayload as the single-document one, so the variant regression
// has to be pinned here too: renaming one culture across several documents
// must not drop the others from any of them.
func TestDocumentUpdateIDsMergesVariantsPerCulture(t *testing.T) {
	bodies := map[string]map[string]any{}
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/"):
			rest := strings.TrimPrefix(req.URL.Path, "/umbraco/management/api/v1/document/")
			id := strings.SplitN(rest, "/", 2)[0]
			if req.Method == http.MethodGet {
				return endpointJSONResponse(http.StatusOK, `{"id":"`+id+`","variants":[{"culture":"en-US","segment":null,"name":"English `+id+`"},{"culture":"da-DK","segment":null,"name":"Danish `+id+`"}],"values":[]}`), nil
			}
			if req.Method == http.MethodPut {
				if rest == id {
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatalf("failed to decode merged payload for %s: %v", id, err)
					}
					bodies[id] = body
				}
				return endpointNoContent(), nil
			}
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})

	if _, err := execute(
		buildRootWithCollections(t, deps),
		"document", "update", "--ids", "a,b",
		"--merge-json", `{"variants":[{"culture":"da-DK","name":"Renamed"}]}`,
		"--force",
	); err != nil {
		t.Fatalf("document update --ids --merge-json failed: %v", err)
	}

	for _, id := range []string{"a", "b"} {
		variants, ok := bodies[id]["variants"].([]any)
		if !ok || len(variants) != 2 {
			t.Fatalf("expected both variants on document %s, got %+v", id, bodies[id]["variants"])
		}
		byCulture := map[string]string{}
		for _, variant := range variants {
			object, _ := variant.(map[string]any)
			culture, _ := object["culture"].(string)
			name, _ := object["name"].(string)
			byCulture[culture] = name
		}
		if byCulture["da-DK"] != "Renamed" || byCulture["en-US"] != "English "+id {
			t.Fatalf("expected only the Danish variant of %s renamed, got %+v", id, byCulture)
		}
	}
}

func TestDocumentUpdateIDsDryRunPlansFirstThreeAndCountsTheRest(t *testing.T) {
	var puts []string
	gets := 0
	base := documentBatchDeps(t, &puts, "")
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") {
			gets++
		}
		return base.HTTPClient.Transport.RoundTrip(req)
	})
	file := t.TempDir() + "/ids.txt"
	_ = os.WriteFile(file, []byte("a\nb\nc\nd\ne\n"), 0o600)
	out, err := execute(buildRootWithCollections(t, deps), "document", "update", "--from-file", file, "--property", "title", "--value", "x", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var result documentBatchResult
	_ = json.Unmarshal([]byte(out), &result)
	if !result.DryRun || result.Total != 5 || result.Planned != 5 || result.Updated != 0 || len(puts) != 0 {
		t.Fatalf("expected a plan-only run over 5 documents, got %+v puts=%v", result, puts)
	}
	if gets != documentBatchPlanWindow {
		t.Fatalf("expected reads for the first %d documents only, got %d", documentBatchPlanWindow, gets)
	}
	if result.Items[0].Plan == nil || result.Items[0].Update != "planned" || result.Items[4].Plan != nil || result.Items[4].Update != "planned" {
		t.Fatalf("expected plans on the first three rows and counted rows after, got %+v", result.Items)
	}
}

func TestDocumentPublishIDsPassesFullJSONBodyAndKeepsAuthExitCode(t *testing.T) {
	var bodies []string
	deps := endpointDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return endpointJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case strings.HasSuffix(req.URL.Path, "/publish") && req.Method == http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			bodies = append(bodies, string(body))
			return endpointNoContent(), nil
		case strings.HasPrefix(req.URL.Path, "/umbraco/management/api/v1/document/") && req.Method == http.MethodGet:
			return endpointJSONResponse(http.StatusOK, `{"id":"x","variants":[{"name":"P"}],"values":[]}`), nil
		}
		return endpointJSONResponse(http.StatusNotFound, `null`), nil
	})
	payload := `{"publishSchedules":[{"culture":"en-US"},{"culture":"da-DK"}]}`
	if _, err := execute(buildRootWithCollections(t, deps), "document", "publish", "--ids", "a,b", "--json", payload, "--force"); err != nil {
		t.Fatalf("publish --ids --json failed: %v", err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], `"da-DK"`) || !strings.Contains(bodies[1], `"da-DK"`) {
		t.Fatalf("expected the --json body sent to every document, got %v", bodies)
	}

	// An auth failure keeps exit 3 instead of being flattened to 4.
	unauthorized := endpointDeps(func(req *http.Request) (*http.Response, error) {
		return endpointJSONResponse(http.StatusUnauthorized, `{"error":"invalid_client"}`), nil
	})
	_, err := execute(buildRootWithCollections(t, unauthorized), "document", "publish", "--ids", "a,b", "--force")
	if err == nil || batchExitCode(err) != 3 {
		t.Fatalf("expected the auth exit code 3 to survive the batch, got %v (code %d)", err, batchExitCode(err))
	}
}
