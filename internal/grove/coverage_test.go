package grove

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestSymbolsAndIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/symbols":
			_ = json.NewEncoder(w).Encode(map[string]any{"symbols": []SymbolRecord{{Name: "X"}}})
		case "/index":
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL)
	got, err := c.Symbols(context.Background(), "x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "X" {
		t.Errorf("got %+v", got)
	}
	if err := c.Index(context.Background(), "/tmp/x"); err != nil {
		t.Fatal(err)
	}
}

func TestPost_BadStatusReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.Deps(context.Background(), "f"); err == nil {
		t.Error("expected error on 500")
	}
}

func TestHealth_ConnRefused(t *testing.T) {
	c := New("http://127.0.0.1:1")
	if err := c.Health(context.Background()); err == nil {
		t.Error("expected conn error")
	}
}

func TestHealth_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if err := c.Health(context.Background()); err == nil {
		t.Error("expected 503 error")
	}
}

func TestEnsureRunning_AlreadyHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := EnsureRunning(context.Background(), srv.URL, "/usr/bin/false", "", time.Second); err != nil {
		t.Errorf("already-healthy: %v", err)
	}
}

func TestEnsureRunning_BinaryMissing(t *testing.T) {
	if err := EnsureRunning(context.Background(), "http://127.0.0.1:1",
		"/no/such/binary/grove-xyz", "", 200*time.Millisecond); err == nil {
		t.Error("expected error")
	}
}

func TestImpactDefaultDepth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Default depth should be 3
		if !contains(string(body), `"maxDepth":3`) {
			http.Error(w, "wrong depth: "+string(body), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"nodes": []any{}})
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.Impact(context.Background(), "Foo", 0); err != nil {
		t.Errorf("default depth: %v", err)
	}
}

func TestSymbolsDefaultLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !contains(string(body), `"limit":50`) {
			http.Error(w, "wrong limit", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"symbols": []any{}})
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.Symbols(context.Background(), "q", 0); err != nil {
		t.Errorf("default limit: %v", err)
	}
}
