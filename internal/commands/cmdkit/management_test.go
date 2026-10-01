package cmdkit

import (
	"testing"
)

func TestIndexerHealthStatusToleratesLegacyStringShape(t *testing.T) {
	if got := IndexerHealthStatus(map[string]any{"healthStatus": "Healthy"}); got != "Healthy" {
		t.Fatalf("expected legacy string shape supported, got %q", got)
	}
	if got := IndexerHealthStatus(map[string]any{"healthStatus": map[string]any{"status": "Corrupt"}}); got != "Corrupt" {
		t.Fatalf("expected nested status, got %q", got)
	}
	if got := IndexerHealthStatus("not-an-object"); got != "" {
		t.Fatalf("expected empty status for non-object payload, got %q", got)
	}
}
