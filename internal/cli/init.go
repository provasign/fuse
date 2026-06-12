package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdInit is the one-command customer setup: register the git merge driver,
// write steering instructions for every file-based agent format, and register
// the fuse MCP server with every detected AI coding tool.
//
//	fuse init [dir] [--global] [--skip-driver]
//
// --global writes MCP config to user-level files (~/.claude.json, ~/.cursor/...)
// instead of the project directory. --skip-driver leaves git config and
// .gitattributes untouched (e.g. when only agent integration is wanted).
func cmdInit(args []string) int {
	global := false
	skipDriver := false
	var rest []string
	for _, a := range args {
		switch a {
		case "--global":
			global = true
		case "--skip-driver":
			skipDriver = true
		default:
			rest = append(rest, a)
		}
	}
	dir := "."
	if len(rest) > 0 {
		dir = rest[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fuse init:", err)
		return 2
	}

	// 1. Merge driver: plain `git merge` / `git rebase` goes through fuse.
	if !skipDriver {
		if err := installMergeDriver(abs); err != nil {
			fmt.Fprintln(os.Stderr, "warning: merge driver not registered:", err)
		} else {
			fmt.Println("registered fuse as git merge driver")
		}
	}

	// 2. Steering instructions so agents use the fuse_* tools correctly.
	writeFuseSteering(abs)

	// 3. MCP registration with every detected AI coding tool.
	fuseBin := detectFuseSelfPath()
	registered := initRegisterFuseMCP(abs, fuseBin, global)
	if len(registered) == 0 {
		fmt.Println("tip: add fuse to your AI tool's MCP config (see README)")
	}
	return 0
}

// installMergeDriver registers fuse in the repo's git config and ensures
// .gitattributes routes supported file types through it.
func installMergeDriver(dir string) error {
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("not a git repository: %s", dir)
	}
	if err := runGit("-C", dir, "config", "merge.fuse.name", "Fuse semantic merge driver"); err != nil {
		return err
	}
	if err := runGit("-C", dir, "config", "merge.fuse.driver", "fuse merge %O %A %B %P"); err != nil {
		return err
	}
	return upsertGitAttributes(filepath.Join(top, ".gitattributes"), mergeDriverAttributes)
}

// mergeDriverAttributes lists the file types fuse handles semantically.
// Shared by `fuse install` and `fuse init`.
var mergeDriverAttributes = []string{
	"*.go merge=fuse",
	"*.ts merge=fuse",
	"*.tsx merge=fuse",
	"*.js merge=fuse",
	"*.jsx merge=fuse",
	"*.mjs merge=fuse",
	"*.cjs merge=fuse",
	"*.py merge=fuse",
	"*.java merge=fuse",
	"*.rs merge=fuse",
	"*.json merge=fuse",
	"*.yaml merge=fuse",
	"*.yml merge=fuse",
	"*.toml merge=fuse",
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// fuseSteering is injected into every agent instruction file. Terse on
// purpose: it rides in the agent's context window for the whole session.
const fuseSteering = `
## Fuse — semantic merge

Fuse is this repo's git merge driver: git merge/rebase already resolves
structural conflicts symbol-by-symbol and records drift evidence in
.git/fuse/drift.json. Use the fuse_* MCP tools when reasoning about merges:

| Situation | Tool |
|---|---|
| Will my committed work merge cleanly against a ref? | fuse_merge_check(ref) — verdict + per-file conflicts, ~40 tokens |
| Merge two versions of content in memory | fuse_preview(base, ours, theirs, path) — nothing written |
| Work a conflict handoff from .git/fuse/ | fuse_resolve(prompt[, resolution, apply]) |
| Blast radius of a change before finishing | fuse_impact(query) — capped list + true count |

### Do NOT
- Do NOT hand-edit conflict markers before trying fuse_resolve — it validates and applies a complete resolution
- Do NOT warn the user about merge conflicts without running fuse_merge_check first
- Do NOT re-derive what changed after a merge lands — drift evidence is already in .git/fuse/drift.json (surfaced as prism_drift when Prism is installed)
`

// writeFuseSteering writes/refreshes the Fuse section in every file-based
// agent instruction format. Re-running replaces a stale section in place.
func writeFuseSteering(projectDir string) {
	targets := []struct {
		name    string
		relPath string
	}{
		{name: "Claude Code", relPath: "CLAUDE.md"},
		{name: "Cursor", relPath: ".cursorrules"},
		{name: "Windsurf", relPath: ".windsurfrules"},
		{name: "GitHub Copilot", relPath: ".github/copilot-instructions.md"},
		{name: "AGENTS.md", relPath: "AGENTS.md"},
		{name: "Gemini CLI", relPath: "GEMINI.md"},
		{name: "Cline", relPath: ".clinerules"},
		{name: "Devin", relPath: ".devin/instructions.md"},
		{name: "Kiro", relPath: ".kiro/steering/fuse.md"},
	}
	for _, t := range targets {
		path := filepath.Join(projectDir, t.relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create directory for %s instructions: %v\n", t.name, err)
			continue
		}
		var content string
		if existing, err := os.ReadFile(path); err == nil {
			content = injectFuseSection(string(existing), fuseSteering)
		} else {
			content = fuseSteering
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write %s instructions: %v\n", t.name, err)
			continue
		}
		fmt.Printf("wrote steering instructions: %s\n", path)
	}
}

// injectFuseSection replaces an existing Fuse steering section with block, or
// appends block if no section is present, so re-init upgrades stale guidance.
// Other tools' sections (e.g. Prism's) are preserved.
func injectFuseSection(content, block string) string {
	const marker = "## Fuse — semantic merge"
	if idx := strings.Index(content, "\n"+marker); idx >= 0 {
		head := content[:idx]
		tail := content[idx+1:]
		// Skip to the end of the old Fuse section: the next "\n## " heading.
		if next := strings.Index(tail[len(marker):], "\n## "); next >= 0 {
			rest := tail[len(marker)+next:]
			return head + block + rest
		}
		return head + block
	}
	if strings.HasPrefix(content, marker) {
		if next := strings.Index(content[len(marker):], "\n## "); next >= 0 {
			return strings.TrimPrefix(block, "\n") + content[len(marker)+next:]
		}
		return block
	}
	return strings.TrimRight(content, "\n") + block
}

// detectFuseSelfPath returns the absolute path of the running binary, falling
// back to "fuse" on PATH.
func detectFuseSelfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "fuse"
	}
	return exe
}

