# Fuse — Implementation Plan

**Project:** Fuse (formerly: git-semantic)  
**CLI:** `fuse`  
**Role:** Intelligent semantic merge driver — understands code structure, not just text lines  
**Language:** Go 1.22+  
**Depends on:** Grove (graph engine for blast radius, breaking change detection, dependency context)  
**Status:** Pre-build — architecture validated, implementation not started  
**Last Updated:** May 26, 2026

---

## Overview

Fuse is an intelligent Git merge driver that operates at the symbol level (functions, classes, methods) rather than the line level. When AI agents generate massive code changes across dozens of files, traditional Git merge fails with 47+ conflicts. Fuse auto-merges 80%+ of those conflicts and generates AI-ready prompts for the rest — all locally, with zero external API calls.

**Relationship to git-semantic:** Fuse is git-semantic rebuilt on top of Grove. The dependency graph, blast radius analysis, and breaking change detection previously built inside git-semantic are now delegated to Grove. Fuse owns the merge pipeline, language strategies, conflict classification, and the merge algorithms themselves.

### What Fuse owns (not Grove)

| Component                 | Description                                                          |
|---------------------------|----------------------------------------------------------------------|
| Merge Pipeline            | 7-phase IntelliMerge orchestrator                                    |
| Language Strategies       | Symbol extraction for merge context (7 initial languages)            |
| Merge Algorithms          | 3-way merge, symbol merge, import merge, config merge, line merge    |
| Conflict Classification   | Categorize conflicts by type (incremental, structural, architectural) |
| Breaking Change Detection | Detect removed exports and signature changes (via Grove)             |
| AI Prompt Generation      | Local prompt files for unresolvable conflicts (zero API calls)       |
| Git Integration           | Register as a custom merge driver in `.gitconfig` / `.gitattributes` |

---

## Repository Layout

```
fuse/
├── cmd/
│   └── fuse/
│       └── main.go                  # Binary entry point
├── internal/
│   ├── config/
│   │   └── config.go                # fuse.yaml, env vars, defaults
│   ├── grove/
│   │   ├── client.go                # Grove HTTP/gRPC client
│   │   └── types.go                 # Grove data type mirrors
│   ├── parser/
│   │   ├── engine.go                # Tree-sitter parser (for merge, not indexing)
│   │   └── languages.go             # Language detection
│   ├── languages/
│   │   ├── strategy.go              # LanguageStrategy interface
│   │   ├── registry.go              # Strategy registry
│   │   └── strategies/
│   │       ├── go.go
│   │       ├── typescript.go
│   │       ├── javascript.go
│   │       ├── python.go
│   │       ├── java.go
│   │       ├── rust.go
│   │       └── data/                # JSON/YAML/TOML config strategies
│   │           ├── json.go
│   │           ├── yaml.go
│   │           └── toml.go
│   ├── merge/
│   │   ├── orchestrator.go          # IntelliMerge 7-phase pipeline
│   │   ├── context.go               # Project context (Grove-backed)
│   │   ├── classification/
│   │   │   ├── engine.go            # Conflict classification logic
│   │   │   ├── types.go             # ConflictType, Severity, Confidence
│   │   │   └── calibration.go       # Dynamic confidence scoring
│   │   ├── strategies/
│   │   │   ├── symbol.go            # Symbol-level three-way merge
│   │   │   ├── imports.go           # Import statement merge
│   │   │   ├── line.go              # Line-level fallback merge
│   │   │   ├── config.go            # Data format merge (JSON/YAML/TOML)
│   │   │   └── threeway.go          # Core three-way merge algorithm
│   │   └── analysis/
│   │       ├── breaking.go          # Breaking change detection (Grove-backed)
│   │       ├── risk.go              # Risk scoring
│   │       └── blast.go             # Blast radius (pure Grove delegation)
│   ├── handoff/
│   │   ├── prompt.go                # AI-ready local prompt generation
│   │   └── audit.go                 # Resolution audit log
│   └── cli/
│       └── commands.go              # cobra command tree
├── testdata/
│   └── fixtures/                    # Merge fixture files per language
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Data Models

```go
// LanguageKey identifies a supported language
type LanguageKey string
const (
    LangGo         LanguageKey = "go"
    LangTypeScript LanguageKey = "typescript"
    LangJavaScript LanguageKey = "javascript"
    LangPython     LanguageKey = "python"
    LangJava       LanguageKey = "java"
    LangRust       LanguageKey = "rust"
    LangJSON       LanguageKey = "json"
    LangYAML       LanguageKey = "yaml"
    LangTOML       LanguageKey = "toml"
)

