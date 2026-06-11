package merge_test

import (
	"context"
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/merge"
)

// Phase-by-phase orchestrator coverage.

func TestMergeIdenticalIsClean(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	src := "package x\nfunc Foo() {}\n"
	res, err := im.Merge(context.Background(), []byte(src), []byte(src), []byte(src), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Strategy != core.StrategyClean {
		t.Errorf("got %v", res.Strategy)
	}
	if res.HasConflict {
		t.Error("unexpected conflict")
	}
}

func TestMergeOursOnlyChange(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := "package x\nfunc A() {}\n"
	ours := "package x\nfunc A() { _ = 1 }\n"
	theirs := base
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.MergedContent, "_ = 1") {
		t.Errorf("expected ours-side body, got: %s", res.MergedContent)
	}
}

func TestMergeNonOverlappingSymbolChanges(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := "package x\n\nfunc A() {}\n\nfunc B() {}\n"
	ours := "package x\n\nfunc A() { _ = 1 }\n\nfunc B() {}\n"
	theirs := "package x\n\nfunc A() {}\n\nfunc B() { _ = 2 }\n"
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Errorf("unexpected conflict: %s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "_ = 1") || !strings.Contains(res.MergedContent, "_ = 2") {
		t.Errorf("expected both changes in merged output:\n%s", res.MergedContent)
	}
}

func TestMergeConfigYAMLDeepMerge(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := "a: 1\nb: 2\n"
	ours := "a: 1\nb: 2\nc: 3\n"
	theirs := "a: 1\nb: 2\nd: 4\n"
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangYAML, "x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Errorf("unexpected conflict: %s", res.MergedContent)
	}
	for _, k := range []string{"a:", "b:", "c:", "d:"} {
		if !strings.Contains(res.MergedContent, k) {
			t.Errorf("missing %s in:\n%s", k, res.MergedContent)
		}
	}
}

func TestMergeUnsupportedLanguageFallsBackToLineMerge(t *testing.T) {
	im := merge.New(nil)
	base := "line1\nline2\nline3\n"
	ours := "line1\nline2-ours\nline3\n"
	theirs := "line1\nline2\nline3-theirs\n"
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangUnknown, "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if res.Strategy != core.StrategyLine {
		t.Errorf("expected line strategy, got %v", res.Strategy)
	}
	if !strings.Contains(res.MergedContent, "line2-ours") || !strings.Contains(res.MergedContent, "line3-theirs") {
		t.Errorf("expected both modifications:\n%s", res.MergedContent)
	}
}

func TestMergePythonClassWithIndependentMethodChanges(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := "class C:\n    def a(self):\n        return 1\n    def b(self):\n        return 2\n"
	ours := "class C:\n    def a(self):\n        return 10\n    def b(self):\n        return 2\n"
	theirs := "class C:\n    def a(self):\n        return 1\n    def b(self):\n        return 20\n"
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangPython, "x.py")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected clean merge via symbol-body line merge, got conflict:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 10") || !strings.Contains(res.MergedContent, "return 20") {
		t.Errorf("expected both method edits in merged output:\n%s", res.MergedContent)
	}
	if res.Stats.SymbolsBase == 0 || res.Stats.SymbolsOurs == 0 || res.Stats.SymbolsTheirs == 0 {
		t.Errorf("missing symbol stats: %+v", res.Stats)
	}
}

func TestMergeTimingRecorded(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	res, _ := im.Merge(context.Background(), []byte("package x\n"), []byte("package x\nfunc A(){}\n"), []byte("package x\n"), core.LangGo, "x.go")
	if res.Stats.TimingMs < 0 {
		t.Errorf("bad timing: %d", res.Stats.TimingMs)
	}
}
