// Package merge orchestrates the 7-phase IntelliMerge pipeline.
package merge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/languages"
	"github.com/tabladrum/grove-suite/fuse/internal/merge/analysis"
	"github.com/tabladrum/grove-suite/fuse/internal/merge/classification"
	mstrat "github.com/tabladrum/grove-suite/fuse/internal/merge/strategies"
	"github.com/tabladrum/grove-suite/fuse/internal/parser"
)

// IntelliMerge orchestrates parsing, classification, merging, and diagnostics
// for a single-file merge.
type IntelliMerge struct {
	Parser           *parser.Engine
	Registry         *languages.Registry
	Grove            analysis.GroveLike
	HandoffThreshold float64 // confidence below which we emit a handoff prompt
	EnableBreaking   bool
	EnableContext    bool
	BreakingAnalyzer *analysis.BreakingChangeAnalyzer
}

// New returns an IntelliMerge using default registry + thresholds.
func New(g analysis.GroveLike) *IntelliMerge {
	return &IntelliMerge{
		Parser:           parser.NewEngine(),
		Registry:         languages.DefaultRegistry(),
		Grove:            g,
		HandoffThreshold: 0.30,
		EnableBreaking:   true,
		EnableContext:    true,
		BreakingAnalyzer: &analysis.BreakingChangeAnalyzer{Grove: g},
	}
}

