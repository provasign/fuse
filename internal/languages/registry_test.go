package languages

import (
	"testing"

	"github.com/provasign/astkit"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry returned nil")
	}
	for _, lang := range []astkit.LanguageKey{"go", "typescript", "javascript", "java", "rust"} {
		if s := r.Get(lang); s == nil {
			t.Errorf("DefaultRegistry missing language %q", lang)
		}
	}
}
