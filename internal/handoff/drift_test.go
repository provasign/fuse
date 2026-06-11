package handoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/provasign/fuse/internal/grove"
)

func TestAppendDrift(t *testing.T) {
	dir := t.TempDir()
	rec := DriftRecord{
		File:     "auth.go",
		Strategy: "symbol",
		Drift: grove.Drift{
			Changed: []grove.DriftSymbol{{
				FilePath:      "auth.go",
				QualifiedName: "Login",
				Kind:          "function",
				Change:        "signature",
				Exported:      true,
				OldSignature:  "func Login(user string) error",
				NewSignature:  "func Login(user, password string) error",
			}},
			Breaking: []grove.DriftSymbol{{
				FilePath: "auth.go", QualifiedName: "Login", Kind: "function",
				Change: "signature", Exported: true,
			}},
		},
	}
	if err := AppendDrift(dir, rec); err != nil {
		t.Fatal(err)
	}
	// Appending again grows the array.
	rec.File = "billing.go"
	if err := AppendDrift(dir, rec); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "drift.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []DriftRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}
	if got[0].Timestamp == "" || got[1].File != "billing.go" {
		t.Fatalf("unexpected records: %+v", got)
	}
	if len(got[0].Drift.Changed) != 1 || got[0].Drift.Changed[0].QualifiedName != "Login" {
		t.Fatalf("drift payload lost: %+v", got[0].Drift)
	}
}
