package merge

import (
	"context"
	"strings"
	"testing"

	"github.com/provasign/fuse/internal/core"
)

// TestMergeClassMethodReorderVsEdit drives the container-aware nested merge:
// ours reorders the methods of a TypeScript class, theirs edits one method's
// body. Line-level merging (git and LCS) is positional and conflicts on the
// move; merging the class's children by symbol key resolves it.
func TestMergeClassMethodReorderVsEdit(t *testing.T) {
	base := []byte(`class Service {
  alpha() {
    return 1;
  }
  beta() {
    return 2;
  }
}
`)
	ours := []byte(`class Service {
  beta() {
    return 2;
  }
  alpha() {
    return 1;
  }
}
`)
	theirs := []byte(`class Service {
  alpha() {
    return 1;
  }
  beta() {
    return 22;
  }
}
`)

	im := New(nil)
	im.EnableBreaking = false
	im.EnableContext = false
	res, err := im.Merge(context.Background(), base, ours, theirs, core.LangTypeScript, "service.ts")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict || res.Strategy == core.StrategyHandoff {
		t.Fatalf("reorder-vs-edit should auto-merge, got strategy=%s conflict=%v\n%s",
			res.Strategy, res.HasConflict, res.MergedContent)
	}
	merged := res.MergedContent
	if !strings.Contains(merged, "return 22;") {
		t.Fatalf("theirs' edit lost:\n%s", merged)
	}
	if strings.Contains(merged, "return 2;\n") && strings.Contains(merged, "return 22;") &&
		strings.Count(merged, "beta()") != 1 {
		t.Fatalf("beta duplicated or stale:\n%s", merged)
	}
	// Ours' ordering wins: beta before alpha.
	if strings.Index(merged, "beta()") > strings.Index(merged, "alpha()") {
		t.Fatalf("ours' method order not preserved:\n%s", merged)
	}
}

// TestMergeClassMethodAddVsEdit: theirs adds a method while ours edits an
// existing one in a way that collides positionally.
func TestMergeClassMethodAddVsEdit(t *testing.T) {
	base := []byte(`class Service {
  alpha() {
    return 1;
  }
}
`)
	ours := []byte(`class Service {
  alpha() {
    return 100;
  }
}
`)
	theirs := []byte(`class Service {
  alpha() {
    return 1;
  }
  gamma() {
    return 3;
  }
}
`)

	im := New(nil)
	im.EnableBreaking = false
	im.EnableContext = false
	res, err := im.Merge(context.Background(), base, ours, theirs, core.LangTypeScript, "service.ts")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasConflict {
		t.Fatalf("add-vs-edit should auto-merge:\n%s", res.MergedContent)
	}
	if !strings.Contains(res.MergedContent, "return 100;") || !strings.Contains(res.MergedContent, "gamma()") {
		t.Fatalf("merged content missing a side:\n%s", res.MergedContent)
	}
}
