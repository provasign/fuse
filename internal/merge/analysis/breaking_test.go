package analysis

import (
	"context"
	"testing"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/grove"
)

type fakeGrove struct {
	impact []grove.ImpactNode
	deps   []grove.Edge
}

func (f *fakeGrove) Impact(_ context.Context, _ string, _ int) ([]grove.ImpactNode, error) {
	return f.impact, nil
}
func (f *fakeGrove) Deps(_ context.Context, _ string) ([]grove.Edge, error) {
	return f.deps, nil
}

func TestAnalyzeRemovedExport(t *testing.T) {
	g := &fakeGrove{impact: []grove.ImpactNode{
		{ID: "1", FilePath: "a.go", Name: "callerA"},
		{ID: "2", FilePath: "b.go", Name: "callerB"},
	}}
	a := &BreakingChangeAnalyzer{Grove: g}
	base := []core.SymbolData{{QualifiedName: "Foo", Name: "Foo", Exported: true, Signature: "func Foo()"}}
	ours := []core.SymbolData{}
	theirs := []core.SymbolData{{QualifiedName: "Foo", Name: "Foo", Exported: true, Signature: "func Foo()"}}
	changes := a.Analyze(context.Background(), "x.go", base, ours, theirs)
	if len(changes) == 0 {
		t.Fatal("expected breaking change")
	}
	if changes[0].Kind != "removed_export" {
		t.Errorf("got %s", changes[0].Kind)
	}
}

func TestAnalyzeRemovedExportedMethod(t *testing.T) {
	g := &fakeGrove{impact: []grove.ImpactNode{{ID: "1", FilePath: "invoice_test.go", Name: "TestRiskBand"}}}
	a := &BreakingChangeAnalyzer{Grove: g}
	base := []core.SymbolData{{QualifiedName: "RiskBand", ParentName: "Invoice", Name: "RiskBand", Exported: true, Signature: "func (invoice Invoice) RiskBand() string"}}
	ours := []core.SymbolData{{QualifiedName: "riskBand", ParentName: "Invoice", Name: "riskBand", Exported: false, Signature: "func (invoice Invoice) riskBand() string"}}
	theirs := ours
	changes := a.Analyze(context.Background(), "invoice.go", base, ours, theirs)
	if len(changes) == 0 {
		t.Fatal("expected exported method removal")
	}
	if changes[0].Symbol != "Invoice.RiskBand" {
		t.Fatalf("symbol = %q, want Invoice.RiskBand", changes[0].Symbol)
	}
}

func TestAnalyzeSignatureChanged(t *testing.T) {
	g := &fakeGrove{impact: []grove.ImpactNode{{ID: "1", FilePath: "a.go", Name: "c"}}}
	a := &BreakingChangeAnalyzer{Grove: g}
	base := []core.SymbolData{{QualifiedName: "Foo", Name: "Foo", Exported: true, Signature: "func Foo(x int)"}}
	ours := []core.SymbolData{{QualifiedName: "Foo", Name: "Foo", Exported: true, Signature: "func Foo(x string)"}}
	theirs := []core.SymbolData{{QualifiedName: "Foo", Name: "Foo", Exported: true, Signature: "func Foo(x float64)"}}
	changes := a.Analyze(context.Background(), "x.go", base, ours, theirs)
	found := false
	for _, c := range changes {
		if c.Kind == "signature_changed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected signature_changed, got %+v", changes)
	}
}

func TestBlastRadiusNilGrove(t *testing.T) {
	out := BlastRadius(context.Background(), nil, "foo", 3)
	if out != nil {
		t.Errorf("expected nil, got %v", out)
	}
}
