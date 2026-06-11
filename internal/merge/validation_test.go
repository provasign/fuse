package merge_test

import (
	"context"
	goparser "go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/merge"
)

// Two-sided edits to different methods of the same TS class must auto-resolve
// via the symbol-body line merge, not conflict at class granularity.
func TestMergeTSClassIndependentMethodChanges(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := `class Service {
  login(): number {
    return 1;
  }
  logout(): number {
    return 2;
  }
}
`
	ours := strings.Replace(base, "return 1;", "return 10;", 1)
	theirs := strings.Replace(base, "return 2;", "return 20;", 1)
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangTypeScript, "svc.ts")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected clean merge, got conflict:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 10;") || !strings.Contains(res.MergedContent, "return 20;") {
		t.Errorf("expected both method edits in merged output:\n%s", res.MergedContent)
	}
}

// Regression: symbols following a class must still receive their merged
// bodies. The old reconstruction cursor got stuck on nested method spans and
// silently emitted the ours-side text for everything after the class.
func TestMergeSymbolAfterClassPreserved(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := `class Box {
  get(): number {
    return 1;
  }
}

function standalone(): number {
  return 5;
}
`
	ours := strings.Replace(base, "return 1;", "return 11;", 1)
	theirs := strings.Replace(base, "return 5;", "return 55;", 1)
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangTypeScript, "box.ts")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected clean merge, got conflict:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 11;") {
		t.Errorf("ours class edit missing:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 55;") {
		t.Errorf("theirs edit to symbol after class was dropped:\n%s", res.MergedContent)
	}
}

// Go const blocks: theirs edits one member, ours edits a function. The
// merged file must keep the block syntax intact (no standalone `const X`
// substituted inside `const (...)`) and still be valid Go.
func TestMergeGoConstBlockMemberEdit(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := `package x

import "net/http"

const (
	A = "one"
	B = "two"
)

func F(r *http.Request) int { return 1 }
`
	ours := strings.Replace(base, "return 1", "return 10", 1)
	theirs := strings.Replace(base, `B = "two"`, `B = "TWO"`, 1)
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected clean merge:\n%s", res.MergedContent)
	}
	fset := token.NewFileSet()
	if _, perr := goparser.ParseFile(fset, "x.go", res.MergedContent, 0); perr != nil {
		t.Fatalf("merged output is not valid Go: %v\n%s", perr, res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 10") || !strings.Contains(res.MergedContent, `B = "TWO"`) {
		t.Errorf("expected both edits:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, `import "net/http"`) {
		t.Errorf("single-import style should be preserved when imports are unchanged:\n%s", res.MergedContent)
	}
}

// Both sides insert different functions at the same location — the classic
// parallel-agent collision. Line merge conflicts; symbol merge must resolve
// it, keep theirs' doc comment, and place theirs' addition near its original
// neighbor rather than at end-of-file.
func TestMergeBothAddFunctionsSamePlace(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := `package x

func A() int { return 1 }

func Z() int { return 26 }
`
	ours := `package x

func A() int { return 1 }

func FromOurs() int { return 2 }

func Z() int { return 26 }
`
	theirs := `package x

func A() int { return 1 }

// FromTheirs has a doc comment that must survive the merge.
func FromTheirs() int { return 3 }

func Z() int { return 26 }
`
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected symbol merge to resolve same-place additions:\n%s", res.MergedContent)
	}
	for _, want := range []string{"FromOurs", "FromTheirs", "doc comment that must survive"} {
		if !strings.Contains(res.MergedContent, want) {
			t.Errorf("missing %q in merged output:\n%s", want, res.MergedContent)
		}
	}
	if strings.Index(res.MergedContent, "FromTheirs") > strings.Index(res.MergedContent, "func Z()") {
		t.Errorf("theirs' addition should be anchored near its neighbor, not appended after Z:\n%s", res.MergedContent)
	}
	fset := token.NewFileSet()
	if _, perr := goparser.ParseFile(fset, "x.go", res.MergedContent, 0); perr != nil {
		t.Fatalf("merged output is not valid Go: %v\n%s", perr, res.MergedContent)
	}
}

// A clean symbol merge must produce output that still parses; the pipeline
// re-parses merged output and falls back to line merge otherwise. Here we
// just assert the invariant holds end-to-end for a Go merge.
func TestMergeOutputParsesCleanly(t *testing.T) {
	im := merge.New(nil)
	im.EnableBreaking = false
	base := "package x\n\nfunc A() int { return 1 }\n\nfunc B() int { return 2 }\n"
	ours := strings.Replace(base, "return 1", "return 10", 1)
	theirs := strings.Replace(base, "return 2", "return 20", 1)
	res, err := im.Merge(context.Background(), []byte(base), []byte(ours), []byte(theirs), core.LangGo, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("expected clean merge:\n%s", res.MergedContent)
	}
	tree, perr := im.Parser.Parse(core.LangGo, []byte(res.MergedContent))
	if perr != nil || tree == nil {
		t.Fatalf("merged output failed to parse: %v", perr)
	}
	defer tree.Close()
	if tree.RootNode().HasError() {
		t.Errorf("merged output has syntax errors:\n%s", res.MergedContent)
	}
}
