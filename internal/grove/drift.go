package grove

import (
	"context"
	"fmt"

	groveeng "github.com/provasign/grove/pkg/grove"
)

// GraphSnapshot is an opaque capture of the Grove code graph, taken before a
// merge so the structural delta can be computed afterwards.
type GraphSnapshot []groveeng.Symbol

// DriftSymbol is one symbol-level entry in a graph drift report.
type DriftSymbol struct {
	FilePath      string `json:"filePath"`
	QualifiedName string `json:"qualifiedName"`
	Kind          string `json:"kind"`
	Change        string `json:"change"` // added | removed | signature | body
	Exported      bool   `json:"exported"`
	OldSignature  string `json:"oldSignature,omitempty"`
	NewSignature  string `json:"newSignature,omitempty"`
}

// Drift is the structural delta of the code graph across a merge, matched by
// stable symbol identity (file path + qualified name + kind) — line shifts
// and content-hash churn do not register. This is the evidence the
// stale-context loop delivers: which symbols changed under everyone's feet,
// and which of those changes break the exported surface.
type Drift struct {
	Added    []DriftSymbol `json:"added,omitempty"`
	Removed  []DriftSymbol `json:"removed,omitempty"`
	Changed  []DriftSymbol `json:"changed,omitempty"`
	Breaking []DriftSymbol `json:"breaking,omitempty"`
}

// Empty reports whether the merge produced no structural change.
func (d Drift) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Summary is a one-line human rendering for merge-driver stderr output.
func (d Drift) Summary() string {
	return fmt.Sprintf("+%d added, -%d removed, ~%d changed symbol(s), %d breaking",
		len(d.Added), len(d.Removed), len(d.Changed), len(d.Breaking))
}

// Snapshot captures the current graph for later drift computation. Call it
// before writing merged content; the index reflects the pre-merge working
// tree because EnsureIndexed runs when the client is constructed.
func (c *Client) Snapshot(ctx context.Context) (GraphSnapshot, error) {
	e, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return e.SnapshotSymbols(ctx), nil
}

// DriftSince reindexes (delta — unchanged files are skipped) and returns the
// structural delta against a snapshot taken before the merge. Use after the
// working tree actually contains the merged result (e.g. post-merge hooks).
func (c *Client) DriftSince(ctx context.Context, snap GraphSnapshot) (Drift, error) {
	e, err := c.ensure(ctx)
	if err != nil {
		return Drift{}, err
	}
	if _, err := e.Index(ctx, c.root); err != nil {
		return Drift{}, fmt.Errorf("grove reindex for drift: %w", err)
	}
	return driftFromDiff(e.DiffSince(ctx, snap)), nil
}

// DriftForMergedFile computes the structural delta a merge produces for one
// file, from the merged bytes alone. This is the merge-driver path: git only
// writes the driver's output to the worktree after the driver exits, so the
// post-merge state is never on disk while we run — Grove parses the merged
// content in memory instead.
func (c *Client) DriftForMergedFile(ctx context.Context, snap GraphSnapshot, relPath string, merged []byte) (Drift, error) {
	e, err := c.ensure(ctx)
	if err != nil {
		return Drift{}, err
	}
	diff, err := e.DiffAgainstFileContent(snap, relPath, merged)
	if err != nil {
		return Drift{}, err
	}
	return driftFromDiff(diff), nil
}

func driftFromDiff(diff groveeng.GraphDiff) Drift {
	var d Drift
	for i := range diff.Added {
		d.Added = append(d.Added, driftSymbol(&diff.Added[i], "added"))
	}
	for i := range diff.Removed {
		d.Removed = append(d.Removed, driftSymbol(&diff.Removed[i], "removed"))
	}
	for _, change := range diff.Changed {
		d.Changed = append(d.Changed, driftChange(change))
	}
	for _, change := range diff.BreakingChanges {
		d.Breaking = append(d.Breaking, driftChange(change))
	}
	return d
}

func driftSymbol(s *groveeng.Symbol, change string) DriftSymbol {
	out := DriftSymbol{
		FilePath:      s.FilePath,
		QualifiedName: s.QualifiedName,
		Kind:          string(s.Kind),
		Change:        change,
		Exported:      s.Exports,
	}
	switch change {
	case "added":
		out.NewSignature = s.Signature
	case "removed":
		out.OldSignature = s.Signature
	}
	return out
}

func driftChange(change groveeng.SymbolChange) DriftSymbol {
	ref := change.Before
	if ref == nil {
		ref = change.After
	}
	kind := "body"
	if change.SignatureChanged {
		kind = "signature"
	}
	if change.After == nil {
		kind = "removed"
	}
	out := DriftSymbol{
		FilePath:      ref.FilePath,
		QualifiedName: ref.QualifiedName,
		Kind:          string(ref.Kind),
		Change:        kind,
		Exported:      ref.Exports,
	}
	if change.Before != nil {
		out.OldSignature = change.Before.Signature
	}
	if change.After != nil {
		out.NewSignature = change.After.Signature
	}
	return out
}
