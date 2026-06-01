package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr runs fn while capturing os.Stderr; returns the captured text.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, _ := os.Pipe()
	orig := os.Stderr
	os.Stderr = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	<-done
	return buf.String()
}

// writeTriple writes base/ours/theirs files into dir and returns their paths.
func writeTriple(t *testing.T, dir, name string, base, ours, theirs []byte) (string, string, string) {
	t.Helper()
	bp := filepath.Join(dir, "base_"+name)
	op := filepath.Join(dir, "ours_"+name)
	tp := filepath.Join(dir, "theirs_"+name)
	for path, content := range map[string][]byte{bp: base, op: ours, tp: theirs} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return bp, op, tp
}

// ── Notification tests ─────────────────────────────────────────────────────

func TestMergeNotification_AutoMergedSuccess(t *testing.T) {
	dir := t.TempDir()
	base := []byte("package x\n\nfunc A() {}\n")
	ours := []byte("package x\n\nfunc A() {}\nfunc B() {}\n")
	theirs := []byte("package x\n\nfunc A() {}\nfunc C() {}\n")
	bp, op, tp := writeTriple(t, dir, "f.go", base, ours, theirs)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "f.go"})
	})

	if code != 0 {
		t.Errorf("exit code=%d, want 0; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "→ auto-merged") {
		t.Errorf("missing auto-merged notification; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "confidence=") {
		t.Errorf("missing confidence in notification; stderr=%q", stderr)
	}
}

func TestMergeNotification_ConflictReported(t *testing.T) {
	dir := t.TempDir()
	base := []byte("package x\n\nfunc Greet() string { return \"hi\" }\n")
	ours := []byte("package x\n\nfunc Greet() string { return \"OURS\" }\n")
	theirs := []byte("package x\n\nfunc Greet() string { return \"THEIRS\" }\n")
	bp, op, tp := writeTriple(t, dir, "f.go", base, ours, theirs)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "f.go"})
	})

	if code != 1 {
		t.Errorf("exit code=%d, want 1 (conflict); stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "→ conflict") {
		t.Errorf("missing conflict notification; stderr=%q", stderr)
	}

	// Merged file must contain conflict markers, NOT be empty.
	merged, _ := os.ReadFile(op)
	for _, marker := range []string{"<<<<<<< HEAD", "=======", ">>>>>>> theirs"} {
		if !bytes.Contains(merged, []byte(marker)) {
			t.Errorf("merged file missing marker %q; got=%s", marker, merged)
		}
	}
}

// ── True failure tests (exit code 2) ───────────────────────────────────────

func TestMergeFailure_BinaryFileRejected(t *testing.T) {
	dir := t.TempDir()
	// NUL byte in first 8KB → binary.
	bin := []byte{0xFF, 0x00, 0x12, 0x34, 0x56}
	bp, op, tp := writeTriple(t, dir, "x.bin", bin, bin, bin)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "x.bin"})
	})
	if code != 2 {
		t.Errorf("binary file: exit=%d, want 2; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "rejected: binary file") {
		t.Errorf("expected binary rejection; stderr=%q", stderr)
	}
}

func TestMergeFailure_TooLargeFileRejected(t *testing.T) {
	dir := t.TempDir()
	// Build an 11 MB text blob.
	big := bytes.Repeat([]byte("a"), 11*1024*1024)
	bp, op, tp := writeTriple(t, dir, "big.go", big, big, big)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "big.go"})
	})
	if code != 2 {
		t.Errorf("large file: exit=%d, want 2; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "exceeds") {
		t.Errorf("expected size rejection; stderr=%q", stderr)
	}
}

func TestMergeFailure_UnreadableOursFile(t *testing.T) {
	dir := t.TempDir()
	bp := filepath.Join(dir, "base.go")
	_ = os.WriteFile(bp, []byte("package x\n"), 0o644)
	op := filepath.Join(dir, "does-not-exist.go")
	tp := filepath.Join(dir, "theirs.go")
	_ = os.WriteFile(tp, []byte("package x\n"), 0o644)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "x.go"})
	})
	if code != 2 {
		t.Errorf("missing ours: exit=%d, want 2; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "cannot read ours") {
		t.Errorf("expected read error; stderr=%q", stderr)
	}
}

