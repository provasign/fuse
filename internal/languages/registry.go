package languages

import (
	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/languages/strategies"
)

// DefaultRegistry returns a Registry pre-populated with every language Fuse
// supports for symbol-level merging (Go, TS, TSX, JS, Python, Java, Rust).
// Config formats (JSON/YAML/TOML) do not need a strategy; they are merged
// structurally.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(strategies.NewGo())

	ts := strategies.NewTypeScript(false)
	r.RegisterAs(core.LangTypeScript, ts)

	tsx := strategies.NewTypeScript(true)
	r.RegisterAs(core.LangTSX, tsx)

	r.RegisterAs(core.LangJavaScript, strategies.NewJavaScript())

	r.Register(strategies.NewPython())
	r.Register(strategies.NewJava())
	r.Register(strategies.NewRust())
	return r
}
