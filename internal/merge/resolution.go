package merge

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/provasign/fuse/internal/parser"
)

// ValidateResolution checks that src is a plausible resolution for filePath:
// non-empty, free of conflict markers, and syntactically valid for AST
// languages. Used before an agent-produced resolution is written to disk.
func ValidateResolution(filePath string, src []byte) error {
	if len(bytes.TrimSpace(src)) == 0 {
		return errors.New("resolution is empty")
	}
	for _, marker := range []string{"<<<<<<<", ">>>>>>>"} {
		if strings.Contains(string(src), marker) {
			return fmt.Errorf("resolution still contains %q conflict markers", marker)
		}
	}
	lang := parser.DetectLanguage(filePath, string(src))
	if parser.IsAST(lang) {
		im := New(nil)
		if !im.parsesClean(lang, src) {
			return fmt.Errorf("resolution does not parse cleanly as %s", lang)
		}
	}
	return nil
}
