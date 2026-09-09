package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mediaFileTestItem = `{"id":"m-1","mediaType":{"id":"mt-1"},"variants":[{"culture":null,"segment":null,"name":"Logo"}],"values":[` +
	`{"alias":"umbracoFile","culture":null,"segment":null,"editorAlias":"Umbraco.UploadField","value":{"src":"/media/abc/old.svg"}},` +
	`{"alias":"umbracoExtension","culture":null,"segment":null,"editorAlias":"Umbraco.Label","value":"svg"}]}`

const mediaFileTestItemAfter = `{"id":"m-1","mediaType":{"id":"mt-1"},"variants":[{"culture":null,"segment":null,"name":"Logo"}],"values":[` +
	`{"alias":"umbracoFile","culture":null,"segment":null,"editorAlias":"Umbraco.UploadField","value":{"src":"/media/abc/new.svg"}},` +
	`{"alias":"umbracoExtension","culture":null,"segment":null,"editorAlias":"Umbraco.Label","value":"svg"}]}`

func writeTestSVG(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMediaReplaceFileUploadsMergesAndVerifies(t *testing.T) {
	filePath := writeTestSVG(t, "new.svg", "<svg>new</svg>")
	gets := 0
	var putBody map[string]any
	var uploadedID string

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodGet:
			gets++
			if gets == 1 {
				return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
			}
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItemAfter), nil
		case req.URL.Path == "/umbraco/management/api/v1/temporary-file":
			_ = req.ParseMultipartForm(1 << 20)
			uploadedID = req.FormValue("id")
			return datatypeJSONResponse(http.StatusCreated, ``), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodPut:
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &putBody)
			return datatypeJSONResponse(http.StatusOK, ``), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "media", "replace-file", "m-1", filePath)
	if err != nil {
		t.Fatalf("replace-file failed: %v", err)
	}
	values := putBody["values"].([]any)
	if len(values) != 2 {
		t.Fatalf("expected both values preserved in PUT body, got %d", len(values))
	}
	first := values[0].(map[string]any)
	if first["alias"] != "umbracoFile" || first["value"].(map[string]any)["temporaryFileId"] != uploadedID || uploadedID == "" {
		t.Fatalf("expected umbracoFile to carry the uploaded temporaryFileId, got %#v (uploaded %q)", first, uploadedID)
	}
	if putBody["variants"].([]any)[0].(map[string]any)["name"] != "Logo" {
		t.Fatalf("expected variants preserved, got %#v", putBody["variants"])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["verified"] != true || payload["changed"] != true || payload["after"].(map[string]any)["src"] != "/media/abc/new.svg" {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestMediaReplaceFileFailsWhenServerEmptiesItemAndPointsAtBackup(t *testing.T) {
	filePath := writeTestSVG(t, "new.svg", "<svg>new</svg>")
	backupPath := filepath.Join(t.TempDir(), "bk", "logo.backup.json")
	gets := 0

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodGet:
			gets++
			if gets == 1 {
				return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
			}
			return datatypeJSONResponse(http.StatusOK, `{"id":"m-1","variants":[{"culture":null,"segment":null,"name":"Logo"}],"values":[]}`), nil
		case req.URL.Path == "/media/abc/old.svg":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/svg+xml"}}, Body: io.NopCloser(strings.NewReader("<svg>old</svg>"))}, nil
		case req.URL.Path == "/umbraco/management/api/v1/temporary-file":
			return datatypeJSONResponse(http.StatusCreated, ``), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodPut:
			return datatypeJSONResponse(http.StatusOK, ``), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	_, err := execute(buildRootWithCollections(t, deps), "media", "replace-file", "m-1", filePath, "--backup="+backupPath)
	if err == nil {
		t.Fatalf("expected replace-file to fail when the item is emptied")
	}
	if !strings.Contains(err.Error(), "no values") || !strings.Contains(err.Error(), "restore-backup "+backupPath) {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		t.Fatalf("expected backup to be written: %v", readErr)
	}
	var envelope backupEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Resource != "media" || envelope.ID != "m-1" || envelope.File == nil || envelope.File.Path != "logo.backup.files/old.svg" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	binary, err := os.ReadFile(filepath.Join(filepath.Dir(backupPath), filepath.FromSlash(envelope.File.Path)))
	if err != nil || string(binary) != "<svg>old</svg>" {
		t.Fatalf("expected the old binary saved next to the envelope, got %q (%v)", binary, err)
	}
}

func TestMediaRestoreBackupReuploadsSavedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logo.backup.files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logo.backup.files", "old.svg"), []byte("<svg>old</svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	envelope := `{"resource":"media","id":"m-1","path":"/media/m-1","savedAt":"2026-09-09T00:00:00Z","entity":` + mediaFileTestItem + `,"file":{"property":"umbracoFile","src":"/media/abc/old.svg","path":"logo.backup.files/old.svg","bytes":14}}`
	backupPath := filepath.Join(dir, "logo.backup.json")
	if err := os.WriteFile(backupPath, []byte(envelope), 0o644); err != nil {
		t.Fatal(err)
	}
	var uploadedID, uploadedContent string
	var putBody map[string]any

	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/temporary-file":
			_ = req.ParseMultipartForm(1 << 20)
			uploadedID = req.FormValue("id")
			file, _, _ := req.FormFile("file")
			content, _ := io.ReadAll(file)
			uploadedContent = string(content)
			return datatypeJSONResponse(http.StatusCreated, ``), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodPut:
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &putBody)
			return datatypeJSONResponse(http.StatusOK, ``), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodGet:
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})

	output, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", backupPath)
	if err != nil {
		t.Fatalf("restore-backup failed: %v", err)
	}
	if uploadedContent != "<svg>old</svg>" {
		t.Fatalf("expected saved binary re-uploaded, got %q", uploadedContent)
	}
	first := putBody["values"].([]any)[0].(map[string]any)
	if first["value"].(map[string]any)["temporaryFileId"] != uploadedID {
		t.Fatalf("expected PUT to reference the re-uploaded temp file, got %#v", first)
	}
	if !strings.Contains(output, `"restored": true`) {
		t.Fatalf("unexpected output: %s", output)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", filepath.Join(dir, "missing.json")); err == nil {
		t.Fatalf("expected missing backup file to fail")
	}
}

