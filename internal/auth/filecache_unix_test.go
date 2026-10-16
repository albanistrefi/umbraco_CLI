//go:build unix

package auth

import (
	"os"
	"syscall"
)

func syscallMkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

type ownedInfo struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (o ownedInfo) Sys() any { return o.stat }

func withOwner(info os.FileInfo, uid int) os.FileInfo {
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid = uint32(uid)
	return ownedInfo{FileInfo: info, stat: &stat}
}
