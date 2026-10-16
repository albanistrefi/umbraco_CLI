//go:build unix

package auth

import (
	"os"
	"syscall"
)

// openTokenFile opens path for reading without following a symlink and
// without blocking on a FIFO; the caller checks the handle's file info.
func openTokenFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// ownedByCurrentUser reports whether info belongs to the user running the
// CLI.
func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// ownerOnly reports whether info allows only its owner to read it.
func ownerOnly(info os.FileInfo) bool {
	return info.Mode().Perm()&0o077 == 0
}

// writableByOthers reports whether a directory lets group or others create
// or replace files in it.
func writableByOthers(info os.FileInfo) bool {
	return info.Mode().Perm()&0o022 != 0
}
