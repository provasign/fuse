package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectFuseSectionAppends(t *testing.T) {
	out := injectFuseSection("# My project\n\nSome instructions.\n", fuseSteering)
	if !strings.Contains(out, "# My project") {
		t.Fatal("existing content lost")
	}
	if !strings.Contains(out, "## Fuse — semantic merge") {
		t.Fatal("fuse section not appended")
	}
}

func TestInjectFuseSectionReplacesStale(t *testing.T) {
	stale := "# Repo\n\n## Fuse — semantic merge\n\nOLD GUIDANCE\n"
	out := injectFuseSection(stale, fuseSteering)
	if strings.Contains(out, "OLD GUIDANCE") {
		t.Fatal("stale fuse section not replaced")
	}
	if strings.Count(out, "## Fuse — semantic merge") != 1 {
		t.Fatal("expected exactly one fuse section")
	}
}

func TestInjectFuseSectionPreservesFollowingSections(t *testing.T) {
	content := "# Repo\n\n## Fuse — semantic merge\n\nOLD\n\n## Prism — context delivery\n\nkeep me\n"
	out := injectFuseSection(content, fuseSteering)
	if strings.Contains(out, "OLD") {
		t.Fatal("stale section survived")
	}
	if !strings.Contains(out, "## Prism — context delivery") || !strings.Contains(out, "keep me") {
		t.Fatal("following section was destroyed")
	}
}

func TestWriteFuseSteeringIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFuseSteering(dir)
	writeFuseSteering(dir) // second run must not duplicate

	for _, rel := range []string{"CLAUDE.md", ".cursorrules", "AGENTS.md", ".kiro/steering/fuse.md"} {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
		if n := strings.Count(string(raw), "## Fuse — semantic merge"); n != 1 {
			t.Fatalf("%s: want 1 fuse section, got %d", rel, n)
		}
	}
}

func TestInitRegisterFuseMCPCreatesAndMerges(t *testing.T) {
	// Keep ensureClaudeCodeApprovesFuse and user-level writers away from the
	// real home directory.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	dir := t.TempDir()

	// Pre-existing .mcp.json with another server must survive the merge.
	pre := []byte(`{"mcpServers":{"prism":{"command":"/usr/local/bin/prism","args":["mcp"]}}}`)
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), pre, 0o644); err != nil {
		t.Fatal(err)
	}

	initRegisterFuseMCP(dir, "/usr/local/bin/fuse", false)

	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		MCPServers map[string]fuseMCPEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid .mcp.json: %v", err)
	}
	if _, ok := doc.MCPServers["prism"]; !ok {
		t.Fatal("existing prism entry destroyed by merge")
	}
	got, ok := doc.MCPServers["fuse"]
	if !ok {
		t.Fatal("fuse entry not registered")
	}
	if got.Command != "/usr/local/bin/fuse" || len(got.Args) != 2 || got.Args[0] != "mcp" || got.Args[1] != dir {
		t.Fatalf("unexpected fuse entry: %+v", got)
	}

	// Cursor and VS Code project configs are created from scratch.
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "mcp.json")); err != nil {
		t.Fatal("cursor config not written")
	}
	vsraw, err := os.ReadFile(filepath.Join(dir, ".vscode", "mcp.json"))
	if err != nil {
		t.Fatal("vscode config not written")
	}
	if !strings.Contains(string(vsraw), `"type": "stdio"`) {
		t.Fatal("vscode config missing stdio type")
	}
}

func TestFuseMCPEntryPresentSkipsRewrite(t *testing.T) {
	dir := t.TempDir()
	entry := fuseMCPEntry{Command: "/usr/local/bin/fuse", Args: []string{"mcp", dir}}
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, buildFuseMCPConfig("fuse", entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fuseMCPEntryPresent(path, "fuse", entry) {
		t.Fatal("identical entry not detected")
	}
	other := fuseMCPEntry{Command: "/other/fuse", Args: entry.Args}
	if fuseMCPEntryPresent(path, "fuse", other) {
		t.Fatal("different command should not match")
	}
}

func TestInstallMergeDriver(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := installMergeDriver(dir); err != nil {
		t.Fatalf("installMergeDriver: %v", err)
	}

	out, err := exec.Command("git", "-C", dir, "config", "merge.fuse.driver").Output()
	if err != nil {
		t.Fatalf("merge.fuse.driver not set: %v", err)
	}
	if want := "fuse merge %O %A %B %P"; strings.TrimSpace(string(out)) != want {
		t.Fatalf("driver = %q, want %q", strings.TrimSpace(string(out)), want)
	}

	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if err != nil {
		t.Fatal(".gitattributes not written")
	}
	if !strings.Contains(string(attrs), "*.go merge=fuse") {
		t.Fatal(".gitattributes missing go rule")
	}

	// Idempotent: second run must not duplicate attribute lines.
	if err := installMergeDriver(dir); err != nil {
		t.Fatalf("second installMergeDriver: %v", err)
	}
	attrs2, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if strings.Count(string(attrs2), "*.go merge=fuse") != 1 {
		t.Fatal("duplicate .gitattributes lines after re-run")
	}
}

func TestInstallMergeDriverNotARepo(t *testing.T) {
	if err := installMergeDriver(t.TempDir()); err == nil {
		t.Fatal("expected error outside a git repository")
	}
}

func TestMergeOrCreateJSONOverwritesInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"mcpServers":{"fuse":{"command":"fuse","args":["mcp"]}}}`)
	out := mergeOrCreateJSON(path, content)
	if string(out) != string(content) {
		t.Fatal("invalid JSON should be overwritten verbatim")
	}
}

func TestStripTOMLNamedTable(t *testing.T) {
	lines := []string{
		"[other]",
		`key = "v"`,
		"[mcp_servers.fuse]",
		`command = "old"`,
		`args = ["mcp"]`,
		"[mcp_servers.prism]",
		`command = "prism"`,
	}
	out := stripTOMLNamedTable(lines, "mcp_servers", "fuse")
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, `command = "old"`) {
		t.Fatal("fuse table not stripped")
	}
	if !strings.Contains(joined, "[mcp_servers.prism]") || !strings.Contains(joined, "[other]") {
		t.Fatal("unrelated tables damaged")
	}
}
