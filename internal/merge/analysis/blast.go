package analysis

import (
	"context"

	"github.com/tabladrum/grove-suite/fuse/internal/grove"
)

// BlastRadius returns the file paths transitively impacted by a symbol per
// Grove's impact query. Returns nil on Grove failure.
func BlastRadius(ctx context.Context, g GroveLike, symbol string, maxDepth int) []string {
	if g == nil {
		return nil
	}
	nodes, err := g.Impact(ctx, symbol, maxDepth)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	for _, n := range nodes {
		if n.FilePath == "" || seen[n.FilePath] {
			continue
		}
		seen[n.FilePath] = true
		files = append(files, n.FilePath)
	}
	return files
}

// Dependents returns the files that depend on filePath per Grove (reverse
// direction of /deps).
func Dependents(ctx context.Context, g GroveLike, filePath string) []string {
	if g == nil {
		return nil
	}
	edges, err := g.Deps(ctx, filePath)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range edges {
		if e.From == "" || seen[e.From] || e.From == filePath {
			continue
		}
		seen[e.From] = true
		out = append(out, e.From)
	}
	_ = grove.Edge{}
	return out
}
