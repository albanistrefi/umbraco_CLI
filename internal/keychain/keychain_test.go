package keychain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type call struct {
	args  []string
	stdin string
}

func fakeRunner(calls *[]call, out string, code int) Runner {
	return func(_ context.Context, args []string, stdin string) (string, int, error) {
		*calls = append(*calls, call{args: args, stdin: stdin})
		return out, code, nil
	}
}

func TestSystemIsUnavailableInTests(t *testing.T) {
	if _, ok := System().(Unavailable); !ok {
		t.Fatalf("a test binary must never get the real keychain")
	}
}

func TestSetSendsTheSecretOnStdinNotInArguments(t *testing.T) {
	var calls []call
	store := &SecurityStore{Run: fakeRunner(&calls, "", 0)}
	if err := store.Set("umbraco-cli access token", "abc123", "umbraco-cli access token", "c2VjcmV0LnRva2Vu"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(calls) != 1 || strings.Join(calls[0].args, " ") != "-i" {
		t.Fatalf("expected one 'security -i', got %+v", calls)
	}
	want := `add-generic-password -U -s "umbraco-cli access token" -a "abc123" -l "umbraco-cli access token" -w "c2VjcmV0LnRva2Vu"` + "\n"
	if calls[0].stdin != want {
		t.Fatalf("stdin:\n got %q\nwant %q", calls[0].stdin, want)
	}
	for _, arg := range calls[0].args {
		if strings.Contains(arg, "c2VjcmV0LnRva2Vu") {
			t.Fatalf("the secret must not be an argument: %v", calls[0].args)
		}
	}
}

func TestSetRefusesValuesThatCouldEscapeTheQuotedWord(t *testing.T) {
	var calls []call
	store := &SecurityStore{Run: fakeRunner(&calls, "", 0)}
	for _, secret := range []string{`a"b`, "a\nb", `a\b`, "a;b`c"} {
		if err := store.Set("s", "a", "l", secret); err == nil {
			t.Fatalf("%q must be refused", secret)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("a refused value must not run security: %+v", calls)
	}
}

func TestGetAndDeleteMapItemNotFound(t *testing.T) {
	var calls []call
	store := &SecurityStore{Run: fakeRunner(&calls, "", 44)}
	if _, err := store.Get("s", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get: expected ErrNotFound, got %v", err)
	}
	if err := store.Delete("s", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: expected ErrNotFound, got %v", err)
	}
	if strings.Join(calls[0].args, " ") != "find-generic-password -s s -a a -w" || strings.Join(calls[1].args, " ") != "delete-generic-password -s s -a a" {
		t.Fatalf("unexpected commands %+v", calls)
	}
	store = &SecurityStore{Run: fakeRunner(&calls, "dG9rZW4=\n", 0)}
	if got, err := store.Get("s", "a"); err != nil || got != "dG9rZW4=" {
		t.Fatalf("get: %q, %v", got, err)
	}
	store = &SecurityStore{Run: fakeRunner(&calls, "", 51)}
	if _, err := store.Get("s", "a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("another exit code is an error, not a miss: %v", err)
	}
}
