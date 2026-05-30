// Package parser provides in-memory Tree-sitter parsing for the merge
// pipeline. Unlike Grove's parser (which indexes files on disk), this engine
// parses three in-memory versions of a file (base, ours, theirs) during a
// single merge operation.
package parser

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	tstsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	tstype "github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

const parseTimeout = 2 * time.Second

// Engine wraps tree-sitter parsers for the languages Fuse supports.
type Engine struct{}

func NewEngine() *Engine { return &Engine{} }

// DetectLanguage returns the language key for path. Optional content hint
// is used for shebang detection in extensionless files.
func DetectLanguage(path, _ string) core.LanguageKey {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return core.LangGo
	case ".ts":
		return core.LangTypeScript
	case ".tsx":
		return core.LangTSX
	case ".js", ".jsx", ".mjs", ".cjs":
		return core.LangJavaScript
	case ".py":
		return core.LangPython
	case ".java":
		return core.LangJava
	case ".rs":
		return core.LangRust
	case ".json":
		return core.LangJSON
	case ".yaml", ".yml":
		return core.LangYAML
	case ".toml":
		return core.LangTOML
	default:
		return core.LangUnknown
	}
}

// Parse parses src under the given language and returns a tree-sitter tree
// (caller must Close it) or nil for non-AST languages (JSON/YAML/TOML).
//
// Returns (nil, nil) if the language has no tree-sitter grammar (config
// formats); returns (nil, err) only on parser failure for AST languages.
func (e *Engine) Parse(lang core.LanguageKey, src []byte) (*sitter.Tree, error) {
	tsLang, ok := treeSitterLanguage(lang)
	if !ok {
		return nil, nil
	}
	p := sitter.NewParser()
	p.SetLanguage(tsLang)
	ctx, cancel := context.WithTimeout(context.Background(), parseTimeout)
	defer cancel()
	tree, err := p.ParseCtx(ctx, nil, src)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func treeSitterLanguage(lang core.LanguageKey) (*sitter.Language, bool) {
	switch lang {
	case core.LangGo:
		return golang.GetLanguage(), true
	case core.LangTypeScript:
		return tstype.GetLanguage(), true
	case core.LangTSX:
		return tstsx.GetLanguage(), true
	case core.LangJavaScript:
		return javascript.GetLanguage(), true
	case core.LangPython:
		return python.GetLanguage(), true
	case core.LangJava:
		return java.GetLanguage(), true
	case core.LangRust:
		return rust.GetLanguage(), true
	default:
		return nil, false
	}
}

// IsAST reports whether the language is parsed via tree-sitter (true) or via
// a structured data merge (false: JSON/YAML/TOML).
func IsAST(lang core.LanguageKey) bool {
	_, ok := treeSitterLanguage(lang)
	return ok
}

// IsConfig reports whether the language is a config/data format handled by
// the deep-merge strategy.
func IsConfig(lang core.LanguageKey) bool {
	return lang == core.LangJSON || lang == core.LangYAML || lang == core.LangTOML
}

// Supported reports whether Fuse can merge files of this language at all.
func Supported(lang core.LanguageKey) bool {
	return IsAST(lang) || IsConfig(lang)
}
