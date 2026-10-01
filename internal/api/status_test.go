package api

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsStatusMatchesWrappedAPIErrors(t *testing.T) {
	err := fmt.Errorf("context: %w", &APIError{StatusCode: 404})
	if !IsStatus(err, 404) {
		t.Fatal("expected a wrapped 404 APIError to match")
	}
	if IsStatus(err, 500) {
		t.Fatal("expected a different status not to match")
	}
	if IsStatus(errors.New("plain"), 404) || IsStatus(nil, 404) {
		t.Fatal("expected non-API errors not to match")
	}
}
