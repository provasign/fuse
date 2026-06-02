package parser

import (
	"testing"

	"github.com/provasign/fuse/internal/core"
)

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path string
		want core.LanguageKey
	}{
		{"foo.go", core.LangGo},
		{"foo.ts", core.LangTypeScript},
		{"foo.tsx", core.LangTSX},
		{"foo.js", core.LangJavaScript},
		{"foo.jsx", core.LangJavaScript},
		{"foo.mjs", core.LangJavaScript},
		{"foo.cjs", core.LangJavaScript},
		{"foo.py", core.LangPython},
		{"foo.java", core.LangJava},
		{"foo.rs", core.LangRust},
		{"foo.json", core.LangJSON},
		{"foo.yaml", core.LangYAML},
		{"foo.yml", core.LangYAML},
		{"foo.toml", core.LangTOML},
		{"foo.txt", core.LangUnknown},
	}
	for _, c := range cases {
		got := DetectLanguage(c.path, "")
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.path, got, c.want)
		}
	}
}

func TestIsAST_IsConfig(t *testing.T) {
	if !IsAST(core.LangGo) {
		t.Error("Go should be AST")
	}
	if IsAST(core.LangJSON) {
		t.Error("JSON should not be AST")
	}
	if !IsConfig(core.LangYAML) {
		t.Error("YAML should be config")
	}
	if IsConfig(core.LangGo) {
		t.Error("Go should not be config")
	}
}

func TestParseGo(t *testing.T) {
	e := NewEngine()
	src := []byte("package x\nfunc Foo() {}\n")
	tree, err := e.Parse(core.LangGo, src)
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("nil tree")
	}
	defer tree.Close()
}
