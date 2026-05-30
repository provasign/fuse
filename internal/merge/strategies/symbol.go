package strategies

import (
	"sort"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// SymbolMergeResult is the output of a symbol-level three-way merge.
type SymbolMergeResult struct {
	Merged     []core.SymbolData    // symbols in stable order (ours order preferred)
	Conflicts  []core.SymbolConflict
	Confidence float64
}

// SymbolMerge performs a symbol-level three-way merge over the three symbol
// maps. Each map is keyed by SymbolData.Key.
//
// Algorithm:
//
//   for key in union(base, ours, theirs):
//     action := ThreeWaySymbol(base[key], ours[key], theirs[key])
//     keep / take-ours / take-theirs / converged / delete / conflict
//
// Order preservation: symbols present in ours appear in their ours-order
// first; symbols only-in-theirs are appended; symbols only-in-base that were
// not deleted are dropped from output (callers already handled deletion).
//
// Confidence is computed as: 1.0 if no conflicts; otherwise the average of
// per-symbol confidence where converged=1.0, single-sided change=0.95,
// conflict=0.45.
func SymbolMerge(base, ours, theirs map[string]core.SymbolData) SymbolMergeResult {
	keys := mergedKeySet(base, ours, theirs)

	merged := make([]core.SymbolData, 0, len(keys))
	var conflicts []core.SymbolConflict
	var confSum float64
	var confN int

	used := map[string]bool{}

	// ordered preference: ours-order, then theirs-only, then base-only
	for _, k := range orderedKeys(ours, theirs, base) {
		if used[k] {
			continue
		}
		used[k] = true
		basePtr := getPtr(base, k)
		oursPtr := getPtr(ours, k)
		theirsPtr := getPtr(theirs, k)
		action := ThreeWaySymbol(basePtr, oursPtr, theirsPtr)

		switch action {
		case ActionKeep:
			if oursPtr != nil {
				merged = append(merged, *oursPtr)
			} else if theirsPtr != nil {
				merged = append(merged, *theirsPtr)
			}
			confSum += 1.0
			confN++
		case ActionConverged:
			if oursPtr != nil {
				merged = append(merged, *oursPtr)
			} else if theirsPtr != nil {
				merged = append(merged, *theirsPtr)
			}
			confSum += 1.0
			confN++
		case ActionUseOurs:
			if oursPtr != nil {
				merged = append(merged, *oursPtr)
				confSum += 0.95
				confN++
			}
		case ActionUseTheirs:
			if theirsPtr != nil {
				merged = append(merged, *theirsPtr)
				confSum += 0.95
				confN++
			}
		case ActionDelete:
			// drop
			confSum += 0.95
			confN++
		case ActionConflict:
			conflicts = append(conflicts, core.SymbolConflict{
				Key:    k,
				Base:   deref(basePtr),
				Ours:   deref(oursPtr),
				Theirs: deref(theirsPtr),
			})
			confSum += 0.45
			confN++
		}
	}

	conf := 1.0
	if confN > 0 {
		conf = confSum / float64(confN)
	}
	if len(conflicts) > 0 && conf > 0.5 {
		conf = 0.45
	}
	return SymbolMergeResult{Merged: merged, Conflicts: conflicts, Confidence: conf}
}

func getPtr(m map[string]core.SymbolData, k string) *core.SymbolData {
	if v, ok := m[k]; ok {
		return &v
	}
	return nil
}

func deref(p *core.SymbolData) core.SymbolData {
	if p == nil {
		return core.SymbolData{}
	}
	return *p
}

func mergedKeySet(base, ours, theirs map[string]core.SymbolData) map[string]bool {
	out := make(map[string]bool, len(base)+len(ours)+len(theirs))
	for k := range base {
		out[k] = true
	}
	for k := range ours {
		out[k] = true
	}
	for k := range theirs {
		out[k] = true
	}
	return out
}

// orderedKeys yields keys preserving primary's ordering (by span), then
// secondary's, then tertiary's keys not yet seen. Maps in Go don't preserve
// insertion order, so we sort by Span.Start as a deterministic proxy.
func orderedKeys(primary, secondary, tertiary map[string]core.SymbolData) []string {
	seen := map[string]bool{}
	var out []string
	appendOrdered := func(m map[string]core.SymbolData) {
		type kv struct {
			k    string
			line int
		}
		var list []kv
		for k, v := range m {
			if seen[k] {
				continue
			}
			list = append(list, kv{k: k, line: v.Span.Start})
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].line == list[j].line {
				return list[i].k < list[j].k
			}
			return list[i].line < list[j].line
		})
		for _, x := range list {
			seen[x.k] = true
			out = append(out, x.k)
		}
	}
	appendOrdered(primary)
	appendOrdered(secondary)
	appendOrdered(tertiary)
	return out
}

// SymbolsToMap builds a key→SymbolData map from a slice. Later entries with
// the same key win.
func SymbolsToMap(syms []core.SymbolData) map[string]core.SymbolData {
	out := make(map[string]core.SymbolData, len(syms))
	for _, s := range syms {
		out[s.QualifiedName] = s
	}
	return out
}
