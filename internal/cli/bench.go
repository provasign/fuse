package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/merge"
	"github.com/provasign/fuse/internal/parser"
)

// cmdBench replays real merge commits from a repository's history: for every
// file modified on both sides of a merge, it re-runs the three-way merge with
// git's line-level algorithm and with fuse, then scores both against the
// resolution the humans actually committed. This is how fuse's auto-resolution
// numbers are measured rather than asserted.
func cmdBench(args []string) int {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	limit := fs.Int("limit", 100, "max merge commits to replay")
	maxFiles := fs.Int("max-files", 0, "stop after this many files (0 = unlimited)")
	jsonOut := fs.Bool("json", false, "emit JSON instead of a table")
	verbose := fs.Bool("v", false, "print one line per replayed file")
	// Accept `fuse bench <repo> --flags`: the flag package stops parsing at
	// the first positional argument, so peel the repo path off first.
	repo := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repo = args[0]
		args = args[1:]
	}
	_ = fs.Parse(args)
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	merges, err := gitLines(repo, "rev-list", "--merges", "--max-count", strconv.Itoa(*limit), "HEAD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuse bench: cannot list merge commits in %s: %v\n", repo, err)
		return 2
	}
	if len(merges) == 0 {
		fmt.Fprintf(os.Stderr, "fuse bench: no merge commits found in %s (squash-only history?)\n", repo)
		return 1
	}

	var (
		sum     benchSummary
		perLang = map[string]*benchSummary{}
		results []benchFileResult
	)
	im := newBenchMerger()
	ctx := context.Background()

	for _, m := range merges {
		if *maxFiles > 0 && sum.FilesReplayed >= *maxFiles {
			break
		}
		parents, err := gitLines(repo, "rev-list", "--parents", "-n", "1", m)
		if err != nil || len(parents) == 0 {
			continue
		}
		fields := strings.Fields(parents[0])
		if len(fields) != 3 { // commit + exactly two parents; skip octopus merges
			continue
		}
		p1, p2 := fields[1], fields[2]
		base, err := gitFirstLine(repo, "merge-base", p1, p2)
		if err != nil || base == "" || base == p1 || base == p2 {
			continue
		}
		sum.MergesReplayed++

		oursFiles, err1 := gitLines(repo, "diff", "--name-only", base, p1)
		theirsFiles, err2 := gitLines(repo, "diff", "--name-only", base, p2)
		if err1 != nil || err2 != nil {
			continue
		}
		inTheirs := make(map[string]bool, len(theirsFiles))
		for _, f := range theirsFiles {
			inTheirs[f] = true
		}

		for _, f := range oursFiles {
			if !inTheirs[f] {
				continue
			}
			if *maxFiles > 0 && sum.FilesReplayed >= *maxFiles {
				break
			}
			r, ok := replayFile(ctx, im, repo, m, base, p1, p2, f)
			if !ok {
				continue
			}
			sum.add(r)
			ls, ok := perLang[r.Language]
			if !ok {
				ls = &benchSummary{}
				perLang[r.Language] = ls
			}
			ls.add(r)
			if *verbose {
				fmt.Printf("%s %s git=%s fuse=%s match=%v\n",
					m[:8], f, cleanWord(r.GitClean), cleanWord(r.FuseClean), r.FuseMatch)
			}
			if *jsonOut {
				results = append(results, r)
			}
		}
	}

	if *jsonOut {
		out, _ := json.MarshalIndent(struct {
			Summary     benchSummary             `json:"summary"`
			PerLanguage map[string]*benchSummary `json:"perLanguage"`
			Files       []benchFileResult        `json:"files"`
		}{sum, perLang, results}, "", "  ")
		fmt.Println(string(out))
		return 0
	}
	printBenchSummary(&sum)
	printPerLanguage(perLang)
	return 0
}

// newBenchMerger returns the merger used for replays: no Grove, no breaking
// change analysis — the benchmark measures the merge algorithm itself.
func newBenchMerger() *merge.IntelliMerge {
	im := merge.New(nil)
	im.EnableBreaking = false
	im.EnableContext = false
	return im
}

