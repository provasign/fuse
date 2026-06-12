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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/provasign/fuse/internal/config"
	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/grove"
	"github.com/provasign/fuse/internal/handoff"
	"github.com/provasign/fuse/internal/mcp"
	"github.com/provasign/fuse/internal/merge"
	"github.com/provasign/fuse/internal/merge/analysis"
	"github.com/provasign/fuse/internal/parser"
	"github.com/provasign/fuse/internal/version"
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
	case "bench":
		return cmdBench(argv[1:])
	case "mcp":
		return cmdMCP(argv[1:])
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
  fuse resolve <conflict-file> [--agent <cmd>] [--apply]
                                             Resolve a conflict via an AI agent (no --agent: print prompt)
  fuse check <file>                          Detect breaking changes for current file (vs HEAD)
  fuse impact <file-or-symbol>               Show blast radius via Grove
  fuse deps <file>                           Show dependencies via Grove
  fuse config                                Print resolved configuration
  fuse bench [repo] [--limit N] [--json]     Replay historical merges and score auto-resolution
  fuse mcp [dir]                             Start the stdio MCP server (agent integration)
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

// newGrove builds the embedded Grove client rooted at the current repo.
// Returns (nil, nil) if Grove is configured as not-required and the index
// cannot be opened. Grove runs in-process — no daemon, no port, no token.
func newGrove(cfg *config.Config, required bool) (analysis.GroveLike, error) {
	cwd, _ := os.Getwd()
	c := grove.New(cfg.GroveURL).WithTokenFromDir(cwd)
	healthTimeout := 10 * time.Second
	if !required {
		healthTimeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		if required {
			return nil, err
		}
		return nil, nil
	}
	// Make sure the index is usable: a fresh clone opens with zero symbols
	// and blast-radius queries would silently return nothing. Best-effort —
	// a merge must not fail because indexing did.
	if cfg.Merge.AutoIndex {
		ictx, icancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer icancel()
		if err := c.EnsureIndexed(ictx); err != nil {
			fmt.Fprintf(os.Stderr, "fuse: grove auto-index failed (%v); cross-file analysis degraded\n", err)
		}
	}
	return c, nil
}