// SymbolData is a symbol extracted for merge purposes
type SymbolData struct {
    Key        string       // unique key within file (e.g., "ClassName.methodName")
    Kind       string       // function | method | class | interface | type | const
    Name       string
    Signature  string
    Body       string       // full source text of the symbol
    Span       LineRange
    ParentKey  string       // for methods: parent class key
    Modifiers  []string
}

// MergeResult is the output of the IntelliMerge pipeline
type MergeResult struct {
    FilePath       string
    Language       LanguageKey
    MergedContent  string       // final merged file content (or empty if unresolvable)
    HasConflict    bool
    Confidence     float64      // 0.0–1.0
    ConflictType   ConflictType
    Severity       ConflictSeverity
    Strategy       MergeStrategy
    BreakingChanges []BreakingChange
    PromptFile     string       // path to AI handoff prompt (if HasConflict)
    AuditEntry     AuditEntry
    Stats          MergeStats
}

type MergeStats struct {
    SymbolsBase    int
    SymbolsOurs    int
    SymbolsTheirs  int
    AutoMerged     int
    Conflicted     int
    TokensOriginal int
    TimingMs       int64
}

// ConflictType categorizes the nature of a merge conflict
type ConflictType string
const (
    ConflictIncremental     ConflictType = "INCREMENTAL"     // small logic changes
    ConflictStructural      ConflictType = "STRUCTURAL"      // refactoring within paradigm
    ConflictArchitectural   ConflictType = "ARCHITECTURAL"   // framework/paradigm changes
    ConflictConfigurational ConflictType = "CONFIGURATIONAL" // config/env changes
    ConflictComplex         ConflictType = "COMPLEX"         // multi-dimensional, needs AI
)

// ConflictSeverity is the assessed severity of a conflict
type ConflictSeverity string
const (
    SeverityLow      ConflictSeverity = "LOW"
    SeverityMedium   ConflictSeverity = "MEDIUM"
    SeverityHigh     ConflictSeverity = "HIGH"
    SeverityCritical ConflictSeverity = "CRITICAL"
)

// MergeStrategy is the selected resolution approach
type MergeStrategy string
const (
    StrategySymbol   MergeStrategy = "symbol"    // symbol-level three-way merge
    StrategyImport   MergeStrategy = "import"    // import statement merge
    StrategyConfig   MergeStrategy = "config"    // deep merge for JSON/YAML/TOML
    StrategyLine     MergeStrategy = "line"      // line-level fallback
    StrategyHandoff  MergeStrategy = "handoff"   // unresolvable → AI prompt
)

// BreakingChange is a detected breaking change during merge
type BreakingChange struct {
    Kind        string   // "removed_export" | "signature_changed" | "broken_import"
    Symbol      string   // affected symbol name
    AffectedFiles []string
    Severity    ConflictSeverity
    Message     string   // human-readable warning
}
```

---

## Phase 1 — Grove Integration

**Goal:** Use Grove's graph data to provide deep merge context without rebuilding graph infrastructure.

### 1.1 Grove Client (`internal/grove/client.go`)

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

// Methods Fuse calls on Grove
func (c *Client) GetImpact(file string, line int) ([]grove.ImpactNode, error)
func (c *Client) GetDeps(filePath string) (*grove.DepsResult, error)
func (c *Client) SearchSymbols(name string) ([]grove.SymbolRecord, error)
func (c *Client) GetSymbolsInFile(filePath string) ([]grove.SymbolRecord, error)
func (c *Client) Index(dir string) error
```

Grove is used for:
1. **Blast radius** (`grove_impact`): before merging, determine which other files depend on changed symbols → informs severity scoring and breaking change warnings
2. **Dependency context** (`grove_deps`): enrich merge context with upstream dependencies → improves classification accuracy
3. **Symbol lookup** (`grove_symbols`): find symbols by name across the entire codebase → resolves broken import detection

