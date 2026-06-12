package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/grove"
)

// fakeGrove satisfies analysis.GroveLike for impact tests.
type fakeGrove struct {
	nodes []grove.ImpactNode
	edges []grove.Edge
}

func (f *fakeGrove) Impact(_ context.Context, _ string, _ int) ([]grove.ImpactNode, error) {
	return f.nodes, nil
}
func (f *fakeGrove) Deps(_ context.Context, _ string) ([]grove.Edge, error) {
	return f.edges, nil
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, errb.String())
	}
}

func writeRepoFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newMergeCheckRepo builds: main edits Helper, branch agent-a edits Login —
// different symbols in the same file, semantically auto-mergeable even
// though git would need a merge.
func newMergeCheckRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@t")
	runGit(t, dir, "config", "user.name", "t")
	writeRepoFile(t, dir, "auth.go", `package main

func Login(user string) error {
	return nil
}

func Helper() int {
	return 1
}
`)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "checkout", "-qb", "agent-a")
	writeRepoFile(t, dir, "auth.go", `package main

func Login(user, password string) error {
	return nil
}

func Helper() int {
	return 1
}
`)
	runGit(t, dir, "commit", "-qam", "login takes password")
	runGit(t, dir, "checkout", "-q", "main")
	writeRepoFile(t, dir, "auth.go", `package main

func Login(user string) error {
	return nil
}

func Helper() int {
	return 2
}
`)
	runGit(t, dir, "commit", "-qam", "helper change")
	// Working ref for the check: we are on main, checking against agent-a.
	return dir
}

func TestMergeCheckAutoMergeable(t *testing.T) {
	dir := newMergeCheckRepo(t)
	h := NewHandler(dir, nil)
	out, err := h.Invoke("fuse_merge_check", map[string]any{"ref": "agent-a"})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(MergeCheckResult)
	if res.Verdict != "clean" || res.OverlappingFiles != 1 || res.AutoMergeable != 1 || res.ConflictCount != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestMergeCheckDetectsConflict(t *testing.T) {
	dir := newMergeCheckRepo(t)
	// Make main also edit Login, colliding with agent-a's Login change.
	writeRepoFile(t, dir, "auth.go", `package main

func Login(token string) error {
	return validate(token)
}

func Helper() int {
	return 2
}
`)
	runGit(t, dir, "commit", "-qam", "main rewrites Login too")

	h := NewHandler(dir, nil)
	out, err := h.Invoke("fuse_merge_check", map[string]any{"ref": "agent-a"})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(MergeCheckResult)
	if res.Verdict != "conflicts" || res.ConflictCount != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].File != "auth.go" {
		t.Fatalf("conflicts = %+v", res.Conflicts)
	}
}

func TestPreviewMergesCleanly(t *testing.T) {
	h := NewHandler(t.TempDir(), nil)
	out, err := h.Invoke("fuse_preview", map[string]any{
		"base":   "package main\n\nfunc A() {}\n\nfunc B() {}\n",
		"ours":   "package main\n\nfunc A() { a() }\n\nfunc B() {}\n",
		"theirs": "package main\n\nfunc A() {}\n\nfunc B() { b() }\n",
		"path":   "x.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(PreviewResult)
	if res.HasConflict {
		t.Fatalf("expected clean merge: %+v", res)
	}
	if !strings.Contains(res.Merged, "a()") || !strings.Contains(res.Merged, "b()") {
		t.Fatalf("merged content lost a side: %q", res.Merged)
	}
}

func TestResolvePromptAndApply(t *testing.T) {
	dir := t.TempDir()
	conflicted := "package main\n\n<<<<<<< ours\nfunc A() { x() }\n=======\nfunc A() { y() }\n>>>>>>> theirs\n"
	writeRepoFile(t, dir, "x.go", conflicted)
	writeRepoFile(t, dir, "prompt.md", "# Conflict\n\n- **File:** x.go\n\nresolve it\n")

	h := NewHandler(dir, nil)

	// Mode 1: fetch the prompt.
	out, err := h.Invoke("fuse_resolve", map[string]any{"prompt": "prompt.md"})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(ResolveResult)
	if !strings.Contains(res.Prompt, "resolve it") || !strings.HasSuffix(res.File, "x.go") {
		t.Fatalf("prompt mode = %+v", res)
	}

	// Mode 2: rejected resolution (still has markers).
	if _, err := h.Invoke("fuse_resolve", map[string]any{
		"prompt": "prompt.md", "resolution": conflicted,
	}); err == nil {
		t.Fatal("marker-laden resolution must be rejected")
	}

	// Mode 3: valid resolution, applied.
	resolved := "package main\n\nfunc A() { x(); y() }\n"
	out, err = h.Invoke("fuse_resolve", map[string]any{
		"prompt": "prompt.md", "resolution": resolved, "apply": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	res = out.(ResolveResult)
	if !res.Validated || !res.Applied {
		t.Fatalf("apply mode = %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "x.go"))
	if string(got) != resolved {
		t.Fatalf("file content = %q", got)
	}
}

func TestImpactCapsAndCounts(t *testing.T) {
	fake := &fakeGrove{}
	for i := 0; i < maxImpactNodes+10; i++ {
		fake.nodes = append(fake.nodes, grove.ImpactNode{ID: string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune(i)), FilePath: "f.go", Name: "N"})
	}
	h := NewHandler(t.TempDir(), fake)
	out, err := h.Invoke("fuse_impact", map[string]any{"query": "Login"})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(ImpactResult)
	if res.Count != maxImpactNodes+10 || len(res.Nodes) != maxImpactNodes || res.Note == "" {
		t.Fatalf("result = %+v (nodes=%d)", res, len(res.Nodes))
	}
}

func TestToolsListSchemasAndCompactOutput(t *testing.T) {
	schemas := ToolSchemas()
	if len(schemas) != 4 {
		t.Fatalf("want 4 tools, got %d", len(schemas))
	}
	for _, s := range schemas {
		name := s["name"].(string)
		desc := s["description"].(string)
		if len(desc) < 40 || len(desc) > 400 {
			t.Errorf("%s description length %d outside terse band", name, len(desc))
		}
		schema := s["inputSchema"].(map[string]any)
		if len(schema["properties"].(map[string]any)) == 0 {
			t.Errorf("%s has no parameter properties", name)
		}
	}

	// Server output must be compact JSON (no indentation): results land in
	// agent context windows.
	h := NewHandler(t.TempDir(), nil)
	srv := NewServer(h)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fuse_preview","arguments":{"ours":"a\n","theirs":"a\n","path":"x.txt"}}}` + "\n")
	var outBuf bytes.Buffer
	if err := srv.Serve(in, &outBuf); err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v (%q)", err, outBuf.String())
	}
	text := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "\n  ") {
		t.Fatalf("tool result is indented (token waste): %q", text)
	}
}
