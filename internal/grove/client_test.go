package grove

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestClient_BearerTokenAttached covers the production-gap fix: fuse
// passing the laptop-mode token through to authenticated /deps and
// /impact endpoints. Without this, fuse check / fuse impact got a 401
// against a token-protected Grove daemon.
func TestClient_BearerTokenAttached(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"edges": []Edge{}})
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.Token = "tok-xyz"
	if _, err := c.Deps(context.Background(), "a.go"); err != nil {
		t.Fatal(err)
	}
	if want := "Bearer tok-xyz"; got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

// TestClient_NoTokenSendsNoAuth confirms we don't accidentally send an
// empty Bearer line — that breaks Grove's parser, which expects either
// no header or "Bearer <non-empty>".
func TestClient_NoTokenSendsNoAuth(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"edges": []Edge{}})
	}))
	defer srv.Close()
	c := New(srv.URL) // no token set
	if _, err := c.Deps(context.Background(), "a.go"); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}

// TestClient_WithTokenFromDir reads the token from .grove/.token in dir
// so the fuse CLI can pick up whatever the local Grove daemon wrote on
// startup, without the user having to plumb a flag.
func TestClient_WithTokenFromDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".grove"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".grove", ".token"), []byte("on-disk-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New("http://localhost:1").WithTokenFromDir(dir)
	if c.Token != "on-disk-token" {
		t.Errorf("Token = %q, want on-disk-token", c.Token)
	}
}

// TestClient_WithTokenFromDirMissing is a silent no-op when the token
// file doesn't exist — non-laptop Grove deployments don't require auth
// and shouldn't break.
func TestClient_WithTokenFromDirMissing(t *testing.T) {
	dir := t.TempDir()
	c := New("http://localhost:1").WithTokenFromDir(dir)
	if c.Token != "" {
		t.Errorf("Token = %q, want empty", c.Token)
	}
}

func TestClientHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientDeps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"edges": []Edge{{From: "a", To: "b", Type: "calls", Confidence: 0.9}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL)
	got, err := c.Deps(context.Background(), "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].From != "a" {
		t.Errorf("got %+v", got)
	}
}

func TestClientImpact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"nodes": []ImpactNode{{ID: "1", FilePath: "x.go", Name: "Foo"}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL)
	got, err := c.Impact(context.Background(), "Foo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Foo" {
		t.Errorf("got %+v", got)
	}
}