func closeGroveClient(c analysis.GroveLike) {
	if c == nil {
		return
	}
	if closer, ok := c.(interface{ Close() }); ok {
		closer.Close()
	}
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
	defer closeGroveClient(groveClient)
	if gerr != nil && cfg.Merge.GroveRequired {
		fmt.Fprintf(os.Stderr, "fuse: Grove is required but not reachable: %v\n", gerr)
		return 2
	}

	im := merge.New(groveClient)
	im.HandoffThreshold = cfg.Merge.HandoffThreshold
	im.EnableBreaking = cfg.Merge.EnableBreakingChange && groveClient != nil
	im.EnableContext = cfg.Merge.EnableContext && groveClient != nil

	// Capture the pre-merge graph for drift evidence. Only meaningful when
	// git gave us the logical repo path (%P): with temp-path-only
	// invocations the snapshot has no symbols at that path and every merged
	// symbol would misreport as "added".
	var preMerge grove.GraphSnapshot
	var driftClient *grove.Client
	if dc, ok := groveClient.(*grove.Client); ok && cfg.Merge.EnableDrift && logicalPath != oursPath {
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		preMerge, _ = dc.Snapshot(sctx)
		scancel()
		driftClient = dc
	}

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

	// Drift evidence: the structural code-graph delta this merge produces.
	// Auto-merges only — a conflicted result still carries markers, so its
	// "symbols" would be noise; the resolved drift lands when the conflict
	// is actually resolved and reindexed.
	if driftClient != nil && gitDir != "" && !res.HasConflict && res.Strategy != core.StrategyHandoff {
		recordDrift(driftClient, preMerge, gitDir, logicalPath, res)
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

// recordDrift appends the structural delta of one merged file to
// .git/fuse/drift.json and prints a one-line summary. Advisory and
// fail-open throughout: a merge never fails because drift capture did.
// The merged bytes are diffed in memory against the pre-merge snapshot —
// git writes %A to the worktree only after the driver exits, so the
// post-merge state is never on disk while we run.
func recordDrift(client *grove.Client, preMerge grove.GraphSnapshot, gitDir, logicalPath string, res *core.MergeResult) {
	if preMerge == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	drift, err := client.DriftForMergedFile(ctx, preMerge, filepath.ToSlash(logicalPath), []byte(res.MergedContent))
	if err != nil || drift.Empty() {
		return
	}
	_ = handoff.AppendDrift(filepath.Join(gitDir, "fuse"), handoff.DriftRecord{
		File:     logicalPath,
		Strategy: string(res.Strategy),
		Drift:    drift,
	})
	fmt.Fprintf(os.Stderr, "fuse: %s graph drift: %s\n", logicalPath, drift.Summary())
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
	// Start from abs itself if it is a directory, otherwise from its parent.
	cur := abs
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		cur = filepath.Dir(abs)
	}
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
	defer closeGroveClient(groveClient)
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
	printDriftStatus(filepath.Join(gitDir, "fuse", "drift.json"))
	return 0
}

// printDriftStatus summarizes recorded graph drift (structural symbol deltas
// per merge) beneath the audit listing.
func printDriftStatus(driftPath string) {
	data, err := os.ReadFile(driftPath)
	if err != nil {
		return
	}
	var records []handoff.DriftRecord
	if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 {
		return
	}
	fmt.Printf("%d merge(s) with graph drift\n", len(records))
	for _, r := range records {
		fmt.Printf("  %s  %s  %s\n", r.Timestamp, r.File, r.Drift.Summary())
		for _, d := range r.Drift.Breaking {
			fmt.Printf("    breaking: %s %s (%s → %s)\n", d.Kind, d.QualifiedName, d.OldSignature, d.NewSignature)
		}
	}
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

// cmdResolve closes the AI loop. Without --agent it prints the handoff
// prompt for manual use. With --agent (or resolve.agent_cmd in fuse.yaml) it
// pipes the prompt into the agent command, validates the returned resolution
// (non-empty, no markers, parses), and with --apply writes it to the
// conflicted file.
func cmdResolve(args []string) int {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	agent := fs.String("agent", "", "shell command reading the prompt on stdin and writing the resolved file to stdout (e.g. \"claude -p\")")
	apply := fs.Bool("apply", false, "write the validated resolution to the conflicted file")
	var promptPath string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		promptPath = args[0]
		args = args[1:]
	}
	_ = fs.Parse(args)
	if fs.NArg() > 0 {
		promptPath = fs.Arg(0)
	}
	if promptPath == "" {
		fmt.Fprintln(os.Stderr, "usage: fuse resolve <conflict-file> [--agent <cmd>] [--apply]")
		return 2
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	cmdStr := *agent
	if cmdStr == "" {
		cmdStr = loadConfig().Resolve.AgentCmd
	}
	if cmdStr == "" {
		// Manual mode: print the prompt for copy/paste into any agent.
		fmt.Println(string(prompt))
		return 0
	}

	target := promptTargetFile(prompt)
	if target == "" {
		fmt.Fprintln(os.Stderr, "fuse resolve: prompt has no '- **File:**' line; cannot locate the conflicted file")
		return 2
	}

	fmt.Fprintf(os.Stderr, "fuse resolve: running agent for %s\n", target)
	agentCmd := newShellCmd(cmdStr)
	agentCmd.Stdin = strings.NewReader(string(prompt))
	agentCmd.Stderr = os.Stderr
	out, err := agentCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse resolve: agent failed: %v\n", err)
		return 2
	}
	resolved := stripCodeFences(out)
	if err := merge.ValidateResolution(target, resolved); err != nil {
		fmt.Fprintf(os.Stderr, "fuse resolve: rejected agent output: %v\n", err)
		return 1
	}

	if !*apply {
		fmt.Print(string(resolved))
		fmt.Fprintf(os.Stderr, "\nfuse resolve: resolution validated (use --apply to write %s)\n", target)
		return 0
	}
	if err := os.WriteFile(target, resolved, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "fuse resolve: cannot write %s: %v\n", target, err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "fuse resolve: wrote %s — review the change, then `git add %s`\n", target, target)
	return 0
}

// promptTargetFile extracts the conflicted file path from the handoff
// prompt's `- **File:** <path>` summary line.
func promptTargetFile(prompt []byte) string {
	for _, line := range strings.Split(string(prompt), "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "- **File:**"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// stripCodeFences removes a single wrapping markdown code fence if the agent
// returned one despite instructions.
func stripCodeFences(out []byte) []byte {
	s := strings.TrimSpace(string(out))
	if !strings.HasPrefix(s, "```") {
		return out
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return out
	}
	return []byte(strings.Join(lines[1:len(lines)-1], "\n") + "\n")
}

func cmdCheck(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse check <file>")
		return 2
	}
	filePath := args[0]

	workingContent, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse check: cannot read %s: %v\n", filePath, err)
		return 2
	}

	// Get the HEAD blob for the same path. If the file is new (no HEAD blob),
	// every exported symbol is "added" — there is nothing to break, so we
	// report a clean result and exit 0.
	headContent, headErr := gitShowHead(filePath)
	if headErr != nil {
		fmt.Printf("fuse check: %s is not tracked at HEAD (new file) — nothing to break\n", filePath)
		return 0
	}

	lang := parser.DetectLanguage(filePath, string(workingContent))
	if !parser.IsAST(lang) {
		fmt.Printf("fuse check: %s language %q is not AST-parseable; skipping breaking-change scan\n", filePath, lang)
		return 0
	}

	im := merge.New(nil)
	strategy := im.Registry.Get(lang)
	if strategy == nil {
		fmt.Printf("fuse check: no strategy for language %q\n", lang)
		return 0
	}

	headTree, hErr := im.Parser.Parse(lang, headContent)
	workTree, wErr := im.Parser.Parse(lang, workingContent)
	defer func() {
		if headTree != nil {
			headTree.Close()
		}
		if workTree != nil {
			workTree.Close()
		}
	}()
	if hErr != nil || wErr != nil {
		fmt.Fprintf(os.Stderr, "fuse check: parse failed (head=%v, work=%v)\n", hErr, wErr)
		return 2
	}

	baseSyms, _ := strategy.Extract(headTree, headContent)
	workSyms, _ := strategy.Extract(workTree, workingContent)

	cfg := loadConfig()
	groveClient, _ := newGrove(cfg, false) // optional — analyzer falls back gracefully when nil
	defer closeGroveClient(groveClient)
	analyzer := &analysis.BreakingChangeAnalyzer{Grove: groveClient}
	// 2-way diff modelled as 3-way with ours==theirs so only removed/added
	// signal fires (signature drift requires both sides to differ from base).
	changes := analyzer.Analyze(context.Background(), filePath, baseSyms, workSyms, workSyms)

	if len(changes) == 0 {
		fmt.Printf("fuse check: %s — no breaking changes vs HEAD\n", filePath)
		return 0
	}
	fmt.Printf("fuse check: %s — %d breaking change(s) vs HEAD\n", filePath, len(changes))
	for _, c := range changes {
		fmt.Printf("  [%s] %s: %s\n", c.Severity, c.Kind, c.Message)
		for _, f := range c.AffectedFiles {
			fmt.Printf("    affects %s\n", f)
		}
	}
	return 1
}

// gitShowHead returns the HEAD blob contents for filePath (relative to repo
// root) or an error if the file isn't tracked or git fails.
func gitShowHead(filePath string) ([]byte, error) {
	c := exec.Command("git", "show", "HEAD:"+filePath)
	out, err := c.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func cmdImpact(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse impact <file-or-symbol>")
		return 2
	}
	query := args[0]
	cfg := loadConfig()
	groveClient, gerr := newGrove(cfg, true)
	defer closeGroveClient(groveClient)
	if gerr != nil {
		fmt.Fprintf(os.Stderr, "fuse impact: Grove required: %v\n", gerr)
		return 2
	}

	ctx := context.Background()
	queries := []string{query}
	// If the arg looks like an existing file, expand to the symbols defined
	// in that file via /deps and call /impact for each. Without this the
	// command silently returns nothing for the common `fuse impact <file>`
	// invocation that the help text advertises.
	if _, err := os.Stat(query); err == nil {
		edges, derr := groveClient.Deps(ctx, query)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "fuse impact: deps lookup failed: %v\n", derr)
			return 2
		}
		var syms []string
		for _, e := range edges {
			if e.Type == "defines" {
				syms = append(syms, e.To)
			}
		}
		if len(syms) > 0 {
			queries = syms
		}
	}

	seen := map[string]bool{}
	var all []groveImpactRow
	for _, q := range queries {
		nodes, err := groveClient.Impact(ctx, q, 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fuse impact: %v\n", err)
			return 2
		}
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			all = append(all, groveImpactRow{ID: n.ID, FilePath: n.FilePath, Name: n.Name})
		}
	}
	if len(all) == 0 {
		fmt.Printf("fuse impact: no downstream impact found for %q\n", query)
		return 0
	}
	for _, n := range all {
		fmt.Printf("%s  %s  %s\n", n.ID, n.FilePath, n.Name)
	}
	return 0
}