func TestUpdateBackupFlagWritesEntityBeforePut(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "doc.json")
	order := []string{}
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodGet:
			order = append(order, "get")
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-1" && req.Method == http.MethodPut:
			order = append(order, "put")
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	output, err := execute(buildRootWithCollections(t, deps), "media", "update", "m-1", "--json", `{"variants":[{"name":"X"}],"values":[]}`, "--backup="+backupPath)
	if err != nil {
		t.Fatalf("update --backup failed: %v", err)
	}
	if strings.Join(order, ",") != "get,put" {
		t.Fatalf("expected backup GET before PUT, got %v", order)
	}
	if !strings.Contains(output, `"backup": "`+backupPath+`"`) {
		t.Fatalf("expected backup path in output: %s", output)
	}
	envelope, err := readBackup(backupPath, "media")
	if err != nil || envelope.ID != "m-1" || envelope.File != nil {
		t.Fatalf("unexpected envelope %+v (%v)", envelope, err)
	}
}

func TestMediaRestoreBackupRefusesMetadataOnlyWhenFileIsGone(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "meta.json")
	envelope := `{"resource":"media","id":"m-1","path":"/media/m-1","savedAt":"2026-09-09T00:00:00Z","entity":` + mediaFileTestItem + `}`
	if err := os.WriteFile(backupPath, []byte(envelope), 0o644); err != nil {
		t.Fatal(err)
	}
	puts := 0
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.Method == http.MethodPut:
			puts++
			return datatypeJSONResponse(http.StatusOK, ``), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", backupPath)
	if err == nil || !strings.Contains(err.Error(), "refusing to restore") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if puts != 0 {
		t.Fatalf("expected no PUT when the file is gone, got %d", puts)
	}
}

