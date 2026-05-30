package merge

import (
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestCombineConfidence(t *testing.T) {
	if got := combineConfidence(0, 0.5); got != 0 {
		t.Errorf("zero: %v", got)
	}
	if got := combineConfidence(0.5, 0); got != 0 {
		t.Errorf("zero second: %v", got)
	}
	if got := combineConfidence(0.4, 0.6); got != 0.5 {
		t.Errorf("avg: %v", got)
	}
}

func TestCountDelta(t *testing.T) {
	from := []core.ImportStatement{{Path: "a"}, {Path: "b"}}
	to := []core.ImportStatement{{Path: "a"}, {Path: "c"}, {Path: "d"}}
	if got := countDelta(from, to); got != 2 {
		t.Errorf("got %d want 2", got)
	}
	if got := countDelta(from, from); got != 0 {
		t.Errorf("equal: %d", got)
	}
	if got := countDelta(nil, to); got != 3 {
		t.Errorf("nil from: %d", got)
	}
}
