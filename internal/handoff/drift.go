package handoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/provasign/fuse/internal/grove"
)

// DriftRecord is one row in `.git/fuse/drift.json`: the structural code-graph
// delta one merge produced. Same posture as the audit log — advisory,
// local-first, append-only — and the evidence shape the stale-context loop
// consumes: intersect Drift with another agent's working set to tell it
// exactly which symbols shifted underneath it.
type DriftRecord struct {
	Timestamp string      `json:"timestamp"`
	File      string      `json:"file"`
	Strategy  string      `json:"strategy"`
	Drift     grove.Drift `json:"drift"`
}

// AppendDrift appends one record to drift.json under outputDir.
func AppendDrift(outputDir string, record DriftRecord) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if record.Timestamp == "" {
		record.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	driftPath := filepath.Join(outputDir, "drift.json")
	var existing []DriftRecord
	if data, err := os.ReadFile(driftPath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}
	existing = append(existing, record)
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(driftPath, out, 0o644)
}
