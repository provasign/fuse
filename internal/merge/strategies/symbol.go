package strategies

import (
	"regexp"
	"sort"
	"strings"

	"github.com/provasign/fuse/internal/core"
)

// SymbolMergeResult is the output of a symbol-level three-way merge.
type SymbolMergeResult struct {
	Merged     []core.SymbolData // symbols in stable order (ours order preferred)
	Conflicts  []core.SymbolConflict
	Confidence float64
}

// SymbolMerge performs a symbol-level three-way merge over the three symbol
// maps. Each map is keyed by SymbolData.Key.
//
// Algorithm:
//
//	for key in union(base, ours, theirs):
//	  action := ThreeWaySymbol(base[key], ours[key], theirs[key])
//	  keep / take-ours / take-theirs / converged / delete / conflict
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

	// Rename-aware pre-pass: a symbol renamed on one side and edited on the
	// other looks like remove-vs-modify (a conflict) plus an unrelated
	// addition. Pair them by name-substituted body identity and merge the
	// edit into the renamed symbol instead.
	renames := resolveRenames(base, ours, theirs)
	for _, r := range renames {
		used[r.oldKey] = true
	}

	// ordered preference: ours-order, then theirs-only, then base-only
	for _, k := range orderedKeys(ours, theirs, base) {
		if used[k] {
			continue
		}
		if r, ok := renames[k]; ok {
			merged = append(merged, r.resolved)
			confSum += 0.80
			confN++
			used[k] = true
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
			// Before declaring a conflict, attempt a line-level three-way
			// merge of the symbol body. Two-sided edits to different regions
			// of one symbol — different methods of the same class, different
			// branches of a large function — resolve cleanly here.
			if basePtr != nil && oursPtr != nil && theirsPtr != nil {
				lm := LineMerge(basePtr.Body, oursPtr.Body, theirsPtr.Body)
				if !lm.HasConflict {
					resolved := *oursPtr
					resolved.Body = lm.Merged
					merged = append(merged, resolved)
					confSum += 0.70
					confN++
					continue
				}
			}
			conflicts = append(conflicts, core.SymbolConflict{
				Key:    k,
				Base:   deref(basePtr),
				Ours:   deref(oursPtr),
				Theirs: deref(theirsPtr),
			})
			// Emit a synthetic merged entry containing git-style conflict
			// markers so reconstructFile preserves the symbol's location
			// instead of silently dropping it. The user (or AI handoff) can
			// then resolve in-place. We use ours' span/qualified-name so the
			// substitution lines up with the original ours buffer.
			confBody := renderConflictMarkers(deref(oursPtr).Body, deref(theirsPtr).Body, deref(basePtr).Body)
			if oursPtr != nil {
				marker := *oursPtr
				marker.Body = confBody
				merged = append(merged, marker)
			} else if theirsPtr != nil {
				marker := *theirsPtr
				marker.Body = confBody
				merged = append(merged, marker)
			}
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

// resolvedRename carries one rename-with-edit resolution, keyed by the
// renamed (new) symbol's key.
type resolvedRename struct {
	oldKey   string
	resolved core.SymbolData
}

// resolveRenames detects symbols renamed on exactly one side while the other
// side edited the original, and merges the edit into the renamed symbol.
//
// Pairing is conservative, mirroring Grove's GraphDiff rename detection:
// the added symbol's body must equal the base body with the old name
// substituted for the new one (word-boundary), kinds must match, and the
// pairing must be unambiguous (exactly one candidate on each side). The
// body merge itself must resolve cleanly at line level or the pair is
// abandoned and the normal conflict path runs.
func resolveRenames(base, ours, theirs map[string]core.SymbolData) map[string]resolvedRename {
	out := map[string]resolvedRename{}
	pairSide := func(renamer, editor map[string]core.SymbolData) {
		// old: in base, gone on renamer side, modified on editor side.
		// new: on renamer side only.
		oldMatches := map[string][]string{} // oldKey → newKeys
		newMatches := map[string][]string{} // newKey → oldKeys
		for oldKey, baseSym := range base {
			if _, stillThere := renamer[oldKey]; stillThere {
				continue
			}
			edited, editorHas := editor[oldKey]
			if !editorHas || edited.Body == baseSym.Body {
				continue // pure delete or rename-without-edit: default actions handle it
			}
			if len(baseSym.Name) < 3 {
				continue
			}
			for newKey, newSym := range renamer {
				if _, inBase := base[newKey]; inBase {
					continue
				}
				if _, inEditor := editor[newKey]; inEditor {
					continue
				}
				if newSym.Kind != baseSym.Kind || newSym.Name == baseSym.Name || len(newSym.Name) < 3 {
					continue
				}
				if renameSub(baseSym.Body, baseSym.Name, newSym.Name) != newSym.Body {
					continue
				}
				oldMatches[oldKey] = append(oldMatches[oldKey], newKey)
				newMatches[newKey] = append(newMatches[newKey], oldKey)
			}
		}
		for oldKey, newKeys := range oldMatches {
			if len(newKeys) != 1 || len(newMatches[newKeys[0]]) != 1 {
				continue // ambiguous; leave to the conflict path
			}
			newKey := newKeys[0]
			baseSym := base[oldKey]
			newSym := renamer[newKey]
			edited := editor[oldKey]
			// Merge the editor's change into the renamed body: rename all
			// three texts to the new name, then three-way line merge.
			lm := LineMerge(
				renameSub(baseSym.Body, baseSym.Name, newSym.Name),
				newSym.Body,
				renameSub(edited.Body, baseSym.Name, newSym.Name),
			)
			if lm.HasConflict {
				continue
			}
			resolved := newSym
			resolved.Body = lm.Merged
			out[newKey] = resolvedRename{oldKey: oldKey, resolved: resolved}
		}
	}
	pairSide(ours, theirs)
	pairSide(theirs, ours)
	return out
}

// renameSub replaces word-boundary occurrences of oldName with newName.
func renameSub(body, oldName, newName string) string {
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(oldName) + `\b`)
	if err != nil {
		return body
	}
	return re.ReplaceAllString(body, newName)
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

// TopLevelSymbols filters out symbols whose spans are contained within
// another symbol's span (class methods, nested functions). Merging happens at
// top-level granularity: container bodies already include their children, and
// overlapping spans would corrupt file reconstruction. Output is ordered by
// span start.
func TopLevelSymbols(syms []core.SymbolData) []core.SymbolData {
	if len(syms) <= 1 {
		return syms
	}
	sorted := make([]core.SymbolData, len(syms))
	copy(sorted, syms)
	// Start ascending; on ties the wider span first so containers win.
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Span.Start == sorted[j].Span.Start {
			return sorted[i].Span.End > sorted[j].Span.End
		}
		return sorted[i].Span.Start < sorted[j].Span.Start
	})
	out := make([]core.SymbolData, 0, len(sorted))
	maxEnd := 0
	for _, s := range sorted {
		if s.Span.Start > maxEnd {
			out = append(out, s)
			maxEnd = s.Span.End
		}
	}
	return out
}

