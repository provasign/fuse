package grove_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tabladrum/grove-suite/fuse/internal/grove"
)

// ── Health: non-200 response ──

func TestHealth_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	err := c.Health(context.Background())
	if err == nil {
		t.Error("expected error for 503")
	}
	if !strings.Contains(err.Error(), "grove health") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── post: non-2xx response returns detailed error ──

func TestPost_Non2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	// Deps calls post("/deps",...)
	_, err := c.Deps(context.Background(), "/some/file.go")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "grove") {
		t.Errorf("error should mention grove: %v", err)
	}
}

// ── Impact: default maxDepth ──

func TestImpact_DefaultMaxDepth(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"nodes": []any{}})
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	_, err := c.Impact(context.Background(), "file.go", 0) // 0 → default 3
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["maxDepth"] != float64(3) {
		t.Errorf("expected maxDepth=3; got %v", gotBody["maxDepth"])
	}
}

// ── Symbols: default limit ──

func TestSymbols_DefaultLimit(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"symbols": []any{}})
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	_, err := c.Symbols(context.Background(), "MyFunc", 0) // 0 → default 50
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["limit"] != float64(50) {
		t.Errorf("expected limit=50; got %v", gotBody["limit"])
	}
}

// ── Impact: error response ──

func TestImpact_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	_, err := c.Impact(context.Background(), "f.go", 2)
	if err == nil {
		t.Error("expected error")
	}
}

// ── Symbols: error response ──

func TestSymbols_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := grove.New(srv.URL)
	_, err := c.Symbols(context.Background(), "X", 5)
	if err == nil {
		t.Error("expected error")
	}
}

// ── EnsureRunning: already healthy ──

func TestEnsureRunning_AlreadyHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	err := grove.EnsureRunning(context.Background(), srv.URL, "nonexistent-grove-binary", 2*time.Second)
	if err != nil {
		t.Errorf("expected nil error when grove is healthy; got: %v", err)
	}
}

// ── EnsureRunning: binary not found → immediate error ──

func TestEnsureRunning_BinaryNotFound(t *testing.T) {
	// Use a port that nothing is listening on so Health fails, then binary not found.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	baseURL := "http://127.0.0.1:" + itoa(port)
	err = grove.EnsureRunning(context.Background(), baseURL, "/nonexistent/path/grove-binary-xyz", 500*time.Millisecond)
	if err == nil {
		t.Error("expected error when binary doesn't exist")
	}
}

// ── EnsureRunning: binary starts but grove never responds ──

func TestEnsureRunning_Timeout(t *testing.T) {
	// Use a port that nothing binds to, and a binary that exists but isn't grove.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	baseURL := "http://127.0.0.1:" + itoa(port)
	// Use /bin/echo as binary — it starts but doesn't serve HTTP.
	err = grove.EnsureRunning(context.Background(), baseURL, "/bin/echo", 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── post: connection refused ──

func TestPost_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	c := grove.New("http://127.0.0.1:" + itoa(port))
	_, err = c.Deps(context.Background(), "file.go")
	if err == nil {
		t.Error("expected connection error")
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
