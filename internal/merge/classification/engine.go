// Package classification categorizes a merge conflict by type and severity.
package classification

import (
	"strings"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// Inputs is the set of signals the classifier examines.
type Inputs struct {
	Language        core.LanguageKey
	BaseSymbols     []core.SymbolData
	OursSymbols     []core.SymbolData
	TheirsSymbols   []core.SymbolData
	SymbolConflicts []core.SymbolConflict
	ImportChanges   ImportChangeSummary
	BreakingChanges []core.BreakingChange
	IsConfigFile    bool
}

// ImportChangeSummary describes how imports changed between base/ours/theirs.
type ImportChangeSummary struct {
	Added   int
	Removed int
	Changed int
}

// Result is what the classifier returns.
type Result struct {
	Type     core.ConflictType
	Severity core.ConflictSeverity
}

// Classify applies a small rule-based classifier producing a (type, severity)
// pair. The rules favor explicit signals (breaking changes, config files) and
// fall back to conflict counts.
func Classify(in Inputs) Result {
	if in.IsConfigFile {
		sev := core.SeverityLow
		if len(in.SymbolConflicts) > 0 {
			sev = core.SeverityMedium
		}
		return Result{Type: core.ConflictConfigurational, Severity: sev}
	}

	// Breaking changes dominate.
	if maxSev := maxBreakingSeverity(in.BreakingChanges); maxSev != core.SeverityNone {
		t := core.ConflictStructural
		if hasArchitecturalSignals(in) {
			t = core.ConflictArchitectural
		}
		return Result{Type: t, Severity: maxSev}
	}

	conflictCount := len(in.SymbolConflicts)
	importChurn := in.ImportChanges.Added + in.ImportChanges.Removed
	totalSymbolDelta := abs(len(in.OursSymbols)-len(in.BaseSymbols)) + abs(len(in.TheirsSymbols)-len(in.BaseSymbols))

	switch {
	case hasArchitecturalSignals(in):
		return Result{Type: core.ConflictArchitectural, Severity: core.SeverityHigh}
	case totalSymbolDelta > 10 || (conflictCount > 3 && importChurn > 5):
		return Result{Type: core.ConflictComplex, Severity: core.SeverityHigh}
	case conflictCount > 3:
		return Result{Type: core.ConflictStructural, Severity: core.SeverityHigh}
	case conflictCount >= 1:
		return Result{Type: core.ConflictStructural, Severity: core.SeverityMedium}
	case importChurn > 0 && conflictCount == 0:
		return Result{Type: core.ConflictIncremental, Severity: core.SeverityLow}
	case totalSymbolDelta > 0:
		return Result{Type: core.ConflictIncremental, Severity: core.SeverityLow}
	default:
		return Result{Type: core.ConflictNone, Severity: core.SeverityNone}
	}
}

func maxBreakingSeverity(bcs []core.BreakingChange) core.ConflictSeverity {
	rank := map[core.ConflictSeverity]int{
		core.SeverityNone: 0, core.SeverityLow: 1, core.SeverityMedium: 2,
		core.SeverityHigh: 3, core.SeverityCritical: 4,
	}
	best := core.SeverityNone
	for _, b := range bcs {
		if rank[b.Severity] > rank[best] {
			best = b.Severity
		}
	}
	return best
}

// hasArchitecturalSignals looks for sync→async or framework-swap patterns by
// scanning the modified symbol bodies for telltale keywords on one side only.
func hasArchitecturalSignals(in Inputs) bool {
	if !signalsInOneSideOnly(in.SymbolConflicts, "async", "await") &&
		!signalsInOneSideOnly(in.SymbolConflicts, "Promise<", "then(") {
		return false
	}
	return true
}

func signalsInOneSideOnly(conflicts []core.SymbolConflict, needles ...string) bool {
	for _, c := range conflicts {
		oursHas := containsAny(c.Ours.Body, needles)
		theirsHas := containsAny(c.Theirs.Body, needles)
		baseHas := containsAny(c.Base.Body, needles)
		if !baseHas && (oursHas != theirsHas) {
			return true
		}
	}
	return false
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