### 1.2 Grove Startup Check

**Grove is required.** Fuse will not operate without a reachable Grove instance.

On startup (`fuse merge` or `fuse install`), Fuse checks `GROVE_URL/health`. If unreachable:
1. Attempt to auto-start: `grove serve --port 7777 &`
2. Wait up to 10 seconds for Grove to become healthy
3. If Grove still unreachable, Fuse exits with a fatal error:
   ```
   fuse: Grove is required but not reachable.
   Install grove and run 'grove init' first, or set GROVE_URL.
   ```

Cross-file blast radius and breaking change detection are correctness features of Fuse, not optional enrichments. A merge without them may silently break callers of changed symbols.

---

## Phase 2 — Parser Engine (for Merge)

**Goal:** Parse source files into symbol trees for the merge pipeline. This is a different use of Tree-sitter than Grove's indexing — Fuse needs to parse three versions (base, ours, theirs) of a file mid-merge.

**Note:** Fuse has its own Tree-sitter usage, separate from Grove, because it needs to parse in-memory strings (not files on disk) during the merge operation.

### 2.1 Language Detection (`internal/parser/languages.go`)

```go
func DetectLanguage(filePath, content string) LanguageKey
// Primary: file extension. Fallback: shebang line, content heuristics.
```

### 2.2 Per-Language Extractors

Fuse shares the same 7-language list as Grove but implements extraction differently — it extracts symbols from in-memory strings for three-way comparison.

Each extractor implements:

```go
type LanguageStrategy interface {
    Language()    LanguageKey
    Extensions()  []string
    Extract(tree *sitter.Tree, src []byte) ([]SymbolData, error)
    ExtractImports(tree *sitter.Tree, src []byte) ([]ImportStatement, error)
    ExtractExports(tree *sitter.Tree, src []byte) ([]ExportStatement, error)
    MergeCapabilities() MergeCapabilities
}

type MergeCapabilities struct {
    SupportsSymbolMerge bool
    SupportsImportMerge bool
    SupportsConfigMerge bool // JSON/YAML/TOML only
}
```

#### Go
Extract: functions (including methods with receivers), type declarations, const blocks, interface declarations. Imports: `import (...)` blocks. Exports: exported (uppercase) symbols.

#### TypeScript / JavaScript
Extract: function declarations, arrow function assignments, class declarations, class methods, interface declarations (TS), type aliases (TS), enum declarations (TS). Imports: `import ... from`, `require(...)`. Exports: `export`, `export default`, `module.exports`.

#### Python
Extract: `def` and `async def` functions, `class` declarations, class methods. Imports: `import X`, `from X import Y`. Exports: module-level public names (no leading underscore).

#### Java
Extract: class, interface, enum, record declarations; method declarations; field declarations. Imports: `import` statements. Exports: `public` declarations.

#### Rust
Extract: `fn` declarations, `impl` blocks (methods), `struct`, `enum`, `trait`, `type` aliases. Imports: `use` statements. Exports: `pub` items.

#### JSON / YAML / TOML (config strategies)
No AST symbol extraction needed. These use deep structural merge algorithms (see Phase 4.4).

---

## Phase 3 — IntelliMerge Pipeline (7 Phases)

**Goal:** Coordinate all merge components for a single file merge.

### 3.1 Orchestrator (`internal/merge/orchestrator.go`)

```go
type IntelliMerge struct {
    parser   *parser.Engine
    registry *languages.Registry
    grove    *grove.Client  // required — startup check (Phase 1.2) guarantees non-nil
    config   *config.Config
}

func (im *IntelliMerge) Merge(
    baseContent, oursContent, theirsContent []byte,
    language LanguageKey,
    filePath string,
    projectRoot string,
) (*MergeResult, error)
```

### 3.2 The 7 Phases

#### Phase 1: Context Building
1. Call Grove `grove_deps` for the file → get dependencies and dependents
2. Call Grove `grove_impact` for changed symbols in `ours` and `theirs` → blast radius
3. Extract git history for recency signals (last 10 commits for this file)
4. Grove is reachable at this point (startup check in Phase 1.2 guarantees it)

