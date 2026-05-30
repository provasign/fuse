package strategies

import (
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func symCov(name, body string) *core.SymbolData {
	return &core.SymbolData{QualifiedName: name, Body: body}
}

func TestThreeWaySymbol_AllBranches(t *testing.T) {
	cases := []struct {
		name             string
		base, ours, them *core.SymbolData
		want             MergeAction
	}{
		{"all-nil", nil, nil, nil, ActionKeep},
		{"both-deleted-since-base", symCov("x", "a"), nil, nil, ActionDelete},
		{"added-by-ours", nil, symCov("x", "a"), nil, ActionUseOurs},
		{"added-by-theirs", nil, nil, symCov("x", "a"), ActionUseTheirs},
		{"added-both-converged", nil, symCov("x", "a"), symCov("x", "a"), ActionConverged},
		{"added-both-divergent", nil, symCov("x", "a"), symCov("x", "b"), ActionConflict},
		{"ours-removed-theirs-same", symCov("x", "a"), nil, symCov("x", "a"), ActionDelete},
		{"ours-removed-theirs-mod", symCov("x", "a"), nil, symCov("x", "b"), ActionConflict},
		{"theirs-removed-ours-same", symCov("x", "a"), symCov("x", "a"), nil, ActionDelete},
		{"theirs-removed-ours-mod", symCov("x", "a"), symCov("x", "b"), nil, ActionConflict},
		{"all-converged", symCov("x", "a"), symCov("x", "b"), symCov("x", "b"), ActionKeep},
		{"ours-unchanged", symCov("x", "a"), symCov("x", "a"), symCov("x", "c"), ActionUseTheirs},
		{"theirs-unchanged", symCov("x", "a"), symCov("x", "c"), symCov("x", "a"), ActionUseOurs},
		{"three-way-conflict", symCov("x", "a"), symCov("x", "b"), symCov("x", "c"), ActionConflict},
	}
	for _, c := range cases {
		if got := ThreeWaySymbol(c.base, c.ours, c.them); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestDeref(t *testing.T) {
	if got := deref(nil); got.QualifiedName != "" {
		t.Error("nil deref")
	}
	s := &core.SymbolData{QualifiedName: "x"}
	if got := deref(s); got.QualifiedName != "x" {
		t.Errorf("got %v", got)
	}
}

func TestAsMap_Variants(t *testing.T) {
	if asMap(map[string]any{"a": 1}) == nil {
		t.Error("string map")
	}
	if asMap(map[any]any{"a": 1}) == nil {
		t.Error("any map")
	}
	// non-map -> nil
	if asMap("not-a-map") != nil {
		t.Error("string should be nil")
	}
}
