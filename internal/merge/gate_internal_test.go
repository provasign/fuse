package merge

import (
	"testing"

	"github.com/provasign/fuse/internal/core"
)

func TestParsesClean(t *testing.T) {
	im := New(nil)
	if !im.parsesClean(core.LangGo, []byte("package x\n\nfunc A() {}\n")) {
		t.Error("valid Go reported as not clean")
	}
	if im.parsesClean(core.LangGo, []byte("package x\n\nfunc A() {{{\n")) {
		t.Error("invalid Go reported as clean")
	}
}

// tree-sitter's Go grammar accepts a `const` keyword repeated inside a const
// block; the stdlib parser must catch it.
func TestParsesCleanRejectsKeywordInsideConstBlock(t *testing.T) {
	im := New(nil)
	src := "package x\n\nconst (\nconst A = 1\n)\n"
	if im.parsesClean(core.LangGo, []byte(src)) {
		t.Error("duplicated const keyword inside block reported as clean")
	}
}

func TestTreeHasErrorNil(t *testing.T) {
	if !treeHasError(nil) {
		t.Error("nil tree must count as erroneous")
	}
}
