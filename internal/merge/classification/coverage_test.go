package classification

import (
	"testing"

	"github.com/provasign/fuse/internal/core"
)

func TestClassify_AllBranches(t *testing.T) {
	cases := []struct {
		name string
		in   Inputs
		want core.ConflictType
	}{
		{"config-file-no-conflicts", Inputs{IsConfigFile: true}, core.ConflictConfigurational},
		{"config-file-with-conflicts", Inputs{IsConfigFile: true, SymbolConflicts: []core.SymbolConflict{{}}}, core.ConflictConfigurational},
		{"breaking-change", Inputs{BreakingChanges: []core.BreakingChange{{Severity: core.SeverityHigh}}}, core.ConflictStructural},
		{"large-delta", Inputs{OursSymbols: make([]core.SymbolData, 15), BaseSymbols: make([]core.SymbolData, 0)}, core.ConflictComplex},
		{"many-conflicts", Inputs{SymbolConflicts: make([]core.SymbolConflict, 5)}, core.ConflictStructural},
		{"one-conflict", Inputs{SymbolConflicts: []core.SymbolConflict{{}}}, core.ConflictStructural},
		{"import-churn-only", Inputs{ImportChanges: ImportChangeSummary{Added: 1}}, core.ConflictIncremental},
		{"symbol-delta-only", Inputs{OursSymbols: make([]core.SymbolData, 2)}, core.ConflictIncremental},
		{"empty", Inputs{}, core.ConflictNone},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got.Type != c.want {
			t.Errorf("%s: got %v want %v", c.name, got.Type, c.want)
		}
	}
}

func TestClassify_Architectural_AsyncSignal(t *testing.T) {
	in := Inputs{
		SymbolConflicts: []core.SymbolConflict{
			{
				Base:   core.SymbolData{Body: "func f() {}"},
				Ours:   core.SymbolData{Body: "async func f() { await x() }"},
				Theirs: core.SymbolData{Body: "func f() { x() }"},
			},
		},
	}
	got := Classify(in)
	if got.Type != core.ConflictArchitectural {
		t.Errorf("got %v want architectural", got.Type)
	}
}

func TestAbs(t *testing.T) {
	if abs(-5) != 5 || abs(5) != 5 || abs(0) != 0 {
		t.Error("abs broken")
	}
}
