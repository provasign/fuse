package merge
package merge

import (
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestRenderImportBlock_Go(t *testing.T) {
	got := renderImportBlock([]core.ImportStatement{
		{Path: "fmt"},
		{Alias: "x", Path: "example.com/x"},
	}, core.LangGo)
	if !strings.Contains(got, `"fmt"`) || !strings.Contains(got, `x "example.com/x"`) {
		t.Errorf("go render: %q", got)
	}
}

func TestRenderImportBlock_Empty(t *testing.T) {
	if got := renderImportBlock(nil, core.LangGo); got != "" {
		t.Errorf("empty: %q", got)
	}
}

func TestRenderImportBlock_Fallback(t *testing.T) {
	got := renderImportBlock([]core.ImportStatement{
		{Raw: "import x"}, {Raw: "import y"},
	}, core.LangJavaScript)
	if !strings.Contains(got, "import x") || !strings.Contains(got, "import y") {
		t.Errorf("fallback: %q", got)
	}
}

func TestInjectImportsAfterPackageDecl_Go(t *testing.T) {
	lines := []string{
		"// header comment",
		"",
		"package x",
		"",
		"func f() {}",
	}
	out := injectImportsAfterPackageDecl(lines, `import ("fmt")`, core.LangGo)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, `import ("fmt")`) {
		t.Errorf("not injected: %q", joined)
	}
	// Must come after package decl
	if strings.Index(joined, "import") < strings.Index(joined, "package x") {
		t.Error("import injected before package")
	}
}

func TestInjectImportsAfterPackageDecl_NonGo(t *testing.T) {
	lines := []string{
		"// hello",
		"",
		"function f() {}",
	}
	out := injectImportsAfterPackageDecl(lines, `import x`, core.LangJavaScript)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "import x") {
		t.Errorf("not injected: %q", joined)
	}
}
