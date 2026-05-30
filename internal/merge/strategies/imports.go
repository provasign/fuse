package strategies

import (
	"sort"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// ImportMergeResult is the output of a three-way import merge.
type ImportMergeResult struct {
	Merged     []core.ImportStatement
	Confidence float64
}

// ImportMerge performs a three-way merge of import statements.
//
//  1. Drop imports that exist in base but are absent from both ours and
//     theirs (both sides removed them).
//  2. Union of imports present in ours and/or theirs.
//  3. Deduplicate by Path (case-sensitive) preferring the ours-side entry.
//  4. Stable order: imports present in ours keep their ours-order; theirs-
//     only imports are appended in their theirs-order. Within each group
//     we keep insertion order.
//
// Confidence is 1.0 if both sides produced identical sets, 0.9 otherwise.
func ImportMerge(base, ours, theirs []core.ImportStatement) ImportMergeResult {
	basePaths := pathSet(base)
	oursPaths := pathSet(ours)
	theirsPaths := pathSet(theirs)

	keep := map[string]bool{}
	for _, imp := range ours {
		keep[imp.Path] = true
	}
	for _, imp := range theirs {
		keep[imp.Path] = true
	}
	// Drop imports that base had but both sides removed.
	for p := range basePaths {
		if !oursPaths[p] && !theirsPaths[p] {
			delete(keep, p)
		}
	}

	// Compose ordered output: ours first, then theirs-only.
	seen := map[string]bool{}
	var out []core.ImportStatement
	for _, imp := range ours {
		if !keep[imp.Path] || seen[imp.Path] {
			continue
		}
		seen[imp.Path] = true
		out = append(out, imp)
	}
	for _, imp := range theirs {
		if !keep[imp.Path] || seen[imp.Path] {
			continue
		}
		seen[imp.Path] = true
		out = append(out, imp)
	}
	// Stable sort within (group, line) to give a deterministic order across runs.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return groupOrder(out[i].Group) < groupOrder(out[j].Group)
		}
		return out[i].Path < out[j].Path
	})

	conf := 0.9
	if samePathSet(oursPaths, theirsPaths) {
		conf = 1.0
	}
	return ImportMergeResult{Merged: out, Confidence: conf}
}

func pathSet(imps []core.ImportStatement) map[string]bool {
	out := make(map[string]bool, len(imps))
	for _, i := range imps {
		out[i.Path] = true
	}
	return out
}

func samePathSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func groupOrder(g string) int {
	switch g {
	case "stdlib":
		return 0
	case "external":
		return 1
	case "relative":
		return 2
	default:
		return 3
	}
}
