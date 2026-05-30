// Package cli implements the fuse command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/fuse/internal/config"
	"github.com/tabladrum/grove-suite/fuse/internal/core"
	"github.com/tabladrum/grove-suite/fuse/internal/grove"
	"github.com/tabladrum/grove-suite/fuse/internal/handoff"
	"github.com/tabladrum/grove-suite/fuse/internal/merge"
	"github.com/tabladrum/grove-suite/fuse/internal/merge/analysis"
	"github.com/tabladrum/grove-suite/fuse/internal/parser"
	"github.com/tabladrum/grove-suite/fuse/internal/version"
)

// Run dispatches subcommands. argv excludes the program name.
func Run(argv []string) int {
	if len(argv) == 0 {
		printUsage(os.Stderr)
		return 2
	}
	switch argv[0] {
	case "version", "--version", "-v":
		fmt.Println("fuse", version.Version)
		return 0
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return 0
	case "merge":
		return cmdMerge(argv[1:])
	case "preview":
		return cmdPreview(argv[1:])
	case "install":
		return cmdInstall(argv[1:])
	case "uninstall":
		return cmdUninstall(argv[1:])
	case "status":
		return cmdStatus(argv[1:])
	case "config":
		return cmdConfig(argv[1:])
	case "check":
		return cmdCheck(argv[1:])
	case "impact":
		return cmdImpact(argv[1:])
	case "deps":
		return cmdDeps(argv[1:])
	case "resolve":
		return cmdResolve(argv[1:])
	case "serve":
		return cmdServe(argv[1:])
	default:
		fmt.Fprintf(os.Stderr, "fuse: unknown command %q\n", argv[0])
		printUsage(os.Stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `fuse — semantic merge driver

Commands:
  fuse merge <base> <ours> <theirs> [path]   Perform a semantic merge; writes result to <ours>
  fuse preview <base> <ours> <theirs>        Print merged result without writing
  fuse install                               Register fuse as a Git merge driver
  fuse uninstall                             Unregister fuse
  fuse status                                Show last audit stats
  fuse resolve <conflict-file>               Print AI handoff prompt
  fuse check <file>                          Detect breaking changes for current file (vs HEAD)
  fuse impact <file>                         Show blast radius via Grove
  fuse deps <file>                           Show dependencies via Grove
  fuse config                                Print resolved configuration
  fuse serve [--port 9999]                   Start HTTP API
  fuse version                               Show version
`)
}

func loadConfig() *config.Config {
	cwd, _ := os.Getwd()
	path := config.LocateConfig(cwd)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse: bad config %q: %v\n", path, err)
		os.Exit(2)
	}
	return cfg
}

