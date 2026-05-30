package handoff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tabladrum/grove-suite/fuse/internal/core"
)

func TestFenceLangAllBranches(t *testing.T) {
	cases := []struct {
		l    core.LanguageKey
		want string
	}{
		{core.LangGo, "go"},
		{core.LangPython, "python"},
		{core.LangTypeScript, "typescript"},
		{core.LangTSX, "typescript"},
		{core.LangJavaScript, "javascript"},
		{core.LangJava, "java"},
		{core.LangRust, "rust"},
		{core.LangJSON, "json"},
		{core.LangYAML, "yaml"},
		{core.LangTOML, "toml"},
		{core.LangUnknown, ""},
	}
	for _, c := range cases {
		if got := fenceLang(c.l); got != c.want {
			t.Errorf("%s got %q want %q", c.l, got, c.want)
		}
	}
}

func TestCodeFence(t *testing.T) {
	if codeFence(core.LangGo) != "```" {
		t.Error("fence")
	}
}

func TestAppendAudit_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	if err := AppendAudit(dir, core.AuditEntry{File: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "audit.json")); err != nil {
		t.Error(err)
	}
	// Append again
	if err := AppendAudit(dir, core.AuditEntry{File: "y"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewGenerator_BadDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(dir, []byte("x"), 0o644)
	if _, err := NewGenerator(dir); err == nil {
		t.Error("expected mkdir error")
	}
}
