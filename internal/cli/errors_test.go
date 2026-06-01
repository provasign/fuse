package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunLineMerge_FromUnsupportedLang(t *testing.T) {
	dir := t.TempDir()
	withDir(t, dir)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	base := filepath.Join(dir, "b")
	ours := filepath.Join(dir, "o")
	theirs := filepath.Join(dir, "t")
	_ = os.WriteFile(base, []byte("a\nb\nc\n"), 0o644)
	// conflicting changes line 2
	_ = os.WriteFile(ours, []byte("a\nB1\nc\n"), 0o644)
	_ = os.WriteFile(theirs, []byte("a\nB2\nc\n"), 0o644)
	code := Run([]string{"merge", base, ours, theirs, "x.txt"})
	if code != 0 && code != 1 {
		t.Errorf("got %d", code)
	}
}
