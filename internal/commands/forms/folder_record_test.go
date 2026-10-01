package forms

import (
	"net/http"
	"strings"
	"testing"

	"umbraco-cli/internal/jsonvalue"
	"umbraco-cli/internal/uuid"
)

const formsTestRecordSetActions = `[
	{"id":"aaaa0010-bbbb-4ccc-8ddd-000000000010","alias":"approve","name":"Approve","needsConfirm":false},
	{"id":"aaaa0011-bbbb-4ccc-8ddd-000000000011","alias":"reject","name":"Reject","needsConfirm":false},
	{"id":"aaaa0012-bbbb-4ccc-8ddd-000000000012","alias":"delete","name":"Delete","needsConfirm":true}
]`

func TestFormsCreateFolderFillsFlagsAndReadsBack(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"POST /folder":                     {status: http.StatusCreated, body: ``, location: "https://example.test" + formsTestPrefix + "/folder/" + formsTestFolderID},
		"GET /folder/" + formsTestFolderID: {status: http.StatusOK, body: `{"id":"` + formsTestFolderID + `","name":"Campaigns","parentId":"` + formsTestOtherID + `"}`},
	})
	output, err := fake.run(t, "forms", "create-folder", "--name", "Campaigns", "--parent", formsTestOtherID, "--id", formsTestFolderID)
	if err != nil {
		t.Fatalf("forms create-folder failed: %v", err)
	}
	body := fake.sent(http.MethodPost)[0].body.(map[string]any)
	if body["name"] != "Campaigns" || body["parentId"] != formsTestOtherID || body["id"] != formsTestFolderID {
		t.Fatalf("unexpected folder body %+v", body)
	}
	result := decodeObject(t, output)
	if result["created"] != true || result["id"] != formsTestFolderID || result["parentId"] != formsTestOtherID {
		t.Fatalf("expected the read-back folder, got %+v", result)
	}

	output, err = fake.run(t, "forms", "create-folder", "--name", "Root level", "--dry-run")
	if err != nil {
		t.Fatalf("forms create-folder --dry-run failed: %v", err)
	}
	planned := decodeObject(t, output)["body"].(map[string]any)
	if value, ok := planned["parentId"]; !ok || value != nil || !uuid.Valid(jsonvalue.Text(planned["id"])) {
		t.Fatalf("expected a generated id and parentId null, got %+v", planned)
	}
	for _, args := range [][]string{
		{"forms", "create-folder"},
		{"forms", "create-folder", "--name", "X", "--parent", "not-a-guid"},
	} {
		if _, err := fake.run(t, args...); err == nil {
			t.Fatalf("expected %v to be refused", args)
		}
	}
	if len(fake.sent(http.MethodPost)) != 1 {
		t.Fatalf("expected no further POSTs, got %+v", fake.sent(http.MethodPost))
	}
}

func TestFormsUpdateFolderSendsOnlyName(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /folder/" + formsTestFolderID: {status: http.StatusOK, body: `{"id":"` + formsTestFolderID + `","name":"Old","parentId":null,"created":"2026-01-01T00:00:00Z"}`},
		"PUT /folder/" + formsTestFolderID: {status: http.StatusOK, body: ``},
	})
	if _, err := fake.run(t, "forms", "update-folder", formsTestFolderID, "--merge-json", `{"name":"New"}`); err != nil {
		t.Fatalf("forms update-folder failed: %v", err)
	}
	body := fake.sent(http.MethodPut)[0].body.(map[string]any)
	if len(body) != 1 || body["name"] != "New" {
		t.Fatalf("expected the UpdateFolderModel {name} only, got %+v", body)
	}
}

func TestFormsDeleteFolderChecksEmptiness(t *testing.T) {
	fullFolder := "aaaa0020-bbbb-4ccc-8ddd-000000000020"
	missing := "aaaa0021-bbbb-4ccc-8ddd-000000000021"
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /folder/" + formsTestFolderID + "/is-empty": {status: http.StatusOK, body: `true`},
		"DELETE /folder/" + formsTestFolderID:            {status: http.StatusOK, body: ``},
		"GET /folder/" + fullFolder + "/is-empty":        {status: http.StatusOK, body: `false`},
		"GET /folder/" + fullFolder:                      {status: http.StatusOK, body: `{"id":"` + fullFolder + `","name":"Full"}`},
		"GET /folder/" + missing + "/is-empty":           {status: http.StatusOK, body: `false`},
	})
	if _, err := fake.run(t, "forms", "delete-folder", formsTestFolderID); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the force gate, got %v", err)
	}
	if _, err := fake.run(t, "forms", "delete-folder", fullFolder, "--force"); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Fatalf("expected a non-empty folder to be refused, got %v", err)
	}
	if _, err := fake.run(t, "forms", "delete-folder", missing, "--force"); err == nil || !strings.Contains(err.Error(), "no Forms folder") {
		t.Fatalf("expected a missing folder to be reported as missing, got %v", err)
	}
	if len(fake.sent(http.MethodDelete)) != 0 {
		t.Fatalf("expected no DELETE for refused folders")
	}
	output, err := fake.run(t, "forms", "delete-folder", formsTestFolderID, "--force")
	if err != nil {
		t.Fatalf("forms delete-folder failed: %v", err)
	}
	if decodeObject(t, output)["deleted"] != true || len(fake.sent(http.MethodDelete)) != 1 {
		t.Fatalf("expected one DELETE and {deleted:true}, got %s", output)
	}
}

