package cmdkit

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestPrintMutationResultAndFlagHelpers(t *testing.T) {
	_, deps := newKitServer(nil)
	cmd := &cobra.Command{Use: "noop", RunE: func(cmd *cobra.Command, args []string) error {
		return PrintMutationResult(cmd, deps, "touched", nil, false)
	}}
	var skip, take int
	var all bool
	var triage ReadTriageOptions
	AddPaginationFlags(cmd, &skip, &take)
	AddAutoPaginationFlag(cmd, &all)
	AddReadTriageFlags(cmd, &triage)
	for _, name := range []string{"skip", "take", "all", "summarize", "ids-only", "first-n"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected --%s", name)
		}
	}
	if cmd.Flags().Lookup("skip").DefValue != "-1" || cmd.Flags().Lookup("take").DefValue != "-1" {
		t.Fatal("expected the -1 pagination sentinel")
	}
	out, _, err := runKit(t, cmd)
	if err != nil || !reflect.DeepEqual(decodeObject(t, out), map[string]any{"touched": true}) {
		t.Fatalf("expected {touched:true}, got %s (%v)", out, err)
	}
	if !reflect.DeepEqual(SortedKeys(map[string]struct{}{"b": {}, "a": {}}), []string{"a", "b"}) {
		t.Fatal("SortedKeys sorts")
	}
	if got := SortedKeys(nil); got == nil || len(got) != 0 {
		t.Fatalf("SortedKeys must return an empty, non-nil slice, got %#v", got)
	}
}

func TestRequiredValuesAndCSVFlags(t *testing.T) {
	mustContain(t, RequireValue("--name", " "), "missing required option: --name")
	if RequireValue("--name", "x") != nil {
		t.Fatal("expected a present value to pass")
	}
	if !reflect.DeepEqual(UniqueCSV(" a,b,,a , c"), []string{"a", "b", "c"}) {
		t.Fatal("UniqueCSV trims and deduplicates in order")
	}
}
