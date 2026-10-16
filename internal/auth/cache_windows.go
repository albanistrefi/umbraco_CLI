//go:build windows

package auth

import "os"

// On Windows the permission bits and the owner id do not apply: the token
// file is protected by the user profile folder's access control.

func openTokenFile(path string) (*os.File, error) { return os.Open(path) }

func ownedByCurrentUser(os.FileInfo) bool { return true }

func ownerOnly(os.FileInfo) bool { return true }

func writableByOthers(os.FileInfo) bool { return false }
