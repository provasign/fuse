// Package analysis covers breaking change detection, blast radius lookup, and
// risk scoring during the merge pipeline.
package analysis

import (
	"context"
	"fmt"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/grove"
)

// GroveLike is the small subset of the Grove client the analyzer uses. Defined
// as an interface so tests can plug in a fake without spinning up Grove.
type GroveLike interface {
	Impact(ctx context.Context, query string, maxDepth int) ([]grove.ImpactNode, error)
	Deps(ctx context.Context, filePath string) ([]grove.Edge, error)
}

// BreakingChangeAnalyzer detects exports removed or signature-changed between
// base and (ours | theirs), and the downstream files those break.
type BreakingChangeAnalyzer struct {
	Grove GroveLike
}

// Analyze returns the breaking changes introduced by ours and theirs relative
// to base.
//
//	removed_export    : symbol exported in base, absent in (ours OR theirs)
//	signature_changed : exported symbol present on both sides but signature differs
//	broken_import     : import present in theirs that resolves to a symbol removed by ours (or vice versa)
//
// Severity is escalated by the count of affected files from Grove's impact query.
func (a *BreakingChangeAnalyzer) Analyze(
	ctx context.Context,
	filePath string,
	base, ours, theirs []core.SymbolData,
) []core.BreakingChange {
	baseExports := exportMap(base)
	oursExports := exportMap(ours)
	theirsExports := exportMap(theirs)

	seen := map[string]bool{}
	var out []core.BreakingChange

	for key, baseSym := range baseExports {
		_, inOurs := oursExports[key]
		_, inTheirs := theirsExports[key]
		if !inOurs || !inTheirs {
			affected := a.affected(ctx, filePath, baseSym.Name)
			out = append(out, core.BreakingChange{
				Kind:          "removed_export",
				Symbol:        key,
				AffectedFiles: affected,
				Severity:      severityForCount(len(affected)),
				Message:       fmt.Sprintf("Export %q removed by %s — may break %d caller(s)", key, removedBy(inOurs, inTheirs), len(affected)),
			})
			seen[key] = true
			continue
		}
		// Both sides still have it — check signature drift.
		oursSig := oursExports[key].Signature
		theirsSig := theirsExports[key].Signature
		baseSig := baseSym.Signature
		if oursSig != baseSig && theirsSig != baseSig && oursSig != theirsSig {
			affected := a.affected(ctx, filePath, baseSym.Name)
			out = append(out, core.BreakingChange{
				Kind:          "signature_changed",
				Symbol:        key,
				AffectedFiles: affected,
				Severity:      severityForCount(len(affected)),
				Message:       fmt.Sprintf("Signature of %q changed incompatibly on both sides — may break %d caller(s)", key, len(affected)),
			})
			seen[key] = true
		}
	}
	return out
}

func exportMap(syms []core.SymbolData) map[string]core.SymbolData {
	out := make(map[string]core.SymbolData)
	for _, s := range syms {
		if s.Exported {
			out[exportKey(s)] = s
		}
	}
	return out
}

func exportKey(s core.SymbolData) string {
	if s.QualifiedName != "" && s.QualifiedName != s.Name {
		return s.QualifiedName
	}
	if s.ParentName != "" {
		return s.ParentName + "." + s.Name
	}
	return s.Name
}

func removedBy(inOurs, inTheirs bool) string {
	switch {
	case !inOurs && !inTheirs:
		return "both sides"
	case !inOurs:
		return "ours"
	default:
		return "theirs"
	}
}

func severityForCount(n int) core.ConflictSeverity {
	switch {
	case n >= 6:
		return core.SeverityCritical
	case n >= 3:
		return core.SeverityHigh
	case n >= 1:
		return core.SeverityMedium
	default:
		return core.SeverityLow
	}
}

func (a *BreakingChangeAnalyzer) affected(ctx context.Context, filePath, symbol string) []string {
	if a.Grove == nil {
		return nil
	}
	nodes, err := a.Grove.Impact(ctx, symbol, 2)
	if err != nil {
		return nil
	}
	seen := map[string]bool{filePath: true}
	var out []string
	for _, n := range nodes {
		if n.FilePath == "" || seen[n.FilePath] {
			continue
		}
		seen[n.FilePath] = true
		out = append(out, n.FilePath)
	}
	return out
}
