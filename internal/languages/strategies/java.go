package strategies

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// Java strategy.
type javaStrategy struct{}

func NewJava() *javaStrategy { return &javaStrategy{} }

func (j *javaStrategy) Language() core.LanguageKey { return core.LangJava }
func (j *javaStrategy) Extensions() []string       { return []string{".java"} }
func (j *javaStrategy) Capabilities() core.MergeCapabilities {
	return core.MergeCapabilities{SupportsSymbolMerge: true, SupportsImportMerge: true}
}

func (j *javaStrategy) Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var out []core.SymbolData
	walkChildren(root, func(n *sitter.Node) {
		switch n.Type() {
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			out = append(out, javaType(n, src)...)
		}
	})
	return out, nil
}

func javaType(n *sitter.Node, src []byte) []core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	kind := "class"
	switch n.Type() {
	case "interface_declaration":
		kind = "interface"
	case "enum_declaration":
		kind = "enum"
	case "record_declaration":
		kind = "type"
	}
	modifiers := javaModifiers(n, src)
	out := []core.SymbolData{{
		Key:       name,
		Kind:      kind,
		Name:      name,
		Signature: extractFirstLine(nodeText(n, src)),
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Modifiers: modifiers,
		Exported:  hasModifier(modifiers, "public"),
	}}
	body := n.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		c := body.Child(i)
		if c == nil {
			continue
		}
		switch c.Type() {
		case "method_declaration":
			methodName := nodeText(c.ChildByFieldName("name"), src)
			mods := javaModifiers(c, src)
			out = append(out, core.SymbolData{
				Key:       name + "." + methodName,
				Kind:      "method",
				Name:      methodName,
				ParentKey: name,
				Signature: extractFirstLine(nodeText(c, src)),
				Body:      nodeText(c, src),
				Span:      nodeSpan(c),
				Modifiers: mods,
				Exported:  hasModifier(mods, "public"),
			})
		case "field_declaration":
			// emit one symbol per declarator
			for k := 0; k < int(c.ChildCount()); k++ {
				vd := c.Child(k)
				if vd == nil || vd.Type() != "variable_declarator" {
					continue
				}
				fieldName := nodeText(vd.ChildByFieldName("name"), src)
				if fieldName == "" {
					continue
				}
				mods := javaModifiers(c, src)
				out = append(out, core.SymbolData{
					Key:       name + "." + fieldName,
					Kind:      "field",
					Name:      fieldName,
					ParentKey: name,
					Signature: extractFirstLine(nodeText(c, src)),
					Body:      nodeText(c, src),
					Span:      nodeSpan(c),
					Modifiers: mods,
					Exported:  hasModifier(mods, "public"),
				})
			}
		}
	}
	return out
}

func javaModifiers(n *sitter.Node, src []byte) []string {
	mods := findChildByType(n, "modifiers")
	if mods == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(mods.ChildCount()); i++ {
		c := mods.Child(i)
		if c == nil {
			continue
		}
		if c.IsNamed() || c.Type() == "public" || c.Type() == "private" ||
			c.Type() == "protected" || c.Type() == "static" || c.Type() == "final" ||
			c.Type() == "abstract" {
			out = append(out, c.Type())
		}
	}
	return out
}

func (j *javaStrategy) ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var imps []core.ImportStatement
	walkChildren(root, func(n *sitter.Node) {
		if n.Type() != "import_declaration" {
			return
		}
		raw := strings.TrimSuffix(strings.TrimSpace(nodeText(n, src)), ";")
		raw = strings.TrimPrefix(raw, "import")
		raw = strings.TrimSpace(raw)
		raw = strings.TrimPrefix(raw, "static")
		raw = strings.TrimSpace(raw)
		imps = append(imps, core.ImportStatement{
			Raw:  nodeText(n, src),
			Path: raw,
			Line: int(n.StartPoint().Row) + 1,
		})
	})
	return imps, nil
}

func (j *javaStrategy) ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error) {
	syms, err := j.Extract(tree, src)
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
