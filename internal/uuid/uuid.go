// Package uuid checks and generates the GUIDs Umbraco keys its entities by.
package uuid

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// NewV4 returns a freshly generated random UUID (RFC 4122 v4).
func NewV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// Valid reports whether value, ignoring surrounding whitespace, has the
// 8-4-4-4-12 hex-digit shape of a GUID. It checks the shape only, not the
// version or variant bits.
func Valid(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexDigit(r) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