#### Phase 2: Symbol Extraction
1. Parse `base`, `ours`, `theirs` using language strategy
2. Build symbol maps: `map[symbolKey]SymbolData` for each version
3. Identify: added symbols (in ours/theirs not in base), deleted symbols, modified symbols
4. Identify: non-overlapping modifications (only in ours OR only in theirs, not both)

#### Phase 3: Recency Analysis
1. Analyze which symbols were most recently modified (from git log)
2. Weight recency: recently-changed symbols → their version preferred in ties
3. Flag symbols with high edit frequency → increase conflict classification severity

#### Phase 4: Project Graph Context (via Grove)
1. Identify architectural patterns from Grove's symbol data (service, controller, repository patterns)
2. Detect cross-file dependencies that the merge might break
3. Calculate complexity metrics: symbol churn, import churn

#### Phase 4.5: Breaking Change Detection
1. Query Grove for all files that import the changed file (`grove_deps` reverse direction)
2. Check: are any exported symbols in `base` missing from `ours` or `theirs`? → `removed_export`
3. Check: are any exported signatures changed in a backward-incompatible way? → `signature_changed`
4. Check: do any symbols in `theirs` import something that `ours` removed? → `broken_import`
5. Severity: affected file count → Critical (6+), High (3–5), Medium (1–2), Low (0)
6. Emit `BreakingChange` entries regardless of whether merge succeeds

#### Phase 5: Conflict Classification
Classify the conflict:

| Type                 | Indicators                                                              |
|----------------------|-------------------------------------------------------------------------|
| `INCREMENTAL`        | Small additions/modifications to function bodies; < 5 symbols changed  |
| `STRUCTURAL`         | Renaming, signature changes, file reorganization                        |
| `ARCHITECTURAL`      | Framework changes, paradigm shift (sync→async, OOP→functional)         |
| `CONFIGURATIONAL`    | JSON/YAML/TOML files, config constants, env var changes                 |
| `COMPLEX`            | All the above mixed; > 10 symbols changed; cross-domain impact         |

Severity assessment:
```
if ConflictType == ARCHITECTURAL or breaking changes (HIGH/CRITICAL): severity = HIGH/CRITICAL
if overlapping modifications > 3 symbols: severity = HIGH
if overlapping modifications 1–3 symbols: severity = MEDIUM
if no overlapping modifications: severity = LOW
```

#### Phase 6: Strategy Selection & Merge Application

Strategy selection matrix:

| Language  | Non-overlapping | Import conflict | Overlapping | Config file |
|-----------|----------------|-----------------|-------------|-------------|
| TS/JS/Go/  | StrategySymbol | StrategyImport  | StrategyLine | N/A        |
| Python/Java/Rust | StrategySymbol | StrategyImport | StrategyLine | N/A   |
| JSON/YAML/TOML   | N/A       | N/A             | N/A         | StrategyConfig |

Apply strategy:
- `StrategySymbol`: three-way merge at symbol granularity (see Phase 4.1)
- `StrategyImport`: union of import sets, deduplicated, ordered
- `StrategyConfig`: deep recursive merge of data structures
- `StrategyLine`: line-level three-way merge (git-compatible fallback)
- `StrategyHandoff`: emit AI prompt file; return conflict markers in output

Confidence thresholds:
- `StrategySymbol` (non-overlapping): 85%
- `StrategyImport`: 90%
- `StrategyConfig`: 80%
- `StrategyLine`: 60–70%
- `StrategyHandoff`: < 30% (unresolvable)

#### Phase 7: Diagnostics Generation
1. Generate human-readable explanation of merge decision
2. Emit `BreakingChange` warnings to stderr
3. If `StrategyHandoff`: write AI prompt file to `.git/fuse/conflict-<hash>.md`
4. Append to audit log `.git/fuse/audit.json`
5. Return `MergeResult` with full metadata

---

## Phase 4 — Merge Algorithms

### 4.1 Symbol-Level Three-Way Merge (`internal/merge/strategies/symbol.go`)

```go
func SymbolMerge(
    base, ours, theirs map[string]SymbolData,
) (merged map[string]SymbolData, conflicts []SymbolConflict)
```

