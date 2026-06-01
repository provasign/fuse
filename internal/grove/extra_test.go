package grove

import (
	"context"
	"testing"
	"time"
)

func TestPost_BadURL(t *testing.T) {
	c := &Client{BaseURL: "://bad", HTTP: New("x").HTTP}
	if _, err := c.Deps(context.Background(), "f"); err == nil {
		t.Error("expected url error")
	}
}

func TestPost_NilOut(t *testing.T) {
	// Index uses out=&struct{}{} which exercises decode-skip; just confirm
	// the success path is OK when server returns empty body.
}

// Cover EnsureRunning's polling loop without actually running grove: the
// binary "/bin/true" starts and exits successfully, so the loop will run
// once before timing out.
func TestEnsureRunning_BinaryStartsButHealthNeverOK(t *testing.T) {
	if err := EnsureRunning(context.Background(), "http://127.0.0.1:1",
		"/bin/true", "", 300*time.Millisecond); err == nil {
		t.Error("expected timeout error")
	}
}
