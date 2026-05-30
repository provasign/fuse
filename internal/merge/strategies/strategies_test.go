package strategies

import (
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func sym(key, body string, start, end int) core.SymbolData {
	return core.SymbolData{QualifiedName: key, Name: key, Body: body, Span: core.LineRange{Start: start, End: end}}
}

func TestThreeWayString(t *testing.T) {
	if a := ThreeWayString("a", "a", "a"); a != ActionConverged {
		t.Errorf("got %v", a)
	}
	if a := ThreeWayString("a", "b", "a"); a != ActionUseOurs {
		t.Errorf("got %v", a)
	}
	if a := ThreeWayString("a", "a", "b"); a != ActionUseTheirs {
		t.Errorf("got %v", a)
	}
	if a := ThreeWayString("a", "b", "b"); a != ActionConverged {
		t.Errorf("got %v", a)
	}
	if a := ThreeWayString("a", "b", "c"); a != ActionConflict {
		t.Errorf("got %v", a)
	}
}

func TestSymbolMergeAddOnEach(t *testing.T) {
	base := map[string]core.SymbolData{"shared": sym("shared", "func shared(){}", 1, 1)}
	ours := map[string]core.SymbolData{
		"shared":  sym("shared", "func shared(){}", 1, 1),
		"newOurs": sym("newOurs", "func newOurs(){}", 2, 2),
	}
	theirs := map[string]core.SymbolData{
		"shared":    sym("shared", "func shared(){}", 1, 1),
		"newTheirs": sym("newTheirs", "func newTheirs(){}", 3, 3),
	}
	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) != 0 {
		t.Errorf("unexpected conflicts: %+v", r.Conflicts)
	}
	if len(r.Merged) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(r.Merged))
	}
}

func TestSymbolMergeConflict(t *testing.T) {
	base := map[string]core.SymbolData{"f": sym("f", "func f(){return 1}", 1, 1)}
	ours := map[string]core.SymbolData{"f": sym("f", "func f(){return 2}", 1, 1)}
	theirs := map[string]core.SymbolData{"f": sym("f", "func f(){return 3}", 1, 1)}
	r := SymbolMerge(base, ours, theirs)
	if len(r.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(r.Conflicts))
	}
	if r.Confidence > 0.5 {
		t.Errorf("confidence too high: %v", r.Confidence)
	}
}

func TestImportMergeUnion(t *testing.T) {
	base := []core.ImportStatement{{Path: "fmt"}}
	ours := []core.ImportStatement{{Path: "fmt"}, {Path: "os"}}
	theirs := []core.ImportStatement{{Path: "fmt"}, {Path: "io"}}
	r := ImportMerge(base, ours, theirs)
	got := map[string]bool{}
	for _, i := range r.Merged {
		got[i.Path] = true
	}
	for _, want := range []string{"fmt", "os", "io"} {
		if !got[want] {
			t.Errorf("missing %q", want)
		}
	}
}

func TestImportMergeRemovedByBoth(t *testing.T) {
	base := []core.ImportStatement{{Path: "fmt"}, {Path: "os"}}
	ours := []core.ImportStatement{{Path: "fmt"}}
	theirs := []core.ImportStatement{{Path: "fmt"}}
	r := ImportMerge(base, ours, theirs)
	for _, i := range r.Merged {
		if i.Path == "os" {
			t.Errorf("os should have been dropped")
		}
	}
}

func TestLineMergeClean(t *testing.T) {
	base := "a\nb\nc\n"
	ours := "a\nb\nc\nd\n"
	theirs := "0\na\nb\nc\n"
	r := LineMerge(base, ours, theirs)
	if r.HasConflict {
		t.Errorf("unexpected conflict: %s", r.Merged)
	}
}

func TestLineMergeConflict(t *testing.T) {
	base := "a\nb\nc\n"
	ours := "a\nX\nc\n"
	theirs := "a\nY\nc\n"
	r := LineMerge(base, ours, theirs)
	if !r.HasConflict {
		t.Errorf("expected conflict, got: %s", r.Merged)
	}
	if !strings.Contains(r.Merged, "<<<<<<<") || !strings.Contains(r.Merged, ">>>>>>>") {
		t.Errorf("missing markers")
	}
}

func TestConfigMergeJSON(t *testing.T) {
	base := `{"a":1,"b":2}`
	ours := `{"a":1,"b":2,"c":3}`
	theirs := `{"a":1,"b":2,"d":4}`
	r := ConfigMerge(core.LangJSON, base, ours, theirs)
	if r.HasConflict {
		t.Errorf("unexpected conflict: %s", r.Merged)
	}
	for _, k := range []string{`"a"`, `"b"`, `"c"`, `"d"`} {
		if !strings.Contains(r.Merged, k) {
			t.Errorf("missing key %s in %s", k, r.Merged)
		}
	}
}

func TestConfigMergeJSONConflict(t *testing.T) {
	base := `{"a":1}`
	ours := `{"a":2}`
	theirs := `{"a":3}`
	r := ConfigMerge(core.LangJSON, base, ours, theirs)
	if !r.HasConflict {
		t.Errorf("expected conflict")
	}
	if !strings.Contains(r.Merged, "__fuse_conflict__") {
		t.Errorf("expected conflict marker: %s", r.Merged)
	}
}