Algorithm:
```
for each symbolKey in union(base.keys, ours.keys, theirs.keys):
  baseSymbol  = base[key]    // nil if not in base
  oursSymbol  = ours[key]    // nil if not in ours
  theirsSymbol = theirs[key] // nil if not in theirs

  case:
    only in ours (added by us)      → include ours (confidence: 0.95)
    only in theirs (added by them)  → include theirs (confidence: 0.95)
    in base, missing from ours      → deleted by us (check blast radius first)
    in base, missing from theirs    → deleted by them (check blast radius first)
    all same (ours == theirs)       → include (confidence: 1.0)
    base == ours (only they changed)→ include theirs (confidence: 0.95)
    base == theirs (only we changed)→ include ours (confidence: 0.95)
    all three differ                → CONFLICT (confidence: 0.45)
```

For symbol deletion: before accepting, call Grove `grove_impact(symbol)` to check blast radius. If > 3 dependents, emit HIGH severity breaking change warning.

### 4.2 Import Statement Merge (`internal/merge/strategies/imports.go`)

```go
func ImportMerge(base, ours, theirs []ImportStatement) (merged []ImportStatement, confidence float64)
```

Algorithm:
1. Union of all imports from `ours` and `theirs`
2. Deduplicate by import path (same path from both sides = include once)
3. Preserve original grouping style (relative vs module vs stdlib) per language conventions
4. Preserve `ours` ordering for imports present in both
5. Add `theirs`-only imports at end of appropriate group
6. Confidence: 90% (import merges rarely have true semantic conflicts)

### 4.3 Line-Level Fallback Merge (`internal/merge/strategies/line.go`)

Fallback for code that Tree-sitter cannot parse or symbol extraction fails:
1. Split base/ours/theirs into lines
2. Compute diff(base, ours) and diff(base, theirs) as line hunks
3. For each hunk: if only one side changes → auto-merge (confidence: 65%)
4. For overlapping hunks: emit standard Git conflict markers (`<<<<<<`, `=======`, `>>>>>>>`), confidence: 0
5. Returns merged content with embedded conflict markers for overlapping hunks

### 4.4 Config Deep Merge (`internal/merge/strategies/config.go`)

For JSON, YAML, TOML files:
1. Parse all three versions into structured data (`map[string]interface{}`)
2. For each key in union(ours.keys, theirs.keys):
   - Only one side changed from base → use that side (confidence: 80%)
   - Both sides changed to same value → use the value (confidence: 100%)
   - Both sides changed to different values → CONFLICT (confidence: 0, emit conflict marker in output as commented block)
3. Re-serialize to original format preserving style (comments in YAML/TOML preserved where possible)

### 4.5 Core Three-Way Diff (`internal/merge/strategies/threeway.go`)

Shared utility used by symbol merge and line merge:

```go
type MergeAction string
const (
    ActionKeep      MergeAction = "keep"       // unchanged
    ActionUseOurs   MergeAction = "use-ours"   // take our version
    ActionUseTheirs MergeAction = "use-theirs" // take their version
    ActionAppend    MergeAction = "append"     // both add something, take both
    ActionDelete    MergeAction = "delete"     // both delete
    ActionConflict  MergeAction = "conflict"   // true conflict
)

func ThreeWayDiff(base, ours, theirs string) MergeAction
```

---

## Phase 5 — Conflict Classification Engine

### 5.1 Classification (`internal/merge/classification/engine.go`)

```go
type ClassificationEngine struct {
    grove *grove.Client
}

func (ce *ClassificationEngine) Classify(
    base, ours, theirs map[string]SymbolData,
    imports ImportChange,
    groveContext *GroveContext,
    gitHistory GitHistory,
) ClassificationResult
```

**5 Classification Factors:**

1. **Symbol change patterns** — count additions, modifications, deletions in ours vs theirs
2. **Architectural pattern detection** — identify migrations (e.g., sync→async rewrite), framework changes (e.g., Express→Fastify)
3. **Change complexity metrics** — `symbolChurn = (added + deleted) / totalBase`, `importChurn`
4. **Grove graph structure** — number of dependents affected, cross-domain reach
5. **Historical outcomes** — if the same file has had many past conflicts, increase severity estimate

**Confidence Modes:**

