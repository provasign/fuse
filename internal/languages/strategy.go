// Package languages defines the LanguageStrategy interface and a registry of
// per-language symbol/import/export extractors used by the merge pipeline.
package languages

import (
	"sync"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// MergeCapabilities is re-exported from core for convenience.
type MergeCapabilities = core.MergeCapabilities

// Strategy is implemented by every supported language.
type Strategy interface {
	Language() core.LanguageKey
	Extensions() []string
	Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error)
	ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error)
	ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error)
	Capabilities() MergeCapabilities
}

// Registry holds Strategy implementations keyed by LanguageKey.
type Registry struct {
	mu sync.RWMutex
	m  map[core.LanguageKey]Strategy
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[core.LanguageKey]Strategy)}
}

// Register adds a strategy. The TSX strategy is typically registered as an
// alias under both LangTypeScript and LangTSX by the caller.
func (r *Registry) Register(s Strategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[s.Language()] = s
}

// RegisterAs adds a strategy under an additional language key (used for
// JS/JSX, TS/TSX aliasing).
func (r *Registry) RegisterAs(key core.LanguageKey, s Strategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key] = s
}

// Get returns the strategy for lang or nil if none.
func (r *Registry) Get(lang core.LanguageKey) Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[lang]
}

// All returns all registered strategies in stable order.
func (r *Registry) All() []Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Strategy, 0, len(r.m))
	seen := map[core.LanguageKey]bool{}
	for _, lang := range []core.LanguageKey{
		core.LangGo, core.LangTypeScript, core.LangTSX, core.LangJavaScript,
		core.LangPython, core.LangJava, core.LangRust,
	} {
		if s, ok := r.m[lang]; ok && !seen[s.Language()] {
			out = append(out, s)
			seen[s.Language()] = true
		}
	}
	return out
}