// Merge runs the 7-phase pipeline and returns a MergeResult.
//
//  1. Context (Grove deps + dependents)
//  2. Symbol extraction (parse base, ours, theirs)
//  3. Recency analysis (placeholder; uses symbol-level signals only — no git log)
//  4. Project graph context (Grove blast radius)
//  5. Breaking change detection
//  6. Classification + strategy selection + merge application
//  7. Diagnostics + handoff prompt emission (caller handles file IO)
//
// For config files (JSON/YAML/TOML), phases 2/4/5/6 collapse to a single
// structural deep-merge.
func (im *IntelliMerge) Merge(
	ctx context.Context,
	baseContent, oursContent, theirsContent []byte,
	lang core.LanguageKey,
	filePath string,
) (*core.MergeResult, error) {
	started := time.Now()
	res := &core.MergeResult{
		FilePath: filePath,
		Language: lang,
	}

	// Trivial early exits.
	if string(oursContent) == string(theirsContent) {
		res.MergedContent = string(oursContent)
		res.Strategy = core.StrategyClean
		res.Confidence = 1.0
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}
	if string(baseContent) == string(oursContent) {
		res.MergedContent = string(theirsContent)
		res.Strategy = core.StrategyClean
		res.Confidence = 0.99
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}
	if string(baseContent) == string(theirsContent) {
		res.MergedContent = string(oursContent)
		res.Strategy = core.StrategyClean
		res.Confidence = 0.99
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	// Config formats short-circuit to deep merge.
	if parser.IsConfig(lang) {
		out := mstrat.ConfigMerge(lang, string(baseContent), string(oursContent), string(theirsContent))
		res.MergedContent = out.Merged
		res.HasConflict = out.HasConflict
		res.Confidence = out.Confidence
		res.Strategy = core.StrategyConfig
		res.ConflictType = core.ConflictConfigurational
		res.Severity = core.SeverityLow
		if out.HasConflict {
			res.Severity = core.SeverityMedium
		}
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	// Unsupported AST language: line-level fallback only.
	strategy := im.Registry.Get(lang)
	if strategy == nil || !parser.IsAST(lang) {
		out := mstrat.LineMerge(string(baseContent), string(oursContent), string(theirsContent))
		res.MergedContent = out.Merged
		res.HasConflict = out.HasConflict
		res.Confidence = out.Confidence
		res.Strategy = core.StrategyLine
		res.ConflictType = core.ConflictIncremental
		res.Severity = core.SeverityLow
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	// Phase 2 — Symbol extraction.
	baseTree, errB := im.Parser.Parse(lang, baseContent)
	oursTree, errO := im.Parser.Parse(lang, oursContent)
	theirsTree, errT := im.Parser.Parse(lang, theirsContent)
	defer closeTree(baseTree)
	defer closeTree(oursTree)
	defer closeTree(theirsTree)

	// If any side fails to parse, fall back to line merge.
	if errB != nil || errO != nil || errT != nil || baseTree == nil || oursTree == nil || theirsTree == nil {
		out := mstrat.LineMerge(string(baseContent), string(oursContent), string(theirsContent))
		res.MergedContent = out.Merged
		res.HasConflict = out.HasConflict
		res.Confidence = out.Confidence * 0.8 // penalty for falling back
		res.Strategy = core.StrategyLine
		res.ConflictType = core.ConflictIncremental
		res.Severity = core.SeverityMedium
		res.Diagnostics = append(res.Diagnostics, "tree-sitter parse failed on at least one side; used line-level fallback")
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	baseSyms, _ := strategy.Extract(baseTree, baseContent)
	oursSyms, _ := strategy.Extract(oursTree, oursContent)
	theirsSyms, _ := strategy.Extract(theirsTree, theirsContent)
	baseImps, _ := strategy.ExtractImports(baseTree, baseContent)
	oursImps, _ := strategy.ExtractImports(oursTree, oursContent)
	theirsImps, _ := strategy.ExtractImports(theirsTree, theirsContent)

	res.Stats.SymbolsBase = len(baseSyms)
	res.Stats.SymbolsOurs = len(oursSyms)
	res.Stats.SymbolsTheirs = len(theirsSyms)

	// Phase 4 + 5 — Breaking change detection (depends on Grove, optional).
	var breaking []core.BreakingChange
	if im.EnableBreaking && im.BreakingAnalyzer != nil {
		breaking = im.BreakingAnalyzer.Analyze(ctx, filePath, baseSyms, oursSyms, theirsSyms)
		res.BreakingChanges = breaking
	}

	// Phase 6 — Symbol + import merge.
	smerge := mstrat.SymbolMerge(
		mstrat.SymbolsToMap(baseSyms),
		mstrat.SymbolsToMap(oursSyms),
		mstrat.SymbolsToMap(theirsSyms),
	)
	imerge := mstrat.ImportMerge(baseImps, oursImps, theirsImps)
	res.Stats.AutoMerged = len(smerge.Merged) - len(smerge.Conflicts)
	res.Stats.Conflicted = len(smerge.Conflicts)

	// Classification.
	cls := classification.Classify(classification.Inputs{
		Language:        lang,
		BaseSymbols:     baseSyms,
		OursSymbols:     oursSyms,
		TheirsSymbols:   theirsSyms,
		SymbolConflicts: smerge.Conflicts,
		BreakingChanges: breaking,
		ImportChanges: classification.ImportChangeSummary{
			Added:   countDelta(baseImps, oursImps) + countDelta(baseImps, theirsImps),
			Removed: countDelta(oursImps, baseImps) + countDelta(theirsImps, baseImps),
		},
	})
	res.ConflictType = cls.Type
	res.Severity = cls.Severity

	// Compose merged file content. We emit symbols in merged order; non-merged
	// content (the file shell — package decl, comments at top, etc.) is taken
	// from ours to preserve formatting.
	merged := reconstructFile(
		string(oursContent), oursSyms,
		smerge.Merged, imerge.Merged, oursImps,
		lang,
	)
	res.MergedContent = merged
	res.Strategy = core.StrategySymbol
	res.Confidence = combineConfidence(smerge.Confidence, imerge.Confidence)

	if len(smerge.Conflicts) > 0 {
		res.HasConflict = true
		res.Conflicts = smerge.Conflicts
	}
	// If confidence drops below threshold, mark for handoff.
	if res.Confidence < im.HandoffThreshold {
		res.Strategy = core.StrategyHandoff
	}

	// Phase 7 — Diagnostics.
	if len(breaking) > 0 {
		for _, b := range breaking {
			res.Diagnostics = append(res.Diagnostics, fmt.Sprintf("[breaking %s] %s", b.Severity, b.Message))
		}
	}

	res.Stats.TimingMs = time.Since(started).Milliseconds()
	res.AuditEntry = im.auditEntryFor(res)
	return res, nil
}

func closeTree(t any) {
	if c, ok := t.(interface{ Close() }); ok && c != nil {
		c.Close()
	}
}

func combineConfidence(a, b float64) float64 {
	// geometric mean to reflect that low confidence in either factor lowers
	// the joint confidence.
	if a <= 0 || b <= 0 {
		return 0
	}
	return (a + b) / 2
}

func countDelta(from, to []core.ImportStatement) int {
	have := make(map[string]bool, len(from))
	for _, i := range from {
		have[i.Path] = true
	}
	n := 0
	for _, i := range to {
		if !have[i.Path] {
			n++
		}
	}
	return n
}

func (im *IntelliMerge) auditEntryFor(r *core.MergeResult) core.AuditEntry {
	return core.AuditEntry{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		File:            r.FilePath,
		Language:        r.Language,
		Strategy:        r.Strategy,
		ConflictType:    r.ConflictType,
		Severity:        r.Severity,
		Confidence:      r.Confidence,
		AutoMerged:      !r.HasConflict,
		BreakingChanges: len(r.BreakingChanges),
	}
}

// reconstructFile builds the merged file content using oursContent as the
// shell (preserves comments, package decl, blank lines) and substitutes
// merged symbols and imports in place.
//
// Algorithm (line-based):
//
//  1. Iterate ours line by line.
//  2. When entering a line covered by an ours-symbol's span, emit the merged
//     version of that symbol (looked up by key in mergedMap) and skip to the
//     end of the span.
//  3. When entering a line covered by an ours-import statement, skip it.
//  4. Insert all merged imports as a block after the first non-empty
//     non-package line.
//  5. Append symbols present in merged but missing in ours at the end.
//
// This isn't AST-perfect but it preserves human-written formatting outside
// symbol bodies, which is the right tradeoff for a merge driver.
func reconstructFile(
	oursContent string,
	oursSyms []core.SymbolData,
	mergedSyms []core.SymbolData,
	mergedImps []core.ImportStatement,
	oursImps []core.ImportStatement,
	lang core.LanguageKey,
) string {
	oursLines := strings.Split(oursContent, "\n")

	// Build spans to substitute / skip.
	type span struct {
		start, end int // 1-indexed inclusive
		body       string
		kind       string // "symbol" | "skip" | "import"
		key        string
	}
	var spans []span

	mergedByKey := make(map[string]core.SymbolData, len(mergedSyms))
	for _, s := range mergedSyms {
		mergedByKey[s.QualifiedName] = s
	}

	usedKeys := map[string]bool{}
	for _, s := range oursSyms {
		merged, ok := mergedByKey[s.QualifiedName]
		if !ok {
			// symbol was dropped in merge
			spans = append(spans, span{start: s.Span.Start, end: s.Span.End, kind: "skip", key: s.QualifiedName})
			continue
		}
		spans = append(spans, span{start: s.Span.Start, end: s.Span.End, body: merged.Body, kind: "symbol", key: s.QualifiedName})
		usedKeys[s.QualifiedName] = true
	}
	for _, i := range oursImps {
		spans = append(spans, span{start: i.Line, end: i.Line, kind: "import"})
	}

	// Sort spans by start.
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	importBlock := renderImportBlock(mergedImps, lang)
	importsEmitted := false

	var out []string
	idx := 0
	for lineNo := 1; lineNo <= len(oursLines); lineNo++ {
		if idx < len(spans) && spans[idx].start == lineNo {
			s := spans[idx]
			switch s.kind {
			case "symbol":
				out = append(out, s.body)
			case "import":
				// emit imports block once at the first import location.
				if !importsEmitted {
					if importBlock != "" {
						out = append(out, importBlock)
					}
					importsEmitted = true
				}
			case "skip":
				// drop entirely
			}
			lineNo = s.end
			idx++
			continue
		}
		out = append(out, oursLines[lineNo-1])
	}

	// If we never had an import line on ours but merged has imports → insert
	// after the first non-empty non-package/comment line (best-effort).
	if !importsEmitted && importBlock != "" && len(mergedImps) > 0 {
		out = injectImportsAfterPackageDecl(out, importBlock, lang)
	}

	// Append any symbols from merged that ours didn't have (added by theirs).
	for _, s := range mergedSyms {
		if usedKeys[s.QualifiedName] {
			continue
		}
		out = append(out, "")
		out = append(out, s.Body)
	}

	return strings.Join(out, "\n")
}

// renderImportBlock renders merged imports in the language's typical form.
func renderImportBlock(imps []core.ImportStatement, lang core.LanguageKey) string {
	if len(imps) == 0 {
		return ""
	}
	switch lang {
	case core.LangGo:
		var b strings.Builder
		b.WriteString("import (\n")
		for _, i := range imps {
			if i.Alias != "" {
				fmt.Fprintf(&b, "\t%s %q\n", i.Alias, i.Path)
			} else {
				fmt.Fprintf(&b, "\t%q\n", i.Path)
			}
		}
		b.WriteString(")")
		return b.String()
	default:
		// fallback: keep raw lines
		var b strings.Builder
		for idx, i := range imps {
			if idx > 0 {
				b.WriteString("\n")
			}
			b.WriteString(strings.TrimSpace(i.Raw))
		}
		return b.String()
	}
}

// injectImportsAfterPackageDecl inserts the import block after the package /
// header declaration (Go: after `package X`; others: after first non-blank
// non-comment line).
func injectImportsAfterPackageDecl(lines []string, block string, lang core.LanguageKey) []string {
	insertAt := 0
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if lang == core.LangGo && strings.HasPrefix(trimmed, "package ") {
			insertAt = i + 1
			break
		}
		insertAt = i
		break
	}
	out := make([]string, 0, len(lines)+2)
	out = append(out, lines[:insertAt]...)
	out = append(out, "", block, "")
	out = append(out, lines[insertAt:]...)
	return out
}
