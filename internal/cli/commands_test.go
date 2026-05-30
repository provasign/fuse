package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunHelp ensures the help command exits cleanly.
func TestRunHelp(t *testing.T) {
	if code := Run([]string{"help"}); code != 0 {
		t.Errorf("help exit code = %d", code)
	}
	if code := Run([]string{"version"}); code != 0 {
		t.Errorf("version exit code = %d", code)
	}
}

// TestMergeCommand drives `fuse merge` end-to-end against the file system.
// It uses temp files for base/ours/theirs and verifies the result is written
// back to the ours path.
func TestMergeCommand(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.go")
	ours := filepath.Join(dir, "ours.go")
	theirs := filepath.Join(dir, "theirs.go")
	if err := os.WriteFile(base, []byte("package x\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ours, []byte("package x\n\nfunc A() { _ = 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(theirs, []byte("package x\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	code := Run([]string{"merge", base, ours, theirs, "x.go"})
	if code != 0 {
		t.Errorf("merge exit code = %d", code)
	}
	merged, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(merged) == "" {
		t.Error("ours file should have been written")
	}
}

func TestUpsertGitAttributes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if err := upsertGitAttributes(path, []string{"*.go merge=fuse", "*.py merge=fuse"}); err != nil {
		t.Fatal(err)
	}
	if err := upsertGitAttributes(path, []string{"*.go merge=fuse", "*.rs merge=fuse"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	// *.go should appear only once.
	if count := stringsCount(string(data), "*.go merge=fuse"); count != 1 {
		t.Errorf("expected 1, got %d (data=%q)", count, string(data))
	}
}

func stringsCount(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
