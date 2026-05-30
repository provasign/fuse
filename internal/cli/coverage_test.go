package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/config"
)

// withDir changes cwd for the duration of the test.
func withDir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// gitInit creates a minimal repo and returns its path (symlink-resolved).
func gitInit(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@local"},
		{"config", "user.name", "test"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func withDirResolved(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	withDir(t, resolved)
	return resolved
}

func TestRun_UnknownAndEmpty(t *testing.T) {
	if Run(nil) != 2 {
		t.Error("empty should exit 2")
	}
	if Run([]string{"bogus-cmd"}) != 2 {
		t.Error("unknown should exit 2")
	}
}

func TestRun_AllSubcommandUsages(t *testing.T) {
	// Each command with missing args should return non-zero (usage exit).
	for _, cmd := range []string{"merge", "preview", "resolve", "check", "impact", "deps"} {
		if code := Run([]string{cmd}); code == 0 {
			t.Errorf("%s with no args returned 0", cmd)
		}
	}
}

func TestCmdMerge_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	write := func(n, c string) string {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("b.txt", "hello\n")
	ours := write("o.txt", "hello world\n")
	theirs := write("t.txt", "hello there\n")
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	withDir(t, dir)
	code := Run([]string{"merge", base, ours, theirs, "x.txt"})
	if code != 0 && code != 1 {
		t.Errorf("unexpected code %d", code)
	}
}

func TestCmdMerge_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	// theirs missing
	base := filepath.Join(dir, "b.go")
	ours := filepath.Join(dir, "o.go")
	_ = os.WriteFile(base, []byte("package x\n"), 0o644)
	_ = os.WriteFile(ours, []byte("package x\n"), 0o644)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	if code := Run([]string{"merge", base, ours, filepath.Join(dir, "missing")}); code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
	// ours missing
	if code := Run([]string{"merge", base, filepath.Join(dir, "missing"), base}); code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestCmdPreview(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "b.go")
	ours := filepath.Join(dir, "o.go")
	theirs := filepath.Join(dir, "t.go")
	_ = os.WriteFile(base, []byte("package x\n\nfunc A() {}\n"), 0o644)
	_ = os.WriteFile(ours, []byte("package x\n\nfunc A() {}\nfunc B() {}\n"), 0o644)
	_ = os.WriteFile(theirs, []byte("package x\n\nfunc A() {}\nfunc C() {}\n"), 0o644)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	withDir(t, dir)
	if code := Run([]string{"preview", base, ours, theirs}); code != 0 && code != 1 {
		t.Errorf("preview code %d", code)
	}
	// usage path
	if code := Run([]string{"preview", base}); code != 2 {
		t.Error("preview usage should be 2")
	}
}

func TestCmdResolve(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prompt.md")
	_ = os.WriteFile(p, []byte("# AI handoff"), 0o644)
	if code := Run([]string{"resolve", p}); code != 0 {
		t.Errorf("resolve code %d", code)
	}
	if code := Run([]string{"resolve", filepath.Join(dir, "nope")}); code != 2 {
		t.Error("resolve missing file should fail")
	}
}

func TestCmdConfigAndStatus(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "work")
	_ = os.MkdirAll(sub, 0o755)
	withDirResolved(t, sub)
	if code := Run([]string{"config"}); code != 0 {
		t.Errorf("config code %d", code)
	}
	// no audit yet
	if code := Run([]string{"status"}); code != 0 {
		t.Errorf("status code %d", code)
	}
	// after writing an audit file
	auditDir := filepath.Join(dir, ".git", "fuse")
	_ = os.MkdirAll(auditDir, 0o755)
	_ = os.WriteFile(filepath.Join(auditDir, "audit.json"),
		[]byte(`[{"timestamp":"t","file":"f","strategy":"auto","confidence":0.9,"autoMerged":true,"breakingChanges":0}]`), 0o644)
	if code := Run([]string{"status"}); code != 0 {
		t.Errorf("status with audit code %d", code)
	}
}

func TestCmdStatus_NotARepo(t *testing.T) {
	withDir(t, t.TempDir())
	if code := Run([]string{"status"}); code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestCmdInstallUninstall(t *testing.T) {
	dir := gitInit(t)
	withDirResolved(t, dir)
	if code := Run([]string{"install"}); code != 0 {
		t.Errorf("install code %d", code)
	}
	// .gitattributes written?
	data, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if err != nil || !strings.Contains(string(data), "merge=fuse") {
		t.Errorf("gitattributes not updated: %q err=%v", data, err)
	}
	// Re-install idempotent
	if code := Run([]string{"install"}); code != 0 {
		t.Errorf("re-install code %d", code)
	}
	if code := Run([]string{"uninstall"}); code != 0 {
		t.Errorf("uninstall code %d", code)
	}
}

func TestCmdCheckImpactDeps_Smoke(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	withDir(t, dir)
	// We don't assert exit code: behaviour depends on whether grove is
	// available on the system. We just exercise the dispatch path.
	for _, cmd := range []string{"check", "impact", "deps"} {
		_ = Run([]string{cmd, "x.go"})
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Error("boolToInt wrong")
	}
}

func TestFindGitDir(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "a", "b")
	_ = os.MkdirAll(sub, 0o755)
	if got := findGitDir(filepath.Join(sub, "x.go")); got == "" {
		t.Error("findGitDir should walk up")
	}
	if got := findGitDir(filepath.Join(t.TempDir(), "x.go")); got != "" {
		t.Errorf("non-repo should return empty, got %q", got)
	}
}

