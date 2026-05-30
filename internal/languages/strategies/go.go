package strategies

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// Go strategy.
type goStrategy struct{}

func NewGo() *goStrategy { return &goStrategy{} }

func (g *goStrategy) Language() core.LanguageKey { return core.LangGo }
func (g *goStrategy) Extensions() []string       { return []string{".go"} }
func (g *goStrategy) Capabilities() core.MergeCapabilities {
	return core.MergeCapabilities{SupportsSymbolMerge: true, SupportsImportMerge: true}
}

func (g *goStrategy) Extract(tree *sitter.Tree, src []byte) ([]core.SymbolData, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var out []core.SymbolData
	walkChildren(root, func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration":
			out = append(out, goFunc(n, src))
		case "method_declaration":
			out = append(out, goMethod(n, src))
		case "type_declaration":
			out = append(out, goTypeDecl(n, src)...)
		case "const_declaration":
			out = append(out, goValueDecl(n, src, "const")...)
		case "var_declaration":
			out = append(out, goValueDecl(n, src, "variable")...)
		}
	})
	return out, nil
}

func (g *goStrategy) ExtractImports(tree *sitter.Tree, src []byte) ([]core.ImportStatement, error) {
	if tree == nil {
		return nil, nil
	}
	root := tree.RootNode()
	var imports []core.ImportStatement
	walkChildren(root, func(n *sitter.Node) {
		if n.Type() != "import_declaration" {
			return
		}
		// Either a single import_spec or an import_spec_list.
		for i := 0; i < int(n.ChildCount()); i++ {
			c := n.Child(i)
			if c == nil {
				continue
			}
			switch c.Type() {
			case "import_spec":
				if imp := goImportSpec(c, src); imp != nil {
					imports = append(imports, *imp)
				}
			case "import_spec_list":
				for j := 0; j < int(c.ChildCount()); j++ {
					cc := c.Child(j)
					if cc != nil && cc.Type() == "import_spec" {
						if imp := goImportSpec(cc, src); imp != nil {
							imports = append(imports, *imp)
						}
					}
				}
			}
		}
	})
	return imports, nil
}

func (g *goStrategy) ExtractExports(tree *sitter.Tree, src []byte) ([]core.ExportStatement, error) {
	syms, err := g.Extract(tree, src)
	if err != nil {
		return nil, err
	}
	var out []core.ExportStatement
	for _, s := range syms {
		if s.Exported {
			out = append(out, core.ExportStatement{Name: s.Name, Kind: s.Kind})
		}
	}
	return out, nil
}

// goFunc handles `func Name(args) ret { body }`.
func goFunc(n *sitter.Node, src []byte) core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	body := nodeText(n, src)
	sig := goFuncSignature(n, src)
	return core.SymbolData{
		Key:       name,
		Kind:      "function",
		Name:      name,
		Signature: sig,
		Body:      body,
		Span:      nodeSpan(n),
		Exported:  isCapitalized(name),
	}
}

// goMethod handles `func (r Recv) Name(args) ret { body }`.
func goMethod(n *sitter.Node, src []byte) core.SymbolData {
	name := nodeText(n.ChildByFieldName("name"), src)
	recv := nodeText(n.ChildByFieldName("receiver"), src)
	recvType := extractReceiverType(recv)
	key := name
	if recvType != "" {
		key = recvType + "." + name
	}
	body := nodeText(n, src)
	return core.SymbolData{
		Key:       key,
		Kind:      "method",
		Name:      name,
		ParentKey: recvType,
		Signature: goFuncSignature(n, src),
		Body:      body,
		Span:      nodeSpan(n),
		Exported:  isCapitalized(name),
	}
}

// goFuncSignature returns everything up to and including the return type but
// excluding the body, e.g. `func (r *T) Foo(x int) error`.
func goFuncSignature(n *sitter.Node, src []byte) string {
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

// extractReceiverType pulls the type name from a receiver clause like
// `(r *Foo)` or `(Foo)`.
func extractReceiverType(recv string) string {
	recv = strings.TrimSpace(recv)
	recv = strings.TrimPrefix(recv, "(")
	recv = strings.TrimSuffix(recv, ")")
	// receiver looks like "r *Foo" or "*Foo" or "Foo" or "r Foo[T]"
	parts := strings.Fields(recv)
	if len(parts) == 0 {
		return ""
	}
	candidate := parts[len(parts)-1]
	candidate = strings.TrimPrefix(candidate, "*")
	if idx := strings.IndexAny(candidate, "[ "); idx >= 0 {
		candidate = candidate[:idx]
	}
	return candidate
}

// goTypeDecl produces a SymbolData per type_spec inside a type_declaration.
func goTypeDecl(n *sitter.Node, src []byte) []core.SymbolData {
	var out []core.SymbolData
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil || c.Type() != "type_spec" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		kind := "type"
		typeBody := c.ChildByFieldName("type")
		if typeBody != nil {
			switch typeBody.Type() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
			}
		}
		out = append(out, core.SymbolData{
			Key:       name,
			Kind:      kind,
			Name:      name,
			Signature: extractFirstLine(nodeText(c, src)),
			Body:      nodeText(c, src),
			Span:      nodeSpan(c),
			Exported:  isCapitalized(name),
		})
	}
	return out
}

// goValueDecl produces a SymbolData per identifier in a const/var declaration.
func goValueDecl(n *sitter.Node, src []byte, kind string) []core.SymbolData {
	var out []core.SymbolData
	collect := func(spec *sitter.Node) {
		for j := 0; j < int(spec.ChildCount()); j++ {
			id := spec.Child(j)
			if id == nil || id.Type() != "identifier" {
				continue
			}
			name := nodeText(id, src)
			out = append(out, core.SymbolData{
				Key:       name,
				Kind:      kind,
				Name:      name,
				Signature: extractFirstLine(nodeText(spec, src)),
				Body:      nodeText(spec, src),
				Span:      nodeSpan(spec),
				Exported:  isCapitalized(name),
			})
		}
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		switch c.Type() {
		case "const_spec", "var_spec":
			collect(c)
		}
	}
	return out
}

// goImportSpec parses one import spec like `alias "path"` or `"path"`.
func goImportSpec(n *sitter.Node, src []byte) *core.ImportStatement {
	pathNode := n.ChildByFieldName("path")
	if pathNode == nil {
		return nil
	}
	rawPath := nodeText(pathNode, src)
	cleaned := strings.Trim(rawPath, `"`)
	alias := ""
	if nameNode := n.ChildByFieldName("name"); nameNode != nil {
		alias = nodeText(nameNode, src)
	}
	group := goImportGroup(cleaned)
	return &core.ImportStatement{
		Raw:   nodeText(n, src),
		Path:  cleaned,
		Alias: alias,
		Group: group,
		Line:  int(n.StartPoint().Row) + 1,
	}
}

// goImportGroup classifies imports as stdlib | external | relative.
func goImportGroup(path string) string {
	if strings.HasPrefix(path, ".") {
		return "relative"
	}
	first := path
	if idx := strings.Index(path, "/"); idx >= 0 {
		first = path[:idx]
	}
	if strings.Contains(first, ".") {
		return "external"
	}
	return "stdlib"
}