- **Static** (default): predefined confidence per conflict type (symbol=85%, import=90%, config=80%)
- **Dynamic** (opt-in via config): log-odds additive model adjusting confidence based on:
  1. Change magnitude factor (0.5–1.0)
  2. AST complexity factor (0.5–1.0)
  3. Conflict surface factor (0.3–1.0)
  4. Semantic similarity of conflicting bodies (0.6–1.0)
  5. Symbol stability factor (private +0.5, exported −0.8 adjustment)

---

## Phase 6 — AI Handoff Prompt Generation

**Goal:** When a conflict cannot be resolved automatically, generate a local AI-ready prompt. Zero external API calls.

### 6.1 Prompt Generator (`internal/handoff/prompt.go`)

Written to `.git/fuse/conflict-<sha>.md` and `.git/fuse/conflict-<sha>.json`.

**Markdown prompt structure:**

```markdown
# Fuse: Unresolvable Merge Conflict

## Summary
- **File:** src/utils/calculator.py
- **Conflict Type:** STRUCTURAL
- **Severity:** HIGH
- **Confidence:** 32%
- **Symbols in conflict:** `calculate`, `validateInput`

## Breaking Changes Detected
⚠️ Export `calculate` signature changed — may break 3 file(s):
   - src/billing/invoice.py (line 45)
   - src/reporting/summary.py (line 12)

## Three-Way Comparison

### BASE (common ancestor)
```<language>
<base symbol body>
```

### OURS (HEAD)
```<language>
<ours symbol body>
```

### THEIRS (incoming branch)
```<language>
<theirs symbol body>
```

## Context (from Grove)
**Dependencies of this file:**
- src/models/types.py → defines `CalculationInput`

**Files that depend on this file:**
- src/billing/invoice.py (calls `calculate`)
- src/reporting/summary.py (calls `calculate`)

## Resolution Task
Produce a merged version of `<file>` that:
1. Resolves the conflict between OURS and THEIRS
2. Preserves backward compatibility for `calculate` (used by 3 files)
3. Passes all existing tests for `calculate`

## Output Format
Return ONLY the complete merged file content, no explanation.
```

### 6.2 Audit Log (`internal/handoff/audit.go`)

Appends to `.git/fuse/audit.json` after each merge:

```json
{
  "timestamp": "ISO8601",
  "file": "src/...",
  "language": "python",
  "strategy": "handoff",
  "conflict_type": "STRUCTURAL",
  "severity": "HIGH",
  "confidence": 0.32,
  "auto_merged": false,
  "breaking_changes": 1,
  "prompt_file": ".git/fuse/conflict-abc123.md"
}
```

---

## Phase 7 — Git Integration

### 7.1 Register as Git Merge Driver

Fuse installs itself as a custom Git merge driver:

```bash
fuse install
```

This writes to `~/.gitconfig`:
```
[merge "fuse"]
    name = Fuse semantic merge driver
    driver = fuse merge %O %A %B %P
```

And to the repo's `.gitattributes`:
```
*.go    merge=fuse
*.ts    merge=fuse
*.tsx   merge=fuse
*.js    merge=fuse
*.jsx   merge=fuse
*.mjs   merge=fuse
*.py    merge=fuse
*.java  merge=fuse
*.rs    merge=fuse
*.json  merge=fuse
*.yaml  merge=fuse
*.yml   merge=fuse
*.toml  merge=fuse
```

### 7.2 Merge Driver Interface

Git calls Fuse with:
```
fuse merge <base-file> <ours-file> <theirs-file> <path>
```

Fuse writes the merged result in-place to `<ours-file>` (Git convention) and exits:
- Exit 0: merge succeeded (no conflicts)
- Exit 1: conflicts remain (conflict markers written to file, AI prompt generated)

---

## Phase 8 — CLI

### Commands

```
fuse install                       Register as Git merge driver in ~/.gitconfig
fuse uninstall                     Remove Git merge driver registration

fuse merge <base> <ours> <theirs>  Perform a semantic merge (3 file paths)
fuse preview <file>                Preview merge without writing output
fuse status                        Show merge driver status and last audit stats
fuse resolve <conflict-file>       Show AI prompt for a pending conflict

fuse check <file>                  Run breaking change detection on current file
fuse impact <file>                 Show blast radius via Grove
fuse deps <file>                   Show dependencies via Grove

fuse cache build                   Pre-build dependency cache
fuse cache clear                   Clear dependency cache

fuse serve --port 9999             Start HTTP API for programmatic usage

fuse config                        Show resolved configuration
```

