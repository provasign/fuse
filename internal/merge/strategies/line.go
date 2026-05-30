package strategies

import (
	"strings"
)

// LineMergeResult is the output of a line-level three-way merge.
type LineMergeResult struct {
	Merged      string
	HasConflict bool
	Confidence  float64
}

// LineMerge is the fallback three-way merge used when language-aware merging
// is unavailable or fails. It mimics git's line-merge driver semantics:
//
//  1. Split each side into lines.
//  2. Diff base→ours and base→theirs as line-level hunks (LCS-based).
//  3. For non-overlapping hunks, accept both sides.
//  4. For overlapping hunks where one side is unchanged, take the changed
//     side.
//  5. For truly overlapping changes, emit git-style conflict markers.
//
// This is intentionally simple — it is the floor of merge quality, used only
// when symbol-level merge isn't applicable.
func LineMerge(base, ours, theirs string) LineMergeResult {
	if ours == theirs {
		return LineMergeResult{Merged: ours, Confidence: 1.0}
	}
	if base == ours {
		return LineMergeResult{Merged: theirs, Confidence: 0.95}
	}
	if base == theirs {
		return LineMergeResult{Merged: ours, Confidence: 0.95}
	}

	baseLines := splitLines(base)
	oursLines := splitLines(ours)
	theirsLines := splitLines(theirs)

	hunksOurs := diffLines(baseLines, oursLines)
	hunksTheirs := diffLines(baseLines, theirsLines)

	merged, conflict := applyHunks(baseLines, oursLines, theirsLines, hunksOurs, hunksTheirs)
	conf := 0.7
	if conflict {
		conf = 0.0
	}
	return LineMergeResult{Merged: joinLines(merged), HasConflict: conflict, Confidence: conf}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// hunk represents a range on the base side mapped to a slice of lines on a
// side (ours or theirs). [baseStart, baseEnd) on base becomes the given lines
// on the side.
type hunk struct {
	baseStart, baseEnd int
	lines              []string
	unchanged          bool
}

// diffLines is a small LCS-based diff that produces hunks describing the
// transformation from base→side.
func diffLines(base, side []string) []hunk {
	lcs := lcsTable(base, side)
	var hunks []hunk
	i, j := 0, 0
	for i < len(base) || j < len(side) {
		// Find next equal pair anchored at (i,j) using lcs table.
		if i < len(base) && j < len(side) && base[i] == side[j] {
			hunks = append(hunks, hunk{baseStart: i, baseEnd: i + 1, lines: []string{base[i]}, unchanged: true})
			i++
			j++
			continue
		}
		// Otherwise: lookahead — either delete from base or insert from side.
		if i < len(base) && (j >= len(side) || lcs[i+1][j] >= lcs[i][j+1]) {
			hunks = append(hunks, hunk{baseStart: i, baseEnd: i + 1, lines: nil})
			i++
		} else {
			hunks = append(hunks, hunk{baseStart: i, baseEnd: i, lines: []string{side[j]}})
			j++
		}
	}
	return coalesceHunks(hunks)
}

// lcsTable builds an (m+1)x(n+1) LCS length table.
func lcsTable(a, b []string) [][]int {
	m, n := len(a), len(b)
	t := make([][]int, m+1)
	for i := range t {
		t[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else if t[i+1][j] >= t[i][j+1] {
				t[i][j] = t[i+1][j]
			} else {
				t[i][j] = t[i][j+1]
			}
		}
	}
	return t
}

// coalesceHunks merges adjacent same-kind hunks for readability.
func coalesceHunks(in []hunk) []hunk {
	if len(in) == 0 {
		return in
	}
	out := []hunk{in[0]}
	for k := 1; k < len(in); k++ {
		last := &out[len(out)-1]
		cur := in[k]
		if last.unchanged == cur.unchanged && last.baseEnd == cur.baseStart {
			last.baseEnd = cur.baseEnd
			last.lines = append(last.lines, cur.lines...)
			continue
		}
		out = append(out, cur)
	}
	return out
}

// applyHunks combines ours-hunks and theirs-hunks into a merged output.
// For each base line index, we decide whether ours, theirs, or both modified
// it. Overlapping non-trivial modifications produce git conflict markers.
func applyHunks(base, ours, theirs []string, hOurs, hTheirs []hunk) ([]string, bool) {
	conflicted := false
	var out []string

	// Build per-base-line "change tags" for both sides.
	oursTags := tagLines(len(base), hOurs)
	theirsTags := tagLines(len(base), hTheirs)

	// Pre-insertions before each base line; keyed by base index (len(base) for tail).
	oursPre := preInsertions(hOurs, len(base))
	theirsPre := preInsertions(hTheirs, len(base))

	for i := 0; i <= len(base); i++ {
		// Insertions before base[i]
		oIns := oursPre[i]
		tIns := theirsPre[i]
		if len(oIns) > 0 || len(tIns) > 0 {
			switch {
			case stringsEqual(oIns, tIns):
				out = append(out, oIns...)
			case len(oIns) == 0:
				out = append(out, tIns...)
			case len(tIns) == 0:
				out = append(out, oIns...)
			default:
				out = append(out, conflictBlock(oIns, tIns)...)
				conflicted = true
			}
		}
		if i == len(base) {
			break
		}
		// Decide outcome for base[i].
		oTag := oursTags[i]
		tTag := theirsTags[i]
		switch {
		case oTag.kind == "keep" && tTag.kind == "keep":
			out = append(out, base[i])
		case oTag.kind == "delete" && tTag.kind == "delete":
			// both deleted
		case oTag.kind == "delete" && tTag.kind == "keep":
			// ours deleted; theirs unchanged → accept deletion
		case oTag.kind == "keep" && tTag.kind == "delete":
			// theirs deleted; ours unchanged → accept deletion
		case oTag.kind == "replace" && tTag.kind == "keep":
			out = append(out, oTag.replacement...)
		case oTag.kind == "keep" && tTag.kind == "replace":
			out = append(out, tTag.replacement...)
		case oTag.kind == "replace" && tTag.kind == "replace":
			if stringsEqual(oTag.replacement, tTag.replacement) {
				out = append(out, oTag.replacement...)
			} else {
				out = append(out, conflictBlock(oTag.replacement, tTag.replacement)...)
				conflicted = true
			}
		case oTag.kind == "replace" && tTag.kind == "delete":
			out = append(out, conflictBlock(oTag.replacement, nil)...)
			conflicted = true
		case oTag.kind == "delete" && tTag.kind == "replace":
			out = append(out, conflictBlock(nil, tTag.replacement)...)
			conflicted = true
		default:
			out = append(out, base[i])
		}
	}
	_ = ours
	_ = theirs
	return out, conflicted
}

type lineTag struct {
	kind        string   // keep | delete | replace
	replacement []string // for replace
}

// tagLines returns one tag per base index based on a diff.
func tagLines(n int, hunks []hunk) []lineTag {
	tags := make([]lineTag, n)
	for i := range tags {
		tags[i] = lineTag{kind: "keep"}
	}
	// Pair up delete-hunks with following insert-hunks at the same base index
	// to detect replacements.
	for k := 0; k < len(hunks); k++ {
		h := hunks[k]
		if h.unchanged {
			continue
		}
		// Coalesced replace hunk: baseEnd > baseStart AND has lines.
		if h.baseEnd > h.baseStart && len(h.lines) > 0 {
			// Spread replacement across the first base line of the range,
			// mark the rest as deleted.
			for i := h.baseStart; i < h.baseEnd; i++ {
				if i == h.baseStart {
					tags[i] = lineTag{kind: "replace", replacement: h.lines}
				} else {
					tags[i] = lineTag{kind: "delete"}
				}
			}
			continue
		}
		// pure delete: lines nil, baseEnd>baseStart
		if len(h.lines) == 0 {
			// Look ahead for adjacent insert-hunk at baseEnd.
			var replacement []string
			if k+1 < len(hunks) && !hunks[k+1].unchanged && hunks[k+1].baseStart == h.baseEnd && hunks[k+1].baseEnd == hunks[k+1].baseStart {
				replacement = hunks[k+1].lines
				hunks[k+1].lines = nil // consume
			}
			for i := h.baseStart; i < h.baseEnd; i++ {
				if replacement != nil {
					tags[i] = lineTag{kind: "replace", replacement: replacement}
					replacement = nil
				} else {
					tags[i] = lineTag{kind: "delete"}
				}
			}
		}
	}
	return tags
}

// preInsertions returns lines inserted *before* base index i, for each i in
// [0, n].
func preInsertions(hunks []hunk, n int) map[int][]string {
	out := make(map[int][]string)
	// pure insert hunks have baseStart == baseEnd; that is the index where they go.
	for _, h := range hunks {
		if h.unchanged {
			continue
		}
		if h.baseStart == h.baseEnd && len(h.lines) > 0 {
			out[h.baseStart] = append(out[h.baseStart], h.lines...)
		}
	}
	_ = n
	return out
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// conflictBlock returns lines wrapped in git-style conflict markers.
func conflictBlock(ours, theirs []string) []string {
	out := []string{"<<<<<<< OURS"}
	out = append(out, ours...)
	out = append(out, "=======")
	out = append(out, theirs...)
	out = append(out, ">>>>>>> THEIRS")
	return out
}
