package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildMergeHistoryRepo creates a repo whose single merge commit contains a
// file both branches modified: ours edits A(), theirs edits B(). Git's
// line-level merge resolves this cleanly (the functions are far apart), and
// the committed resolution contains both edits.
func buildMergeHistoryRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=bench", "GIT_AUTHOR_EMAIL=b@x",
			"GIT_COMMITTER_NAME=bench", "GIT_COMMITTER_EMAIL=b@x",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	base := "package x\n\nfunc A() int { return 1 }\n\nfunc B() int { return 2 }\n"
	run("init", "-q", "-b", "main")
	write(base)
	run("add", "x.go")
	run("commit", "-q", "-m", "base")

	run("checkout", "-q", "-b", "feature")
	write(strings.Replace(base, "return 2", "return 20", 1))
	run("commit", "-aqm", "theirs: edit B")

	run("checkout", "-q", "main")
	write(strings.Replace(base, "return 1", "return 10", 1))
	run("commit", "-aqm", "ours: edit A")

	run("merge", "-q", "--no-edit", "feature")
	return dir
}

func TestBenchReplaysMergeHistory(t *testing.T) {
	repo := buildMergeHistoryRepo(t)

	if code := Run([]string{"bench", repo, "--json"}); code != 0 {
		t.Fatalf("bench exited %d", code)
	}
}

func TestBenchReplayFileScoresCleanMerge(t *testing.T) {
	repo := buildMergeHistoryRepo(t)

	merges, err := gitLines(repo, "rev-list", "--merges", "HEAD")
	if err != nil || len(merges) != 1 {
		t.Fatalf("expected one merge commit, got %v (%v)", merges, err)
	}
	parents, err := gitLines(repo, "rev-list", "--parents", "-n", "1", merges[0])
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(parents[0])
	if len(fields) != 3 {
		t.Fatalf("expected two parents, got %v", fields)
	}
	base, err := gitFirstLine(repo, "merge-base", fields[1], fields[2])
	if err != nil {
		t.Fatal(err)
	}

	im := newBenchMerger()
	r, ok := replayFile(t.Context(), im, repo, merges[0], base, fields[1], fields[2], "x.go")
	if !ok {
		t.Fatal("expected x.go to be replayable")
	}
	if !r.GitClean {
		t.Error("git should auto-merge distant edits")
	}
	if !r.FuseClean {
		t.Error("fuse should auto-merge symbol-disjoint edits")
	}
	if !r.FuseMatch {
		t.Error("fuse output should match the human resolution")
	}
}

func TestBenchNoMergeHistory(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if code := Run([]string{"bench", dir}); code == 0 {
		t.Error("expected nonzero exit when repo has no merges")
	}
}

func TestNormalizeForCompare(t *testing.T) {
	a := "a  \r\nb\n\n\n"
	b := "a\nb"
	if normalizeForCompare(a) != normalizeForCompare(b) {
		t.Errorf("normalization mismatch: %q vs %q", normalizeForCompare(a), normalizeForCompare(b))
	}
}