// newGrove builds the Grove client (and optionally ensures Grove is running).
// Returns (nil, nil) if Grove is configured as not-required and unreachable.
func newGrove(cfg *config.Config, required bool) (analysis.GroveLike, error) {
	if !required {
		c := grove.New(cfg.GroveURL)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := c.Health(ctx); err != nil {
			return nil, nil
		}
		return c, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := grove.EnsureRunning(ctx, cfg.GroveURL, cfg.GroveBinary, 10*time.Second); err != nil {
		return nil, err
	}
	return grove.New(cfg.GroveURL), nil
}

// cmdMerge implements `fuse merge <base> <ours> <theirs> [path]`.
//
// Git invokes us with %O %A %B %P. We MUST write the merged result back to
// %A (the ours-file). Exit 0 means clean; exit 1 means conflict markers
// written; exit 2 means hard failure (binary, too large, unreadable).
//
// Every invocation prints a single-line status to stderr so the user can
// see what fuse did, even on success:
//
//	fuse: <file> [<strategy>] → <result> (confidence=X.XX)
func cmdMerge(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: fuse merge <base> <ours> <theirs> [path]")
		return 2
	}
	basePath, oursPath, theirsPath := args[0], args[1], args[2]
	logicalPath := oursPath
	if len(args) >= 4 {
		logicalPath = args[3]
	}

	baseContent, _ := os.ReadFile(basePath)
	oursContent, err := os.ReadFile(oursPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse: %s → rejected: cannot read ours: %v\n", logicalPath, err)
		return 2
	}
	theirsContent, err := os.ReadFile(theirsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse: %s → rejected: cannot read theirs: %v\n", logicalPath, err)
		return 2
	}

	// Safety guard 1: refuse binary files. Merging would corrupt them.
	if isBinary(oursContent) || isBinary(theirsContent) || isBinary(baseContent) {
		fmt.Fprintf(os.Stderr, "fuse: %s → rejected: binary file (fuse only handles text)\n", logicalPath)
		return 2
	}

	// Safety guard 2: refuse very large files. Tree-sitter and our reconstruction
	// have superlinear behavior beyond this. Git's default driver can still try.
	const maxFileBytes = 10 * 1024 * 1024 // 10 MB
	if len(oursContent) > maxFileBytes || len(theirsContent) > maxFileBytes || len(baseContent) > maxFileBytes {
		fmt.Fprintf(os.Stderr, "fuse: %s → rejected: file exceeds %d bytes\n", logicalPath, maxFileBytes)
		return 2
	}

	cfg := loadConfig()
	lang := parser.DetectLanguage(logicalPath, string(oursContent))
	if !parser.Supported(lang) {
		// Defer to line-level three-way merge.
		code := runLineMergeAndWrite(oursPath, logicalPath, baseContent, oursContent, theirsContent)
		notifyLineResult(logicalPath, code)
		return code
	}

	groveClient, gerr := newGrove(cfg, cfg.Merge.GroveRequired && cfg.Merge.EnableBreakingChange)
	if gerr != nil && cfg.Merge.GroveRequired {
		fmt.Fprintf(os.Stderr, "fuse: Grove is required but not reachable: %v\n", gerr)
		return 2
	}

	im := merge.New(groveClient)
	im.HandoffThreshold = cfg.Merge.HandoffThreshold
	im.EnableBreaking = cfg.Merge.EnableBreakingChange && groveClient != nil
	im.EnableContext = cfg.Merge.EnableContext && groveClient != nil

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := im.Merge(ctx, baseContent, oursContent, theirsContent, lang, logicalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse: merge failed: %v\n", err)
		return 2
	}

	// Handoff prompt + audit.
	gitDir := findGitDir(oursPath)
	if gitDir != "" {
		fuseDir := filepath.Join(gitDir, "fuse")
		if res.Strategy == core.StrategyHandoff && len(res.Conflicts) > 0 {
			gen, err := handoff.NewGenerator(fuseDir)
			if err == nil {
				dependents := analysis.Dependents(ctx, groveClient, logicalPath)
				promptPath, _ := gen.Write(handoff.PromptInputs{
					FilePath:        logicalPath,
					Language:        lang,
					ConflictType:    res.ConflictType,
					Severity:        res.Severity,
					Confidence:      res.Confidence,
					Conflicts:       res.Conflicts,
					BreakingChanges: res.BreakingChanges,
					Dependents:      dependents,
				})
				res.PromptFile = promptPath
				res.AuditEntry.PromptFile = promptPath
			}
		}
		_ = handoff.AppendAudit(fuseDir, res.AuditEntry)
	}

	if err := os.WriteFile(oursPath, []byte(res.MergedContent), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "fuse: cannot write merged: %v\n", err)
		return 2
	}

	for _, d := range res.Diagnostics {
		fmt.Fprintln(os.Stderr, "fuse:", d)
	}
	if res.HasConflict || res.Strategy == core.StrategyHandoff {
		fmt.Fprintf(os.Stderr, "fuse: %s [%s] → conflict (confidence=%.2f)\n", logicalPath, res.Strategy, res.Confidence)
		if res.PromptFile != "" {
			fmt.Fprintf(os.Stderr, "fuse: AI prompt: %s\n", res.PromptFile)
		}
		return 1
	}
	fmt.Fprintf(os.Stderr, "fuse: %s [%s] → auto-merged (confidence=%.2f)\n", logicalPath, res.Strategy, res.Confidence)
	return 0
}

// isBinary returns true if data looks like a binary file. Heuristic: any
// NUL byte in the first 8 KB. Matches git's own binary detection.
func isBinary(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// notifyLineResult prints a status line for the line-merge fallback path so
// the user always sees what fuse did, regardless of language support.
func notifyLineResult(logicalPath string, code int) {
	switch code {
	case 0:
		fmt.Fprintf(os.Stderr, "fuse: %s [line] → auto-merged\n", logicalPath)
	case 1:
		fmt.Fprintf(os.Stderr, "fuse: %s [line] → conflict markers written\n", logicalPath)
	default:
		fmt.Fprintf(os.Stderr, "fuse: %s [line] → failed (exit %d)\n", logicalPath, code)
	}
}

// runLineMergeAndWrite is used for languages we don't support: a pure
// line-level three-way merge with conflict markers.
func runLineMergeAndWrite(oursPath, logicalPath string, base, ours, theirs []byte) int {
	im := merge.New(nil)
	ctx := context.Background()
	res, err := im.Merge(ctx, base, ours, theirs, core.LangUnknown, logicalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse: line merge failed: %v\n", err)
		return 2
	}
	_ = os.WriteFile(oursPath, []byte(res.MergedContent), 0o644)
	if res.HasConflict {
		return 1
	}
	return 0
}

func extractConflicts(_ *core.MergeResult) []core.SymbolConflict { return nil }

var _ = extractConflicts

// findGitDir walks up from path until it finds a `.git` directory or file.
// Returns the .git directory path, or "" if not in a repo.
func findGitDir(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	cur := filepath.Dir(abs)
	for {
		candidate := filepath.Join(cur, ".git")
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				return candidate
			}
			// .git file (worktree) — read gitdir: pointer
			data, err := os.ReadFile(candidate)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if strings.HasPrefix(line, "gitdir:") {
					return strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
				}
			}
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

func cmdPreview(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: fuse preview <base> <ours> <theirs>")
		return 2
	}
	baseContent, _ := os.ReadFile(args[0])
	oursContent, err := os.ReadFile(args[1])
	if err != nil {
		return 2
	}
	theirsContent, err := os.ReadFile(args[2])
	if err != nil {
		return 2
	}
	cfg := loadConfig()
	groveClient, _ := newGrove(cfg, false)
	im := merge.New(groveClient)
	im.EnableBreaking = false
	im.EnableContext = false
	lang := parser.DetectLanguage(args[1], string(oursContent))
	res, err := im.Merge(context.Background(), baseContent, oursContent, theirsContent, lang, args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fuse:", err)
		return 2
	}
	fmt.Println(res.MergedContent)
	if res.HasConflict {
		fmt.Fprintf(os.Stderr, "\n--- conflicts present (strategy=%s, confidence=%.2f) ---\n", res.Strategy, res.Confidence)
		return 1
	}
	return 0
}

