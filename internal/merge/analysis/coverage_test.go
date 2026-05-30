package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/grove"
)

type fakeGroveCov struct {
	impactNodes []grove.ImpactNode
	impactErr   error
	depsEdges   []grove.Edge
	depsErr     error
}

func (f *fakeGroveCov) Impact(_ context.Context, _ string, _ int) ([]grove.ImpactNode, error) {
	return f.impactNodes, f.impactErr
}
func (f *fakeGroveCov) Deps(_ context.Context, _ string) ([]grove.Edge, error) {
	return f.depsEdges, f.depsErr
}

func TestBlastRadius(t *testing.T) {
	if got := BlastRadius(context.Background(), nil, "x", 3); got != nil {
		t.Errorf("nil grove should return nil: %v", got)
	}
	g := &fakeGroveCov{impactNodes: []grove.ImpactNode{
		{FilePath: "a.go"}, {FilePath: "b.go"}, {FilePath: "a.go"}, {FilePath: ""},
	}}
	got := BlastRadius(context.Background(), g, "X", 3)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("got %v", got)
	}
	g.impactErr = errors.New("boom")
	if got := BlastRadius(context.Background(), g, "X", 3); got != nil {
		t.Errorf("error should return nil, got %v", got)
	}
}

func TestDependents(t *testing.T) {
	if got := Dependents(context.Background(), nil, "x.go"); got != nil {
		t.Errorf("nil grove should return nil: %v", got)
	}
	g := &fakeGroveCov{depsEdges: []grove.Edge{
		{From: "a.go"}, {From: "b.go"}, {From: "a.go"}, {From: ""}, {From: "self.go"},
	}}
	got := Dependents(context.Background(), g, "self.go")
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("got %v", got)
	}
	g.depsErr = errors.New("x")
	if got := Dependents(context.Background(), g, "x"); got != nil {
		t.Errorf("error path: %v", got)
	}
}

func TestRemovedBy(t *testing.T) {
	cases := []struct {
		ours, theirs bool
		want         string
	}{
		{false, false, "both sides"},
		{false, true, "ours"},
		{true, false, "theirs"},
	}
	for _, c := range cases {
		if got := removedBy(c.ours, c.theirs); got != c.want {
			t.Errorf("(%v,%v) got %q want %q", c.ours, c.theirs, got, c.want)
		}
	}
}

func TestSeverityForCount(t *testing.T) {
	cases := []struct {
		n    int
		want core.ConflictSeverity
	}{
		{0, core.SeverityLow},
		{1, core.SeverityMedium},
		{3, core.SeverityHigh},
		{6, core.SeverityCritical},
		{10, core.SeverityCritical},
	}
	for _, c := range cases {
		if got := severityForCount(c.n); got != c.want {
			t.Errorf("n=%d got %v want %v", c.n, got, c.want)
		}
	}
}