type benchFileResult struct {
	Merge     string `json:"merge"`
	File      string `json:"file"`
	Language  string `json:"language"`
	Strategy  string `json:"strategy"`
	GitClean  bool   `json:"gitClean"`
	FuseClean bool   `json:"fuseClean"`
	GitMatch  bool   `json:"gitMatchesHuman"`
	FuseMatch bool   `json:"fuseMatchesHuman"`
}

type benchSummary struct {
	MergesReplayed int `json:"mergesReplayed"`
	FilesReplayed  int `json:"filesReplayed"`

	GitConflicted        int `json:"gitConflicted"`
	FuseResolvedOfThose  int `json:"fuseResolvedWhereGitConflicted"`
	FuseMatchOfThose     int `json:"fuseMatchesHumanWhereGitConflicted"`
	GitClean             int `json:"gitClean"`
	FuseCleanOfGitClean  int `json:"fuseCleanWhereGitClean"`
	FuseMatchOfGitClean  int `json:"fuseMatchesHumanWhereGitClean"`
	GitMatchOfGitClean   int `json:"gitMatchesHumanWhereGitClean"`
	FuseConflictGitClean int `json:"fuseConflictedWhereGitClean"`
}

func (s *benchSummary) add(r benchFileResult) {
	s.FilesReplayed++
	if r.GitClean {
		s.GitClean++
		if r.GitMatch {
			s.GitMatchOfGitClean++
		}
		if r.FuseClean {
			s.FuseCleanOfGitClean++
			if r.FuseMatch {
				s.FuseMatchOfGitClean++
			}
		} else {
			s.FuseConflictGitClean++
		}
		return
	}
	s.GitConflicted++
	if r.FuseClean {
		s.FuseResolvedOfThose++
		if r.FuseMatch {
			s.FuseMatchOfThose++
		}
	}
}

// replayFile re-merges one file from a historical merge commit. Returns
// ok=false when the file is not replayable (missing on a side, binary,
// unsupported language).
func replayFile(ctx context.Context, im *merge.IntelliMerge, repo, m, base, p1, p2, f string) (benchFileResult, bool) {
	baseB, errB := gitBlob(repo, base, f)
	oursB, errO := gitBlob(repo, p1, f)
	theirsB, errT := gitBlob(repo, p2, f)
	humanB, errH := gitBlob(repo, m, f)
	if errB != nil || errO != nil || errT != nil || errH != nil {
		return benchFileResult{}, false
	}
	if isBinary(baseB) || isBinary(oursB) || isBinary(theirsB) {
		return benchFileResult{}, false
	}
	const maxBenchBytes = 1 << 20
	if len(baseB) > maxBenchBytes || len(oursB) > maxBenchBytes || len(theirsB) > maxBenchBytes {
		return benchFileResult{}, false
	}
	lang := parser.DetectLanguage(f, string(oursB))
	if !parser.Supported(lang) {
		return benchFileResult{}, false
	}

	gitOut, gitClean, gitErr := gitMergeFile(baseB, oursB, theirsB)
	if gitErr != nil {
		return benchFileResult{}, false
	}

	res, err := im.Merge(ctx, baseB, oursB, theirsB, lang, f)
	if err != nil {
		return benchFileResult{}, false
	}
	fuseClean := !res.HasConflict && res.Strategy != core.StrategyHandoff

	human := normalizeForCompare(string(humanB))
	return benchFileResult{
		Merge:     m,
		File:      f,
		Language:  string(lang),
		Strategy:  string(res.Strategy),
		GitClean:  gitClean,
		FuseClean: fuseClean,
		GitMatch:  gitClean && normalizeForCompare(gitOut) == human,
		FuseMatch: fuseClean && normalizeForCompare(res.MergedContent) == human,
	}, true
}

