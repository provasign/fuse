package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotFound_NoError(t *testing.T) {
	cfg, err := Load("/nonexistent/fuse.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Error("nil cfg")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("GROVE_URL", "http://envurl:1234")
	t.Setenv("FUSE_GROVE_REQUIRED", "true")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GroveURL != "http://envurl:1234" {
		t.Errorf("got %q", cfg.GroveURL)
	}
	if !cfg.Merge.GroveRequired {
		t.Error("required false")
	}
}

func TestLoad_BadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fuse.yaml")
	_ = os.WriteFile(p, []byte("\tnot: valid: yaml: :"), 0o644)
	if _, err := Load(p); err == nil {
		t.Error("expected yaml error")
	}
}

func TestLocateConfig_Cov(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "fuse.yaml"), []byte(""), 0o644)
	if got := LocateConfig(sub); got == "" {
		t.Error("should find")
	}
	if got := LocateConfig("/no/such"); got != "" {
		t.Errorf("got %q", got)
	}
}
