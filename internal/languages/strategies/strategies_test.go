package strategies_test

import (
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/languages"
	"github.com/tabladrum/grove-suite/fuse/internal/languages/strategies"
	"github.com/tabladrum/grove-suite/fuse/internal/parser"
)

func extract(t *testing.T, s languages.Strategy, src string) []core.SymbolData {
	t.Helper()
	eng := parser.NewEngine()
	tree, err := eng.Parse(s.Language(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	syms, err := s.Extract(tree, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return syms
}

func extractImps(t *testing.T, s languages.Strategy, src string) []core.ImportStatement {
	t.Helper()
	eng := parser.NewEngine()
	tree, err := eng.Parse(s.Language(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	imps, _ := s.ExtractImports(tree, []byte(src))
	return imps
}

func keys(syms []core.SymbolData) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Key)
	}
	return out
}

func containsAll(haystack, needles []string) bool {
	have := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		have[h] = true
	}
	for _, n := range needles {
		if !have[n] {
			return false
		}
	}
	return true
}

func TestGoSymbols(t *testing.T) {
	s := strategies.NewGo()
	src := `package x

import "fmt"

type Foo struct{ A int }

func (f *Foo) Bar() int { return f.A }

func Baz() string { return "hi" }

const X = 1
`
	syms := extract(t, s, src)
	if !containsAll(keys(syms), []string{"Foo", "Foo.Bar", "Baz", "X"}) {
		t.Errorf("got keys %v", keys(syms))
	}
	imps := extractImps(t, s, src)
	if len(imps) != 1 || imps[0].Path != "fmt" {
		t.Errorf("imports = %+v", imps)
	}
}

func TestPythonSymbols(t *testing.T) {
	s := strategies.NewPython()
	src := strings.Join([]string{
		"import os",
		"def foo():",
		"    return 1",
		"class Bar:",
		"    def baz(self):",
		"        return 2",
		"_private = 1",
	}, "\n")
	syms := extract(t, s, src)
	if !containsAll(keys(syms), []string{"foo", "Bar", "Bar.baz"}) {
		t.Errorf("got keys %v", keys(syms))
	}
}

func TestTypeScriptSymbols(t *testing.T) {
	s := strategies.NewTypeScript(false)
	src := `import { x } from "./m";
export function f(): number { return 1; }
export class C { m() { return 2; } }
export interface I { x: number; }
`
	syms := extract(t, s, src)
	if !containsAll(keys(syms), []string{"f", "C", "C.m", "I"}) {
		t.Errorf("got keys %v", keys(syms))
	}
	imps := extractImps(t, s, src)
	if len(imps) != 1 || imps[0].Path != "./m" {
		t.Errorf("imports = %+v", imps)
	}
}

func TestJavaSymbols(t *testing.T) {
	s := strategies.NewJava()
	src := `package p;
public class Foo {
  public int x;
  public int bar() { return x; }
}
`
	syms := extract(t, s, src)
	if !containsAll(keys(syms), []string{"Foo", "Foo.bar", "Foo.x"}) {
		t.Errorf("got keys %v", keys(syms))
	}
}

func TestRustSymbols(t *testing.T) {
	s := strategies.NewRust()
	src := `pub fn foo() -> i32 { 1 }
struct Bar;
impl Bar { fn baz(&self) -> i32 { 2 } }
`
	syms := extract(t, s, src)
	if !containsAll(keys(syms), []string{"foo", "Bar"}) {
		t.Errorf("got keys %v", keys(syms))
	}
}
