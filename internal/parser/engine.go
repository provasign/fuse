// Package parser provides in-memory Tree-sitter parsing for the merge
// pipeline. It is a thin wrapper around astkit's parser engine.
package parser

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/provasign/astkit"
	"github.com/provasign/fuse/internal/core"
)

// Engine wraps astkit's parser engine for Fuse's in-memory three-way merge.
type Engine struct {
	inner *astkit.Engine
}

func NewEngine() *Engine { return &Engine{inner: astkit.NewEngine()} }

// DetectLanguage returns the language key for path.
func DetectLanguage(path, content string) core.LanguageKey {
	return astkit.DetectLanguage(path, content)
}

// Parse parses src under the given language and returns a tree-sitter tree
// (caller must Close it) or nil for non-AST languages (JSON/YAML/TOML).
func (e *Engine) Parse(lang core.LanguageKey, src []byte) (*sitter.Tree, error) {
	return e.inner.Parse(context.Background(), lang, src)
}

// IsAST reports whether the language is parsed via tree-sitter.
func IsAST(lang core.LanguageKey) bool { return astkit.IsAST(lang) }

// IsConfig reports whether the language is a config/data format handled by
// the deep-merge strategy.
func IsConfig(lang core.LanguageKey) bool { return astkit.IsConfigData(lang) }

// Supported reports whether Fuse can merge files of this language at all.
func Supported(lang core.LanguageKey) bool { return IsAST(lang) || IsConfig(lang) }
