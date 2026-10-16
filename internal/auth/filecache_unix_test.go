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

// fifoFeedingAToken opens a FIFO at path for writing and queues a valid
// token file's bytes, returning a func that closes the writer.
func fifoFeedingAToken(path string, payload []byte) (func(), error) {
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return nil, err
	}
	writer, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		if _, err := writer.Write(payload); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	return func() { _ = writer.Close() }, nil
}
