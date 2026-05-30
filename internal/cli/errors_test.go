package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fakeGroveBroken returns 500 to /deps and /impact (health OK).
func fakeGroveBroken(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/deps", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) })
	mux.HandleFunc("/impact", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestCmdCheckImpactDeps_GroveError(t *testing.T) {
	dir := gitInit(t)
	sub := filepath.Join(dir, "work")
	_ = os.MkdirAll(sub, 0o755)
	resolved, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatal(err)
	}
	url := fakeGroveBroken(t)
	writeFuseConfig(t, resolved, url)
	t.Setenv("GROVE_URL", url)
	withDir(t, resolved)

	for _, cmd := range []string{"check", "impact", "deps"} {
		if code := Run([]string{cmd, "x.go"}); code != 2 {
			t.Errorf("%s expected 2 on grove error, got %d", cmd, code)
		}
	}
}

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