// gitMergeFile runs git's own three-way line merge as the baseline.
func gitMergeFile(base, ours, theirs []byte) (string, bool, error) {
	dir, err := os.MkdirTemp("", "fuse-bench-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(dir)
	bf := filepath.Join(dir, "base")
	of := filepath.Join(dir, "ours")
	tf := filepath.Join(dir, "theirs")
	for p, c := range map[string][]byte{bf: base, of: ours, tf: theirs} {
		if err := os.WriteFile(p, c, 0o644); err != nil {
			return "", false, err
		}
	}
	cmd := exec.Command("git", "merge-file", "-p", of, bf, tf)
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err == nil {
		return out.String(), true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() > 0 {
		return out.String(), false, nil // >0 = number of conflicts
	}
	return "", false, err
}

// normalizeForCompare reduces formatting noise: per-line trailing whitespace
// and trailing blank lines do not count as a mismatch.
func normalizeForCompare(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func gitLines(repo string, args ...string) ([]string, error) {
	out, err := gitCapture(repo, args...)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func gitFirstLine(repo string, args ...string) (string, error) {
	lines, err := gitLines(repo, args...)
	if err != nil || len(lines) == 0 {
		return "", err
	}
	return lines[0], nil
}

func gitBlob(repo, rev, path string) ([]byte, error) {
	out, err := gitCaptureBytes(repo, "show", rev+":"+path)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func gitCapture(repo string, args ...string) (string, error) {
	out, err := gitCaptureBytes(repo, args...)
	return string(out), err
}

func gitCaptureBytes(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func cleanWord(clean bool) string {
	if clean {
		return "clean"
	}
	return "conflict"
}

func pct(n, of int) string {
	if of == 0 {
		return "  n/a"
	}
	return fmt.Sprintf("%3.0f%%", 100*float64(n)/float64(of))
}

// printPerLanguage prints one compact row per language so corpus runs on
// polyglot repos report where the headroom actually is.
func printPerLanguage(perLang map[string]*benchSummary) {
	if len(perLang) < 2 {
		return
	}
	langs := make([]string, 0, len(perLang))
	for l := range perLang {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	fmt.Printf("\nper language:\n")
	fmt.Printf("%-12s %7s %14s %18s %20s\n", "", "files", "git conflicted", "fuse resolved", "byte-match human")
	for _, l := range langs {
		s := perLang[l]
		fmt.Printf("%-12s %7d %14d %12d (%s) %14d (%s)\n",
			l, s.FilesReplayed, s.GitConflicted,
			s.FuseResolvedOfThose, pct(s.FuseResolvedOfThose, s.GitConflicted),
			s.FuseMatchOfThose, pct(s.FuseMatchOfThose, s.FuseResolvedOfThose))
	}
}

func printBenchSummary(s *benchSummary) {
	fmt.Printf("fuse bench: replayed %d merge commits, %d files modified on both sides\n\n",
		s.MergesReplayed, s.FilesReplayed)
	fmt.Printf("%-22s %7s %16s %16s\n", "", "files", "fuse resolved", "matches human")
	fmt.Printf("%-22s %7d %10d (%s) %10d (%s)\n",
		"git conflicted", s.GitConflicted,
		s.FuseResolvedOfThose, pct(s.FuseResolvedOfThose, s.GitConflicted),
		s.FuseMatchOfThose, pct(s.FuseMatchOfThose, s.FuseResolvedOfThose))
	fmt.Printf("%-22s %7d %10d (%s) %10d (%s)\n",
		"git auto-merged", s.GitClean,
		s.FuseCleanOfGitClean, pct(s.FuseCleanOfGitClean, s.GitClean),
		s.FuseMatchOfGitClean, pct(s.FuseMatchOfGitClean, s.FuseCleanOfGitClean))
	if s.GitClean > 0 {
		fmt.Printf("\ngit matches human on its auto-merges: %d/%d (%s)\n",
			s.GitMatchOfGitClean, s.GitClean, pct(s.GitMatchOfGitClean, s.GitClean))
	}
	if s.FuseConflictGitClean > 0 {
		fmt.Printf("fuse conflicted where git was clean: %d\n", s.FuseConflictGitClean)
	}
	fmt.Println("\nnote: \"matches human\" compares against the file recorded in the merge")
	fmt.Println("commit; humans sometimes make unrelated edits while resolving, so a")
	fmt.Println("mismatch is not necessarily a wrong merge.")
}
