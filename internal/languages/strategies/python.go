package strategies

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// Python strategy: indent-based, classes contain methods.
type pythonStrategy struct{}

func NewPython() *pythonStrategy { return &pythonStrategy{} }

func (p *pythonStrategy) Language() core.LanguageKey { return core.LangPython }
func (p *pythonStrategy) Extensions() []string       { return []string{".py"} }
func (p *pythonStrategy) Capabilities() core.MergeCapabilities {
	return core.MergeCapabilities{SupportsSymbolMerge: true, SupportsImportMerge: true}
}

func (p *pythonStrategy) Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var out []core.SymbolData
	walkChildren(root, func(n *sitter.Node) {
		switch n.Type() {
		case "function_definition":
			out = append(out, pyFunc(n, src, ""))
		case "decorated_definition":
			if def := findDecoratedTarget(n); def != nil {
				switch def.Type() {
				case "function_definition":
					out = append(out, pyFunc(n, src, ""))
				case "class_definition":
					out = append(out, pyClass(n, src)...)
				}
			}
		case "class_definition":
			out = append(out, pyClass(n, src)...)
		case "expression_statement":
			// module-level assignments (constants)
			if assign := findChildByType(n, "assignment"); assign != nil {
				if left := assign.ChildByFieldName("left"); left != nil && left.Type() == "identifier" {
					name := nodeText(left, src)
					out = append(out, core.SymbolData{
						Key:       name,
						Kind:      "variable",
						Name:      name,
						Signature: extractFirstLine(nodeText(n, src)),
						Body:      nodeText(n, src),
						Span:      nodeSpan(n),
						Exported:  !strings.HasPrefix(name, "_"),
					})
				}
			}
		}
	})
	return out, nil
}

// findDecoratedTarget returns the underlying function/class inside a
// decorated_definition node.
func findDecoratedTarget(n *sitter.Node) *sitter.Node {
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		if c.Type() == "function_definition" || c.Type() == "class_definition" {
			return c
		}
	}
	return nil
}

// pyFunc extracts a function (possibly wrapped by decorated_definition).
// parent is the enclosing class name (empty for module-level functions).
func pyFunc(n *sitter.Node, src []byte, parent string) core.SymbolData {
	def := n
	if n.Type() == "decorated_definition" {
		if t := findDecoratedTarget(n); t != nil && t.Type() == "function_definition" {
			def = t
		}
	}
	nameNode := def.ChildByFieldName("name")
	name := nodeText(nameNode, src)
	key := name
	kind := "function"
	if parent != "" {
		key = parent + "." + name
		kind = "method"
	}
	return core.SymbolData{
		Key:       key,
		Kind:      kind,
		Name:      name,
		ParentKey: parent,
		Signature: pyFuncSignature(def, src),
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Exported:  !strings.HasPrefix(name, "_"),
	}
}

// pyFuncSignature returns `def name(params) -> return:` minus the body.
func pyFuncSignature(n *sitter.Node, src []byte) string {
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

// pyClass returns the class itself plus one SymbolData per method inside it.
func pyClass(n *sitter.Node, src []byte) []core.SymbolData {
	def := n
	if n.Type() == "decorated_definition" {
		if t := findDecoratedTarget(n); t != nil && t.Type() == "class_definition" {
			def = t
		}
	}
	nameNode := def.ChildByFieldName("name")
	name := nodeText(nameNode, src)
	classBody := def.ChildByFieldName("body")
	out := []core.SymbolData{{
		Key:       name,
		Kind:      "class",
		Name:      name,
		Signature: pyFuncSignature(def, src),
		Body:      nodeText(n, src),
		Span:      nodeSpan(n),
		Exported:  !strings.HasPrefix(name, "_"),
	}}
	if classBody == nil {
		return out
	}
	for i := 0; i < int(classBody.ChildCount()); i++ {
		c := classBody.Child(i)
		if c == nil {
			continue
		}
		switch c.Type() {
		case "function_definition":
			out = append(out, pyFunc(c, src, name))
		case "decorated_definition":
			if t := findDecoratedTarget(c); t != nil && t.Type() == "function_definition" {
				out = append(out, pyFunc(c, src, name))
			}
		}
	}
	return out
}

func (p *pythonStrategy) ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var imps []core.ImportStatement
	walkChildren(root, func(n *sitter.Node) {
		switch n.Type() {
		case "import_statement":
			// import X, Y, Z
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(i)
				if c == nil {
					continue
				}
				if c.Type() == "dotted_name" || c.Type() == "aliased_import" {
					path := nodeText(c, src)
					imps = append(imps, core.ImportStatement{
						Raw: nodeText(n, src), Path: path, Line: int(n.StartPoint().Row) + 1,
					})
				}
			}
		case "import_from_statement":
			modNode := n.ChildByFieldName("module_name")
			mod := nodeText(modNode, src)
			imps = append(imps, core.ImportStatement{
				Raw: nodeText(n, src), Path: mod, Line: int(n.StartPoint().Row) + 1,
			})
		}
	})
	return imps, nil
}

func (p *pythonStrategy) ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error) {
	syms, err := p.Extract(tree, src)
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
