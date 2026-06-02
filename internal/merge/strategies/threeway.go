// Package strategies implements three-way merge algorithms.
package strategies

import "github.com/provasign/fuse/internal/core"

// MergeAction enumerates the outcomes of a single three-way decision.
type MergeAction string

const (
	ActionKeep      MergeAction = "keep"       // all three equal (or only-base)
	ActionUseOurs   MergeAction = "use-ours"   // ours changed from base, theirs didn't
	ActionUseTheirs MergeAction = "use-theirs" // theirs changed from base, ours didn't
	ActionConverged MergeAction = "converged"  // both sides made the same change
	ActionDelete    MergeAction = "delete"     // both sides removed the value
	ActionConflict  MergeAction = "conflict"   // ours and theirs both changed differently
)

// ThreeWayString returns the merge action for three string values.
// missing values should be encoded as the empty string by the caller; callers
// that need to distinguish missing-from-empty should use ThreeWayPointer.
func ThreeWayString(base, ours, theirs string) MergeAction {
	if ours == theirs {
		return ActionConverged
	}
	if ours == base {
		return ActionUseTheirs
	}
	if theirs == base {
		return ActionUseOurs
	}
	return ActionConflict
}

// ThreeWaySymbol returns the merge action for three SymbolData pointers.
// nil pointers indicate the symbol does not exist on that side.
func ThreeWaySymbol(base, ours, theirs *core.SymbolData) MergeAction {
	switch {
	case base == nil && ours == nil && theirs == nil:
		return ActionKeep // pathological; caller shouldn't invoke with all nil
	case ours == nil && theirs == nil:
		return ActionDelete
	case base == nil && ours != nil && theirs == nil:
		return ActionUseOurs
	case base == nil && ours == nil && theirs != nil:
		return ActionUseTheirs
	case base == nil && ours != nil && theirs != nil:
		if ours.Body == theirs.Body {
			return ActionConverged
		}
		return ActionConflict
	case base != nil && ours == nil && theirs != nil:
		if theirs.Body == base.Body {
			return ActionDelete // we removed it, they didn't change it → accept removal
		}
		return ActionConflict // we removed, they modified → conflict
	case base != nil && ours != nil && theirs == nil:
		if ours.Body == base.Body {
			return ActionDelete
		}
		return ActionConflict
	default:
		// all three present
		if ours.Body == theirs.Body {
			return ActionKeep
		}
		if ours.Body == base.Body {
			return ActionUseTheirs
		}
		if theirs.Body == base.Body {
			return ActionUseOurs
		}
		return ActionConflict
	}
}