// fuseMCPEntry is the JSON shape every MCP-compatible tool config expects.
type fuseMCPEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// initRegisterFuseMCP writes MCP server config for every detected AI tool and
// returns the list of files written.
func initRegisterFuseMCP(projectDir, fuseBin string, global bool) []string {
	var written []string
	entry := fuseMCPEntry{Command: fuseBin, Args: []string{"mcp", projectDir}}
	home, _ := os.UserHomeDir()

	type writer struct {
		name  string
		path  string
		build func() []byte
	}
	writers := []writer{
		{
			// Claude Code: .mcp.json at repo root (project) or ~/.claude.json (global).
			name: "Claude Code",
			path: func() string {
				if global {
					return filepath.Join(home, ".claude.json")
				}
				return filepath.Join(projectDir, ".mcp.json")
			}(),
			build: func() []byte { return buildFuseMCPConfig("fuse", entry) },
		},
		{
			name: "Cursor",
			path: func() string {
				if global {
					return filepath.Join(home, ".cursor", "mcp.json")
				}
				return filepath.Join(projectDir, ".cursor", "mcp.json")
			}(),
			build: func() []byte { return buildFuseMCPConfig("fuse", entry) },
		},
		{
			name: "Windsurf",
			path: func() string {
				if global {
					return filepath.Join(home, ".windsurf", "mcp.json")
				}
				return filepath.Join(projectDir, ".windsurf", "mcp.json")
			}(),
			build: func() []byte { return buildFuseMCPConfig("fuse", entry) },
		},
		{
			// Zed: user-level settings only; patch "context_servers".
			name: "Zed",
			path: filepath.Join(home, ".config", "zed", "settings.json"),
			build: func() []byte {
				type zedSettings struct {
					ContextServers map[string]fuseMCPEntry `json:"context_servers"`
				}
				b, _ := json.MarshalIndent(zedSettings{ContextServers: map[string]fuseMCPEntry{"fuse": entry}}, "", "  ")
				return b
			},
		},
		{
			// VS Code native MCP host: {"servers":{"fuse":{"type":"stdio",...}}}.
			name: "VS Code",
			path: filepath.Join(projectDir, ".vscode", "mcp.json"),
			build: func() []byte {
				type vscodeServer struct {
					Type    string   `json:"type"`
					Command string   `json:"command"`
					Args    []string `json:"args"`
				}
				type vscodeMCP struct {
					Servers map[string]vscodeServer `json:"servers"`
				}
				b, _ := json.MarshalIndent(vscodeMCP{Servers: map[string]vscodeServer{
					"fuse": {Type: "stdio", Command: fuseBin, Args: entry.Args},
				}}, "", "  ")
				return b
			},
		},
	}

	for _, w := range writers {
		parent := filepath.Dir(w.path)
		isGlobalUserDir := strings.HasPrefix(parent, home)
		if _, err := os.Stat(parent); err != nil {
			if !global && !isGlobalUserDir {
				if mkErr := os.MkdirAll(parent, 0o755); mkErr != nil {
					fmt.Fprintf(os.Stderr, "warning: could not create %s config dir: %v\n", w.name, mkErr)
					continue
				}
			} else {
				continue // user-level tool not installed — skip
			}
		}
		// Rewriting .mcp.json resets Claude Code's approval state; skip when
		// the fuse entry is already exact.
		if filepath.Base(w.path) == ".mcp.json" && fuseMCPEntryPresent(w.path, "fuse", entry) {
			fmt.Printf("already registered with %s: %s\n", w.name, w.path)
			written = append(written, w.path)
			ensureClaudeCodeApprovesFuse("fuse")
			continue
		}
		merged := mergeOrCreateJSON(w.path, w.build())
		if err := os.WriteFile(w.path, merged, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write %s config (%s): %v\n", w.name, w.path, err)
			continue
		}
		fmt.Printf("registered with %s: %s\n", w.name, w.path)
		written = append(written, w.path)
		if filepath.Base(w.path) == ".mcp.json" {
			ensureClaudeCodeApprovesFuse("fuse")
		}
	}

	// Codex CLI uses TOML; only write when ~/.codex exists.
	codexPath := filepath.Join(home, ".codex", "config.toml")
	if _, err := os.Stat(filepath.Dir(codexPath)); err == nil {
		if err := writeFuseCodexConfig(codexPath, fuseBin, entry.Args); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write Codex CLI config: %v\n", err)
		} else {
			fmt.Printf("registered with Codex CLI: %s\n", codexPath)
			written = append(written, codexPath)
		}
	}
	return written
}