const mediaFileTestVariantItem = `{"id":"m-3","mediaType":{"id":"mt-1"},"variants":[{"culture":"en-US","segment":null,"name":"Logo"},{"culture":"da-DK","segment":null,"name":"Logo"}],"values":[` +
	`{"alias":"umbracoFile","culture":"en-US","segment":null,"editorAlias":"Umbraco.UploadField","value":{"src":"/media/abc/en.svg"}},` +
	`{"alias":"umbracoFile","culture":"da-DK","segment":null,"editorAlias":"Umbraco.UploadField","value":{"src":"/media/abc/da.svg"}}]}`

func TestMediaReplaceFileRequiresCultureForVariantsAndCarriesIt(t *testing.T) {
	filePath := writeTestSVG(t, "new.svg", "<svg>new</svg>")
	var putBody map[string]any
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-3" && req.Method == http.MethodGet:
			return datatypeJSONResponse(http.StatusOK, mediaFileTestVariantItem), nil
		case req.URL.Path == "/umbraco/management/api/v1/temporary-file":
			return datatypeJSONResponse(http.StatusCreated, ``), nil
		case req.URL.Path == "/umbraco/management/api/v1/media/m-3" && req.Method == http.MethodPut:
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &putBody)
			return datatypeJSONResponse(http.StatusOK, ``), nil
		default:
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "media", "replace-file", "m-3", filePath)
	if err == nil || !strings.Contains(err.Error(), "--culture") || !strings.Contains(err.Error(), "en-US, da-DK") {
		t.Fatalf("expected culture selection error, got %v", err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "replace-file", "m-3", filePath, "--culture", "da-DK"); err != nil {
		t.Fatalf("replace-file with --culture failed: %v", err)
	}
	values := putBody["values"].([]any)
	if len(values) != 2 {
		t.Fatalf("expected the da-DK entry replaced, not appended, got %d entries", len(values))
	}
	da := values[1].(map[string]any)
	if da["culture"] != "da-DK" || da["value"].(map[string]any)["temporaryFileId"] == nil {
		t.Fatalf("expected da-DK entry to carry the temp file, got %#v", da)
	}
	if values[0].(map[string]any)["value"].(map[string]any)["src"] != "/media/abc/en.svg" {
		t.Fatalf("expected en-US entry untouched, got %#v", values[0])
	}
}