func cmdInstall(_ []string) int {
	// Set the merge driver in git config (local repo).
	if err := runGit("config", "merge.fuse.name", "Fuse semantic merge driver"); err != nil {
		fmt.Fprintln(os.Stderr, "fuse install:", err)
		return 2
	}
	if err := runGit("config", "merge.fuse.driver", "fuse merge %O %A %B %P"); err != nil {
		fmt.Fprintln(os.Stderr, "fuse install:", err)
		return 2
	}
	// Append to .gitattributes for supported file types.
	attrs := []string{
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
	if err := upsertGitAttributes(".gitattributes", attrs); err != nil {
		fmt.Fprintln(os.Stderr, "fuse install:", err)
		return 2
	}
	fmt.Println("fuse installed. Registered as merge driver in this repo.")
	return 0
}

func cmdUninstall(_ []string) int {
	_ = runGit("config", "--unset", "merge.fuse.driver")
	_ = runGit("config", "--unset", "merge.fuse.name")
	fmt.Println("fuse uninstalled.")
	return 0
}

func cmdStatus(_ []string) int {
	cwd, _ := os.Getwd()
	gitDir := findGitDir(cwd)
	if gitDir == "" {
		fmt.Println("not in a git repo")
		return 1
	}
	auditPath := filepath.Join(gitDir, "fuse", "audit.json")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		fmt.Printf("no audit log yet (%s)\n", auditPath)
		return 0
	}
	var entries []core.AuditEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("%d merges recorded\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  %s  %s  strategy=%s  conf=%.2f  conflicts=%d  breaking=%d\n",
			e.Timestamp, e.File, e.Strategy, e.Confidence,
			boolToInt(!e.AutoMerged), e.BreakingChanges)
	}
	return 0
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func cmdConfig(_ []string) int {
	cfg := loadConfig()
	out, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(out))
	return 0
}

func cmdResolve(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse resolve <conflict-file>")
		return 2
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}

func cmdCheck(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse check <file>")
		return 2
	}
	cfg := loadConfig()
	groveClient, gerr := newGrove(cfg, true)
	if gerr != nil {
		fmt.Fprintf(os.Stderr, "fuse check: Grove required: %v\n", gerr)
		return 2
	}
	deps, err := groveClient.Deps(context.Background(), args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	for _, e := range deps {
		fmt.Printf("%s -%s-> %s (conf=%.2f)\n", e.From, e.Type, e.To, e.Confidence)
	}
	return 0
}

func cmdImpact(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse impact <file>")
		return 2
	}
	cfg := loadConfig()
	groveClient, gerr := newGrove(cfg, true)
	if gerr != nil {
		return 2
	}
	nodes, err := groveClient.Impact(context.Background(), args[0], 3)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	for _, n := range nodes {
		fmt.Printf("%s  %s  %s\n", n.ID, n.FilePath, n.Name)
	}
	return 0
}

func cmdDeps(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse deps <file>")
		return 2
	}
	cfg := loadConfig()
	groveClient, gerr := newGrove(cfg, true)
	if gerr != nil {
		return 2
	}
	edges, err := groveClient.Deps(context.Background(), args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	for _, e := range edges {
		fmt.Printf("%s -%s-> %s\n", e.From, e.Type, e.To)
	}
	return 0
}

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 9999, "port")
	_ = fs.Parse(args)
	cfg := loadConfig()
	if *port != 9999 {
		cfg.Server.Port = *port
	}
	return startServer(cfg)
}

// upsertGitAttributes ensures each line in lines is present in path.
func upsertGitAttributes(path string, lines []string) error {
	existing := map[string]bool{}
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			existing[strings.TrimSpace(l)] = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, l := range lines {
		if existing[l] {
			continue
		}
		if _, err := fmt.Fprintln(f, l); err != nil {
			return err
		}
	}
	return nil
}

func runGit(args ...string) error {
	cmd := newCmd("git", args...)
	return cmd.Run()
}
