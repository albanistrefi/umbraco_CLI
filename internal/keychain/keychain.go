// Package keychain stores short secrets in the operating system's
// credential store. Only the macOS keychain is supported, through
// /usr/bin/security; elsewhere the store is unavailable and callers carry on
// without it. Nothing is ever written to a plain file.
package keychain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// ErrNotFound is returned by Get and Delete when there is no such item.
var ErrNotFound = errors.New("keychain: item not found")

// ErrUnavailable is returned by every call on a platform without a
// supported credential store.
var ErrUnavailable = errors.New("keychain: no supported credential store on this system")

// Store reads and writes one secret per service and account.
type Store interface {
	Get(service string, account string) (string, error)
	Set(service string, account string, label string, secret string) error
	Delete(service string, account string) error
}

const securityPath = "/usr/bin/security"

// commandTimeout bounds one call to the credential store.
const commandTimeout = 5 * time.Second

// System returns the platform's credential store. Test binaries always get
// an unavailable store, so no test can read or write the user's keychain.
func System() Store {
	if testing.Testing() || runtime.GOOS != "darwin" {
		return Unavailable{}
	}
	if _, err := os.Stat(securityPath); err != nil {
		return Unavailable{}
	}
	return &SecurityStore{Run: runSecurity}
}

// Unavailable is the store on platforms without one.
type Unavailable struct{}

func (Unavailable) Get(string, string) (string, error)       { return "", ErrUnavailable }
func (Unavailable) Set(string, string, string, string) error { return ErrUnavailable }
func (Unavailable) Delete(string, string) error              { return ErrUnavailable }

// Runner runs /usr/bin/security with args and stdin, returning its stdout
// and exit code.
type Runner func(ctx context.Context, args []string, stdin string) (string, int, error)

// SecurityStore is the macOS keychain, through /usr/bin/security.
type SecurityStore struct {
	Run Runner
}

// securityItemNotFound is security's exit code for a missing item
// (errSecItemNotFound).
const securityItemNotFound = 44

// safeValue keeps values free of quotes, backslashes and line breaks, the
// characters that could escape a quoted word in security's interactive
// command line. Access tokens, hashes and labels fit in it.
var safeValue = regexp.MustCompile(`^[A-Za-z0-9 ._~+/=:@()-]*$`)

// Set adds or replaces the item. The secret travels on security's stdin, in
// interactive mode (security -i), never in its arguments, so it does not
// show up in the process list.
func (s *SecurityStore) Set(service string, account string, label string, secret string) error {
	for _, value := range []string{service, account, label, secret} {
		if !safeValue.MatchString(value) {
			return fmt.Errorf("keychain: refusing a value with characters outside %s", safeValue)
		}
	}
	line := fmt.Sprintf("add-generic-password -U -s %q -a %q -l %q -w %q\n", service, account, label, secret)
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	_, code, err := s.Run(ctx, []string{"-i"}, line)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("keychain: security add-generic-password exited %d", code)
	}
	return nil
}

func (s *SecurityStore) Get(service string, account string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, code, err := s.Run(ctx, []string{"find-generic-password", "-s", service, "-a", account, "-w"}, "")
	if err != nil {
		return "", err
	}
	switch code {
	case 0:
		return string(bytes.TrimRight([]byte(out), "\r\n")), nil
	case securityItemNotFound:
		return "", ErrNotFound
	}
	return "", fmt.Errorf("keychain: security find-generic-password exited %d", code)
}

func (s *SecurityStore) Delete(service string, account string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	_, code, err := s.Run(ctx, []string{"delete-generic-password", "-s", service, "-a", account}, "")
	if err != nil {
		return err
	}
	switch code {
	case 0:
		return nil
	case securityItemNotFound:
		return ErrNotFound
	}
	return fmt.Errorf("keychain: security delete-generic-password exited %d", code)
}

func runSecurity(ctx context.Context, args []string, stdin string) (string, int, error) {
	cmd := exec.CommandContext(ctx, securityPath, args...)
	cmd.Stdin = bytes.NewBufferString(stdin)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return "", 0, err
	}
	return stdout.String(), 0, nil
}
