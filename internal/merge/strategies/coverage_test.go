package strategies

import (
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestSymbolsToMap(t *testing.T) {
	syms := []core.SymbolData{
		{QualifiedName: "A"}, {QualifiedName: "B"}, {QualifiedName: "A"},
	}
	m := SymbolsToMap(syms)
	if len(m) != 2 {
		t.Errorf("got %d, want 2", len(m))
	}
	if _, ok := m["A"]; !ok {
		t.Error("missing A")
	}
}

func TestGroupOrder(t *testing.T) {
	cases := []struct {
		g    string
		want int
	}{
		{"stdlib", 0},
		{"external", 1},
		{"relative", 2},
		{"other", 3},
	}
	for _, c := range cases {
		if got := groupOrder(c.g); got != c.want {
			t.Errorf("%s got %d want %d", c.g, got, c.want)
		}
	}
}

func TestParserAndEncoderForAllConfigLangs(t *testing.T) {
	for _, lang := range []core.LanguageKey{core.LangJSON, core.LangYAML, core.LangTOML} {
		p := parserFor(lang)
		e := encoderFor(lang)
		if p == nil || e == nil {
			t.Errorf("%s parser/encoder missing", lang)
			continue
		}
		// Empty string returns nil, nil
		if _, err := p(""); err != nil {
			t.Errorf("%s empty: %v", lang, err)
		}
		// Bad input returns error
		if _, err := p("\x00garbage"); err == nil {
			t.Errorf("%s expected parse error", lang)
		}
		// Encoder round-trip
		out, err := e(map[string]any{"k": "v"})
		if err != nil || out == "" {
			t.Errorf("%s encode: %v out=%q", lang, err, out)
		}
	}
}

func TestParserFor_Unknown(t *testing.T) {
	if parserFor(core.LangGo) != nil {
		t.Error("go should not have config parser")
	}
	if encoderFor(core.LangGo) != nil {
		t.Error("go should not have config encoder")
	}
}

func TestNormalizeMap(t *testing.T) {
	in := map[any]any{
		"k": map[any]any{"k2": 1},
		"l": []any{map[any]any{"x": 1}},
	}
	out := normalizeMap(in)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("not converted: %T", out)
	}
	if _, ok := m["k"].(map[string]any); !ok {
		t.Error("nested not converted")
	}
	// pass-through for non-map non-slice
	if v := normalizeMap(42); v != 42 {
		t.Errorf("scalar got %v", v)
	}
	// passthrough for map[string]any
	if v := normalizeMap(map[string]any{"a": map[any]any{"b": 1}}); v == nil {
		t.Error("got nil")
	}
}