// MergeKey identifies a symbol within one file for three-way matching.
// QualifiedName alone collides in Go — method `(c *Context) Negotiate` and
// `type Negotiate` both carry QualifiedName "Negotiate" — so the parent /
// receiver name disambiguates.
func MergeKey(s core.SymbolData) string {
	if s.ParentName != "" && !strings.HasPrefix(s.QualifiedName, s.ParentName+".") {
		return s.ParentName + "." + s.QualifiedName
	}
	return s.QualifiedName
}

// SymbolsToMap builds a MergeKey→SymbolData map from a slice. Later entries
// with the same key win.
func SymbolsToMap(syms []core.SymbolData) map[string]core.SymbolData {
	out := make(map[string]core.SymbolData, len(syms))
	for _, s := range syms {
		out[MergeKey(s)] = s
	}
	return out
}

// renderConflictMarkers produces a git-style conflict block for the body of
// one symbol. Empty sides are rendered as a placeholder comment so the block
// remains valid in most languages.
func renderConflictMarkers(ours, theirs, base string) string {
	if ours == "" {
		ours = "// (symbol removed in ours)"
	}
	if theirs == "" {
		theirs = "// (symbol removed in theirs)"
	}
	var b strings.Builder
	b.WriteString("<<<<<<< HEAD\n")
	b.WriteString(ours)
	if !strings.HasSuffix(ours, "\n") {
		b.WriteString("\n")
	}
	if base != "" {
		b.WriteString("||||||| base\n")
		b.WriteString(base)
		if !strings.HasSuffix(base, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("=======\n")
	b.WriteString(theirs)
	if !strings.HasSuffix(theirs, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(">>>>>>> theirs")
	return b.String()
}
