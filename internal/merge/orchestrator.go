// Package merge orchestrates the IntelliMerge escalation ladder: git-parity
// line merge, fine-grained LCS line merge, then symbol-level merge — each
// rung gated by a clean re-parse of the produced output.
package merge

import (
	"context"
	"fmt"
	goparser "go/parser"
	"go/token"
	"sort"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/languages"
	"github.com/provasign/fuse/internal/merge/analysis"
	"github.com/provasign/fuse/internal/merge/classification"
	mstrat "github.com/provasign/fuse/internal/merge/strategies"
	"github.com/provasign/fuse/internal/parser"
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

// Merge runs the pipeline and returns a MergeResult.
//
//  1. Trivial exits (identical sides, one-sided change)
//  2. Symbol extraction (parse base, ours, theirs)
//  3. Breaking change detection (Grove blast radius, optional)
//  4. Escalation ladder:
//     a. git-equivalent line merge — clean + parses → ship (git parity)
//     b. fine-grained LCS line merge — clean + parses → ship
//     c. symbol-level merge + reconstruction — validated by re-parse
//  5. Classification + diagnostics + handoff prompt emission (caller
//     handles file IO)
//
// Config files (JSON/YAML/TOML) use the line merge first and a structural
// deep-merge on conflict.
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

	// Config formats: try the git-equivalent line merge first — it preserves
	// the author's formatting, comments, and key order exactly. The structural
	// deep merge (which re-serializes the document) only runs on conflict.
	if parser.IsConfig(lang) {
		if lineOut := mstrat.GitLineMerge(string(baseContent), string(oursContent), string(theirsContent)); !lineOut.HasConflict {
			res.MergedContent = lineOut.Merged
			res.Strategy = core.StrategyLine
			res.Confidence = 0.95
			res.ConflictType = core.ConflictConfigurational
			res.Severity = core.SeverityLow
			res.Stats.TimingMs = time.Since(started).Milliseconds()
			res.AuditEntry = im.auditEntryFor(res)
			return res, nil
		}
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
		out := mstrat.GitLineMerge(string(baseContent), string(oursContent), string(theirsContent))
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
		out := mstrat.GitLineMerge(string(baseContent), string(oursContent), string(theirsContent))
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

	// Phase 6a — Git-equivalent line merge first. When it is clean and the
	// output parses, ship it: it preserves byte-level fidelity, and fuse must
	// never do worse than git. The symbol pipeline takes over only where
	// line-level merging fails — which is exactly where it adds value.
	lineOut := mstrat.GitLineMerge(string(baseContent), string(oursContent), string(theirsContent))
	if !lineOut.HasConflict && im.parsesClean(lang, []byte(lineOut.Merged)) {
		res.MergedContent = lineOut.Merged
		res.Strategy = core.StrategyLine
		res.Confidence = 0.95
		res.ConflictType = core.ConflictIncremental
		res.Severity = core.SeverityLow
		for _, b := range breaking {
			res.Diagnostics = append(res.Diagnostics, fmt.Sprintf("[breaking %s] %s", b.Severity, b.Message))
		}
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	// Phase 6a' — internal LCS line merge. Its hunking is finer-grained than
	// git merge-file and resolves adjacent-but-disjoint edits git treats as
	// conflicting. Consulted only when git's merge failed, so it can only
	// improve on the baseline; gated by a clean parse.
	if lcs := mstrat.LineMerge(string(baseContent), string(oursContent), string(theirsContent)); !lcs.HasConflict && im.parsesClean(lang, []byte(lcs.Merged)) {
		res.MergedContent = lcs.Merged
		res.Strategy = core.StrategyLine
		res.Confidence = 0.85
		res.ConflictType = core.ConflictIncremental
		res.Severity = core.SeverityLow
		for _, b := range breaking {
			res.Diagnostics = append(res.Diagnostics, fmt.Sprintf("[breaking %s] %s", b.Severity, b.Message))
		}
		res.Stats.TimingMs = time.Since(started).Milliseconds()
		res.AuditEntry = im.auditEntryFor(res)
		return res, nil
	}

	// Phase 6b — Symbol + import merge. Merge operates on top-level symbols
	// only: container bodies (classes) include their children, and nested
	// spans would overlap during reconstruction. Breaking-change analysis
	// above still sees the full nested symbol set.
	topBase := mstrat.TopLevelSymbols(baseSyms)
	topOurs := mstrat.TopLevelSymbols(oursSyms)
	topTheirs := mstrat.TopLevelSymbols(theirsSyms)
	smerge := mstrat.SymbolMerge(
		mstrat.SymbolsToMap(topBase),
		mstrat.SymbolsToMap(topOurs),
		mstrat.SymbolsToMap(topTheirs),
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
		string(oursContent), topOurs,
		smerge.Merged, imerge.Merged, oursImps,
		lang,
		string(theirsContent), topTheirs,
	)
	res.MergedContent = merged
	res.Strategy = core.StrategySymbol
	res.Confidence = combineConfidence(smerge.Confidence, imerge.Confidence)

	if len(smerge.Conflicts) > 0 {
		res.HasConflict = true
		res.Conflicts = smerge.Conflicts
	}

	// Post-merge validation gate: a clean symbol merge must still parse.
	// Reconstruction is line-span based, so if it introduced syntax errors
	// the inputs didn't have, the merge is corrupt — discard it and surface
	// the line-level result (with its conflict markers) instead. Skipped when
	// conflict markers are present (those intentionally break syntax).
	if !res.HasConflict {
		inputsClean := !treeHasError(baseTree) && !treeHasError(oursTree) && !treeHasError(theirsTree)
		if inputsClean && !im.parsesClean(lang, []byte(merged)) {
			res.MergedContent = lineOut.Merged
			res.HasConflict = true
			res.Confidence = lineOut.Confidence * 0.8
			res.Strategy = core.StrategyLine
			diag := "post-merge validation failed: reconstructed file did not parse cleanly; surfaced line-level conflict instead"
			if !lineOut.HasConflict {
				diag = "post-merge validation failed on both the symbol and line merges; manual review required"
			}
			res.Diagnostics = append(res.Diagnostics, diag)
		}
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

// treeHasError reports whether the parse tree contains syntax errors.
func treeHasError(t *sitter.Tree) bool {
	if t == nil {
		return true
	}
	root := t.RootNode()
	return root == nil || root.HasError()
}

// parsesClean reports whether src parses without syntax errors. Go addition-
// ally goes through the stdlib parser: tree-sitter's Go grammar tolerates
// constructs gofmt rejects (e.g. a `const` keyword repeated inside a const
// block), and the gate exists precisely to catch those.
func (im *IntelliMerge) parsesClean(lang core.LanguageKey, src []byte) bool {
	if lang == core.LangGo {
		fset := token.NewFileSet()
		if _, err := goparser.ParseFile(fset, "merged.go", src, 0); err != nil {
			return false
		}
	}
	t, err := im.Parser.Parse(lang, src)
	if err != nil || t == nil {
		return false
	}
	defer t.Close()
	return !treeHasError(t)
}

func combineConfidence(a, b float64) float64 {
	// arithmetic mean of the two factors; zero confidence in either factor
	// zeroes the joint confidence.
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
	theirsContent string,
	theirsSyms []core.SymbolData,
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
		mergedByKey[mstrat.MergeKey(s)] = s
	}

	usedKeys := map[string]bool{}
	for _, s := range oursSyms {
		merged, ok := mergedByKey[mstrat.MergeKey(s)]
		if !ok {
			// symbol was dropped in merge
			spans = append(spans, span{start: s.Span.Start, end: s.Span.End, kind: "skip", key: mstrat.MergeKey(s)})
			continue
		}
		usedKeys[mstrat.MergeKey(s)] = true
		if merged.Body == s.Body {
			// Unchanged relative to ours — leave the original lines alone.
			// Body can carry a synthetic standalone-decl form (Go const/var
			// group members), so substituting it would mangle formatting.
			continue
		}
		body := adaptBodyToContext(merged.Body, oursLines, s.Span.Start, lang)
		spans = append(spans, span{start: s.Span.Start, end: s.Span.End, body: body, kind: "symbol", key: mstrat.MergeKey(s)})
	}
	// If the merged import set is identical to ours, keep ours' original
	// import lines (no re-rendering: preserves single-import style, grouping,
	// comments).
	keepOursImports := sameImportSet(oursImps, mergedImps)
	if !keepOursImports {
		for _, i := range oursImps {
			spans = append(spans, span{start: i.Line, end: i.Line, kind: "import"})
		}
	}

	// Sort spans by start (wider first on ties), then drop any span that
	// overlaps an earlier one. Overlapping substitution would duplicate or
	// corrupt output; the contained content is already inside the kept span.
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end > spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	kept := spans[:0]
	maxEnd := 0
	for _, s := range spans {
		if s.start > maxEnd {
			kept = append(kept, s)
			maxEnd = s.end
		}
	}
	spans = kept

	// Symbols added by theirs are inserted near their original neighbors:
	// right after the symbol that precedes them in the theirs buffer when
	// that neighbor exists in ours, carrying contiguous leading comment lines
	// along. Symbols without a resolvable anchor are appended at the end.
	theirsLines := strings.Split(theirsContent, "\n")
	oursSpanByKey := make(map[string]core.LineRange, len(oursSyms))
	for _, s := range oursSyms {
		oursSpanByKey[mstrat.MergeKey(s)] = s.Span
	}
	sortedTheirs := make([]core.SymbolData, len(theirsSyms))
	copy(sortedTheirs, theirsSyms)
	sort.SliceStable(sortedTheirs, func(i, j int) bool { return sortedTheirs[i].Span.Start < sortedTheirs[j].Span.Start })
	theirsPos := make(map[string]int, len(sortedTheirs))
	for i, s := range sortedTheirs {
		theirsPos[mstrat.MergeKey(s)] = i
	}
	insertAfter := map[int][]string{}
	var trailing []string
	for _, s := range mergedSyms {
		k := mstrat.MergeKey(s)
		if usedKeys[k] {
			continue
		}
		text := theirsSymbolText(theirsLines, s)
		anchor := 0
		if pos, ok := theirsPos[k]; ok {
			for j := pos - 1; j >= 0; j-- {
				if span, ok2 := oursSpanByKey[mstrat.MergeKey(sortedTheirs[j])]; ok2 {
					anchor = span.End
					break
				}
			}
		}
		if anchor > 0 {
			insertAfter[anchor] = append(insertAfter[anchor], text)
		} else {
			trailing = append(trailing, text)
		}
	}

	importBlock := renderImportBlock(mergedImps, lang)
	importsEmitted := keepOursImports

	var out []string
	flushInserts := func(afterLine int) {
		for _, t := range insertAfter[afterLine] {
			out = append(out, "", t)
		}
	}
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
			flushInserts(lineNo)
			continue
		}
		out = append(out, oursLines[lineNo-1])
		flushInserts(lineNo)
	}

	// If we never had an import line on ours but merged has imports → insert
	// after the first non-empty non-package/comment line (best-effort).
	if !importsEmitted && importBlock != "" && len(mergedImps) > 0 {
		out = injectImportsAfterPackageDecl(out, importBlock, lang)
	}

	// Symbols with no positional anchor (added by theirs in a region ours
	// doesn't share) go at the end.
	for _, t := range trailing {
		out = append(out, "", t)
	}

	return strings.Join(out, "\n")
}

// theirsSymbolText returns the text to insert for a theirs-added symbol,
// expanded to include contiguous leading comment lines from the theirs
// buffer (symbol spans exclude doc comments). Falls back to the merged Body
// when the span doesn't reproduce the body — e.g. synthetic conflict-marker
// bodies.
func theirsSymbolText(theirsLines []string, s core.SymbolData) string {
	if s.Span.Start < 1 || s.Span.End > len(theirsLines) || s.Span.Start > s.Span.End {
		return s.Body
	}
	spanText := strings.Join(theirsLines[s.Span.Start-1:s.Span.End], "\n")
	if strings.TrimSpace(spanText) != strings.TrimSpace(s.Body) {
		return s.Body
	}
	start := s.Span.Start
	for start-1 >= 1 {
		t := strings.TrimSpace(theirsLines[start-2])
		if t != "" && (strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") ||
			strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*")) {
			start--
			continue
		}
		break
	}
	return strings.Join(theirsLines[start-1:s.Span.End], "\n")
}

// adaptBodyToContext strips the synthetic decl keyword astkit prepends to Go
// const/var/type group members. Their Body is a standalone declaration while
// their span covers only the bare spec inside a `const (` / `var (` / `type (`
// block; substituting the standalone form verbatim would duplicate the
// keyword inside the block.
func adaptBodyToContext(body string, oursLines []string, startLine int, lang core.LanguageKey) string {
	if lang != core.LangGo || startLine < 1 || startLine > len(oursLines) {
		return body
	}
	orig := strings.TrimSpace(oursLines[startLine-1])
	for _, kw := range []string{"const ", "var ", "type "} {
		if strings.HasPrefix(body, kw) && !strings.HasPrefix(orig, kw) {
			return strings.TrimPrefix(body, kw)
		}
	}
	return body
}

// sameImportSet reports whether a and b contain the same (alias, path) pairs.
func sameImportSet(a, b []core.ImportStatement) bool {
	if len(a) != len(b) {
		return false
	}
	count := make(map[string]int, len(a))
	for _, i := range a {
		count[i.Alias+"\x00"+i.Path]++
	}
	for _, i := range b {
		k := i.Alias + "\x00" + i.Path
		count[k]--
		if count[k] < 0 {
			return false
		}
	}
	return true
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
