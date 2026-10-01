package uuid

import (
	"strings"
	"testing"
)

func TestNewV4IsAValidVersion4UUID(t *testing.T) {
	for i := 0; i < 20; i++ {
		id, err := NewV4()
		if err != nil || !Valid(id) || id[14] != '4' || !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatalf("expected an RFC 4122 v4 UUID, got %q (%v)", id, err)
		}
	}
}

func TestValidChecksTheGUIDShape(t *testing.T) {
	for _, bad := range []string{"", "not-a-uuid", "0123456789ab-cdef-0123-4567-89abcdef0123", "g1234567-89ab-cdef-0123-456789abcdef"} {
		if Valid(bad) {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
	if !Valid(" 01234567-89AB-cdef-0123-456789abcdef ") {
		t.Fatal("expected a padded mixed-case UUID to be accepted")
	}
}
