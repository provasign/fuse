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

func TestTreeHasErrorNil(t *testing.T) {
	if !treeHasError(nil) {
		t.Error("nil tree must count as erroneous")
	}
}