func TestFormsRecordActionResolvesAliasAndGatesDelete(t *testing.T) {
	recordID := "aaaa0030-bbbb-4ccc-8ddd-000000000030"
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /record-set-actions": {status: http.StatusOK, body: formsTestRecordSetActions},
		"POST /form/" + formsTestFormID + "/record/actions/aaaa0010-bbbb-4ccc-8ddd-000000000010/execute": {status: http.StatusOK, body: ``},
		"POST /form/" + formsTestFormID + "/record/actions/aaaa0012-bbbb-4ccc-8ddd-000000000012/execute": {status: http.StatusOK, body: ``},
	})
	output, err := fake.run(t, "forms", "record-action", formsTestFormID, "approve", "--record-ids", recordID)
	if err != nil {
		t.Fatalf("forms record-action approve failed: %v", err)
	}
	posts := fake.sent(http.MethodPost)
	keys, _ := posts[0].body.(map[string]any)["recordKeys"].([]any)
	if len(posts) != 1 || len(keys) != 1 || keys[0] != recordID {
		t.Fatalf("expected one execute with the record key, got %+v", posts)
	}
	result := decodeObject(t, output)
	if result["executed"] != true || result["action"] != "approve" {
		t.Fatalf("expected an executed summary without record contents, got %+v", result)
	}

	if _, err := fake.run(t, "forms", "record-action", formsTestFormID, "delete", "--record-ids", recordID); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected the delete action to be force-gated, got %v", err)
	}
	if _, err := fake.run(t, "forms", "record-action", formsTestFormID, "Delete", "--record-ids", recordID, "--force"); err != nil {
		t.Fatalf("forms record-action delete --force failed: %v", err)
	}
	if _, err := fake.run(t, "forms", "record-action", formsTestFormID, "archive", "--record-ids", recordID); err == nil || !strings.Contains(err.Error(), "approve, delete, reject") {
		t.Fatalf("expected an unknown action to list the available aliases, got %v", err)
	}
	if _, err := fake.run(t, "forms", "record-action", formsTestFormID, "approve", "--record-ids", "42"); err == nil || !strings.Contains(err.Error(), "uniqueIds") {
		t.Fatalf("expected numeric record ids to be refused, got %v", err)
	}
	if got := len(fake.sent(http.MethodPost)); got != 2 {
		t.Fatalf("expected exactly the approve and forced delete executes, got %d", got)
	}
}

func TestFormsRecordActionsList(t *testing.T) {
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /record-set-actions": {status: http.StatusOK, body: formsTestRecordSetActions},
	})
	output, err := fake.run(t, "forms", "record-actions", "--fields", "alias")
	if err != nil {
		t.Fatalf("forms record-actions failed: %v", err)
	}
	if !strings.Contains(output, `"alias": "approve"`) || strings.Contains(output, `"needsConfirm"`) {
		t.Fatalf("expected projected aliases, got %s", output)
	}
}

