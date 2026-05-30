package classification

import (
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestClassifyConfig(t *testing.T) {
	r := Classify(Inputs{IsConfigFile: true})
	if r.Type != core.ConflictConfigurational {
		t.Errorf("got %v", r.Type)
	}
	if r.Severity != core.SeverityLow {
		t.Errorf("got %v", r.Severity)
	}
}

func TestClassifyBreakingDominates(t *testing.T) {
	r := Classify(Inputs{
		BreakingChanges: []core.BreakingChange{{Kind: "removed_export", Severity: core.SeverityCritical}},
	})
	if r.Severity != core.SeverityCritical {
		t.Errorf("expected critical, got %v", r.Severity)
	}
}

func TestClassifyIncrementalWhenNothing(t *testing.T) {
	r := Classify(Inputs{})
	if r.Type != core.ConflictIncremental && r.Type != core.ConflictNone {
		t.Errorf("got %v", r.Type)
	}
}

func TestClassifyStructuralWithSymbolConflicts(t *testing.T) {
	r := Classify(Inputs{
		SymbolConflicts: []core.SymbolConflict{{Key: "f"}, {Key: "g"}},
	})
	if r.Type != core.ConflictStructural && r.Type != core.ConflictIncremental && r.Type != core.ConflictComplex {
		t.Errorf("got %v", r.Type)
	}
}
