package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestGeneratorWrite(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGenerator(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, err := g.Write(PromptInputs{
		FilePath:     "x.go",
		Language:     core.LangGo,
		ConflictType: core.ConflictStructural,
		Severity:     core.SeverityHigh,
		Confidence:   0.2,
		Conflicts: []core.SymbolConflict{
			{Key: "Foo", Base: core.SymbolData{Body: "func Foo(){return 1}"}, Ours: core.SymbolData{Body: "func Foo(){return 2}"}, Theirs: core.SymbolData{Body: "func Foo(){return 3}"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Errorf("expected .md path, got %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Foo") {
		t.Errorf("expected Foo in prompt body:\n%s", string(data))
	}
}

func TestAppendAudit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := AppendAudit(dir, core.AuditEntry{File: "x", Strategy: core.StrategySymbol}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"file":`) != 3 {
		t.Errorf("expected 3 entries, got %s", string(data))
	}
}
