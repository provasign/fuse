package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/merge"
	"github.com/provasign/fuse/internal/merge/analysis"
	"github.com/provasign/fuse/internal/parser"
)

// Caps keep results summary-first: counts are always exact; per-item detail
// is truncated so one tool call can't flood an agent's context window.
const (
	maxCheckedFiles  = 200
	maxConflictItems = 20
	maxBreakingItems = 10
	maxImpactNodes   = 50
)

// Handler holds the backend state shared by the fuse_* tools.
type Handler struct {
	Root  string
	Grove analysis.GroveLike // nil when Grove is unavailable; tools degrade
	Merge *merge.IntelliMerge
}

// NewHandler constructs a handler rooted at root.
func NewHandler(root string, groveClient analysis.GroveLike) *Handler {
	im := merge.New(groveClient)
	im.EnableBreaking = groveClient != nil
	im.EnableContext = false // MCP results carry their own structure
	return &Handler{Root: root, Grove: groveClient, Merge: im}
}

// Invoke dispatches one tool call.
func (h *Handler) Invoke(name string, args map[string]any) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	switch name {
	case "fuse_merge_check":
		return h.toolMergeCheck(ctx, args)
	case "fuse_preview":
		return h.toolPreview(ctx, args)
	case "fuse_resolve":
		return h.toolResolve(args)
	case "fuse_impact":
		return h.toolImpact(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ─── fuse_merge_check ────────────────────────────────────────────────────────

// ConflictItem is one file that would not merge cleanly.
type ConflictItem struct {
	File       string  `json:"file"`
	Strategy   string  `json:"strategy"`
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
}

// BreakingItem is one breaking change a clean auto-merge would introduce.
type BreakingItem struct {
	File     string `json:"file"`
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
}

// MergeCheckResult is the fuse_merge_check response. Verdict first; lists
// only where something needs attention.
type MergeCheckResult struct {
	Verdict          string         `json:"verdict"` // clean | conflicts | error detail in message
	Ref              string         `json:"ref"`
	MergeBase        string         `json:"mergeBase"`
	OverlappingFiles int            `json:"overlappingFiles"`
	AutoMergeable    int            `json:"autoMergeable"`
	ConflictCount    int            `json:"conflictCount"`
	Conflicts        []ConflictItem `json:"conflicts,omitempty"`
	BreakingCount    int            `json:"breakingCount"`
	Breaking         []BreakingItem `json:"breaking,omitempty"`
	Note             string         `json:"note,omitempty"`
}

// toolMergeCheck dry-runs a semantic merge of HEAD against ref: would the
// agent's committed work merge cleanly? Nothing is written; blobs come from
// git and merge in memory.
func (h *Handler) toolMergeCheck(ctx context.Context, args map[string]any) (any, error) {
	ref := stringArg(args, "ref", "")
	if ref == "" {
		ref = "origin/main"
	}
	mergeBase, err := h.git("merge-base", "HEAD", ref)
	if err != nil {
		return nil, fmt.Errorf("merge-base HEAD %s: %w", ref, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	ours, err := h.gitNameOnly(mergeBase, "HEAD")
	if err != nil {
		return nil, err
	}
	theirs, err := h.gitNameOnly(mergeBase, ref)
	if err != nil {
		return nil, err
	}
	theirSet := make(map[string]bool, len(theirs))
	for _, f := range theirs {
		theirSet[f] = true
	}
	var overlap []string
	for _, f := range ours {
		if theirSet[f] {
			overlap = append(overlap, f)
		}
	}
	sort.Strings(overlap)

	result := MergeCheckResult{
		Verdict:          "clean",
		Ref:              ref,
		MergeBase:        shortSHA(mergeBase),
		OverlappingFiles: len(overlap),
	}
	if len(overlap) > maxCheckedFiles {
		result.Note = fmt.Sprintf("checked first %d of %d overlapping files", maxCheckedFiles, len(overlap))
		overlap = overlap[:maxCheckedFiles]
	}

	for _, file := range overlap {
		baseBlob := h.gitBlobOrEmpty(mergeBase, file)
		oursBlob := h.gitBlobOrEmpty("HEAD", file)
		theirsBlob := h.gitBlobOrEmpty(ref, file)
		if isBinary(baseBlob) || isBinary(oursBlob) || isBinary(theirsBlob) {
			continue
		}
		lang := parser.DetectLanguage(file, string(oursBlob))
		res, err := h.Merge.Merge(ctx, baseBlob, oursBlob, theirsBlob, lang, file)
		if err != nil {
			result.ConflictCount++
			if len(result.Conflicts) < maxConflictItems {
				result.Conflicts = append(result.Conflicts, ConflictItem{File: file, Strategy: "error", Severity: "HIGH"})
			}
			continue
		}
		if res.HasConflict || res.Strategy == core.StrategyHandoff {
			result.ConflictCount++
			if len(result.Conflicts) < maxConflictItems {
				result.Conflicts = append(result.Conflicts, ConflictItem{
					File:       file,
					Strategy:   string(res.Strategy),
					Type:       string(res.ConflictType),
					Severity:   string(res.Severity),
					Confidence: round2(res.Confidence),
				})
			}
			continue
		}
		result.AutoMergeable++
		for _, bc := range res.BreakingChanges {
			result.BreakingCount++
			if len(result.Breaking) < maxBreakingItems {
				result.Breaking = append(result.Breaking, BreakingItem{
					File: file, Symbol: bc.Symbol, Kind: bc.Kind, Severity: string(bc.Severity),
				})
			}
		}
	}
	if result.ConflictCount > 0 {
		result.Verdict = "conflicts"
	}
	return result, nil
}

// ─── fuse_preview ────────────────────────────────────────────────────────────

// PreviewResult is the fuse_preview response: the merged content is the
// payload the caller asked for; everything else is one line of metadata.
type PreviewResult struct {
	Merged      string  `json:"merged"`
	HasConflict bool    `json:"hasConflict"`
	Strategy    string  `json:"strategy"`
	Confidence  float64 `json:"confidence"`
}

func (h *Handler) toolPreview(ctx context.Context, args map[string]any) (any, error) {
	ours := stringArg(args, "ours", "")
	theirs := stringArg(args, "theirs", "")
	if ours == "" && theirs == "" {
		return nil, fmt.Errorf("fuse_preview: ours and theirs are required")
	}
	base := stringArg(args, "base", "")
	path := stringArg(args, "path", "file.txt")
	lang := parser.DetectLanguage(path, ours)
	res, err := h.Merge.Merge(ctx, []byte(base), []byte(ours), []byte(theirs), lang, path)
	if err != nil {
		return nil, err
	}
	return PreviewResult{
		Merged:      res.MergedContent,
		HasConflict: res.HasConflict,
		Strategy:    string(res.Strategy),
		Confidence:  round2(res.Confidence),
	}, nil
}

// ─── fuse_resolve ────────────────────────────────────────────────────────────

// ResolveResult is the fuse_resolve response.
type ResolveResult struct {
	File      string `json:"file"`
	Prompt    string `json:"prompt,omitempty"`
	Validated bool   `json:"validated"`
	Applied   bool   `json:"applied"`
}

// toolResolve has two modes. Without a resolution it returns the handoff
// prompt so the agent can resolve in-context. With a resolution it
// validates (non-empty, no markers, parses) and optionally applies.
func (h *Handler) toolResolve(args map[string]any) (any, error) {
	promptPath := stringArg(args, "prompt", "")
	if promptPath == "" {
		return nil, fmt.Errorf("fuse_resolve: prompt (path to handoff prompt) is required")
	}
	if !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(h.Root, promptPath)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, err
	}
	target := promptTargetFile(prompt)
	if target == "" {
		return nil, fmt.Errorf("fuse_resolve: prompt has no '- **File:**' line; cannot locate the conflicted file")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(h.Root, target)
	}

	resolution := stringArg(args, "resolution", "")
	if resolution == "" {
		return ResolveResult{File: target, Prompt: string(prompt)}, nil
	}
	if err := merge.ValidateResolution(target, []byte(resolution)); err != nil {
		return nil, fmt.Errorf("resolution rejected: %w", err)
	}
	out := ResolveResult{File: target, Validated: true}
	if boolArg(args, "apply") {
		if err := os.WriteFile(target, []byte(resolution), 0o644); err != nil {
			return nil, err
		}
		out.Applied = true
	}
	return out, nil
}

// promptTargetFile extracts the conflicted file path from the handoff
// prompt's `- **File:** <path>` summary line.
func promptTargetFile(prompt []byte) string {
	for _, line := range strings.Split(string(prompt), "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "- **File:**"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// ─── fuse_impact ─────────────────────────────────────────────────────────────

// ImpactNode is one blast-radius entry.
type ImpactNode struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Name string `json:"name"`
}

// ImpactResult is the fuse_impact response.
type ImpactResult struct {
	Query string       `json:"query"`
	Count int          `json:"count"`
	Nodes []ImpactNode `json:"nodes,omitempty"`
	Note  string       `json:"note,omitempty"`
}

func (h *Handler) toolImpact(ctx context.Context, args map[string]any) (any, error) {
	if h.Grove == nil {
		return nil, fmt.Errorf("fuse_impact: Grove index unavailable")
	}
	query := stringArg(args, "query", "")
	if query == "" {
		return nil, fmt.Errorf("fuse_impact: query (file or symbol) is required")
	}
	maxDepth := intArg(args, "maxDepth", 3)

	queries := []string{query}
	// File queries expand to the symbols the file defines.
	if _, err := os.Stat(filepath.Join(h.Root, filepath.FromSlash(query))); err == nil {
		if edges, derr := h.Grove.Deps(ctx, query); derr == nil {
			var syms []string
			for _, e := range edges {
				if e.Type == "defines" {
					syms = append(syms, e.To)
				}
			}
			if len(syms) > 0 {
				queries = syms
			}
		}
	}

	seen := map[string]bool{}
	result := ImpactResult{Query: query}
	for _, q := range queries {
		nodes, err := h.Grove.Impact(ctx, q, maxDepth)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			result.Count++
			if len(result.Nodes) < maxImpactNodes {
				result.Nodes = append(result.Nodes, ImpactNode{ID: n.ID, File: n.FilePath, Name: n.Name})
			}
		}
	}
	if result.Count > len(result.Nodes) {
		result.Note = fmt.Sprintf("showing %d of %d impacted symbols", len(result.Nodes), result.Count)
	}
	return result, nil
}

// ─── git helpers ─────────────────────────────────────────────────────────────

func (h *Handler) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", h.Root}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func (h *Handler) gitNameOnly(from, to string) ([]string, error) {
	out, err := h.git("diff", "--name-only", from+".."+to)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// gitBlobOrEmpty returns the blob at rev:path, or empty content when the
// path does not exist at rev (added/deleted on one side).
func (h *Handler) gitBlobOrEmpty(rev, path string) []byte {
	out, err := h.git("show", rev+":"+path)
	if err != nil {
		return nil
	}
	return []byte(out)
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func isBinary(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	return bytes.IndexByte(data[:limit], 0) >= 0
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func stringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func intArg(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}