func TestFormsRecordUpdateAndRetry(t *testing.T) {
	recordID := "aaaa0030-bbbb-4ccc-8ddd-000000000030"
	fieldID := "aaaa0031-bbbb-4ccc-8ddd-000000000031"
	workflowID := "aaaa0032-bbbb-4ccc-8ddd-000000000032"
	fake := newFormsFake(map[string]formsFakeResponse{
		"PUT /form/" + formsTestFormID + "/record/" + recordID:                                         {status: http.StatusOK, body: ``},
		"POST /form/" + formsTestFormID + "/record/" + recordID + "/workflow/" + workflowID + "/retry": {status: http.StatusOK, body: ``},
	})
	output, err := fake.run(t, "forms", "record-update", formsTestFormID, recordID, "--json", `[{"fieldId":"`+fieldID+`","values":["corrected"]}]`)
	if err != nil {
		t.Fatalf("forms record-update failed: %v", err)
	}
	entries, _ := fake.sent(http.MethodPut)[0].body.([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["fieldId"] != fieldID {
		t.Fatalf("expected the field array as the PUT body, got %+v", fake.sent(http.MethodPut))
	}
	if decodeObject(t, output)["updated"] != true {
		t.Fatalf("expected {updated:true}, got %s", output)
	}
	for _, payload := range []string{`{"fieldId":"x"}`, `[]`, `[{"values":["x"]}]`, `[{"fieldId":"` + fieldID + `","values":"x"}]`} {
		if _, err := fake.run(t, "forms", "record-update", formsTestFormID, recordID, "--json", payload); err == nil {
			t.Fatalf("expected payload %s to be refused", payload)
		}
	}
	if len(fake.sent(http.MethodPut)) != 1 {
		t.Fatalf("expected no PUT for refused payloads")
	}

	output, err = fake.run(t, "forms", "record-workflow-retry", formsTestFormID, recordID, workflowID)
	if err != nil {
		t.Fatalf("forms record-workflow-retry failed: %v", err)
	}
	if decodeObject(t, output)["retried"] != true {
		t.Fatalf("expected {retried:true}, got %s", output)
	}
}

func TestFormsPrevalueSourceCRUD(t *testing.T) {
	sourceID := "aaaa0040-bbbb-4ccc-8ddd-000000000040"
	typeID := "aaaa0041-bbbb-4ccc-8ddd-000000000041"
	fake := newFormsFake(map[string]formsFakeResponse{
		"GET /prevalue-source/scaffold":       {status: http.StatusOK, body: `{"id":"` + sourceID + `","unique":"` + sourceID + `","name":"","fieldPreValueSourceTypeId":"00000000-0000-0000-0000-000000000000","settings":{},"cachePrevaluesFor":"00:00:00"}`},
		"POST /prevalue-source":               {status: http.StatusCreated, body: ``, location: "https://example.test" + formsTestPrefix + "/prevalue-source/" + sourceID},
		"GET /prevalue-source/" + sourceID:    {status: http.StatusOK, body: `{"id":"` + sourceID + `","name":"Countries","fieldPreValueSourceTypeId":"` + typeID + `","settings":{"TextFile":"countries.txt"}}`},
		"PUT /prevalue-source/" + sourceID:    {status: http.StatusOK, body: ``},
		"DELETE /prevalue-source/" + sourceID: {status: http.StatusOK, body: ``},
		"GET /prevalue-source":                {status: http.StatusOK, body: `{"total":1,"items":[{"id":"` + sourceID + `","name":"Countries"}]}`},
		"GET /prevalue-source-type":           {status: http.StatusOK, body: `[{"id":"` + typeID + `","alias":"getValuesFromTextFile"}]`},
	})
	if _, err := fake.run(t, "forms", "prevalue-source", "create", "--json", `{"name":"Countries"}`); err == nil || !strings.Contains(err.Error(), "fieldPreValueSourceTypeId") {
		t.Fatalf("expected a missing type id to be refused, got %v", err)
	}
	if _, err := fake.run(t, "forms", "prevalue-source", "create", "--json", `{"name":"Countries","fieldPreValueSourceTypeId":"`+typeID+`","settings":{"TextFile":"countries.txt"}}`); err != nil {
		t.Fatalf("prevalue-source create failed: %v", err)
	}
	created := fake.sent(http.MethodPost)[0].body.(map[string]any)
	if created["id"] != sourceID || created["fieldPreValueSourceTypeId"] != typeID || created["cachePrevaluesFor"] != "00:00:00" {
		t.Fatalf("expected the payload merged onto the scaffold, got %+v", created)
	}

	if _, err := fake.run(t, "forms", "prevalue-source", "update", sourceID, "--merge-json", `{"name":"Countries (EU)"}`); err != nil {
		t.Fatalf("prevalue-source update failed: %v", err)
	}
	updated := fake.sent(http.MethodPut)[0].body.(map[string]any)
	if updated["id"] != sourceID || updated["name"] != "Countries (EU)" || updated["settings"].(map[string]any)["TextFile"] != "countries.txt" {
		t.Fatalf("expected a merged update keeping settings, got %+v", updated)
	}

	if _, err := fake.run(t, "forms", "prevalue-source", "delete", sourceID); err == nil {
		t.Fatalf("expected the delete to be force-gated")
	}
	if _, err := fake.run(t, "forms", "prevalue-source", "delete", sourceID, "--force"); err != nil {
		t.Fatalf("prevalue-source delete --force failed: %v", err)
	}

	output, err := fake.run(t, "forms", "prevalue-source", "list", "--fields", "id")
	if err != nil || !strings.Contains(output, sourceID) {
		t.Fatalf("prevalue-source list failed: %v %s", err, output)
	}
	output, err = fake.run(t, "forms", "prevalue-source", "types", "--fields", "alias")
	if err != nil || !strings.Contains(output, "getValuesFromTextFile") {
		t.Fatalf("prevalue-source types failed: %v %s", err, output)
	}
	output, err = fake.run(t, "forms", "prevalue-source", "get", sourceID, "--fields", "name")
	if err != nil || !strings.Contains(output, "Countries") {
		t.Fatalf("prevalue-source get failed: %v %s", err, output)
	}
}
