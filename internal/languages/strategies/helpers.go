// Package strategies implements per-language symbol/import/export extraction
// from tree-sitter ASTs.
package strategies

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// walkChildren yields each child of n.
func walkChildren(n *sitter.Node, fn func(*sitter.Node)) {
	if n == nil {
		return
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		c := n.Child(i)
		if c != nil {
			fn(c)
		}
	}
}

// findChildByType returns the first immediate child of n of the given type.
func findChildByType(n *sitter.Node, kind string) *sitter.Node {
	if n == nil {
		return nil
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		c := n.Child(i)
		if c != nil && c.Type() == kind {
			return c
		}
	}
	return nil
}

// nodeText returns the source text covered by n.
func nodeText(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	start, end := n.StartByte(), n.EndByte()
	if int(end) > len(src) {
		end = uint32(len(src))
	}
	if start > end {
		return ""
	}
	return string(src[start:end])
}

// nodeSpan returns the 1-indexed inclusive line range covered by n.
func nodeSpan(n *sitter.Node) core.LineRange {
	if n == nil {
		return core.LineRange{}
	}
	return core.LineRange{
		Start: int(n.StartPoint().Row) + 1,
		End:   int(n.EndPoint().Row) + 1,
	}
}

// isCapitalized reports whether name starts with an uppercase ASCII letter.
func isCapitalized(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// hasModifier reports whether modifiers contains m.
func hasModifier(modifiers []string, m string) bool {
	for _, x := range modifiers {
		if x == m {
			return true
		}
	}
	return false
}

// extractFirstLine returns the first non-empty trimmed line of body, useful as
// a fallback signature when AST doesn't have a dedicated signature node.
func extractFirstLine(body string) string {
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			return body[:i]
		}
	}
	return body
}