func TestMediaRestoreBackupRejectsCraftedPathsAndEndpoints(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/umbraco/management/api/v1/security/back-office/token" {
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		requests++
		return datatypeJSONResponse(http.StatusOK, ``), nil
	})

	traversal := filepath.Join(dir, "t.json")
	if err := os.WriteFile(traversal, []byte(`{"resource":"media","id":"m-1","entity":`+mediaFileTestItem+`,"file":{"property":"umbracoFile","src":"/media/abc/old.svg","path":"../secret.txt","bytes":7}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", traversal); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected traversal refusal, got %v", err)
	}

	wrongPath := filepath.Join(dir, "p.json")
	if err := os.WriteFile(wrongPath, []byte(`{"resource":"media","id":"m-1","path":"/user-group/admin","entity":`+mediaFileTestItem+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", wrongPath); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected endpoint mismatch refusal, got %v", err)
	}

	badID := filepath.Join(dir, "i.json")
	if err := os.WriteFile(badID, []byte(`{"resource":"media","id":"../user-group/admin","entity":`+mediaFileTestItem+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", badID); err == nil || !strings.Contains(err.Error(), "invalid id") {
		t.Fatalf("expected invalid id refusal, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no API writes for rejected envelopes, got %d", requests)
	}
}

func TestBackupAutoNamesAreUniqueAndNeverOverwrite(t *testing.T) {
	a := resolveBackupPath("", "media", "m-1")
	b := resolveBackupPath("", "media", "m-1")
	if a == b {
		t.Fatalf("expected distinct auto names, got %s twice", a)
	}
	target := filepath.Join(t.TempDir(), "x.json")
	if _, err := writeBackup(target, "media", "m-1", "/media/m-1", map[string]any{"values": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := writeBackup(target, "media", "m-1", "/media/m-1", map[string]any{"values": []any{}}, nil); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected exclusive create to refuse, got %v", err)
	}
}

func TestRenameVariantsOnlyTouchesSelectedVariant(t *testing.T) {
	var item map[string]any
	_ = json.Unmarshal([]byte(mediaFileTestVariantItem), &item)
	selected, err := selectMediaFileValue(item, "umbracoFile", "da-DK", "")
	if err != nil {
		t.Fatal(err)
	}
	renamed := renameVariants(item, "Nyt logo", selected)
	if renamed[0].(map[string]any)["name"] != "Logo" || renamed[1].(map[string]any)["name"] != "Nyt logo" {
		t.Fatalf("expected only da-DK renamed, got %#v", renamed)
	}
}

func TestBackupBinaryNameIsSanitizedAndEnvelopeReservedFirst(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.json")
	bin := &backupBinary{Property: "umbracoFile", Src: `/media/abc/..\evil.txt`, Content: []byte("a")}
	if _, err := writeBackup(target, "media", "m-1", "/media/m-1", map[string]any{}, bin); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "x.files"))
	if len(entries) != 1 || strings.ContainsAny(entries[0].Name(), `\/`) || strings.Contains(entries[0].Name(), "..") {
		t.Fatalf("expected a sanitized single file, got %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatalf("binary escaped the .files directory")
	}
	// Re-using the envelope path must fail before the old binary is touched.
	before, _ := os.ReadFile(filepath.Join(dir, "x.files", entries[0].Name()))
	if _, err := writeBackup(target, "media", "m-1", "/media/m-1", map[string]any{}, &backupBinary{Property: "umbracoFile", Src: bin.Src, Content: []byte("zz")}); err == nil {
		t.Fatalf("expected reuse to fail")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "x.files", entries[0].Name()))
	if string(before) != string(after) {
		t.Fatalf("old binary was overwritten: %q → %q", before, after)
	}
}

func TestMetadataOnlyRestoreChecksEveryFileReference(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "meta.json")
	envelope := `{"resource":"media","id":"m-3","path":"/media/m-3","entity":` + mediaFileTestVariantItem + `}`
	if err := os.WriteFile(backupPath, []byte(envelope), 0o644); err != nil {
		t.Fatal(err)
	}
	puts := 0
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case "/media/abc/en.svg":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<svg/>"))}, nil
		default:
			if req.Method == http.MethodPut {
				puts++
			}
			return datatypeJSONResponse(http.StatusNotFound, `null`), nil
		}
	})
	_, err := execute(buildRootWithCollections(t, deps), "media", "restore-backup", backupPath)
	if err == nil || !strings.Contains(err.Error(), "da.svg") || puts != 0 {
		t.Fatalf("expected refusal naming the dead da-DK file with no PUT, got %v (puts=%d)", err, puts)
	}
}

func TestUpdateBackupPathSurvivesNonEmptyPutBody(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "b.json")
	deps := datatypeDeps(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/security/back-office/token":
			return datatypeJSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		case req.Method == http.MethodGet:
			return datatypeJSONResponse(http.StatusOK, mediaFileTestItem), nil
		default:
			return datatypeJSONResponse(http.StatusOK, `{"id":"m-1"}`), nil
		}
	})
	output, err := execute(buildRootWithCollections(t, deps), "media", "update", "m-1", "--json", `{"variants":[{"name":"X"}],"values":[]}`, "--backup="+backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"backup": "`+backupPath+`"`) || !strings.Contains(output, `"id": "m-1"`) {
		t.Fatalf("expected backup path alongside the server body, got %s", output)
	}
}