type groveImpactRow struct {
	ID, FilePath, Name string
}

func cmdDeps(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: fuse deps <file>")
		return 2
	}
	cfg := loadConfig()
	groveClient, gerr := newGrove(cfg, true)
	defer closeGroveClient(groveClient)
	if gerr != nil {
		fmt.Fprintf(os.Stderr, "fuse deps: Grove required: %v\n", gerr)
		return 2
	}
	edges, err := groveClient.Deps(context.Background(), args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(edges) == 0 {
		fmt.Printf("fuse deps: no edges recorded for %s (run `grove index .` first?)\n", args[0])
		return 0
	}
	for _, e := range edges {
		fmt.Printf("%s -%s-> %s\n", e.From, e.Type, e.To)
	}
	return 0
}

// cmdMCP starts the stdio MCP server — fuse's agent-facing surface
// (fuse_merge_check, fuse_preview, fuse_resolve, fuse_impact).
func cmdMCP(args []string) int {
	root := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		root = args[0]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fuse mcp:", err)
		return 2
	}
	cfg := loadConfig()
	groveClient, _ := newGrove(cfg, false)
	defer closeGroveClient(groveClient)
	h := mcp.NewHandler(abs, groveClient)
	if err := mcp.NewServer(h).Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fuse mcp:", err)
		return 1
	}
	return 0
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
