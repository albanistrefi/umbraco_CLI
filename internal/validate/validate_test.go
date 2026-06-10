package validate

import "testing"

func TestStringRejectsControlChars(t *testing.T) {
	if err := String("hello\x07world"); err == nil {
		t.Fatalf("expected control chars to fail")
	}
}

func TestStringAllowsPlainText(t *testing.T) {
	if err := String("partner-list"); err != nil {
		t.Fatalf("expected plain identifier to pass: %v", err)
	}
}

func TestResourceIDRejectsQueryChars(t *testing.T) {
	for _, id := range []string{"abc?fields=name", "abc#frag", "abc%2F"} {
		if err := ResourceID(id); err == nil {
			t.Fatalf("expected invalid id %q to fail", id)
		}
	}
}

func TestResourceIDAllowsGUIDs(t *testing.T) {
	if err := ResourceID("62689bb1-3a4d-478f-a7b1-1c0e560d4748"); err != nil {
		t.Fatalf("expected GUID to pass: %v", err)
	}
}