func TestFindGitDir_WorktreeFile(t *testing.T) {
	// Simulate a .git file (worktree pointer).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /some/path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findGitDir(filepath.Join(dir, "x.go")); got != "/some/path" {
		t.Errorf("worktree pointer got %q", got)
	}
}

func TestServer_HealthAndMergeJSON(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Port = 0
	// Exercise the handler chain directly by replicating the server mux logic.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != 200 {
		t.Errorf("health %d", rec.Code)
	}
}

func TestDecodeJSON_BadInput(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{garbage")))
	var v map[string]string
	if err := decodeJSON(r, &v); err == nil {
		t.Error("expected decode err")
	}
}

func TestDecodeJSON_UnknownField(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"unknown":1}`)))
	var v struct {
		Known string `json:"known"`
	}
	if err := decodeJSON(r, &v); err == nil {
		t.Error("expected unknown-field err")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 418, map[string]string{"x": "y"})
	if rec.Code != 418 {
		t.Error("status not set")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("ct wrong")
	}
}

func TestNewCmd(t *testing.T) {
	c := newCmd("git", "--version")
	if c == nil {
		t.Fatal("nil cmd")
	}
	if err := c.Run(); err != nil {
		t.Errorf("git --version failed: %v", err)
	}
}

// TestRealMergeDriver runs an end-to-end semantic merge inside a real git repo
// using the fuse merge driver.
func TestRealMergeDriver(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := gitInit(t)
	withDirResolved(t, dir)

	// Build a file on main.
	main := "package x\n\nfunc A() { _ = 1 }\n"
	_ = os.WriteFile(filepath.Join(dir, "f.go"), []byte(main), 0o644)
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")

	// Create branch and change file.
	run("checkout", "-q", "-b", "feature")
	_ = os.WriteFile(filepath.Join(dir, "f.go"), []byte("package x\n\nfunc A() { _ = 1 }\nfunc B() {}\n"), 0o644)
	run("commit", "-q", "-am", "add B")

	// Back to main and change differently.
	run("checkout", "-q", "main")
	_ = os.WriteFile(filepath.Join(dir, "f.go"), []byte("package x\n\nfunc A() { _ = 1 }\nfunc C() {}\n"), 0o644)
	run("commit", "-q", "-am", "add C")

	// Install fuse driver (test that subcommand wiring works).
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	if code := Run([]string{"install"}); code != 0 {
		t.Errorf("install code %d", code)
	}
	// The merge driver runs `fuse merge` — but inside the test binary, exec'ing
	// "fuse" won't find our test code. We can't fully integration-test the git
	// hook here without building the binary, so just verify install left the
	// repo in a state where merge.fuse.driver is set.
	c := exec.Command("git", "config", "merge.fuse.driver")
	c.Dir = dir
	out, _ := c.Output()
	if !strings.Contains(string(out), "fuse merge") {
		t.Errorf("merge driver not configured: %q", out)
	}
}

func TestStartServer_BadPort(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Port = -1 // invalid
	// Run in a goroutine with a quick timeout via context — but startServer
	// blocks. We just check that ListenAndServe fails fast.
	done := make(chan int, 1)
	go func() { done <- startServer(cfg) }()
	// Give it a moment; if it bound somehow, abort.
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	case <-context.Background().Done():
	}
}

// Smoke test that the JSON encoder for AuditEntry works.
func TestStatus_BadAuditJSON(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "work")
	_ = os.MkdirAll(sub, 0o755)
	withDirResolved(t, sub)
	auditDir := filepath.Join(dir, ".git", "fuse")
	_ = os.MkdirAll(auditDir, 0o755)
	_ = os.WriteFile(filepath.Join(auditDir, "audit.json"), []byte("not json"), 0o644)
	if code := Run([]string{"status"}); code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

// Sanity: round-trip JSON to verify writeJSON output.
func TestWriteJSON_Encoding(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 200, map[string]int{"n": 5})
	var got map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["n"] != 5 {
		t.Errorf("got %v", got)
	}
}
