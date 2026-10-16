package keychain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const fakeKeychainPath = "/Users/someone/Library/Keychains/login.keychain-db"

type call struct {
	args  []string
	stdin string
}

// fakeSecurity answers 'security default-keychain' with defaultOut and
// defaultCode, and every other command with out and code. It records all
// calls. Nothing here runs the real security tool.
type fakeSecurity struct {
	calls       []call
	defaultOut  string
	defaultCode int
	out         string
	code        int
}

func (f *fakeSecurity) run(_ context.Context, args []string, stdin string) (string, int, error) {
	f.calls = append(f.calls, call{args: args, stdin: stdin})
	if len(args) == 1 && args[0] == "default-keychain" {
		return f.defaultOut, f.defaultCode, nil
	}
	return f.out, f.code, nil
}

func (f *fakeSecurity) commands() []string {
	names := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		names = append(names, c.args[0])
	}
	return names
}

type storeOptions struct {
	keychainExists bool
	interactive    bool
	env            map[string]string
}

func newTestStore(fake *fakeSecurity, opts storeOptions) *SecurityStore {
	if fake.defaultOut == "" && fake.defaultCode == 0 {
		fake.defaultOut = `    "` + fakeKeychainPath + `"` + "\n"
	}
	return &SecurityStore{
		Run:         fake.run,
		Stat:        func(path string) bool { return opts.keychainExists && path == fakeKeychainPath },
		Interactive: func() bool { return opts.interactive },
		Getenv:      func(name string) string { return opts.env[name] },
	}
}

var ready = storeOptions{keychainExists: true, interactive: true}

func TestSystemIsUnavailableInTests(t *testing.T) {
	if _, ok := System().(Unavailable); !ok {
		t.Fatalf("a test binary must never get the real keychain")
	}
}

func TestAnInteractiveSessionWithAKeychainWrites(t *testing.T) {
	fake := &fakeSecurity{}
	store := newTestStore(fake, ready)
	if err := store.Set("umbraco-cli access token", "abc123", "umbraco-cli access token", "c2VjcmV0LnRva2Vu"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if strings.Join(fake.commands(), ",") != "default-keychain,-i" {
		t.Fatalf("expected the keychain check, then one 'security -i': %v", fake.commands())
	}
	want := `add-generic-password -U -s "umbraco-cli access token" -a "abc123" -l "umbraco-cli access token" -w "c2VjcmV0LnRva2Vu" "` + fakeKeychainPath + `"` + "\n"
	if fake.calls[1].stdin != want {
		t.Fatalf("stdin:\n got %q\nwant %q", fake.calls[1].stdin, want)
	}
	for _, arg := range fake.calls[1].args {
		if strings.Contains(arg, "c2VjcmV0LnRva2Vu") {
			t.Fatalf("the secret must not be an argument: %v", fake.calls[1].args)
		}
	}
}

func TestNoDefaultKeychainMeansNoWriteAndNoPrompt(t *testing.T) {
	for name, fake := range map[string]*fakeSecurity{
		"default-keychain fails":           {defaultCode: 50},
		"default-keychain prints nothing":  {defaultOut: "\n"},
		"the keychain file does not exist": {},
	} {
		t.Run(name, func(t *testing.T) {
			opts := ready
			opts.keychainExists = false
			store := newTestStore(fake, opts)
			if err := store.Set("s", "a", "l", "secret"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("set: expected ErrUnavailable, got %v", err)
			}
			if _, err := store.Get("s", "a"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("get: expected ErrUnavailable, got %v", err)
			}
			if err := store.Delete("s", "a"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("delete: expected ErrUnavailable, got %v", err)
			}
			if strings.Join(fake.commands(), ",") != "default-keychain" {
				t.Fatalf("only the read-only keychain check may run, and only once: %v", fake.commands())
			}
		})
	}
}

func TestANonInteractiveSessionNeverWrites(t *testing.T) {
	for name, opts := range map[string]storeOptions{
		"no terminal":    {keychainExists: true, interactive: false},
		"CI":             {keychainExists: true, interactive: true, env: map[string]string{"CI": "true"}},
		"GitHub Actions": {keychainExists: true, interactive: true, env: map[string]string{"GITHUB_ACTIONS": "true"}},
		"Azure DevOps":   {keychainExists: true, interactive: true, env: map[string]string{"TF_BUILD": "True"}},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeSecurity{}
			store := newTestStore(fake, opts)
			if err := store.Set("s", "a", "l", "secret"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("expected no write, got %v", err)
			}
			for _, c := range fake.calls {
				if c.args[0] == "-i" {
					t.Fatalf("a non-interactive session must not write: %v", fake.commands())
				}
			}
			// Reads of an existing keychain stay allowed.
			fake.code = 44
			if _, err := store.Get("s", "a"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("reads stay allowed: %v", err)
			}
		})
	}
}

func TestSetRefusesValuesThatCouldEscapeTheQuotedWord(t *testing.T) {
	fake := &fakeSecurity{}
	store := newTestStore(fake, ready)
	for _, secret := range []string{`a"b`, "a\nb", `a\b`, "a;b`c"} {
		if err := store.Set("s", "a", "l", secret); err == nil {
			t.Fatalf("%q must be refused", secret)
		}
	}
	if strings.Contains(strings.Join(fake.commands(), ","), "-i") {
		t.Fatalf("a refused value must not reach security: %v", fake.commands())
	}
}

func TestGetAndDeleteMapItemNotFoundAndNameTheKeychain(t *testing.T) {
	fake := &fakeSecurity{code: 44}
	store := newTestStore(fake, ready)
	if _, err := store.Get("s", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get: expected ErrNotFound, got %v", err)
	}
	if err := store.Delete("s", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: expected ErrNotFound, got %v", err)
	}
	if strings.Join(fake.calls[1].args, " ") != "find-generic-password -s s -a a -w "+fakeKeychainPath ||
		strings.Join(fake.calls[2].args, " ") != "delete-generic-password -s s -a a "+fakeKeychainPath {
		t.Fatalf("every call names the resolved keychain: %+v", fake.calls)
	}
	fake = &fakeSecurity{out: "dG9rZW4=\n"}
	store = newTestStore(fake, ready)
	if got, err := store.Get("s", "a"); err != nil || got != "dG9rZW4=" {
		t.Fatalf("get: %q, %v", got, err)
	}
	fake = &fakeSecurity{code: 51}
	store = newTestStore(fake, ready)
	if _, err := store.Get("s", "a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("another exit code is an error, not a miss: %v", err)
	}
}