func TestMergeFailure_BothSidesDeletedSymbol(t *testing.T) {
	// Both sides remove the same symbol but add different replacements.
	// Expected: not a true failure (exit 2) — this is just a normal symbol
	// merge where both sides made independent changes. We just assert it
	// completes without crashing and notifies the user.
	dir := t.TempDir()
	base := []byte("package x\n\nfunc Old() {}\n")
	ours := []byte("package x\n\nfunc NewA() {}\n")
	theirs := []byte("package x\n\nfunc NewB() {}\n")
	bp, op, tp := writeTriple(t, dir, "f.go", base, ours, theirs)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "f.go"})
	})
	if code == 2 {
		t.Errorf("unexpected hard failure; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "fuse: f.go") {
		t.Errorf("missing notification; stderr=%q", stderr)
	}
}

// ── Complex conflict scenarios ─────────────────────────────────────────────

func TestComplexMerge_MultiSymbolConflict(t *testing.T) {
	// Three symbols, two conflict, one converges. Expect conflict markers
	// only on the two that disagree; converged stays clean.
	dir := t.TempDir()
	base := []byte(`package x

func A() string { return "a" }
func B() string { return "b" }
func C() string { return "c" }
`)
	ours := []byte(`package x

func A() string { return "OURS_A" }
func B() string { return "shared" }
func C() string { return "OURS_C" }
`)
	theirs := []byte(`package x

func A() string { return "THEIRS_A" }
func B() string { return "shared" }
func C() string { return "THEIRS_C" }
`)
	bp, op, tp := writeTriple(t, dir, "multi.go", base, ours, theirs)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	stderr := captureStderr(t, func() {
		_ = Run([]string{"merge", bp, op, tp, "multi.go"})
	})

	merged, _ := os.ReadFile(op)
	// A and C must have conflict markers; B must not.
	mergedStr := string(merged)
	conflictCount := strings.Count(mergedStr, "<<<<<<< HEAD")
	if conflictCount < 2 {
		t.Errorf("expected ≥2 conflict blocks, got %d; merged=%s", conflictCount, mergedStr)
	}
	if !strings.Contains(mergedStr, "shared") {
		t.Errorf("converged symbol B lost; merged=%s", mergedStr)
	}
	if !strings.Contains(stderr, "→ conflict") {
		t.Errorf("missing conflict notification; stderr=%q", stderr)
	}
}

func TestComplexMerge_AddedOnBothSidesIdentical(t *testing.T) {
	// Both branches added the SAME new function. Should converge cleanly.
	dir := t.TempDir()
	base := []byte("package x\n\nfunc A() {}\n")
	added := []byte("package x\n\nfunc A() {}\n\nfunc NewSymbol() int { return 42 }\n")
	bp, op, tp := writeTriple(t, dir, "f.go", base, added, added)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "f.go"})
	})

	if code != 0 {
		t.Errorf("identical add: exit=%d, want 0; stderr=%s", code, stderr)
	}
	merged, _ := os.ReadFile(op)
	if !bytes.Contains(merged, []byte("NewSymbol")) {
		t.Errorf("converged symbol missing; merged=%s", merged)
	}
	if bytes.Contains(merged, []byte("<<<<<<<")) {
		t.Errorf("converged add must not produce conflict markers; merged=%s", merged)
	}
}

func TestComplexMerge_UnparseableFile(t *testing.T) {
	// Syntax errors in all three sides. Parser will fail; fuse should fall
	// back to line merge rather than crash.
	dir := t.TempDir()
	garbage := []byte("package x\n\n!!! this is not Go code !!!\n")
	bp, op, tp := writeTriple(t, dir, "broken.go", garbage, garbage, garbage)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	var code int
	stderr := captureStderr(t, func() {
		code = Run([]string{"merge", bp, op, tp, "broken.go"})
	})
	if code == 2 {
		// Allowable — unparseable could be a hard failure.
		if !strings.Contains(stderr, "fuse:") {
			t.Errorf("hard failure must notify; stderr=%q", stderr)
		}
	} else {
		// Otherwise must produce SOME output and notify.
		if !strings.Contains(stderr, "fuse: broken.go") {
			t.Errorf("missing notification; stderr=%q", stderr)
		}
	}
}

func TestComplexMerge_UnsupportedLanguageLineFallback(t *testing.T) {
	// .md is unsupported → line merge. Should still notify.
	dir := t.TempDir()
	base := []byte("# Title\n\nBody\n")
	ours := []byte("# Title\n\nBody ours\n")
	theirs := []byte("# Title\n\nBody theirs\n")
	bp, op, tp := writeTriple(t, dir, "README.md", base, ours, theirs)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	stderr := captureStderr(t, func() {
		_ = Run([]string{"merge", bp, op, tp, "README.md"})
	})
	if !strings.Contains(stderr, "[line]") {
		t.Errorf("expected [line] strategy notification; stderr=%q", stderr)
	}
}
