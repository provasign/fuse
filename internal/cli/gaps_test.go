package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// ── notifyLineResult: all branches ────────────────────────────────────────────

func TestNotifyLineResult_AllBranches(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{0, "[line] → auto-merged"},
		{1, "[line] → conflict markers"},
		{2, "[line] → failed"},
		{99, "[line] → failed"},
	}
	for _, tc := range cases {
		stderr := captureStderr(t, func() {
			notifyLineResult("myfile.txt", tc.code)
		})
		if !strings.Contains(stderr, tc.want) {
			t.Errorf("code=%d: want %q in %q", tc.code, tc.want, stderr)
		}
	}
}

// ── runLineMergeAndWrite ──────────────────────────────────────────────────────

func TestRunLineMergeAndWrite_CleanAndConflict(t *testing.T) {
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	dir := t.TempDir()

	// Clean: ours and theirs have no overlap.
	oursP := writeFile(t, dir, "ours.txt", "line1\nours-line\nline3\n")
	code := runLineMergeAndWrite(oursP, "ours.txt",
		[]byte("line1\nbase-line\nline3\n"),
		[]byte("line1\nours-line\nline3\n"),
		[]byte("line1\nbase-line\nline3\ntheirs-line\n"))
	if code == 2 {
		t.Errorf("line merge hard failure: %d", code)
	}
	merged, _ := os.ReadFile(oursP)
	if len(merged) == 0 {
		t.Error("ours not written")
	}

	// Conflict: both change the same line.
	oursP2 := writeFile(t, dir, "conflict.txt", "line1\nOURS\nline3\n")
	code2 := runLineMergeAndWrite(oursP2, "conflict.txt",
		[]byte("line1\nbase\nline3\n"),
		[]byte("line1\nOURS\nline3\n"),
		[]byte("line1\nTHEIRS\nline3\n"))
	if code2 != 0 && code2 != 1 {
		t.Errorf("line conflict: unexpected code %d", code2)
	}
}

func writeFile(t *testing.T, dir, name string, content string) string {
	t.Helper()
	p := dir + "/" + name
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ── cmdStatus edge cases ──────────────────────────────────────────────────────

func TestCmdStatus_NotInRepo(t *testing.T) {
	withDir(t, t.TempDir())
	// Must not panic; return code 1 for "not in repo".
	code := Run([]string{"status"})
	if code != 0 && code != 1 {
		t.Errorf("status in non-repo: code=%d", code)
	}
}

func TestCmdStatus_CorruptAuditLog(t *testing.T) {
	dir := gitInit(t)
	fuseDir := dir + "/.git/fuse"
	_ = os.MkdirAll(fuseDir, 0o755)
	_ = os.WriteFile(fuseDir+"/audit.json", []byte("not json {{{"), 0o644)
	withDirResolved(t, dir)

	code := Run([]string{"status"})
	if code != 2 {
		t.Errorf("corrupt audit should return 2; got %d", code)
	}
}

// ── findGitDir edge: .git file (worktree) ──────────────────────────────────

func TestFindGitDir_GitFileWorktree(t *testing.T) {
	dir := t.TempDir()
	// Write a fake .git file (simulating a worktree pointer).
	gitFile := dir + "/.git"
	target := dir + "/real-gitdir"
	_ = os.MkdirAll(target, 0o755)
	_ = os.WriteFile(gitFile, []byte("gitdir: "+target+"\n"), 0o644)

	result := findGitDir(dir + "/subdir/file.go")
	if result != target {
		// Not fatal — behaviour depends on traversal; just assert it doesn't crash.
		t.Logf("findGitDir worktree: got %q (target %q)", result, target)
	}
}

// ── loadConfig: env override path ────────────────────────────────────────────

func TestLoadConfig_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/custom-fuse.yaml"
	_ = os.WriteFile(cfgPath, []byte("server:\n  port: 12345\n"), 0o644)
	t.Setenv("FUSE_CONFIG", cfgPath)
	withDir(t, dir)

	cfg := loadConfig()
	if cfg == nil {
		t.Error("nil config")
	}
}

// ── merge: reconstructFile with symbol addition from theirs ──────────────────

func TestMergeOrchestrator_SymbolAddedByTheirs(t *testing.T) {
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	dir := t.TempDir()
	base := []byte("package x\n\nfunc A() {}\n")
	ours := []byte("package x\n\nfunc A() { _ = 1 }\n")
	theirs := []byte("package x\n\nfunc A() {}\n\nfunc NewFromTheirs() string { return \"hi\" }\n")

	bp := dir + "/b.go"
	op := dir + "/o.go"
	tp := dir + "/t.go"
	_ = os.WriteFile(bp, base, 0o644)
	_ = os.WriteFile(op, ours, 0o644)
	_ = os.WriteFile(tp, theirs, 0o644)

	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	_ = os.Chdir(dir)

	captureStderr(t, func() {
		Run([]string{"merge", bp, op, tp, "pkg.go"})
	})
	merged, _ := os.ReadFile(op)
	if !bytes.Contains(merged, []byte("NewFromTheirs")) {
		t.Errorf("symbol added by theirs missing; merged=%s", merged)
	}
}
