package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAndLoadNoFile(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GroveURL != "embedded://grove" {
		t.Errorf("unexpected GroveURL %q (embedded mode has no daemon URL)", cfg.GroveURL)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("unexpected port %d", cfg.Server.Port)
	}
	if cfg.Merge.HandoffThreshold == 0 {
		t.Error("expected non-zero handoff threshold")
	}
}

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fuse.yaml")
	body := `version: 1
grove_url: "http://example:1"
merge:
  handoff_threshold: 0.5
  grove_required: false
server:
  port: 1234
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GroveURL != "http://example:1" {
		t.Errorf("got %q", cfg.GroveURL)
	}
	if cfg.Merge.HandoffThreshold != 0.5 {
		t.Errorf("got %v", cfg.Merge.HandoffThreshold)
	}
	if cfg.Server.Port != 1234 {
		t.Errorf("got %d", cfg.Server.Port)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("GROVE_URL", "http://envset:9")
	cfg, _ := Load("")
	if cfg.GroveURL != "http://envset:9" {
		t.Errorf("env override failed: %q", cfg.GroveURL)
	}
}

func TestLocateConfig(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "fuse.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LocateConfig(sub)
	if got != cfgPath {
		t.Errorf("got %q want %q", got, cfgPath)
	}
}
