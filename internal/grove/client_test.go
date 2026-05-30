package grove

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
