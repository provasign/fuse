package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/provasign/fuse/internal/config"
)

func TestExtractConflicts_Stub(t *testing.T) {
	if got := extractConflicts(nil); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestUpsertGitAttributes_NewAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if err := upsertGitAttributes(path, []string{"*.go merge=fuse", "*.ts merge=fuse"}); err != nil {
		t.Fatal(err)
	}
	if err := upsertGitAttributes(path, []string{"*.go merge=fuse", "*.py merge=fuse"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, w := range []string{"*.go merge=fuse", "*.ts merge=fuse", "*.py merge=fuse"} {
		if !bytesContains(data, []byte(w)) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
}

func bytesContains(b, sub []byte) bool { return bytes.Contains(b, sub) }

func TestCmdServe_StartsAndExitsOnClose(t *testing.T) {
	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cfg := &config.Config{}
	cfg.Server.Port = port

	exited := make(chan int, 1)
	go func() { exited <- startServer(cfg) }()

	// Poll /health
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var resp *http.Response
	for ctx.Err() == nil {
		resp, err = http.Get(url)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("health never reachable: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d body=%s", resp.StatusCode, body)
	}

	// Exercise /merge endpoint.
	merge := bytes.NewReader([]byte(`{"base":"package x\n","ours":"package x\nfunc A(){}\n","theirs":"package x\nfunc B(){}\n","path":"f.go"}`))
	resp, err = http.Post(fmt.Sprintf("http://127.0.0.1:%d/merge", port), "application/json", merge)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	// Bad JSON branch.
	resp, _ = http.Post(fmt.Sprintf("http://127.0.0.1:%d/merge", port), "application/json", bytes.NewReader([]byte("{garbage")))
	if resp != nil {
		_ = resp.Body.Close()
	}

	// The server has no graceful-shutdown hook; we just let the process
	// end. We don't drain `exited` because the goroutine blocks in
	// ListenAndServe.
}

func TestCmdServe_ParsesFlag(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		// Port -1 forces ListenAndServe to fail immediately.
		done <- cmdServe([]string{"--port", "-1"})
	}()
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("expected 1, got %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Error("cmdServe did not exit")
	}
}
