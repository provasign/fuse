package merge

import (
	"context"
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
)

// TestMergeRenameVsSignatureEdit drives the full pipeline: ours renames a
// function, theirs changes its signature on the same line — git and the LCS
// line merge both conflict, and the rename-aware symbol merge resolves it.
func TestMergeRenameVsSignatureEdit(t *testing.T) {
	base := []byte(`package main

func ProcessOrder(id string) error {
	validate(id)
	return queue.Push(id)
}
`)
	ours := []byte(`package main

func HandleOrder(id string) error {
	validate(id)
	return queue.Push(id)
}
`)
	theirs := []byte(`package main

func ProcessOrder(id string, force bool) error {
	validate(id)
	return queue.Push(id)
}
`)

	im := New(nil)
	im.EnableBreaking = false
	im.EnableContext = false
	res, err := im.Merge(context.Background(), base, ours, theirs, core.LangGo, "orders.go")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict || res.Strategy == core.StrategyHandoff {
		t.Fatalf("rename-vs-edit should auto-merge, got strategy=%s conflict=%v\n%s",
			res.Strategy, res.HasConflict, res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "func HandleOrder(id string, force bool) error") {
		t.Fatalf("merged content missing renamed+re-signatured function:\n%s", res.MergedContent)
	}
	if strings.Contains(res.MergedContent, "ProcessOrder") {
		t.Fatalf("old name survived the merge:\n%s", res.MergedContent)
	}
}
