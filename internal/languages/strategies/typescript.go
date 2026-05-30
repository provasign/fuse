package strategies

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// jsLike is a base strategy shared by TypeScript, TSX, and JavaScript.
type jsLike struct {
	lang core.LanguageKey
	exts []string
}

func NewTypeScript(tsx bool) *jsLike {
	if tsx {
		return &jsLike{lang: core.LangTSX, exts: []string{".tsx"}}
	}
	return &jsLike{lang: core.LangTypeScript, exts: []string{".ts"}}
}

func NewJavaScript() *jsLike {
	return &jsLike{lang: core.LangJavaScript, exts: []string{".js", ".jsx", ".mjs", ".cjs"}}
}

func (j *jsLike) Language() core.LanguageKey { return j.lang }
func (j *jsLike) Extensions() []string       { return j.exts }
func (j *jsLike) Capabilities() core.MergeCapabilities {
	return core.MergeCapabilities{SupportsSymbolMerge: true, SupportsImportMerge: true}
}

// jsNamedSymbol describes the top-level statement kinds we surface.
func (j *jsLike) Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var out []core.SymbolData
	walkChildren(root, func(n *sitter.Node) {
		// export wraps an inner declaration we still want to surface.
		exported := false
		inner := n
		if n.Type() == "export_statement" {
			exported = true
			if d := findFirstChildIn(n, []string{
				"function_declaration", "class_declaration", "interface_declaration",
				"type_alias_declaration", "enum_declaration", "lexical_declaration",
				"variable_declaration",
			}); d != nil {
				inner = d
			}
		}
		switch inner.Type() {
		case "function_declaration":
			out = append(out, jsFunc(inner, src, exported))
		case "class_declaration":
			out = append(out, jsClass(inner, src, exported)...)
		case "interface_declaration":
			out = append(out, jsNamed(inner, src, "interface", exported))
		case "type_alias_declaration":
			out = append(out, jsNamed(inner, src, "type", exported))
		case "enum_declaration":
			out = append(out, jsNamed(inner, src, "enum", exported))
		case "lexical_declaration", "variable_declaration":
			out = append(out, jsVar(inner, src, exported)...)
		}
	})
	return out, nil
}

func findFirstChildIn(n *sitter.Node, kinds []string) *sitter.Node {
	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		want[k] = true
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c != nil && want[c.Type()] {
			return c
		}
	}
	return nil
}

func jsFunc(n *sitter.Node, src []byte, exported bool) core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	body := n.ChildByFieldName("body")
	sig := nodeText(n, src)
	if body != nil {
		end := body.StartByte()
		if int(end) > len(src) {
			end = uint32(len(src))
		}
		sig = strings.TrimSpace(string(src[n.StartByte():end]))
	}
	return core.SymbolData{
		Key:       name,
		Kind:      "function",
		Name:      name,
		Signature: sig,
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Exported:  exported,
	}
}

func jsNamed(n *sitter.Node, src []byte, kind string, exported bool) core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	return core.SymbolData{
		Key:       name,
		Kind:      kind,
		Name:      name,
		Signature: extractFirstLine(nodeText(n, src)),
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Exported:  exported,
	}
}

func jsClass(n *sitter.Node, src []byte, exported bool) []core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	out := []core.SymbolData{{
		Key:       name,
		Kind:      "class",
		Name:      name,
		Signature: extractFirstLine(nodeText(n, src)),
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Exported:  exported,
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
		case "method_definition":
			methodName := nodeText(c.ChildByFieldName("name"), src)
			key := name + "." + methodName
			out = append(out, core.SymbolData{
				Key:       key,
				Kind:      "method",
				Name:      methodName,
				ParentKey: name,
				Signature: extractFirstLine(nodeText(c, src)),
				Body:      nodeText(c, src),
				Span:      nodeSpan(c),
				Exported:  exported,
			})
		case "field_definition", "public_field_definition":
			fieldName := nodeText(c.ChildByFieldName("name"), src)
			if fieldName == "" {
				continue
			}
			out = append(out, core.SymbolData{
				Key:       name + "." + fieldName,
				Kind:      "field",
				Name:      fieldName,
				ParentKey: name,
				Signature: extractFirstLine(nodeText(c, src)),
				Body:      nodeText(c, src),
				Span:      nodeSpan(c),
				Exported:  exported,
			})
		}
	}
	return out
}

func jsVar(n *sitter.Node, src []byte, exported bool) []core.SymbolData {
	var out []core.SymbolData
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil || c.Type() != "variable_declarator" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil || nameNode.Type() != "identifier" {
			continue
		}
		name := nodeText(nameNode, src)
		kind := "const"
		out = append(out, core.SymbolData{
			Key:       name,
			Kind:      kind,
			Name:      name,
			Signature: extractFirstLine(nodeText(c, src)),
			Body:      nodeText(c, src),
			Span:      nodeSpan(c),
			Exported:  exported,
		})
	}
	return out
}

func (j *jsLike) ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var imps []core.ImportStatement
	walkChildren(root, func(n *sitter.Node) {
		if n.Type() != "import_statement" {
			return
		}
		sourceNode := n.ChildByFieldName("source")
		path := strings.Trim(nodeText(sourceNode, src), `"'`+"`")
		imps = append(imps, core.ImportStatement{
			Raw:  nodeText(n, src),
			Path: path,
			Line: int(n.StartPoint().Row) + 1,
		})
	})
	return imps, nil
}

func (j *jsLike) ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error) {
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
