// Package core contains shared data types used across Fuse subpackages.
package core

// LanguageKey identifies a supported language for the merge pipeline.
type LanguageKey string

const (
	LangGo         LanguageKey = "go"
	LangTypeScript LanguageKey = "typescript"
	LangTSX        LanguageKey = "tsx"
	LangJavaScript LanguageKey = "javascript"
	LangPython     LanguageKey = "python"
	LangJava       LanguageKey = "java"
	LangRust       LanguageKey = "rust"
	LangJSON       LanguageKey = "json"
	LangYAML       LanguageKey = "yaml"
	LangTOML       LanguageKey = "toml"
	LangUnknown    LanguageKey = ""
)

// LineRange is an inclusive 1-indexed source span.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SymbolData is a symbol extracted for merge purposes (in-memory parse of one
// version of a file).
type SymbolData struct {
	Key       string    `json:"key"`  // unique within file (e.g. "ClassName.methodName")
	Kind      string    `json:"kind"` // function | method | class | interface | type | const | struct | trait | enum
	Name      string    `json:"name"`
	Signature string    `json:"signature"`
	Body      string    `json:"body"` // full source text of the symbol
	Span      LineRange `json:"span"`
	ParentKey string    `json:"parentKey,omitempty"`
	Modifiers []string  `json:"modifiers,omitempty"`
	Exported  bool      `json:"exported"`
}

// ImportStatement is a parsed import line/clause.
type ImportStatement struct {
	Raw   string `json:"raw"`  // original source text
	Path  string `json:"path"` // import path / module / package
	Alias string `json:"alias,omitempty"`
	Group string `json:"group,omitempty"` // stdlib | external | relative (best-effort)
	Line  int    `json:"line"`
}

// ExportStatement represents a public/export declaration in the file.
type ExportStatement struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // function | class | type | const | default
}

// MergeCapabilities describes what kinds of merge a language strategy supports.
type MergeCapabilities struct {
	SupportsSymbolMerge bool
	SupportsImportMerge bool
	SupportsConfigMerge bool
}

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
	Kind          string           `json:"kind"` // removed_export | signature_changed | broken_import
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
