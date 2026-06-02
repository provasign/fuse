// Package core contains shared data types used across Fuse subpackages.
package core

import "github.com/provasign/astkit"

// LanguageKey is re-exported from astkit (shared with Grove).
type LanguageKey = astkit.LanguageKey

const (
	LangGo         = astkit.LangGo
	LangTypeScript = astkit.LangTypeScript
	LangTSX        = astkit.LangTSX
	LangJavaScript = astkit.LangJavaScript
	LangPython     = astkit.LangPython
	LangJava       = astkit.LangJava
	LangRust       = astkit.LangRust
	LangJSON       = astkit.LangJSON
	LangYAML       = astkit.LangYAML
	LangTOML       = astkit.LangTOML
	LangUnknown    = astkit.LangUnknown
)

// LineRange, SymbolData, ImportStatement are re-exported from astkit so that
// Grove and Fuse share a single source of truth for tree-sitter output.
type (
	LineRange       = astkit.LineRange
	SymbolData      = astkit.Symbol
	ImportStatement = astkit.ImportStatement
)

// ConflictType categorizes the nature of a merge conflict.
type ConflictType string

const (
	ConflictNone            ConflictType = "NONE"
	ConflictIncremental     ConflictType = "INCREMENTAL"
	ConflictStructural      ConflictType = "STRUCTURAL"
	ConflictArchitectural   ConflictType = "ARCHITECTURAL"
	ConflictConfigurational ConflictType = "CONFIGURATIONAL"
	ConflictComplex         ConflictType = "COMPLEX"
)

// ConflictSeverity is the assessed severity of a conflict.
type ConflictSeverity string

const (
	SeverityNone     ConflictSeverity = "NONE"
	SeverityLow      ConflictSeverity = "LOW"
	SeverityMedium   ConflictSeverity = "MEDIUM"
	SeverityHigh     ConflictSeverity = "HIGH"
	SeverityCritical ConflictSeverity = "CRITICAL"
)

// MergeStrategy is the selected resolution approach.
type MergeStrategy string

const (
	StrategySymbol  MergeStrategy = "symbol"
	StrategyImport  MergeStrategy = "import"
	StrategyConfig  MergeStrategy = "config"
	StrategyLine    MergeStrategy = "line"
	StrategyHandoff MergeStrategy = "handoff"
	StrategyClean   MergeStrategy = "clean" // no changes / identical
)

// BreakingChange is a detected breaking change during merge.
type BreakingChange struct {
	Kind          string           `json:"kind"`
	Symbol        string           `json:"symbol"`
	AffectedFiles []string         `json:"affectedFiles,omitempty"`
	Severity      ConflictSeverity `json:"severity"`
	Message       string           `json:"message"`
}

// MergeStats records counters for one merge.
type MergeStats struct {
	SymbolsBase   int   `json:"symbolsBase"`
	SymbolsOurs   int   `json:"symbolsOurs"`
	SymbolsTheirs int   `json:"symbolsTheirs"`
	AutoMerged    int   `json:"autoMerged"`
	Conflicted    int   `json:"conflicted"`
	TimingMs      int64 `json:"timingMs"`
}

// AuditEntry is one row in `.git/fuse/audit.json`.
type AuditEntry struct {
	Timestamp       string           `json:"timestamp"`
	File            string           `json:"file"`
	Language        LanguageKey      `json:"language"`
	Strategy        MergeStrategy    `json:"strategy"`
	ConflictType    ConflictType     `json:"conflictType"`
	Severity        ConflictSeverity `json:"severity"`
	Confidence      float64          `json:"confidence"`
	AutoMerged      bool             `json:"autoMerged"`
	BreakingChanges int              `json:"breakingChanges"`
	PromptFile      string           `json:"promptFile,omitempty"`
}

// MergeResult is the output of the IntelliMerge pipeline for one file.
type MergeResult struct {
	FilePath        string           `json:"filePath"`
	Language        LanguageKey      `json:"language"`
	MergedContent   string           `json:"mergedContent"`
	HasConflict     bool             `json:"hasConflict"`
	Confidence      float64          `json:"confidence"`
	ConflictType    ConflictType     `json:"conflictType"`
	Severity        ConflictSeverity `json:"severity"`
	Strategy        MergeStrategy    `json:"strategy"`
	BreakingChanges []BreakingChange `json:"breakingChanges,omitempty"`
	Conflicts       []SymbolConflict `json:"conflicts,omitempty"`
	PromptFile      string           `json:"promptFile,omitempty"`
	AuditEntry      AuditEntry       `json:"auditEntry"`
	Stats           MergeStats       `json:"stats"`
	Diagnostics     []string         `json:"diagnostics,omitempty"`
}

// SymbolConflict records a single symbol where ours and theirs both diverged
// from base.
type SymbolConflict struct {
	Key    string     `json:"key"`
	Base   SymbolData `json:"base"`
	Ours   SymbolData `json:"ours"`
	Theirs SymbolData `json:"theirs"`
}
