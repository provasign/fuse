package strategies

import (
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
)

// TestSymbolMergeRenameVsEdit: ours renames ProcessOrder → HandleOrder
// (body identical modulo name), theirs edits ProcessOrder's body. The old
// behavior was a remove-vs-modify conflict plus an unrelated addition; the
// rename-aware merge applies theirs' edit to the renamed symbol.
func TestSymbolMergeRenameVsEdit(t *testing.T) {
	baseBody := "func ProcessOrder(id string) error {\n\tvalidate(id)\n\treturn queue.Push(id)\n}"
	renamedBody := strings.ReplaceAll(baseBody, "ProcessOrder", "HandleOrder")
	editedBody := "func ProcessOrder(id string) error {\n\tvalidate(id)\n\tmetrics.Count(\"order\")\n\treturn queue.Push(id)\n}"

	base := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", baseBody, 1, 4)}
	ours := map[string]core.SymbolData{"HandleOrder": sym("HandleOrder", renamedBody, 1, 4)}
	theirs := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", editedBody, 1, 5)}

	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) != 0 {
		t.Fatalf("rename-vs-edit must not conflict: %+v", r.Conflicts)
	}
	if len(r.Merged) != 1 {
		t.Fatalf("expected exactly the renamed symbol, got %d: %+v", len(r.Merged), r.Merged)
	}
	got := r.Merged[0]
	if got.Name != "HandleOrder" {
		t.Fatalf("merged symbol = %q, want HandleOrder", got.Name)
	}
	if !strings.Contains(got.Body, "HandleOrder") || strings.Contains(got.Body, "ProcessOrder") {
		t.Fatalf("body not fully renamed: %q", got.Body)
	}
	if !strings.Contains(got.Body, "metrics.Count") {
		t.Fatalf("editor's change lost: %q", got.Body)
	}
}

// TestSymbolMergeRenameVsEditTheirsSide: same scenario mirrored — theirs
// renames, ours edits.
func TestSymbolMergeRenameVsEditTheirsSide(t *testing.T) {
	baseBody := "func ProcessOrder(id string) error {\n\tvalidate(id)\n\treturn queue.Push(id)\n}"
	renamedBody := strings.ReplaceAll(baseBody, "ProcessOrder", "HandleOrder")
	editedBody := "func ProcessOrder(id string) error {\n\tvalidate(id)\n\tmetrics.Count(\"order\")\n\treturn queue.Push(id)\n}"

	base := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", baseBody, 1, 4)}
	ours := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", editedBody, 1, 5)}
	theirs := map[string]core.SymbolData{"HandleOrder": sym("HandleOrder", renamedBody, 1, 4)}

	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) != 0 {
		t.Fatalf("rename-vs-edit must not conflict: %+v", r.Conflicts)
	}
	if len(r.Merged) != 1 || r.Merged[0].Name != "HandleOrder" || !strings.Contains(r.Merged[0].Body, "metrics.Count") {
		t.Fatalf("merged = %+v", r.Merged)
	}
}

// TestSymbolMergeRenameAmbiguousStaysConflict: two same-bodied symbols
// removed and two added — pairing would be a guess, so the conflict path
// must run unchanged.
func TestSymbolMergeRenameAmbiguousStaysConflict(t *testing.T) {
	bodyA := "func NAME() error {\n\treturn errNotImplemented\n}"
	mk := func(name string) core.SymbolData {
		return sym(name, strings.ReplaceAll(bodyA, "NAME", name), 1, 3)
	}
	base := map[string]core.SymbolData{"oldA": mk("oldA"), "oldB": mk("oldB")}
	// ours renames both ambiguously; theirs edits both originals.
	ours := map[string]core.SymbolData{"newA": mk("newA"), "newB": mk("newB")}
	edited := func(name string) core.SymbolData {
		s := mk(name)
		s.Body = strings.Replace(s.Body, "return", "log()\n\treturn", 1)
		return s
	}
	theirs := map[string]core.SymbolData{"oldA": edited("oldA"), "oldB": edited("oldB")}

	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) == 0 {
		t.Fatalf("ambiguous rename candidates must stay conflicts, got %+v", r.Merged)
	}
}

// TestSymbolMergeRenameWithConflictingEditFallsBack: the rename pairing is
// unambiguous but the body edit collides with the rename side's own body
// change — the pair must be abandoned, not force-merged.
func TestSymbolMergeRenameWithConflictingEditFallsBack(t *testing.T) {
	baseBody := "func ProcessOrder(id string) error {\n\treturn queue.Push(id)\n}"
	// Renamed AND line edited on the same line the editor touches → the
	// name-substituted bodies no longer match, so no pair forms at all.
	renamedBody := "func HandleOrder(id string) error {\n\treturn queue.PushFront(id)\n}"
	editedBody := "func ProcessOrder(id string) error {\n\treturn queue.PushBack(id)\n}"

	base := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", baseBody, 1, 3)}
	ours := map[string]core.SymbolData{"HandleOrder": sym("HandleOrder", renamedBody, 1, 3)}
	theirs := map[string]core.SymbolData{"ProcessOrder": sym("ProcessOrder", editedBody, 1, 3)}

	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) == 0 {
		t.Fatalf("conflicting rename+edit must surface as conflict, got %+v", r.Merged)
	}
}

// TestImportMergeOneSidedRemovalWins: an import removed on one side while
// the other side didn't touch it must stay removed (the old union semantics
// resurrected it).
func TestImportMergeOneSidedRemovalWins(t *testing.T) {
	imp := func(path string, line int) core.ImportStatement {
		return core.ImportStatement{Path: path, Line: line}
	}
	base := []core.ImportStatement{imp("fmt", 1), imp("os", 2), imp("strings", 3)}
	ours := []core.ImportStatement{imp("fmt", 1), imp("strings", 2)}                                   // removed os
	theirs := []core.ImportStatement{imp("fmt", 1), imp("os", 2), imp("strings", 3), imp("errors", 4)} // added errors

	r := ImportMerge(base, ours, theirs)
	paths := map[string]bool{}
	for _, i := range r.Merged {
		paths[i.Path] = true
	}
	if paths["os"] {
		t.Fatalf("one-sided removal of os was resurrected: %+v", r.Merged)
	}
	if !paths["fmt"] || !paths["strings"] || !paths["errors"] {
		t.Fatalf("expected fmt+strings+errors, got %+v", r.Merged)
	}
}
