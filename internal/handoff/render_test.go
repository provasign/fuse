package handoff

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestWriteAndRenderMarkdown_Full(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGenerator(dir)
	if err != nil {
		t.Fatal(err)
	}
	in := PromptInputs{
		FilePath:     "x.go",
		Language:     core.LangGo,
		ConflictType: core.ConflictStructural,
		Severity:     core.SeverityHigh,
		Confidence:   0.75,
		Conflicts: []core.SymbolConflict{
			{
				Key:    "Foo",
				Base:   core.SymbolData{Body: "func Foo() {}"},
				Ours:   core.SymbolData{Body: "func Foo() { _ = 1 }"},
				Theirs: core.SymbolData{Body: "func Foo() { _ = 2 }"},
			},
		},
		BreakingChanges: []core.BreakingChange{
			{Severity: core.SeverityHigh, Kind: "removed", Message: "Foo removed", AffectedFiles: []string{"a.go"}},
		},
		Dependencies: []string{"fmt"},
		Dependents:   []string{"main.go"},
	}
	path, err := g.Write(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "conflict-") {
		t.Errorf("path: %s", path)
	}
	data, _ := os.ReadFile(path)
	body := string(data)
	for _, w := range []string{"Foo", "BASE", "OURS", "THEIRS", "Dependencies", "main.go", "removed"} {
		if !strings.Contains(body, w) {
			t.Errorf("md missing %q", w)
		}
	}
	// JSON sibling exists
	jsonPath := strings.TrimSuffix(path, ".md") + ".json"
	if _, err := os.Stat(jsonPath); err != nil {
		t.Error(err)
	}
}

func TestWrite_ReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce read-only directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	g := &Generator{OutputDir: dir}
	if _, err := g.Write(PromptInputs{FilePath: "x"}); err == nil {
		t.Error("expected write error on read-only dir")
	}
}
