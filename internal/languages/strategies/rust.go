package strategies

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// Rust strategy.
type rustStrategy struct{}

func NewRust() *rustStrategy { return &rustStrategy{} }

func (r *rustStrategy) Language() core.LanguageKey { return core.LangRust }
func (r *rustStrategy) Extensions() []string       { return []string{".rs"} }
func (r *rustStrategy) Capabilities() core.MergeCapabilities {
	return core.MergeCapabilities{SupportsSymbolMerge: true, SupportsImportMerge: true}
}

func (r *rustStrategy) Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var out []core.SymbolData
	walkChildren(root, func(n *sitter.Node) {
		out = append(out, rustItem(n, src, "")...)
	})
	return out, nil
}

func rustItem(n *sitter.Node, src []byte, parent string) []core.SymbolData {
	exported := isRustPub(n, src)
	switch n.Type() {
	case "function_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		key := name
		if parent != "" {
			key = parent + "." + name
		}
		return []core.SymbolData{{
			Key: key, Kind: "function", Name: name, ParentKey: parent,
			Signature: rustFuncSignature(n, src),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "struct_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		return []core.SymbolData{{
			Key: name, Kind: "struct", Name: name,
			Signature: extractFirstLine(nodeText(n, src)),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "enum_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		return []core.SymbolData{{
			Key: name, Kind: "enum", Name: name,
			Signature: extractFirstLine(nodeText(n, src)),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "trait_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		return []core.SymbolData{{
			Key: name, Kind: "trait", Name: name,
			Signature: extractFirstLine(nodeText(n, src)),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "type_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		return []core.SymbolData{{
			Key: name, Kind: "type", Name: name,
			Signature: extractFirstLine(nodeText(n, src)),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "const_item", "static_item":
		name := nodeText(n.ChildByFieldName("name"), src)
		return []core.SymbolData{{
			Key: name, Kind: "const", Name: name,
			Signature: extractFirstLine(nodeText(n, src)),
			Body:      nodeText(n, src),
			Span:      nodeSpan(n),
			Exported:  exported,
		}}
	case "impl_item":
		typeName := nodeText(n.ChildByFieldName("type"), src)
		var out []core.SymbolData
		body := n.ChildByFieldName("body")
		if body == nil {
			return out
		}
		for i := 0; i < int(body.ChildCount()); i++ {
			c := body.Child(i)
			if c != nil {
				out = append(out, rustItem(c, src, typeName)...)
			}
		}
		return out
	}
	return nil
}

func isRustPub(n *sitter.Node, src []byte) bool {
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		if c.Type() == "visibility_modifier" {
			return strings.HasPrefix(strings.TrimSpace(nodeText(c, src)), "pub")
		}
	}
	return false
}

func rustFuncSignature(n *sitter.Node, src []byte) string {
	body := n.ChildByFieldName("body")
	if body == nil {
		return extractFirstLine(nodeText(n, src))
	}
	end := body.StartByte()
	if int(end) > len(src) {
		end = uint32(len(src))
	}
	return strings.TrimSpace(string(src[n.StartByte():end]))
}

func (r *rustStrategy) ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var imps []core.ImportStatement
	walkChildren(root, func(n *sitter.Node) {
		if n.Type() != "use_declaration" {
			return
		}
		raw := strings.TrimSpace(nodeText(n, src))
		path := strings.TrimSuffix(strings.TrimPrefix(raw, "use "), ";")
		imps = append(imps, core.ImportStatement{
			Raw: raw, Path: strings.TrimSpace(path),
			Line: int(n.StartPoint().Row) + 1,
		})
	})
	return imps, nil
}

func (r *rustStrategy) ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error) {
	syms, err := r.Extract(tree, src)
	if err != nil {
		return nil, err
	}
	var out []core.ExportStatement
	for _, s := range syms {
		if s.Exported && s.ParentKey == "" {
			out = append(out, core.ExportStatement{Name: s.Name, Kind: s.Kind})
		}
	}
	return out, nil
}
