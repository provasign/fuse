package grove

import (
	"context"
	"testing"
)

// TestDriftReportsRename: a symbol renamed with an identical body must show
// as one "renamed" entry carrying the old name — not an unrelated
// removal + addition.
func TestDriftReportsRename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "orders.go", `package main

func ProcessOrder(id string) error {
	validate(id)
	return queue.Push(id)
}
`)
	c := New("").WithTokenFromDir(dir)
	defer c.Close()
	ctx := context.Background()
	if err := c.Index(ctx, dir); err != nil {
		t.Fatalf("index: %v", err)
	}
	snap, err := c.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	drift, err := c.DriftForMergedFile(ctx, snap, "orders.go", []byte(`package main

func HandleOrder(id string) error {
	validate(id)
	return queue.Push(id)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(drift.Renamed) != 1 {
		t.Fatalf("renamed = %+v (full drift %+v)", drift.Renamed, drift)
	}
	r := drift.Renamed[0]
	if r.QualifiedName != "HandleOrder" || r.RenamedFrom != "ProcessOrder" || r.Change != "renamed" {
		t.Fatalf("rename entry = %+v", r)
	}
	if len(drift.Added) != 0 || len(drift.Removed) != 0 {
		t.Fatalf("rename leaked into add/remove: %+v", drift)
	}
	// Exported rename breaks callers of the old name.
	if len(drift.Breaking) != 1 || drift.Breaking[0].RenamedFrom != "ProcessOrder" {
		t.Fatalf("breaking = %+v", drift.Breaking)
	}
}

// TestDriftForMergedFile covers the merge-driver flow: the merged content
// exists only in memory (git writes the driver output to the worktree after
// the driver exits), so drift must be computable without touching disk —
// and must report only symbols whose structure actually changed, not the
// whole file's ID churn.
func TestDriftForMergedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "auth.go", `package main

func Login(user string) error { return nil }

func Logout() {}
`)
	c := New("").WithTokenFromDir(dir)
	defer c.Close()
	ctx := context.Background()
	if err := c.Index(ctx, dir); err != nil {
		t.Fatalf("index: %v", err)
	}

	snap, err := c.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap) == 0 {
		t.Fatal("empty snapshot")
	}

	merged := []byte(`package main

// Login now requires a password.
func Login(user, password string) error { return nil }

func Logout() {}

func Refresh() {}
`)
	drift, err := c.DriftForMergedFile(ctx, snap, "auth.go", merged)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected drift")
	}
	if len(drift.Added) != 1 || drift.Added[0].QualifiedName != "Refresh" {
		t.Fatalf("added = %+v", drift.Added)
	}
	if len(drift.Changed) != 1 || drift.Changed[0].QualifiedName != "Login" || drift.Changed[0].Change != "signature" {
		t.Fatalf("changed = %+v", drift.Changed)
	}
	if len(drift.Breaking) != 1 || drift.Breaking[0].QualifiedName != "Login" {
		t.Fatalf("breaking = %+v", drift.Breaking)
	}
	if drift.Changed[0].OldSignature == drift.Changed[0].NewSignature {
		t.Fatalf("signatures should differ: %+v", drift.Changed[0])
	}

	// Logout is untouched (only shifted by the comment line) — it must not
	// appear anywhere in the drift.
	for _, list := range [][]DriftSymbol{drift.Added, drift.Removed, drift.Changed, drift.Breaking} {
		for _, d := range list {
			if d.QualifiedName == "Logout" {
				t.Fatalf("untouched Logout misreported: %+v", d)
			}
		}
	}

	// Identical content ⇒ empty drift.
	same, err := c.DriftForMergedFile(ctx, snap, "auth.go", []byte(`package main

func Login(user string) error { return nil }

func Logout() {}
`))
	if err != nil {
		t.Fatalf("drift same: %v", err)
	}
	if !same.Empty() {
		t.Fatalf("identical merge content must produce empty drift: %+v", same)
	}
}
