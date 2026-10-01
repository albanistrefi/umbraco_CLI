package cmdkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func TestCreateCommandGeneratesIDAndEchoesIdentity(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"POST " + testMgmt + "/widget": fixed(201, ""),
	})
	spec := CreateSpec{Use: "create", Path: "/widget", ResultKeys: []string{"kind"}}
	_, _, err := runKit(t, CreateCommand(deps, spec))
	mustContain(t, err, "missing required option: --json")

	out, _, err := runKit(t, CreateCommand(deps, spec), "--json", `{"name":"W","kind":"k","ignored":1}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got := decodeObject(t, out)
	id, _ := got["id"].(string)
	if !IsUUIDLike(id) || got["name"] != "W" || got["kind"] != "k" || got["ignored"] != nil {
		t.Fatalf("expected a generated id plus identity keys, got %v", got)
	}
	if body := server.requests[0].Body; body["id"] != id {
		t.Fatalf("expected the POSTed body to carry the generated id, got %v", body)
	}
	_, _, err = runKit(t, CreateCommand(deps, spec), "--json", `{"name":`)
	mustContain(t, err, "invalid --json JSON")
}

func TestCreateCommandHooksRunInContractOrder(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"POST " + testMgmt + "/widget/create-and-go": fixed(200, `{"id":"server-id"}`),
	})
	var calls []string
	var label string
	spec := CreateSpec{
		Use:          "create",
		Path:         "/widget",
		PayloadUsage: "Widget payload",
		APIPrefix:    "",
		Validate: func() error {
			calls = append(calls, "validate")
			return nil
		},
		Normalize: func(body map[string]any) error {
			calls = append(calls, "normalize")
			body["normalized"] = true
			return nil
		},
		Base: func(ctx context.Context) (map[string]any, error) {
			calls = append(calls, "base")
			return map[string]any{"fromBase": true, "name": "base"}, nil
		},
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			cmd.Flags().StringVar(&label, "label", "", "Label")
			return func(body map[string]any) error {
				calls = append(calls, "flags")
				if label != "" {
					body["label"] = label
				}
				return nil
			}
		},
		RouteOverride: func() string {
			calls = append(calls, "route")
			return "/widget/create-and-go"
		},
	}
	cmd := CreateCommand(deps, spec)
	if usage := cmd.Flags().Lookup("json").Usage; usage != "Widget payload" {
		t.Fatalf("expected PayloadUsage to set --json help, got %q", usage)
	}
	out, _, err := runKit(t, cmd, "--json", `{"name":"mine"}`, "--label", "L")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"validate", "normalize", "base", "flags", "route"}) {
		t.Fatalf("unexpected hook order %v", calls)
	}
	body := server.requests[0].Body
	if body["name"] != "mine" || body["fromBase"] != true || body["normalized"] != true || body["label"] != "L" {
		t.Fatalf("expected the caller's JSON merged over Base plus flag values, got %v", body)
	}
	if got := decodeObject(t, out); got["id"] != "server-id" {
		t.Fatalf("expected the server result to pass through, got %v", got)
	}

	// Base makes --json optional.
	if _, _, err := runKit(t, CreateCommand(deps, spec)); err != nil {
		t.Fatalf("create from base only: %v", err)
	}

	failing := spec
	failing.Validate = func() error { return fmt.Errorf("bad flags") }
	_, _, err = runKit(t, CreateCommand(deps, failing))
	mustContain(t, err, "bad flags")
}

func TestCreateCommandPrintTemplateAndDryRun(t *testing.T) {
	server, deps := newKitServer(nil)
	spec := CreateSpec{Use: "create", Path: "/document", TemplateKey: "document.create"}
	out, _, err := runKit(t, CreateCommand(deps, spec), "--print-template")
	if err != nil {
		t.Fatalf("print-template: %v", err)
	}
	if len(decodeObject(t, out)) == 0 {
		t.Fatal("expected a non-empty template")
	}
	out, _, err = runKit(t, CreateCommand(deps, spec), "--json", `{"id":"fixed"}`, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	got := decodeObject(t, out)
	if got["dryRun"] != true || got["method"] != "POST" || got["path"] != testMgmt+"/document" {
		t.Fatalf("expected a dry-run plan, got %v", got)
	}
	if len(server.requests) != 0 {
		t.Fatalf("dry-run must not send, got %+v", server.requests)
	}
	if CreateCommand(deps, CreateSpec{Use: "create"}).Flags().Lookup("print-template") != nil {
		t.Fatal("--print-template must only exist when a TemplateKey is set")
	}
}

func TestUpdateCommandReplaceMergeAndBind(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/widget/w-1": fixed(200, `{"id":"w-1","name":"Old","keep":true,"readOnly":1}`),
		"PUT " + testMgmt + "/widget/w-1": fixed(204, ""),
	})
	spec := UpdateSpec{
		Use:             "update <id>",
		Path:            func(args []string) string { return api.JoinPath("/widget/%s", args[0]) },
		NormalizeMerged: StripFields("readOnly"),
		BindArgs: func(args []string, body map[string]any) error {
			if id, ok := body["id"].(string); ok && id != args[0] {
				return fmt.Errorf("body id %s does not match %s", id, args[0])
			}
			body["id"] = args[0]
			return nil
		},
	}
	_, _, err := runKit(t, UpdateCommand(deps, spec), "w-1")
	mustContain(t, err, "exactly one of --json")
	_, _, err = runKit(t, UpdateCommand(deps, spec), "w-1", "--json", `{}`, "--merge-json", `{}`)
	mustContain(t, err, "exactly one of --json")

	out, _, err := runKit(t, UpdateCommand(deps, spec), "w-1", "--merge-json", `{"name":"New"}`)
	if err != nil {
		t.Fatalf("merge update: %v", err)
	}
	if got := decodeObject(t, out); !reflect.DeepEqual(got, map[string]any{"updated": true}) {
		t.Fatalf("expected an empty 204 to read as updated, got %v", got)
	}
	put := server.requests[len(server.requests)-1]
	if put.Method != "PUT" || !reflect.DeepEqual(put.Body, map[string]any{"id": "w-1", "name": "New", "keep": true}) {
		t.Fatalf("expected merged body without response-only fields, got %+v", put)
	}

	if _, _, err := runKit(t, UpdateCommand(deps, spec), "w-1", "--json", `{"name":"Full","readOnly":2}`); err != nil {
		t.Fatalf("replace update: %v", err)
	}
	put = server.requests[len(server.requests)-1]
	if !reflect.DeepEqual(put.Body, map[string]any{"id": "w-1", "name": "Full"}) {
		t.Fatalf("expected --json replacement normalized and bound, got %v", put.Body)
	}

	_, _, err = runKit(t, UpdateCommand(deps, spec), "w-1", "--json", `{"id":"other"}`)
	mustContain(t, err, "does not match")

	rejecting := spec
	rejecting.Normalize = func(map[string]any) error { return fmt.Errorf("rejected input") }
	_, _, err = runKit(t, UpdateCommand(deps, rejecting), "w-1", "--merge-json", `{}`)
	mustContain(t, err, "rejected input")
	_, _, err = runKit(t, UpdateCommand(deps, rejecting), "w-1", "--json", `{}`)
	mustContain(t, err, "rejected input")
	_, _, err = runKit(t, UpdateCommand(deps, spec), "missing", "--merge-json", `{}`)
	if !IsAPIStatus(err, 404) {
		t.Fatalf("expected the merge fetch 404 to surface, got %v", err)
	}
}

func TestUpdateCommandBackupAndDryRun(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"GET " + testMgmt + "/widget/w-1": fixed(200, `{"id":"w-1","name":"Old"}`),
		"PUT " + testMgmt + "/widget/w-1": fixed(200, `{"id":"w-1","name":"New"}`),
	})
	spec := UpdateSpec{Use: "update <id>", Path: func(args []string) string { return api.JoinPath("/widget/%s", args[0]) }}
	backupPath := filepath.Join(t.TempDir(), "w.backup.json")
	out, _, err := runKit(t, UpdateCommand(deps, spec), "w-1", "--json", `{"name":"New"}`, "--backup="+backupPath)
	if err != nil {
		t.Fatalf("update --backup: %v", err)
	}
	got := decodeObject(t, out)
	if got["backup"] != backupPath || got["name"] != "New" {
		t.Fatalf("expected the result to carry the backup path, got %v", got)
	}
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	var envelope BackupEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Resource != "widget" || envelope.Entity["name"] != "Old" {
		t.Fatalf("expected the pre-change entity in the backup, got %+v (%v)", envelope, err)
	}

	before := len(server.requests)
	out, _, err = runKit(t, UpdateCommand(deps, spec), "w-1", "--json", `{"name":"New"}`, "--dry-run", "--backup="+backupPath)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got := decodeObject(t, out); got["dryRun"] != true || got["method"] != "PUT" {
		t.Fatalf("expected a PUT plan, got %v", got)
	}
	if len(server.requests) != before {
		t.Fatalf("dry-run must neither back up nor send, got %+v", server.requests[before:])
	}
}

func TestDeleteCommandIsGated(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"DELETE " + testMgmt + "/widget/w-1": fixed(204, ""),
	})
	spec := DeleteSpec{Use: "delete <id>", Short: "Delete", Path: func(args []string) string { return api.JoinPath("/widget/%s", args[0]) }}
	_, _, err := runKit(t, DeleteCommand(deps, spec), "w-1")
	mustContain(t, err, "umbraco widget delete permanently deletes; pass --force to confirm or --dry-run to rehearse")
	out, _, err := runKit(t, DeleteCommand(deps, spec), "w-1", "--dry-run")
	if err != nil || decodeObject(t, out)["method"] != "DELETE" {
		t.Fatalf("expected a DELETE plan, got %s (%v)", out, err)
	}
	out, _, err = runKit(t, DeleteCommand(deps, spec), "w-1", "--force")
	if err != nil {
		t.Fatalf("delete --force: %v", err)
	}
	if got := decodeObject(t, out); !reflect.DeepEqual(got, map[string]any{"deleted": true}) {
		t.Fatalf("expected {deleted:true}, got %v", got)
	}
	if len(server.requests) != 1 {
		t.Fatalf("expected exactly one DELETE, got %+v", server.requests)
	}
	_, _, err = runKit(t, DeleteCommand(deps, spec), "missing", "--force")
	if !IsAPIStatus(err, 404) {
		t.Fatalf("expected the 404 to surface, got %v", err)
	}
}

func TestTargetActionCommandFallsBackOnMethodNotAllowed(t *testing.T) {
	server, deps := newKitServer(map[string]func(kitRequest) (int, string){
		"PUT " + testMgmt + "/widget/w-1/move":  fixed(405, `{"title":"Method Not Allowed","status":405}`),
		"POST " + testMgmt + "/widget/w-1/move": fixed(204, ""),
		"PUT " + testMgmt + "/widget/bad/move":  fixed(400, `{"title":"Bad","status":400}`),
	})
	spec := TargetActionSpec{
		Use:  "move <id>",
		Verb: "moved",
		Candidates: func(args []string) []MutationCandidate {
			path := api.JoinPath("/widget/%s/move", args[0])
			return []MutationCandidate{{Method: "PUT", Path: path}, {Method: "POST", Path: path}}
		},
	}
	_, _, err := runKit(t, TargetActionCommand(deps, spec), "w-1")
	mustContain(t, err, "missing required option: --to")

	out, _, err := runKit(t, TargetActionCommand(deps, spec), "w-1", "--to", "p-2")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if got := decodeObject(t, out); !reflect.DeepEqual(got, map[string]any{"moved": true}) {
		t.Fatalf("expected {moved:true}, got %v", got)
	}
	if len(server.requests) != 2 || server.requests[1].Method != "POST" ||
		!reflect.DeepEqual(server.requests[1].Body, map[string]any{"target": map[string]any{"id": "p-2"}}) {
		t.Fatalf("expected PUT then POST with the --to envelope, got %+v", server.requests)
	}

	if _, _, err := runKit(t, TargetActionCommand(deps, spec), "w-1", "--json", `{"target":null}`); err != nil {
		t.Fatalf("move --json: %v", err)
	}
	if last := server.requests[len(server.requests)-1]; last.Body["target"] != nil {
		t.Fatalf("expected the raw --json body, got %v", last.Body)
	}

	before := len(server.requests)
	_, _, err = runKit(t, TargetActionCommand(deps, spec), "bad", "--to", "p")
	if !IsAPIStatus(err, 400) || len(server.requests) != before+1 {
		t.Fatalf("expected a non-retriable 400 to stop the fallback, got %v after %d requests", err, len(server.requests)-before)
	}
	if _, err := MutateWithFallback(context.Background(), deps.Client, nil, api.RequestOptions{}); err != nil {
		t.Fatalf("expected no candidates to be a no-op, got %v", err)
	}
}

func TestResolveUpdateBodyRejectsMalformedPatch(t *testing.T) {
	_, deps := newKitServer(nil)
	_, err := ResolveUpdateBody(context.Background(), deps.Client, "/x", "", "", `[1]`, nil, nil)
	mustContain(t, err, "--merge-json must be a JSON object")
	_, err = ResolveUpdateBody(context.Background(), deps.Client, "/x", "", `nope`, "", nil, nil)
	mustContain(t, err, "invalid --json JSON")
	failMerged := func(map[string]any) error { return fmt.Errorf("merged rejected") }
	_, err = ResolveUpdateBody(context.Background(), deps.Client, "/x", "", `{}`, "", nil, failMerged)
	mustContain(t, err, "merged rejected")
}
