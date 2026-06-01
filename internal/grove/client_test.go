package grove

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHealth_RequiresRoot(t *testing.T) {
	err := New("").Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "WithTokenFromDir") {
		t.Fatalf("expected missing-root error, got %v", err)
	}
}

func TestEnsureRunning_RequiresRoot(t *testing.T) {
	err := EnsureRunning(context.Background(), "", "", "", 0)
	if err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("expected root-required error, got %v", err)
	}
}

func TestClient_IndexSymbolsDepsImpact(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, dir, "main.go", "package main\n\nfunc Main() {}\n")
	_ = writeFile(t, dir, "use.go", "package main\n\nfunc Use() { Main() }\n")

	c := New("").WithTokenFromDir(dir)
	ctx := context.Background()
	if err := c.Index(ctx, dir); err != nil {
		t.Fatalf("index: %v", err)
	}

	syms, err := c.Symbols(ctx, "Main", 10)
	if err != nil {
		t.Fatalf("symbols: %v", err)
	}
	if len(syms) == 0 {
		t.Fatalf("expected symbols for Main")
	}

	edges, err := c.Deps(ctx, syms[0].FilePath)
	if err != nil {
		t.Fatalf("deps: %v", err)
	}
	_ = edges

	impact, err := c.Impact(ctx, "Main", 3)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	_ = impact

	c.Close()
}

func TestClient_OptionalDefaultsAndClose(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, dir, "main.go", "package main\n\nfunc Main() {}\n")

	c := New("").WithTokenFromDir(dir)
	ctx := context.Background()
	if err := c.Index(ctx, ""); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Exercise default-parameter branches.
	if _, err := c.Symbols(ctx, "Main", 0); err != nil {
		t.Fatalf("symbols default limit: %v", err)
	}
	if _, err := c.Impact(ctx, "Main", 0); err != nil {
		t.Fatalf("impact default depth: %v", err)
	}

	// Close should be idempotent and safe on an already-closed client.
	c.Close()
	c.Close()
}

func TestClient_Index_EmptyDirUsesRoot(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, dir, "a.go", "package main\n\nfunc A() {}\n")

	c := New("").WithTokenFromDir(dir)
	defer c.Close()
	if err := c.Index(context.Background(), ""); err != nil {
		t.Fatalf("index with empty dir: %v", err)
	}
}

func TestWithTokenFromDir_PreservesInputOnAbsError(t *testing.T) {
	c := New("").WithTokenFromDir("./")
	if c.root == "" {
		t.Fatal("expected root to be set")
	}
}

func TestEnsureRunning_Success(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, dir, "main.go", "package main\n\nfunc Main() {}\n")
	if err := EnsureRunning(context.Background(), "", "", dir, 0); err != nil {
		t.Fatalf("ensure running: %v", err)
	}
}