---

## Phase 9 — Testing Strategy

### Unit Tests

- Per-language symbol extraction: parse fixture files → assert extracted symbols
- Three-way merge algorithm: known base/ours/theirs → assert merge action
- Symbol merge: various combination cases → assert merged output and confidence
- Import merge: deduplicated union with ordering → assert output
- Config merge: JSON/YAML/TOML fixtures with conflicts → assert output
- Classification engine: known symbol change patterns → assert conflict type and severity
- Breaking change detection: fixture with removed exports → assert breaking change entries

### Integration Tests (against `testdata/fixtures/`)

One fixture per scenario per language:
- `non_overlapping_change` — both sides change different functions → expect auto-merge
- `import_conflict` — both add different imports → expect import merge
- `symbol_deletion_with_dependents` — one side deletes exported symbol → expect HIGH breaking change
- `architectural_rewrite` — async→sync migration → expect ARCHITECTURAL + COMPLEX
- `config_deep_merge` — nested JSON both sides change different keys → expect config merge
- `true_conflict` — both sides change same function differently → expect handoff prompt

### Performance Benchmarks

| Benchmark                          | Target  |
|------------------------------------|---------|
| Parse 2000-line file               | < 50ms  |
| Symbol extraction (500 LOC)        | < 20ms  |
| Full 7-phase merge pipeline        | < 200ms |
| Grove blast radius call            | < 100ms |
| Config deep merge (1000-key YAML)  | < 30ms  |
| Memory peak during merge           | < 100MB |

---

## Configuration (`fuse.yaml`)

```yaml
version: 1
grove_url: "http://localhost:7777"   # Grove server URL (optional; graceful degradation if absent)
grove_binary: "grove"

merge:
  confidence_mode: "static"          # static | dynamic
  handoff_threshold: 0.30            # confidence below this → AI handoff prompt
  enable_breaking_change: true       # detect breaking changes via Grove
  enable_context: true               # use Grove for cross-file context

git:
  auto_install: false                # auto-register merge driver on grove init
  attributes_scope: "repo"          # repo | global

server:
  port: 9999
```

---

## Phased Delivery Schedule

| Phase | Deliverable                                              | Depends on      |
|-------|----------------------------------------------------------|-----------------|
| 1     | Grove client + graceful degradation                      | Grove ≥ Phase 6 |
| 2     | Parser engine for 7 languages (in-memory parse)          | —               |
| 3     | Language strategies (symbol + import extraction)         | Phase 2         |
| 4     | Three-way merge algorithms (symbol, import, config, line)| Phase 3         |
| 5     | 7-phase IntelliMerge pipeline                            | Phase 1, 3, 4   |
| 6     | Breaking change detection (via Grove)                    | Phase 1, 5      |
| 7     | Conflict classification engine                           | Phase 5         |
| 8     | AI handoff prompt generation                             | Phase 5, 7      |
| 9     | Git integration (install/uninstall, merge driver)        | Phase 5         |
| 10    | CLI (all commands)                                       | Phase 9         |
| 11    | HTTP API                                                 | Phase 5         |
| 12    | Tests + benchmarks                                       | All phases      |

---

## Key Design Constraints (Non-Negotiable)

1. **Zero external API calls** — all processing is local. AI handoff prompts are written to files; Fuse never calls OpenAI, Anthropic, or any external service.
2. **Grove is required** — Fuse will not start without a reachable Grove instance. Startup auto-start logic (same pattern as Prism) attempts to launch the `grove` binary if the configured URL is unreachable. Cross-file blast radius and breaking change detection are correctness features, not optional enrichments.
3. **Tree-sitter in Fuse is for merge, not indexing** — Fuse uses Tree-sitter to parse in-memory content during merge. Grove handles all persistent indexing.
4. **Confidence-first design** — every merge decision has a confidence score. Below `handoff_threshold` (default 0.30), always generate an AI handoff prompt rather than producing a potentially wrong merge.
5. **Git driver contract** — Fuse exits 0 (clean merge) or 1 (conflict markers in file). Git expects this contract precisely.
6. **Symbol key stability** — the symbol key used for three-way comparison must be stable across renames within the same version (keyed on qualified name, not line number).
