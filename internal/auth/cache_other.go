//go:build !unix && !windows

package auth

import "os"

// Platforms without Unix permissions or Windows ACLs (plan9, js/wasm,
// wasip1) read the token file plainly and apply no owner or permission
// checks.

func openTokenFile(path string) (*os.File, error) { return os.Open(path) }

func ownedByCurrentUser(os.FileInfo) bool { return true }

func ownerOnly(os.FileInfo) bool { return true }

func writableByOthers(os.FileInfo) bool { return false }
