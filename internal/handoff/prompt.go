// Package handoff produces local AI-ready conflict prompts and append-only
// audit log entries. No external API calls are made.
package handoff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

// PromptInputs is the data needed to build a Markdown handoff prompt.
type PromptInputs struct {
	FilePath        string
	Language        core.LanguageKey
	ConflictType    core.ConflictType
	Severity        core.ConflictSeverity
	Confidence      float64
	Conflicts       []core.SymbolConflict
	BreakingChanges []core.BreakingChange
	Dependencies    []string
	Dependents      []string
}

// Generator writes prompts under .git/fuse/.
type Generator struct {
	OutputDir string // typically <gitDir>/fuse
}

// NewGenerator returns a Generator writing to outputDir, creating it if
// absent.
func NewGenerator(outputDir string) (*Generator, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	return &Generator{OutputDir: outputDir}, nil
}

// Write emits a Markdown prompt and JSON payload pair; returns the Markdown
// file path.
func (g *Generator) Write(in PromptInputs) (string, error) {
	hash := promptHash(in)
	mdPath := filepath.Join(g.OutputDir, "conflict-"+hash+".md")
	jsonPath := filepath.Join(g.OutputDir, "conflict-"+hash+".json")
	if err := os.WriteFile(mdPath, []byte(renderMarkdown(in)), 0o644); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(in, "", "  ")
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return mdPath, err
	}
	return mdPath, nil
}

func promptHash(in PromptInputs) string {
	h := sha256.New()
	h.Write([]byte(in.FilePath))
	for _, c := range in.Conflicts {
		h.Write([]byte(c.Key))
		h.Write([]byte(c.Ours.Body))
		h.Write([]byte(c.Theirs.Body))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func renderMarkdown(in PromptInputs) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Fuse: Unresolvable Merge Conflict\n\n")
	fmt.Fprintf(&b, "## Summary\n")
	fmt.Fprintf(&b, "- **File:** %s\n", in.FilePath)
	fmt.Fprintf(&b, "- **Language:** %s\n", in.Language)
	fmt.Fprintf(&b, "- **Conflict Type:** %s\n", in.ConflictType)
	fmt.Fprintf(&b, "- **Severity:** %s\n", in.Severity)
	fmt.Fprintf(&b, "- **Confidence:** %.0f%%\n", in.Confidence*100)
	if len(in.Conflicts) > 0 {
		var names []string
		for _, c := range in.Conflicts {
			names = append(names, "`"+c.Key+"`")
		}
		fmt.Fprintf(&b, "- **Symbols in conflict:** %s\n", strings.Join(names, ", "))
	}
	fmt.Fprintln(&b)

	if len(in.BreakingChanges) > 0 {
		fmt.Fprintf(&b, "## Breaking Changes Detected\n")
		for _, bc := range in.BreakingChanges {
			fmt.Fprintf(&b, "- **[%s]** %s — %s\n", bc.Severity, bc.Kind, bc.Message)
			for _, af := range bc.AffectedFiles {
				fmt.Fprintf(&b, "    - %s\n", af)
			}
		}
		fmt.Fprintln(&b)
	}

	if len(in.Conflicts) > 0 {
		fmt.Fprintf(&b, "## Three-Way Comparisons\n\n")
		fence := codeFence(in.Language)
		for _, c := range in.Conflicts {
			fmt.Fprintf(&b, "### Symbol: `%s`\n\n", c.Key)
			fmt.Fprintf(&b, "**BASE (common ancestor)**\n\n%s%s\n%s\n%s\n\n", fence, fenceLang(in.Language), c.Base.Body, fence)
			fmt.Fprintf(&b, "**OURS (HEAD)**\n\n%s%s\n%s\n%s\n\n", fence, fenceLang(in.Language), c.Ours.Body, fence)
			fmt.Fprintf(&b, "**THEIRS (incoming)**\n\n%s%s\n%s\n%s\n\n", fence, fenceLang(in.Language), c.Theirs.Body, fence)
		}
	}

	if len(in.Dependencies) > 0 || len(in.Dependents) > 0 {
		fmt.Fprintf(&b, "## Context (from Grove)\n")
		if len(in.Dependencies) > 0 {
			fmt.Fprintf(&b, "**Dependencies:**\n")
			for _, d := range in.Dependencies {
				fmt.Fprintf(&b, "- %s\n", d)
			}
		}
		if len(in.Dependents) > 0 {
			fmt.Fprintf(&b, "\n**Files that depend on this file:**\n")
			for _, d := range in.Dependents {
				fmt.Fprintf(&b, "- %s\n", d)
			}
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "## Resolution Task\n")
	fmt.Fprintf(&b, "Produce a merged version of `%s` that:\n", in.FilePath)
	fmt.Fprintf(&b, "1. Reconciles the symbol-level conflicts between OURS and THEIRS\n")
	fmt.Fprintf(&b, "2. Preserves backward compatibility for any exported symbols listed above\n")
	fmt.Fprintf(&b, "3. Compiles and passes existing tests\n\n")
	fmt.Fprintf(&b, "## Output Format\n")
	fmt.Fprintf(&b, "Return ONLY the complete merged file content, no commentary, no fences.\n")
	return b.String()
}

func codeFence(_ core.LanguageKey) string { return "```" }

func fenceLang(l core.LanguageKey) string {
	switch l {
	case core.LangGo:
		return "go"
	case core.LangPython:
		return "python"
	case core.LangTypeScript, core.LangTSX:
		return "typescript"
	case core.LangJavaScript:
		return "javascript"
	case core.LangJava:
		return "java"
	case core.LangRust:
		return "rust"
	case core.LangJSON:
		return "json"
	case core.LangYAML:
		return "yaml"
	case core.LangTOML:
		return "toml"
	}
	return ""
}

// AppendAudit appends one MergeResult-derived entry to audit.json.
func AppendAudit(outputDir string, entry core.AuditEntry) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	auditPath := filepath.Join(outputDir, "audit.json")
	var existing []core.AuditEntry
	if data, err := os.ReadFile(auditPath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}
	existing = append(existing, entry)
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(auditPath, out, 0o644)
}
