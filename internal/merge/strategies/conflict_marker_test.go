package strategies

import (
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
)

func TestSymbolMerge_ConflictPreservesSymbol(t *testing.T) {
	// Regression test: a conflicting symbol used to be silently dropped from
	// the merged output, leaving the file body empty. Fuse must now emit it
	// as a synthetic merged symbol carrying git-style conflict markers so
	// the reconstructed file preserves the symbol's slot.
	base := map[string]core.SymbolData{
		"Greet": {QualifiedName: "Greet", Body: "func Greet() { return \"a\" }"},
	}
	ours := map[string]core.SymbolData{
		"Greet": {QualifiedName: "Greet", Body: "func Greet() { return \"OURS\" }"},
	}
	theirs := map[string]core.SymbolData{
		"Greet": {QualifiedName: "Greet", Body: "func Greet() { return \"THEIRS\" }"},
	}
	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(r.Conflicts))
	}
	if len(r.Merged) != 1 {
		t.Fatalf("conflicted symbol must still appear in Merged, got %d", len(r.Merged))
	}
	body := r.Merged[0].Body
	for _, marker := range []string{"<<<<<<< HEAD", "OURS", "=======", "THEIRS", ">>>>>>> theirs"} {
		if !strings.Contains(body, marker) {
			t.Errorf("merged body missing %q\nbody=%s", marker, body)
		}
	}
}

func TestRenderConflictMarkers_Empty(t *testing.T) {
	out := renderConflictMarkers("", "", "")
	if !strings.Contains(out, "removed in ours") || !strings.Contains(out, "removed in theirs") {
		t.Errorf("missing placeholder: %s", out)
	}
}

func TestRenderConflictMarkers_NoBase(t *testing.T) {
	out := renderConflictMarkers("a\n", "b\n", "")
	if strings.Contains(out, "||||||| base") {
		t.Errorf("should not have base marker: %s", out)
	}
}
