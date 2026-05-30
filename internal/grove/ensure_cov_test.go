package grove

import (
	"context"
	"testing"
	"time"
)

func TestEnsureRunning_SpawnFails_NoBinary(t *testing.T) {
	err := EnsureRunning(context.Background(), "http://127.0.0.1:1", "/nonexistent/xxx/yyy", 50*time.Millisecond)
	if err == nil {
		t.Error("expected error")
	}
}

func TestEnsureRunning_SpawnTrueButNotHealthy(t *testing.T) {
	err := EnsureRunning(context.Background(), "http://127.0.0.1:1", "/bin/true", 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}