// buildFuseMCPConfig returns {"mcpServers":{"<name>": entry}} JSON.
func buildFuseMCPConfig(name string, e fuseMCPEntry) []byte {
	type envelope struct {
		MCPServers map[string]fuseMCPEntry `json:"mcpServers"`
	}
	b, _ := json.MarshalIndent(envelope{MCPServers: map[string]fuseMCPEntry{name: e}}, "", "  ")
	return b
}

// fuseMCPEntryPresent reports whether path already has an identical
// mcpServers entry for name.
func fuseMCPEntryPresent(path, name string, want fuseMCPEntry) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		MCPServers map[string]fuseMCPEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	got, ok := doc.MCPServers[name]
	if !ok || got.Command != want.Command || len(got.Args) != len(want.Args) {
		return false
	}
	for i, a := range want.Args {
		if got.Args[i] != a {
			return false
		}
	}
	return true
}

// mergeOrCreateJSON deep-merges content into the JSON file at path. Nested
// maps under shared keys ("mcpServers", "context_servers", "servers") are
// merged rather than replaced, so other tools' entries survive.
func mergeOrCreateJSON(path string, content []byte) []byte {
	existing, err := os.ReadFile(path)
	if err != nil {
		return content
	}
	var base, overlay map[string]json.RawMessage
	if err := json.Unmarshal(existing, &base); err != nil {
		return content // not valid JSON — overwrite
	}
	if err := json.Unmarshal(content, &overlay); err != nil {
		return content
	}
	if base == nil {
		base = make(map[string]json.RawMessage)
	}
	for k, v := range overlay {
		if existingVal, ok := base[k]; ok {
			var baseNested, newNested map[string]json.RawMessage
			if json.Unmarshal(existingVal, &baseNested) == nil && json.Unmarshal(v, &newNested) == nil {
				for nk, nv := range newNested {
					baseNested[nk] = nv
				}
				merged, _ := json.Marshal(baseNested)
				base[k] = merged
				continue
			}
		}
		base[k] = v
	}
	out, _ := json.MarshalIndent(base, "", "  ")
	return out
}

// ensureClaudeCodeApprovesFuse adds serverName to enabledMcpjsonServers in
// ~/.claude/settings.json so Claude Code trusts the server without an
// interactive prompt after every `fuse init`.
func ensureClaudeCodeApprovesFuse(serverName string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".claude", "settings.json")
	var doc map[string]any
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &doc) //nolint:errcheck
	}
	if doc == nil {
		doc = map[string]any{}
	}
	var servers []any
	if s, ok := doc["enabledMcpjsonServers"].([]any); ok {
		servers = s
	}
	for _, s := range servers {
		if s == serverName {
			return
		}
	}
	servers = append(servers, serverName)
	doc["enabledMcpjsonServers"] = servers
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		return
	}
	fmt.Printf("approved %s in Claude Code MCP settings\n", serverName)
}

// writeFuseCodexConfig upserts an [mcp_servers.fuse] entry in Codex CLI's
// config.toml, removing any stale fuse entries first.
func writeFuseCodexConfig(path, fuseBin string, args []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir codex config dir: %w", err)
	}
	var lines []string
	if raw, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	lines = stripTOMLNamedTable(lines, "mcp_servers", "fuse")
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	lines = append(lines,
		"[mcp_servers.fuse]",
		`type = "stdio"`,
		fmt.Sprintf("command = %q", fuseBin),
		"args = ["+strings.Join(quoted, ", ")+"]",
	)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// stripTOMLNamedTable removes a [section.target] table and its body.
func stripTOMLNamedTable(lines []string, section, targetName string) []string {
	header := "[" + section + "." + targetName + "]"
	var out []string
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) != header {
			out = append(out, lines[i])
			i++
			continue
		}
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			i++
		}
	}
	return out
}
