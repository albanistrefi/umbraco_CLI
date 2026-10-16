//go:build windows

package auth

import (
	"errors"
	"os"
)

func syscallMkfifo(string) error { return errors.New("no FIFOs on Windows") }

func withOwner(info os.FileInfo, _ int) os.FileInfo { return info }
