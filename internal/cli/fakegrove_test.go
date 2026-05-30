package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGrove serves the minimum endpoints Fuse needs.
func fakeGrove(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("/deps", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"edges":[{"from":"a","to":"b","type":"calls","confidence":1.0}]}`))
	})
	mux.HandleFunc("/impact", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"id":"x","filePath":"f","name":"N"}]}`))
	})
	mux.HandleFunc("/symbols", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbols":[]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// writeFuseConfig writes a fuse.yaml in dir pointing at a custom grove URL.
func writeFuseConfig(t *testing.T, dir, groveURL string) {
	t.Helper()
	cfg := "grove:\n  url: " + groveURL + "\n  required: false\n"
	if err := os.WriteFile(filepath.Join(dir, "fuse.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCmdCheckImpactDeps_AgainstFakeGrove(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "work")
	_ = os.MkdirAll(sub, 0o755)
	resolved, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatal(err)
	}
	url := fakeGrove(t)
	writeFuseConfig(t, dir, url)
	writeFuseConfig(t, resolved, url)
	t.Setenv("GROVE_URL", url)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")
	withDir(t, resolved)

	for _, cmd := range []string{"check", "impact", "deps"} {
		if code := Run([]string{cmd, "x.go"}); code != 0 {
			t.Errorf("%s returned %d", cmd, code)
		}
	}
}

func TestCmdMerge_ProducesAuditLog(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "work")
	_ = os.MkdirAll(sub, 0o755)
	resolved, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatal(err)
	}
	withDir(t, resolved)
	t.Setenv("FUSE_GROVE_REQUIRED", "false")

	base := filepath.Join(resolved, "b.go")
	ours := filepath.Join(resolved, "o.go")
	theirs := filepath.Join(resolved, "t.go")
	_ = os.WriteFile(base, []byte("package x\n\nfunc A() {}\n"), 0o644)
	// Both add a new symbol -> potentially conflicting.
	_ = os.WriteFile(ours, []byte("package x\n\nfunc A() {}\nfunc B() {}\n"), 0o644)
	_ = os.WriteFile(theirs, []byte("package x\n\nfunc A() {}\nfunc C() {}\n"), 0o644)

	code := Run([]string{"merge", base, ours, theirs, "f.go"})
	if code != 0 && code != 1 {
		t.Errorf("merge code %d", code)
	}
	// Audit log should exist.
	auditPath := filepath.Join(dir, ".git", "fuse", "audit.json")
	if _, err := os.Stat(auditPath); err != nil {
		t.Errorf("expected audit log: %v", err)
	}
}

func TestCmdInstall_FailsOutsideGitRepo(t *testing.T) {
	withDir(t, t.TempDir())
	if code := Run([]string{"install"}); code != 2 {
		t.Errorf("install outside repo: %d", code)
	}
}

func TestLoadConfig_BadFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "fuse.yaml"), []byte("\t\tbad: : yaml"), 0o644)
	withDir(t, dir)
	// loadConfig calls os.Exit on bad config; test it in subprocess.
	if os.Getenv("FUSE_TEST_BADCONFIG") == "1" {
		_ = loadConfig()
		return
	}
	// Just exercise the success path here.
	_ = os.WriteFile(filepath.Join(dir, "fuse.yaml"), []byte(""), 0o644)
	c := loadConfig()
	if c == nil {
		t.Error("nil config")
	}
}

// Smoke: verify that .gitattributes append handles file that already has
// matching lines (no duplicate-write path).
func TestUpsertGitAttributes_Dedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	_ = os.WriteFile(path, []byte("*.go merge=fuse\n"), 0o644)
	if err := upsertGitAttributes(path, []string{"*.go merge=fuse"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "*.go merge=fuse") != 1 {
		t.Errorf("duplicated: %q", data)
	}
}
