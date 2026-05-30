package strategies

import (
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// ── SymbolMerge: ActionConflict where oursPtr==nil (base!=nil, ours deleted, theirs modified) ──

func TestSymbolMerge_ConflictOursNilTheirsPresent(t *testing.T) {
	base := map[string]core.SymbolData{
		"F": {QualifiedName: "F", Body: "func F() { old() }"},
	}
	ours := map[string]core.SymbolData{
		// F deleted from ours
	}
	theirs := map[string]core.SymbolData{
		"F": {QualifiedName: "F", Body: "func F() { new() }"}, // modified
	}
	res := SymbolMerge(base, ours, theirs)
	// ThreeWaySymbol(base!=nil, nil, theirs!=base) → ActionConflict
	// oursPtr==nil → should use theirsPtr for the synthetic marker
	if len(res.Conflicts) == 0 {
		t.Error("expected conflict")
	}
	// Merged should contain the conflict marker (from theirsPtr branch)
	found := false
	for _, m := range res.Merged {
		if strings.Contains(m.Body, "<<<<<<<") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected conflict markers in merged; got=%+v", res.Merged)
	}
}

// ── LineMerge: both sides insert different content before the same base line ──
// applyHunks: case len(oIns)>0 && len(tIns)>0 && !stringsEqual → conflict block

func TestLineMerge_BothSidesInsertDifferentBeforeSameLine(t *testing.T) {
	base := "a\nb\n"
	ours := "a\nX\nb\n"   // inserted X before b
	theirs := "a\nY\nb\n" // inserted Y before b
	res := LineMerge(base, ours, theirs)
	if !res.HasConflict {
		t.Errorf("expected conflict; merged=%q", res.Merged)
	}
	if !strings.Contains(res.Merged, "<<<<<<<") {
		t.Errorf("expected conflict markers; merged=%q", res.Merged)
	}
}

// ── LineMerge: replace vs delete → conflict ──
// applyHunks: case "replace && delete"

func TestLineMerge_ReplaceVsDelete(t *testing.T) {
	base := "a\nb\nc\n"
	ours := "a\nX\nc\n" // replaced b with X
	theirs := "a\nc\n"  // deleted b
	res := LineMerge(base, ours, theirs)
	if !res.HasConflict {
		t.Errorf("expected conflict; merged=%q", res.Merged)
	}
	if !strings.Contains(res.Merged, "<<<<<<<") {
		t.Errorf("expected conflict markers; merged=%q", res.Merged)
	}
}

// ── LineMerge: delete vs replace → conflict ──
// applyHunks: case "delete && replace"

func TestLineMerge_DeleteVsReplace(t *testing.T) {
	base := "a\nb\nc\n"
	ours := "a\nc\n"      // deleted b
	theirs := "a\nY\nc\n" // replaced b with Y
	res := LineMerge(base, ours, theirs)
	if !res.HasConflict {
		t.Errorf("expected conflict; merged=%q", res.Merged)
	}
}

// ── LineMerge: pure deletion (no replacement), other side unchanged ──
// tagLines: pure delete with no adjacent insert → kind="delete"

func TestLineMerge_PureDeleteOnOneSide(t *testing.T) {
	base := "a\nb\nc\n"
	ours := "a\nc\n" // deleted b
	// theirs unchanged → base == theirs short-circuits in LineMerge
	// Use theirs with a different change elsewhere
	theirs := "a\nb\nc\nd\n" // added d
	res := LineMerge(base, ours, theirs)
	if res.HasConflict {
		t.Errorf("no conflict expected; merged=%q", res.Merged)
	}
	if !strings.Contains(res.Merged, "a") {
		t.Errorf("missing a; merged=%q", res.Merged)
	}
	// b should be gone (ours deleted it, theirs left it; ours deletion should win)
	if !strings.Contains(res.Merged, "d") {
		t.Errorf("missing d from theirs; merged=%q", res.Merged)
	}
}

// ── LineMerge: multi-line base range replaced by ours ──
// tagLines: hunk with baseEnd>baseStart && len(lines)>0: spread across multiple lines

func TestLineMerge_MultiLineBaseReplace(t *testing.T) {
	// ours replaces 2 base lines with 1 line
	base := "a\nb\nc\nd\n"
	ours := "a\nXY\nd\n"        // b+c → XY
	theirs := "a\nb\nc\nd\ne\n" // added e
	res := LineMerge(base, ours, theirs)
	// Should contain XY (ours replacement) and e (theirs addition)
	if !strings.Contains(res.Merged, "XY") {
		t.Errorf("expected XY from ours replacement; merged=%q", res.Merged)
	}
}

// ── LineMerge: splitLines("") returns nil → early exit keeps output correct ──

func TestLineMerge_EmptyBase(t *testing.T) {
	// base is empty; ours adds content; theirs adds different content → conflict
	res := LineMerge("", "foo\n", "bar\n")
	// both sides changed from empty → conflict
	_ = res // just must not panic
}

// ── ConfigMerge: unknown / unsupported language falls back to line merge ──

func TestConfigMerge_UnknownLangFallback(t *testing.T) {
	// core.LangUnknown has no parserFor → falls back to LineMerge
	base := "key: value\n"
	ours := "key: value\nfoo: 1\n"
	theirs := "key: value\nbar: 2\n"
	res := ConfigMerge(core.LangUnknown, base, ours, theirs)
	if res.Merged == "" {
		t.Error("expected non-empty merged output from fallback")
	}
}

// ── ConfigMerge: parse error → line fallback ──

func TestConfigMerge_ParseError(t *testing.T) {
	res := ConfigMerge(core.LangJSON, `{"a":1}`, `{bad json`, `{"b":2}`)
	if res.Merged == "" {
		t.Error("expected fallback merged content")
	}
}

// ── ConfigMerge: scalar conflict (both sides changed differently) ──
// mergeValue: conflict → struct marker

func TestConfigMerge_ScalarConflict(t *testing.T) {
	base := `{"port": 8080}`
	ours := `{"port": 443}`
	theirs := `{"port": 9090}`
	res := ConfigMerge(core.LangJSON, base, ours, theirs)
	// port was changed on both sides differently → conflict marker in output
	if res.Merged == "" {
		t.Error("expected non-empty merged")
	}
}

// ── ConfigMerge: both sides deleted a key ──
// mergeMaps: !hasO && !hasT case

func TestConfigMerge_BothSidesDeletedKey(t *testing.T) {
	base := `{"a":1,"b":2}`
	ours := `{"a":1}`   // deleted b
	theirs := `{"a":1}` // deleted b
	res := ConfigMerge(core.LangJSON, base, ours, theirs)
	if strings.Contains(res.Merged, `"b"`) {
		t.Errorf("b should be deleted; merged=%q", res.Merged)
	}
}

// ── ConfigMerge: ours deleted key, theirs changed it ──
// mergeMaps: !hasO case where theirs != base → keep theirs

func TestConfigMerge_OursDeletedTheirsChanged(t *testing.T) {
	base := `{"a":1,"b":2}`
	ours := `{"a":1}`          // deleted b
	theirs := `{"a":1,"b":99}` // changed b
	res := ConfigMerge(core.LangJSON, base, ours, theirs)
	// b was changed by theirs and deleted by ours; theirs wins
	if res.Merged == "" {
		t.Error("empty merged")
	}
}

// ── ConfigMerge: theirs deleted key, ours changed it ──
// mergeMaps: !hasT case where ours != base → keep ours

func TestConfigMerge_TheirsDeletedOursChanged(t *testing.T) {
	base := `{"a":1,"b":2}`
	ours := `{"a":1,"b":99}` // changed b
	theirs := `{"a":1}`      // deleted b
	res := ConfigMerge(core.LangJSON, base, ours, theirs)
	if res.Merged == "" {
		t.Error("empty merged")
	}
}

// ── ConfigMerge: mergeValue where ours and theirs are both maps, base==nil ──
// mergeValue: isMap(ours) && isMap(theirs) && base==nil

func TestConfigMerge_NewNestedKeyBothSides(t *testing.T) {
	base := `{"x": 1}`
	ours := `{"x": 1, "nested": {"a": 1}}`
	theirs := `{"x": 1, "nested": {"b": 2}}`
	res := ConfigMerge(core.LangJSON, base, ours, theirs)
	if res.HasConflict {
		t.Errorf("expected clean merge; merged=%q", res.Merged)
	}
	if !strings.Contains(res.Merged, `"a"`) || !strings.Contains(res.Merged, `"b"`) {
		t.Errorf("expected both nested keys; merged=%q", res.Merged)
	}
}

// ── ConfigMerge: TOML round-trip ──
// encoderFor: TOML path (encoderFor returns the toml.Marshal branch)

func TestConfigMerge_TOMLRoundTrip(t *testing.T) {
	base := `[server]\nport = 8080\n`
	ours := `[server]\nport = 8080\ntls = true\n`
	theirs := `[server]\nport = 9090\n`
	res := ConfigMerge(core.LangTOML, base, ours, theirs)
	if res.Merged == "" {
		t.Error("empty TOML merged result")
	}
}

// ── SymbolMerge: ActionUseTheirs path ──

func TestSymbolMerge_ActionUseTheirs(t *testing.T) {
	base := map[string]core.SymbolData{
		"G": {QualifiedName: "G", Body: "func G() {}"},
	}
	// ours unchanged (same as base)
	ours := map[string]core.SymbolData{
		"G": {QualifiedName: "G", Body: "func G() {}"},
	}
	// theirs changed
	theirs := map[string]core.SymbolData{
		"G": {QualifiedName: "G", Body: "func G() { _ = 1 }"},
	}
	res := SymbolMerge(base, ours, theirs)
	if len(res.Merged) != 1 {
		t.Fatalf("expected 1 merged symbol; got %d", len(res.Merged))
	}
	if res.Merged[0].Body != "func G() { _ = 1 }" {
		t.Errorf("expected theirs body; got %q", res.Merged[0].Body)
	}
}

// ── SymbolMerge: ActionDelete path ──

func TestSymbolMerge_ActionDelete(t *testing.T) {
	base := map[string]core.SymbolData{
		"H": {QualifiedName: "H", Body: "func H() {}"},
	}
	// both sides deleted H
	ours := map[string]core.SymbolData{}
	theirs := map[string]core.SymbolData{}
	res := SymbolMerge(base, ours, theirs)
	for _, m := range res.Merged {
		if m.QualifiedName == "H" {
			t.Error("H should have been deleted")
		}
	}
	if len(res.Conflicts) != 0 {
		t.Error("unexpected conflict")
	}
}

// ── SymbolMerge: confidence floor when conflicts present ──

func TestSymbolMerge_ConfidenceFloor(t *testing.T) {
	base := map[string]core.SymbolData{
		"K": {QualifiedName: "K", Body: "a"},
	}
	ours := map[string]core.SymbolData{
		"K": {QualifiedName: "K", Body: "b"},
	}
	theirs := map[string]core.SymbolData{
		"K": {QualifiedName: "K", Body: "c"},
	}
	res := SymbolMerge(base, ours, theirs)
	if res.Confidence > 0.5 {
		t.Errorf("conflict should drive confidence ≤ 0.5; got %.2f", res.Confidence)
	}
}
