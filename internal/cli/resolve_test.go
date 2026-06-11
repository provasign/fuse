package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeResolveFixture(t *testing.T) (promptPath, targetPath string) {
	t.Helper()
	dir := t.TempDir()
	targetPath = filepath.Join(dir, "x.go")
	if err := os.WriteFile(targetPath, []byte("package x\n<<<<<<< HEAD\nfunc A() {}\n=======\nfunc B() {}\n>>>>>>> theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompt := "# Fuse: Unresolvable Merge Conflict\n\n## Summary\n- **File:** " + targetPath + "\n- **Language:** go\n"
	promptPath = filepath.Join(dir, "conflict-abc.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		t.Fatal(err)
	}
	return promptPath, targetPath
}

func TestResolveAgentApplyWritesValidatedFile(t *testing.T) {
	promptPath, targetPath := writeResolveFixture(t)
	// Fake agent: emits a valid resolution wrapped in fences (which must be
	// stripped) regardless of the prompt on stdin.
	agent := `cat > /dev/null; printf '` + "```" + `go\npackage x\n\nfunc A() {}\n\nfunc B() {}\n` + "```" + `\n'`
	if code := Run([]string{"resolve", promptPath, "--agent", agent, "--apply"}); code != 0 {
		t.Fatalf("resolve exited %d", code)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "<<<<<<<") {
		t.Errorf("conflict markers survived: %s", got)
	}
	if !strings.Contains(string(got), "func A() {}") || !strings.Contains(string(got), "func B() {}") {
		t.Errorf("unexpected resolution: %s", got)
	}
}

func TestResolveAgentRejectsInvalidOutput(t *testing.T) {
	promptPath, targetPath := writeResolveFixture(t)
	before, _ := os.ReadFile(targetPath)

	// Agent returns garbage that is not valid Go.
	if code := Run([]string{"resolve", promptPath, "--agent", "cat > /dev/null; echo 'not valid go {{{'", "--apply"}); code == 0 {
		t.Fatal("expected nonzero exit for invalid agent output")
	}
	after, _ := os.ReadFile(targetPath)
	if string(before) != string(after) {
		t.Error("target file must not be modified when validation fails")
	}
}

func TestResolveAgentRejectsRemainingMarkers(t *testing.T) {
	promptPath, _ := writeResolveFixture(t)
	agent := `cat > /dev/null; printf 'package x\n<<<<<<< HEAD\nfunc A() {}\n>>>>>>> theirs\n'`
	if code := Run([]string{"resolve", promptPath, "--agent", agent}); code == 0 {
		t.Fatal("expected nonzero exit when markers remain")
	}
}

func TestResolveWithoutAgentPrintsPrompt(t *testing.T) {
	promptPath, _ := writeResolveFixture(t)
	if code := Run([]string{"resolve", promptPath}); code != 0 {
		t.Fatalf("resolve exited %d", code)
	}
}

func TestPromptTargetFile(t *testing.T) {
	if got := promptTargetFile([]byte("x\n- **File:** a/b.go\n")); got != "a/b.go" {
		t.Errorf("got %q", got)
	}
	if got := promptTargetFile([]byte("no file line")); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestStripCodeFences(t *testing.T) {
	in := "```go\npackage x\n```"
	if got := string(stripCodeFences([]byte(in))); got != "package x\n" {
		t.Errorf("got %q", got)
	}
	plain := "package x\n"
	if got := string(stripCodeFences([]byte(plain))); got != plain {
		t.Errorf("got %q", got)
	}
}
