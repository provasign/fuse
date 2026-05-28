# git-semantic: Complete Implementation & Migration Guide

**Purpose**: Unified guide for understanding, recreating, and migrating git-semantic
**Version**: 1.0.0 (Production-Ready) **Last Updated**: May 22, 2026 **Audience**: AI systems, developers (TypeScript & Go)

-----

## Document Navigation

This guide is organized into three major parts:

1. **Part 1: Project Overview & Quick Start** - Start here to understand what git-semantic is
1. **Part 2: TypeScript Implementation Guide** - Complete specification for TypeScript implementation
1. **Part 3: Go Conversion Guide** - Convert TypeScript implementation to Go

**Reading Paths**:

- **For TypeScript Recreation**: Read Part 1 (30 min) → Part 2 (2-3 hours) → Start coding
- **For Go Conversion**: Read Part 1 (30 min) → Part 2 (1 hour, skim) → Part 3 (1-2 hours) → Start coding
- **For Understanding**: Read Part 1 only (30 min)

-----

# Part 1: Project Overview & Quick Start

## What is git-semantic?

git-semantic is an **intelligent semantic merge driver** that understands code structure rather than just text lines. It solves the critical problem of AI-generated code merges where traditional line-based Git merging fails spectacularly.

### Core Value Proposition

When AI agents (GitHub Copilot, Claude, GPT-4) generate massive code changes across dozens of files, traditional Git merge creates 47+ conflicts across 18 files. git-semantic uses:

- **Structural analysis** - Tree-sitter AST parsing - **Intelligent conflict classification**

…to auto-merge 80%+ of these conflicts and generate AI-ready prompts for the rest.

### Key Statistics

- **133 TypeScript source files** (~25,446 lines of code)
- **26 languages supported** with full strategy-based extraction
- **897 test cases** covering all merge scenarios
- **v1.0.0 Production-Ready** release
- **Zero external API calls** - all processing local, privacy-first

-----

## Visual Architecture

```
CLI Layer (index.ts)
merge | install | status | preview | resolve
                    |
                    ▼
        IntelliMerge Orchestrator
  7 Phases: Context → Symbols → Recency → Graph →
  Breaking Changes → Classification → Strategy
        |           |           |
        ▼           ▼           ▼
   Parser      Language      Merge
  (Tree-       Strategy      Strategy
   sitter)    (26 lang)     (5 types)
```

-----

## Quick Start by Use Case

### TypeScript Recreation

```bash
# 1. Read Part 2 of this guide (2-3 hours)
# 2. Start Phase 1: Foundation
mkdir git-semantic && cd git-semantic
npm init -y
npm install web-tree-sitter commander ora diff js-yaml @ltd/j-toml
# 3. Follow implementation plan in Part 2
```

### Go Conversion

```bash
# 1. Read Part 2 (1 hour, skim) and Part 3 (1-2 hours)
# 2. Start Phase 1: Foundation
mkdir git-semantic && cd git-semantic
go mod init github.com/your-org/git-semantic
go get github.com/smacker/go-tree-sitter
go get github.com/spf13/cobra
# 3. Follow conversion plan in Part 3
```

-----

## Critical Components to Understand

### 1. Tree-sitter Integration (CRITICAL)

- **What**: WASM-based parser for 26 languages
- **Why**: Enables structural (AST) analysis vs line-based
- **TypeScript**: Uses `web-tree-sitter` (WASM)
- **Go**: Uses `go-tree-sitter` (CGO bindings)

### 2. Three-Way Merge Algorithm

- **What**: Core merge logic using symbol-level diffs
- **Why**: Enables non-overlapping changes to auto-merge
- **Details**: See Part 2, “Key Algorithms” section

### 3. Symbol Extraction

- **What**: Extract functions, classes, methods from AST
- **Why**: Enables symbol-level merging
- **Details**: See Part 2, “Symbol Extraction Engine” section

### 4. IntelliMerge Orchestrator

- **What**: 7-phase merge pipeline
- **Why**: Coordinates all components
- **Details**: See Part 2, “IntelliMerge Orchestrator” section

### 5. Language Strategy Pattern

- **What**: Strategy pattern for 26 languages
- **Why**: Enables extensible language support
- **Details**: See Part 2, “Language Strategy System” section

-----

## Technology Stack Reference

### TypeScript Stack

```
Runtime:        Node.js 18+
Language:       TypeScript 5.3+ (strict mode)
Parser:         web-tree-sitter 0.22.6 (WASM)
CLI:            commander 11.1.0
Testing:        jest 29.7.0
JSON/YAML/TOML: js-yaml, @ltd/j-toml
Diff:           diff 5.2.0
Progress:       ora 5.4.1
```

### Go Stack (Recommended for Production)

```
Language:       Go 1.21+
Parser:         go-tree-sitter (CGO)
CLI:            cobra 1.8.0
Testing:        testing (standard library)
JSON/YAML/TOML: yaml.v3, toml
Diff:           go-diff
Progress:       spinner
```

-----

## What Makes git-semantic Special

1. **Symbol-Level Merging**: Not line-based like traditional Git
1. **Project-Aware**: Uses dependency graphs and context
1. **Graduated Resolution**: Auto-merge high confidence → Generate AI prompts for low
1. **Privacy-First**: No external API calls ever
1. **26 Languages**: Comprehensive language support
1. **Breaking Change Detection**: Warns about removed exports, signature changes

### Common Misconceptions

❌ “It’s just a fancy diff tool” ✅ It’s a structural merge system with AST analysis

❌ “It calls OpenAI/Claude APIs” ✅ It generates local prompts, never calls external APIs

❌ “It auto-merges everything” ✅ It auto-merges high-confidence changes, generates prompts for complex cases

❌ “It only works for TypeScript/JavaScript” ✅ It supports 26 languages with full strategy-based extraction

-----

## Success Criteria

### Functional Metrics

- ✅ All 26 languages parse correctly
- ✅ Symbol extraction works for all full-support languages
- ✅ Non-overlapping changes auto-merge with 85%+ confidence
- ✅ Breaking changes detected and warnings shown
- ✅ AI prompts generated for low-confidence conflicts
- ✅ Dependency cache provides O(1) lookups

### Performance Metrics

- ✅ Small files (< 100 LOC): < 50ms total time
- ✅ Large files (500-2000 LOC): < 200ms total time
- ✅ Memory usage: < 200MB peak (TypeScript), < 100MB (Go)
- ✅ Cache build: < 30 seconds for 1,000 files

### Quality Metrics

- ✅ 897 tests passing
- ✅ No false positives (auto-merge when conflict exists)
- ✅ No false negatives (conflict when auto-merge possible)
- ✅ Zero crashes on malformed input
- ✅ Graceful degradation on parse failures

-----

# Part 2: TypeScript Implementation Guide

## Architecture Overview

### System Design Philosophy

1. **Semantic Understanding**: Merge at symbol level (functions, classes, methods) not line level
1. **Project-Aware Context**: Consider dependencies, dependents, architectural patterns
1. **Graduated Resolution**: Auto-merge high-confidence → Generate AI prompts for low-confidence
1. **Privacy-First**: All processing local, no external API calls ever
1. **Transparency**: Clear confidence scores and reasoning for all decisions

### Core Architecture (3-Tier)

```
CLI Layer (index.ts)
Commands: merge, install, status, preview, resolve, cache
                        |
                        ▼
        IntelliMerge Orchestrator Layer
  ┌─────────────┬────────────────┬──────────────────┐
  | Context     | Classification | Breaking Change  |
  | Engine      | Engine         | Detector         |
  ├─────────────┼────────────────┼──────────────────┤
  | Project     | Strategy       | AI Agent         |
  | Graph       | Selector       | Handoff          |
  └─────────────┴────────────────┴──────────────────┘
                        |
                        ▼
              Foundation Layer
  ┌──────────────┬──────────────┬──────────────────┐
  | Tree-sitter  | Symbol       | Language         |
  | Parser       | Extractor    | Strategies (26)  |
  | (WASM)       |              |                  |
  └──────────────┴──────────────┴──────────────────┘
```

-----

## Complete Directory Structure

```
git-semantic/
├── src/                               # 133 TypeScript source files
│   ├── index.ts                       # CLI entry point (592 lines)
│   ├── config.ts                      # Configuration management
│   ├── constants.ts                   # Global constants
│   ├── parser/
│   │   └── index.ts                   # Parser manager with timeout protection
│   ├── languages/                     # Language-specific logic
│   │   ├── core/
│   │   │   ├── LanguageRegistry.ts    # Central language registry
│   │   │   ├── LanguageStrategy.ts    # Base strategy interface
│   │   │   └── index.ts               # Public API
│   │   └── strategies/                # 26 language strategies
│   │       ├── JavaScriptStrategy.ts  # Full symbol extraction
│   │       ├── TypeScriptStrategy.ts  # Full symbol extraction
│   │       ├── PythonStrategy.ts      # Full symbol extraction
│   │       ├── JavaStrategy.ts        # Full symbol extraction
│   │       ├── GoStrategy.ts          # Full symbol extraction
│   │       ├── RustStrategy.ts        # Full symbol extraction
│   │       └── [20 more strategies]
│   ├── merge/                         # Core merge logic
│   │   ├── core/
│   │   │   ├── intelliMerge.ts        # Main orchestrator (7 phases)
│   │   │   ├── context.ts             # Project context builder
│   │   │   ├── dependencyCache.ts     # O(1) dependency lookups
│   │   │   └── diagnostics.ts         # Human-readable explanations
│   │   ├── classification/
│   │   │   ├── classificationEngine.ts # Conflict categorization
│   │   │   ├── classificationTypes.ts  # Type definitions
│   │   │   ├── classification.ts       # Main classification logic
│   │   │   └── confidenceCalibration.ts # Dynamic confidence scoring
│   │   ├── strategies/                # Merge strategies
│   │   │   ├── mergeStrategies.ts     # Strategy orchestrator
│   │   │   ├── symbolMerge.ts         # Symbol-level merging
│   │   │   ├── importMerge.ts         # Import statement merging
│   │   │   ├── lineMerge.ts           # Line-level fallback
│   │   │   ├── dataFormatMerge.ts     # JSON/YAML/TOML deep merge
│   │   │   ├── overlappingMerge.ts    # Overlapping change handling
│   │   │   └── threeWay.ts            # Three-way merge algorithm
│   │   └── analysis/
│   │       ├── breakingChangeDetector.ts # Detect removed exports, signature changes
│   │       ├── riskAssessment.ts      # Risk scoring
│   │       ├── enhancedDiagnostics.ts # Detailed diagnostics
│   │       ├── contextAwarePrompts.ts # Context-rich AI prompts
│   │       ├── blastRadiusAnalyzer.ts # Impact analysis
│   │       └── index.ts               # Public API
│   ├── graph/
│   │   └── projectGraphBuilder.ts     # Dependency graph construction
│   ├── agent-handoff/                 # AI prompt generation
│   │   ├── promptGenerator.ts         # Generate AI-ready prompts
│   │   ├── terminalOutput.ts          # Terminal formatting
│   │   └── index.ts                   # Public API
│   └── cli/
│       ├── commands/
│       ├── gitHelpers.ts              # Git operations
│       └── languageDetection.ts       # Language detection
├── __tests__/                         # 897 test cases
├── grammars/                          # Tree-sitter WASM grammars (26 languages)
├── scripts/                           # Build and validation
├── docs/                              # Comprehensive documentation
├── package.json                       # NPM configuration
├── tsconfig.json                      # TypeScript configuration
└── jest.config.js                     # Jest test configuration
```

-----

## Core Components: Deep Dive

### 1. Parser Module (`src/parser/index.ts`)

**Purpose**: Manage Tree-sitter WASM parsers for 26 languages

**Key Features**: WASM-first approach (cross-platform, sandboxed) - Lazy grammar loading with caching - Parse timeout protection (500ms default) - Memory limits (64MB default) - Magic number validation (`\0asm`)

**Core Interface**:

```typescript
interface ParsedTree {
  language: LanguageKey;
  languageImpl: TreeSitter.Language;
  tree: TreeSitter.Tree;
  rootNode: TreeSitter.Tree['rootNode'];
}

async function parseCode(
  code: string,
  language: LanguageKey,
  timeoutMs?: number
): Promise<ParsedTree | null>
```

**Implementation Notes**: Tree-sitter runtime initialization on first use - Grammar files stored in `grammars/` directory - Each language has dedicated WASM binary - Validate WASM magic number before loading - Graceful degradation on parse failures

-----

### 2. Language Strategy System (`src/languages/`)

**Purpose**: Language-specific symbol extraction, import detection, environment variable detection

**Architecture**: Strategy Pattern with 26 implementations

**Base Interface** (`LanguageStrategy.ts`):

```typescript
interface LanguageStrategy {
  name: string;
  extensions: string[];

  // Symbol extraction
  extractSymbols(node: TreeSitter.Node, code: string): SymbolData[];

  // Import/export detection
  extractImports(node: TreeSitter.Node, code: string): ImportInfo[];
  extractExports(node: TreeSitter.Node, code: string): ExportInfo[];

  // Environment variable detection
  extractEnvVars(node: TreeSitter.Node, code: string): string[];

  // Comment patterns
  getCommentPatterns(): CommentPattern[];

  // Merge capabilities
  getMergeCapabilities?(): MergeCapabilities;
}
```

**Full-Featured Strategies** (Complete symbol extraction):

1. **JavaScriptStrategy** - Functions, classes, methods, imports, exports, env vars
1. **TypeScriptStrategy** - All JS features + interfaces, type aliases, generics
1. **PythonStrategy** - Functions, classes, async/await, decorators, imports
1. **JavaStrategy** - Classes, methods, interfaces, sealed classes, records
1. **GoStrategy** - Functions, methods, structs, interfaces, type definitions

**Config Strategies** (Deep merge support): JSONStrategy, YAMLStrategy, TOMLStrategy - Structural deep merging

**Basic Strategies** (Structural parsing only): C, C++, C#, Rust, Kotlin, Swift, Scala, Ruby, PHP, CSS, HTML, SQL, Markdown, etc.

-----

### 3. IntelliMerge Orchestrator (`src/merge/core/intelliMerge.ts`)

**Purpose**: Main merge orchestrator coordinating all components

**7-Phase Merge Pipeline**:

```typescript
class IntelliMerge {
  async merge(
    baseCode: string,
    oursCode: string,
    theirsCode: string,
    language: LanguageKey,
    filePath: string,
    projectRoot?: string
  ): Promise<IntelliMergeResult>
}
```

**Phase 1: Context Building** (Optional, default ON) - Parse import/export statements - Find dependency files (O(1) with cache) - Find dependent files (O(1) with cache) - Extract configuration references - Retrieve Git history

**Phase 2: Symbol Extraction** - Parse base, ours, theirs with Tree-sitter - Extract symbols using language strategy - Build symbol maps (key → SymbolData)

**Phase 3: Recency Analysis** - Analyze change timing - Detect recent patterns - Guide merge decisions

**Phase 4: Project Graph** (Optional, default ON) - Build dependency graph - Detect architectural patterns - Calculate complexity metrics

**Phase 4.5: Breaking Change Detection** (NEW in v0.4.0) - Detect removed exports - Find broken imports - Detect signature changes (async/sync, parameters) - Calculate severity (Critical/High/Medium/Low) - Generate actionable warnings

**Phase 5: Classification** - Categorize conflict type (INCREMENTAL, STRUCTURAL, ARCHITECTURAL, etc.) - Assess severity (LOW, MEDIUM, HIGH, CRITICAL) - Calculate base confidence score

**Phase 6: Strategy Selection & Application** - Select best merge strategy - Apply structural merge - Generate merged content or conflict markers

**Phase 7: Diagnostics Generation** - Build human-readable explanations - Generate recommendations - Create AI-ready prompts if needed

-----

### 4. Classification Engine (`src/merge/classification/`)

**Purpose**: Intelligent conflict categorization

**Conflict Types**:

```typescript
enum ConflictType {
  INCREMENTAL,     // Small logic changes
  STRUCTURAL,      // Refactoring within paradigm
  ARCHITECTURAL,   // Framework/paradigm changes
  CONFIGURATIONAL, // Config/env changes
  COMPLEX          // Multi-dimensional, needs AI
}
```

**Severity Levels**:

```typescript
enum ConflictSeverity {
  LOW,      // High confidence auto-merge
  MEDIUM,   // Review recommended
  HIGH,     // Manual review required
  CRITICAL  // Human resolution required
}
```

**Classification Factors**: Symbol change patterns (additions, modifications, deletions) - Architectural pattern detection (migrations, refactoring) - Change complexity metrics (symbol churn, import churn) - Project graph structure (dependency complexity) - Historical outcome data

**Confidence Modes**:

1. **Static Mode** (default):
- Predefined confidence values by conflict type
- Fast and predictable
- Symbol merge = 85%, Import merge = 90%, Config merge = 80%
1. **Dynamic Mode** (advanced):
- Log-odds additive model with 5 factors:
   1. Change Magnitude (0.5-1.0)
   1. AST Complexity (0.5-1.0)
   1. Conflict Surface (0.3-1.0)
   1. Semantic Similarity (0.6-1.0)
   1. Symbol Stability (private=+0.5, exported=-0.8)
- More conservative for complex merges
- Better for large projects with complex dependencies

-----

### 5. Merge Strategies (`src/merge/strategies/`)

**Strategy Hierarchy**:

1. **Symbol Merge** (`symbolMerge.ts`)
- Merge at function/class/method level
- Detect non-overlapping changes
- Apply three-way merge algorithm
- Confidence: 85% (non-overlapping), 45% (overlapping)
1. **Import Merge** (`importMerge.ts`)
- Combine import statements from both branches
- Deduplicate imports
- Preserve order and structure
- Confidence: 90%
1. **Config Merge** (`dataFormatMerge.ts`)
- Deep merge for JSON, YAML, TOML
- Structural awareness
- Conflict detection at key level
- Confidence: 80%
1. **Line Merge** (`lineMerge.ts`)
- Statement-level merge for function bodies
- Nested merge for complex structures
- Fallback for non-parseable code
- Confidence: 60-70%
1. **Three-Way Merge** (`threeWay.ts`)
- Core three-way merge algorithm
- Symbol-level diff computation
- Merge action determination (keep, use-ours, use-theirs, delete, append)
- Conflict detection (both modified same symbol)

-----

### 6. Context Engine (`src/merge/core/context.ts`)

**Purpose**: Build project-wide context beyond the conflicted file

**Context Snapshot**:

```typescript
interface ContextSnapshot {
  conflictedFile: FileContext;
  dependencies: FileContext[];    // Files this imports from
  dependents: FileContext[];      // Files that import this
  configuration: ConfigurationContext;
  gitHistory: GitContext;
}
```

**Performance**: O(1) lookups via DependencyCacheManager

**Dependency Cache** (`dependencyCache.ts`): Hash-based O(1) dependency lookups - Stored in `.git/git-semantic/dep-cache.json` - Automatic invalidation via file mtime - 40x-30000x speedup vs O(N) file scanning - Graceful fallback if cache unavailable

**Cache Structure**:

```typescript
interface DependencyCache {
  version: number;
  projectRoot: string;
  files: {
    [filePath: string]: {
      mtime: number;
      imports: string[];    // Files this imports
      exports: string[];    // Symbols this exports
      dependents: string[]; // Files importing this
    }
  }
}
```

-----

### 7. Breaking Change Detector (`src/merge/analysis/breakingChangeDetector.ts`)

**Purpose**: Detect breaking changes during merges (NEW in v0.4.0)

**Detection Capabilities**:

1. **Export Changes**
- Removed exports → Find affected dependents
- Modified signatures → Detect parameter/return type changes
- New exports → Track safe additions
1. **Import Impact**
- Broken imports → Symbols no longer exist
- Reference validation → Ensure imported symbols available
1. **Signature Analysis**
- Parameter count changes
- Async/Promise transitions (sync ↔ async)
- Return type modifications

**Severity Classification**: **Critical**: 6+ files affected by removed export - **High**: 3-5 files or broken imports - **Medium**: 1-2 files affected - **Low**: No dependents affected

**Warning Examples**:

```
⚠ Export 'validateToken' removed — affects 2 file(s): src/auth/service.ts
⚠ Function 'calculate' signature changed — may break 3 file(s)
⚠ Import 'UserSession' from 'types' no longer exists
```

-----

### 8. AI Agent Handoff (`src/agent-handoff/`)

**Purpose**: Generate local AI-ready prompts when auto-merge fails

**Key Principle**: ZERO external API calls - all AI interaction via local files

**Generated Files**:

1. `.git/semantic-merge/conflict-<id>.md` - Human-readable prompt
1. `.git/semantic-merge/conflict-<id>.json` - Machine-readable context
1. `.git/semantic-merge/audit.json` - Resolution audit log

**Prompt Structure**:

```markdown
# Merge Conflict Resolution

## Conflict Summary
- File: src/utils/calculator.py
- Function: calculate
- Classification: Overlapping Modification
- Severity: Medium
- Confidence: 45%

## Three-Way Comparison
### BASE (common ancestor)
[code]

### OURS (current branch)
[code]

### THEIRS (incoming branch)
[code]

## Analysis
- Change in OURS: [description]
- Change in THEIRS: [description]
- Conflict Reason: [explanation]

## Project Context
- Function Usage: Called from X files
- Dependencies: [list]
- Breaking Changes: [warnings]

## Suggested Resolution Strategies
### Option 1: [strategy]
[code]
Pros: [list]
Cons: [list]

### Option 2: [strategy]
[code]
Pros: [list]
Cons: [list]
```

**Workflow**: 1. IntelliMerge fails to auto-resolve 2. Prompt files generated with unique 8-char conflict ID 3. User runs `git-semantic status --ai-ready` to list conflicts 4. User copies prompt to AI assistant 5. User applies AI suggestion to file 6. User runs `git-semantic resolve <conflict-id>` to complete

-----

### 9. Configuration System (`src/config.ts`)

**Purpose**: Centralized configuration management

**Configuration File**: `.git-semantic.json` (optional, repository root)

**Core Settings**:

```typescript
interface ResolvedConfig {
  features: {
    functionBodyAutoMerge: boolean;   // default: false
    contextEngineEnabled: boolean;    // default: true
    projectGraphEnabled: boolean;     // default: true
  };

  strategies: {
    confidenceThreshold: number;      // default: 0.7
  };

  confidenceMode: 'static' | 'dynamic'; // default: 'static'

  classificationThresholds: {
    highSymbolChurn: number;          // default: 0.3
    highImportChurn: number;          // default: 0.4
    highConfigChurn: number;          // default: 0.5
    architecturalChange: number;      // default: 0.6
  };

  git: {
    historyDepth: number;             // default: 10
    analyzeUncommittedChanges: boolean; // default: true
  };
}
```

**Key Configurations**:

1. **functionBodyAutoMerge** (default: false) — `false`: Function body changes → conflict (AI/manual resolution). `true`: Attempt statement-level merge. Rationale: Function logic changes are complex, AI resolution safer.
1. **contextEngineEnabled** (default: true) — `true`: Parse dependencies, dependents, Git history (slower, smarter). `false`: Minimal context, fast but less intelligent.
1. **projectGraphEnabled** (default: true) — `true`: Build dependency graph, detect patterns (slower, smarter). `false`: No project-wide analysis (faster, simpler).
1. **confidenceMode** (default: ‘static’) — `static`: Predefined confidence values (fast, predictable). `dynamic`: Mathematical adjustments with 5 factors (conservative, nuanced).

-----

## Complete Merge Flow: Step-by-Step

### Scenario: Git calls our merge driver

```bash
# User runs: git merge feature-branch
# Git detects conflict in src/utils/calculator.py
# Git calls: git-semantic merge %O %A %B -- %P
```

**Step 1**: Try Git’s default merge first - Save clean “ours” content - Run `git merge-file` - If success (exit 0) → Done ✅ - If conflicts (exit 1-127) → Continue to semantic merge

**Step 2**: Detect language - Use file extension (`.py` → Python) - Fallback to content analysis if needed - Load PythonStrategy from registry

**Step 3**: Check language capabilities - Does Python strategy support line merge? - Check `preferLineMerge` and `lineMergeFailureBehavior` - Try fast-path line merge if applicable

**Step 4**: Initialize IntelliMerge - Create IntelliMerge instance - Load configuration from `.git-semantic.json` - Initialize DependencyCacheManager

**Step 5**: Build context (if enabled)

```typescript
// Phase 1: Context Building
const context = await buildMergeContext(
  filePath,
  projectRoot,
  baseCode,
  oursCode,
  theirsCode,
  language
);
// → Parse imports/exports
// → Find dependencies (O(1) cache lookup)
// → Find dependents (O(1) cache lookup)
// → Extract config references
// → Get Git history
```

**Step 6**: Extract symbols

```typescript
// Phase 2: Symbol Extraction
const baseParsed = await parseCode(baseCode, 'python');
const oursParsed = await parseCode(oursCode, 'python');
const theirsParsed = await parseCode(theirsCode, 'python');

const baseSymbols = extractSymbols(baseParsed.rootNode, baseCode, 'python');
const oursSymbols = extractSymbols(oursParsed.rootNode, oursCode, 'python');
const theirsSymbols = extractSymbols(theirsParsed.rootNode, theirsCode, 'python');
// → Functions: calculate(), process(), etc.
// → Classes: Calculator, Processor, etc.
// → Methods: self.compute(), etc.
```

**Steps 7-14**: Continue through all phases (Recency, Graph, Breaking Change Detection, Classification, Strategy, Diagnostics)

-----

## Implementation Requirements

### Technology Stack

**Runtime**: Node.js 18+ (for native ES2020 support) - TypeScript 5.3+ (strict mode)

**Core Dependencies**:

```json
{
  "web-tree-sitter": "^0.22.6",       // Tree-sitter WASM runtime
  "tree-sitter-wasms": "^0.1.11",     // Pre-built WASM grammars
  "commander": "^11.1.0",             // CLI framework
  "ora": "^5.4.1",                    // Spinner/progress
  "diff": "^5.2.0",                   // Text diffing
  "js-yaml": "^4.1.1",               // YAML parsing
  "@ltd/j-toml": "^1.38.0",          // TOML parsing
  "glob": "^13.0.0",                  // File pattern matching
  "fs-extra": "^11.2.0"              // Enhanced file operations
}
```

**Dev Dependencies**:

```json
{
  "jest": "^29.7.0",                  // Test framework
  "ts-jest": "^29.1.1",              // TypeScript Jest
  "@types/*": "..."                   // TypeScript definitions
}
```

**TypeScript Configuration** (`tsconfig.json`):

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "CommonJS",
    "moduleResolution": "node",
    "esModuleInterop": true,
    "forceConsistentCasingInFileNames": true,
    "strict": true,
    "skipLibCheck": true,
    "outDir": "dist",
    "rootDir": "src",
    "resolveJsonModule": true,
    "types": ["jest", "node"]
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist", "__tests__"]
}
```

-----

## Key Algorithms

### Three-Way Merge Algorithm

```typescript
// Pseudo-code for core three-way merge
function threeWayMerge(base, ours, theirs) {
  // 1. Extract symbols from all versions
  const baseSymbols = extractSymbols(base);
  const oursSymbols = extractSymbols(ours);
  const theirsSymbols = extractSymbols(theirs);

  // 2. Compute symbol-level diffs
  const allKeys = new Set([
    ...baseSymbols.keys(),
    ...oursSymbols.keys(),
    ...theirsSymbols.keys()
  ]);

  // 3. Determine merge action for each symbol
  const mergeActions = new Map();

  for (const key of allKeys) {
    const inBase = baseSymbols.has(key);
    const inOurs = oursSymbols.has(key);
    const inTheirs = theirsSymbols.has(key);

    if (!inBase && inOurs && !inTheirs) {
      // Only we added it
      mergeActions.set(key, { action: 'append-ours', symbol: oursSymbols.get(key) });
    }
    else if (!inBase && !inOurs && inTheirs) {
      // Only they added it
      mergeActions.set(key, { action: 'append-theirs', symbol: theirsSymbols.get(key) });
    }
    else if (inBase && inOurs && !inTheirs) {
      // They deleted it
      mergeActions.set(key, { action: 'delete', symbol: baseSymbols.get(key) });
    }
    else if (inBase && !inOurs && inTheirs) {
      // We deleted it
      mergeActions.set(key, { action: 'delete', symbol: baseSymbols.get(key) });
    }
    else if (inBase && inOurs && inTheirs) {
      const baseText = baseSymbols.get(key).text;
      const oursText = oursSymbols.get(key).text;
      const theirsText = theirsSymbols.get(key).text;

      if (oursText === baseText && theirsText !== baseText) {
        // Only they modified
        mergeActions.set(key, { action: 'use-theirs', symbol: theirsSymbols.get(key) });
      }
      else if (theirsText === baseText && oursText !== baseText) {
        // Only we modified
        mergeActions.set(key, { action: 'use-ours', symbol: oursSymbols.get(key) });
      }
      else if (oursText === theirsText) {
        // Same modification
        mergeActions.set(key, { action: 'use-ours', symbol: oursSymbols.get(key) });
      }
      else {
        // True conflict — both modified differently
        mergeActions.set(key, {
          action: 'conflict',
          base: baseSymbols.get(key),
          ours: oursSymbols.get(key),
          theirs: theirsSymbols.get(key)
        });
      }
    }
  }

  // 4. Apply merge actions
  return applyMergeActions(mergeActions, base, ours, theirs);
}
```

-----

## Quick Start for AI Systems

### Minimum Viable Implementation

**Phase 1**: Basic structure (Day 1-2) 1. Initialize TypeScript project 2. Integrate Tree-sitter WASM 3. Implement 1 language strategy (Python or JavaScript) 4. Basic three-way merge algorithm 5. CLI with merge command

**Phase 2**: Core features (Day 3-5) 1. Symbol extraction for 5 languages 2. Import merging 3. Classification engine (basic) 4. Context engine (without cache) 5. CLI `install` command

**Phase 3**: Intelligence (Day 6-8) 1. Project graph builder 2. Breaking change detector 3. AI prompt generator 4. Dependency cache 5. Dynamic confidence scoring

**Phase 4**: Polish (Day 9-10) 1. Complete language support (26 languages) 2. Comprehensive tests 3. Documentation 4. Performance optimization 5. Error handling

-----

# Part 3: Go Conversion Guide

## Why Go?

### Advantages of Go Implementation

1. **Performance**: Compiled binaries (no Node.js runtime) - Lower memory footprint (~50MB vs ~200MB) - Faster startup time (~10ms vs ~100ms) - Better CPU utilization
1. **Deployment**: Single static binary (no dependencies) - No npm/node_modules - Cross-compilation built-in - Smaller distribution size (~10MB vs ~50MB with node_modules)
1. **Concurrency**: Native goroutines for parallel parsing - Excellent channel-based communication - Better multi-core utilization
1. **Type Safety**: Stronger compile-time guarantees - No `any` types - Explicit error handling

### Trade-offs

**TypeScript Advantages**: Rich ecosystem (npm packages) - Easier Tree-sitter WASM integration - Better JSON/YAML handling libraries - More familiar to web developers

**Go Advantages**: Better performance - Single binary deployment - Superior concurrency model - More predictable runtime behavior

-----

## Architecture Mapping

### Directory Structure Comparison

**Go Structure**:

```
git-semantic/
├── cmd/
│   └── git-semantic/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── parser/                  # Tree-sitter integration
│   │   ├── parser.go
│   │   └── grammar.go
│   ├── languages/               # Language strategies
│   │   ├── strategy.go          # Interface definition
│   │   ├── registry.go          # Strategy registry
│   │   └── strategies/          # Implementations
│   ├── merge/                   # Core merge logic
│   │   ├── intellimerge.go      # Main orchestrator
│   │   ├── context.go           # Context builder
│   │   ├── classification.go    # Conflict classification
│   │   ├── strategies/          # Merge strategies
│   │   ├── analysis/            # Analysis modules
│   │   └── graph/               # Project graph
│   ├── agent/                   # AI prompt generation
│   ├── cli/                     # CLI commands
│   ├── config/                  # Configuration
│   └── types/                   # Shared types
├── internal/
│   └── util/                    # Internal utilities
├── grammars/                    # Tree-sitter grammars
├── go.mod                       # Module definition
└── go.sum                       # Dependency checksums
```

-----

## Type System Conversion

### TypeScript → Go Type Mappings

**Basic Types**:

```go
// TypeScript → Go
// string       → string
// number       → float64 (or int, int64)
// boolean      → bool
// any          → interface{} (avoid when possible)
// unknown      → interface{} + type assertion
// void         → (no return value)
// null/undefined → nil (use pointers for optional values)
// Array<T>     → []T
// Map<K, V>    → map[K]V
// Set<T>       → map[T]struct{} (or use library)
```

**Interface Conversion**:

```typescript
// TypeScript
interface SymbolData {
  key: string;
  name: string;
  type: SymbolKind;
  text: string;
  startIndex: number;
  endIndex: number;
}
```

```go
// Go
type SymbolData struct {
    Key        string     `json:"key"`
    Name       string     `json:"name"`
    Type       SymbolKind `json:"type"`
    Text       string     `json:"text"`
    StartIndex int        `json:"startIndex"`
    EndIndex   int        `json:"endIndex"`
}
```

**Enum Conversion**:

```typescript
// TypeScript
enum SymbolKind {
  Function  = "function",
  Method    = "method",
  Class     = "class",
  Interface = "interface"
}
```

```go
// Go (using constants)
type SymbolKind string

const (
    SymbolKindFunction  SymbolKind = "function"
    SymbolKindMethod    SymbolKind = "method"
    SymbolKindClass     SymbolKind = "class"
    SymbolKindInterface SymbolKind = "interface"
)
```

-----

## Core Library Replacements

### 1. Tree-sitter Integration

**TypeScript** (`web-tree-sitter`):

```typescript
import Parser from 'web-tree-sitter';

await Parser.init();
const parser = new Parser();
const lang = await Parser.Language.load('grammars/tree-sitter-python.wasm');
parser.setLanguage(lang);
const tree = parser.parse(code);
```

**Go** (`go-tree-sitter`):

```go
import (
    sitter "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/python"
)

parser := sitter.NewParser()
parser.SetLanguage(python.GetLanguage())
tree, err := parser.ParseCtx(context.Background(), nil, []byte(code))
if err != nil {
    return nil, fmt.Errorf("parse failed: %w", err)
}
```

**Key Differences**: Go uses CGO bindings (not WASM) - Each language needs separate import - No async initialization needed - Context support for cancellation

### 2. CLI Framework

**TypeScript** (Commander.js):

```typescript
import { Command } from 'commander';

const program = new Command();
program
  .command('merge')
  .argument('<base>', 'Base file')
  .action(async (base, ours, theirs) => {
    // Implementation
  });
```

**Go** (Cobra):

```go
import "github.com/spf13/cobra"

var mergeCmd = &cobra.Command{
    Use:   "merge <base> <ours> <theirs>",
    Short: "Git merge driver entrypoint",
    Args:  cobra.ExactArgs(3),
    RunE: func(cmd *cobra.Command, args []string) error {
        base, ours, theirs := args[0], args[1], args[2]
        // Implementation
        return nil
    },
}
```

-----

## Tree-sitter Integration (Critical)

### Challenge: WASM vs CGO

**Go Approach**: Uses CGO bindings via go-tree-sitter - Compiles C code at build time - Requires C compiler - Better performance

### Go Tree-sitter Setup

**1. Install Dependencies**:

```bash
# Ubuntu/Debian
sudo apt-get install build-essential

# macOS
xcode-select --install

# Windows
# Install MinGW-w64 or use WSL
```

**2. Add to go.mod**:

```
require (
    github.com/smacker/go-tree-sitter v0.0.0-20230720070738-0d8a9f78d8f8
)
```

**3. Parser Manager** (`pkg/parser/parser.go`):

```go
package parser

import (
    "context"
    "fmt"
    "time"

    sitter "github.com/smacker/go-tree-sitter"
)

type Parser struct {
    parser      *sitter.Parser
    timeout     time.Duration
    languageMap map[string]*sitter.Language
}

func NewParser() *Parser {
    return &Parser{
        parser:      sitter.NewParser(),
        timeout:     500 * time.Millisecond,
        languageMap: initLanguageMap(),
    }
}

func initLanguageMap() map[string]*sitter.Language {
    return map[string]*sitter.Language{
        "typescript": typescript.GetLanguage(),
        "python":     python.GetLanguage(),
        "go":         golang.GetLanguage(),
        // ... add all 26 languages
    }
}

type ParsedTree struct {
    Language   string
    Tree       *sitter.Tree
    RootNode   *sitter.Node
    SourceCode []byte
}

func (p *Parser) ParseCode(code []byte, language string) (*ParsedTree, error) {
    lang, ok := p.languageMap[language]
    if !ok {
        return nil, fmt.Errorf("unsupported language: %s", language)
    }

    p.parser.SetLanguage(lang)

    // Parse with timeout
    ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
    defer cancel()

    tree, err := p.parser.ParseCtx(ctx, nil, code)
    if err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }

    rootNode := tree.RootNode()

    return &ParsedTree{
        Language:   language,
        Tree:       tree,
        RootNode:   rootNode,
        SourceCode: code,
    }, nil
}
```

-----

## Concurrency Patterns

### Parallel File Parsing

**TypeScript** (sequential with Promise.all):

```typescript
const [baseParsed, oursParsed, theirsParsed] = await Promise.all([
  parseCode(baseCode, language),
  parseCode(oursCode, language),
  parseCode(theirsCode, language)
]);
```

**Go** (goroutines with errgroup):

```go
import "golang.org/x/sync/errgroup"

var g errgroup.Group
var baseParsed, oursParsed, theirsParsed *ParsedTree

g.Go(func() error {
    var err error
    baseParsed, err = parser.ParseCode([]byte(baseCode), language)
    return err
})

g.Go(func() error {
    var err error
    oursParsed, err = parser.ParseCode([]byte(oursCode), language)
    return err
})

g.Go(func() error {
    var err error
    theirsParsed, err = parser.ParseCode([]byte(theirsCode), language)
    return err
})

if err := g.Wait(); err != nil {
    return nil, fmt.Errorf("parsing failed: %w", err)
}
```

-----

## Error Handling

### TypeScript vs Go Error Patterns

**TypeScript** (try-catch):

```typescript
async function parseCode(code: string): Promise<ParsedTree> {
  try {
    const tree = parser.parse(code);
    return { tree, rootNode: tree.rootNode };
  } catch (error) {
    throw new Error(`Parse failed: ${error.message}`);
  }
}
```

**Go** (explicit errors):

```go
func parseCode(code []byte) (*ParsedTree, error) {
    tree, err := parser.ParseCtx(context.Background(), nil, code)
    if err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }

    return &ParsedTree{
        Tree:     tree,
        RootNode: tree.RootNode(),
    }, nil
}
```

-----

## Complete IntelliMerge Implementation (Go Skeleton)

```go
// pkg/merge/intellimerge.go
package merge

import (
    "context"
    "fmt"

    "github.com/your-org/git-semantic/pkg/parser"
    "github.com/your-org/git-semantic/pkg/types"
    "github.com/your-org/git-semantic/pkg/config"
)

type IntelliMerge struct {
    parser *parser.Parser
    config *config.Config
}

func NewIntelliMerge() *IntelliMerge {
    return &IntelliMerge{
        parser: parser.NewParser(),
        config: config.LoadConfig(),
    }
}

type MergeResult struct {
    Success        bool
    MergedContent  string
    Confidence     float64
    Strategy       string
    Classification *types.ConflictClassification
    Diagnostics    *types.Diagnostics
    Context        *types.ContextSnapshot
}

func (im *IntelliMerge) Merge(
    baseCode, oursCode, theirsCode []byte,
    language, filePath, projectRoot string,
) (*MergeResult, error) {
    // Phase 1: Context Building (if enabled)
    var ctx *types.ContextSnapshot
    if im.config.Features.ContextEngineEnabled {
        var err error
        ctx, err = im.buildContext(filePath, projectRoot, baseCode,
            oursCode, theirsCode, language)
        if err != nil {
            return nil, fmt.Errorf("context build failed: %w", err)
        }
    }

    // Phase 2: Symbol Extraction (parallel)
    symbols, err := im.extractSymbols(baseCode, oursCode, theirsCode, language)
    if err != nil {
        return nil, fmt.Errorf("symbol extraction failed: %w", err)
    }

    // Phase 3: Recency Analysis
    recency := im.analyzeRecency(ctx)

    // Phase 4: Project Graph (if enabled)
    var graph *types.ProjectGraph
    if im.config.Features.ProjectGraphEnabled {
        graph = im.buildProjectGraph(ctx, symbols)
    }

    // Phase 4.5: Breaking Change Detection
    breakingChanges := im.detectBreakingChanges(symbols, ctx)

    // Phase 5: Classification
    classification := im.classifyConflict(symbols, ctx, graph)

    // Phase 6: Strategy Selection & Application
    strategyResult := im.applyStrategy(classification, symbols,
        baseCode, oursCode, theirsCode)

    // Phase 7: Diagnostics Generation
    diagnostics := im.generateDiagnostics(classification,
        strategyResult, breakingChanges)

    return &MergeResult{
        Success:        strategyResult.Success,
        MergedContent:  strategyResult.Content,
        Confidence:     strategyResult.Confidence,
        Strategy:       strategyResult.Strategy,
        Classification: classification,
        Diagnostics:    diagnostics,
        Context:        ctx,
    }, nil
}
```

-----

## Common Pitfalls

### 1. CGO Dependencies

**Problem**: go-tree-sitter uses CGO, requires C compiler

**Solution**: Document C compiler requirements - Provide pre-built binaries for major platforms - Use GitHub Actions for cross-compilation - Consider static linking for easier distribution

### 2. Null vs Empty Slices

**Problem**: Go distinguishes between nil and empty slices

```go
// Good:
symbols := make([]Symbol, 0) // Empty slice (not nil)
if len(symbols) == 0 {
    // This is better
}
```

### 3. Defer in Loops

**Bad**:

```go
for _, file := range files {
    f, _ := os.Open(file)
    defer f.Close() // Will close all at end, not per iteration
}
```

**Good**:

```go
for _, file := range files {
    func() {
        f, _ := os.Open(file)
        defer f.Close() // Closes at end of anonymous function
        // Process file
    }()
}
```

-----

## Step-by-Step Conversion Plan

### Phase 1: Foundation (Week 1)

**Day 1-2: Project Setup** - Initialize Go module (`go mod init`) - Set up directory structure (`cmd/`, `pkg/`, `internal/`) - Install go-tree-sitter and language grammars - Set up Cobra CLI framework - Configure build tools (Makefile, goreleaser)

**Day 3-4: Parser Module** - Implement `pkg/parser/parser.go` with all 26 languages - Create language registry - Add timeout and cancellation support - Write parser tests

**Day 5-7: Type System** - Convert all TypeScript interfaces to Go structs - Implement core types (`types/symbol.go`, `types/merge.go`) - Add JSON tags for serialization - Implement type conversion helpers

### Phase 2: Language Strategies (Week 2)

**Day 8-10: Strategy Framework** - Define `LanguageStrategy` interface - Implement strategy registry - Create base strategy with common functionality

**Day 11-14: Implement Strategies** - JavaScript/TypeScript strategy - Python strategy - Go strategy - Java strategy - Rust strategy - 21 other language strategies (basic support)

### Phase 3: Core Merge Logic (Week 3)

**Day 15-16: Three-Way Merge** - Implement core three-way merge algorithm - Symbol extraction for all languages - Symbol-level diff computation - Merge action determination

**Day 17-18: Merge Strategies** - Symbol merge strategy - Import merge strategy - Config merge strategy (JSON/YAML/TOML) - Line merge fallback

**Day 19-21: IntelliMerge Orchestrator** - Implement 7-phase merge pipeline - Context builder - Recency analysis - Project graph builder

### Phase 4: Intelligence & Analysis (Week 4)

**Day 22-23: Classification Engine** - Conflict type classification - Severity assessment - Pattern detection - Confidence scoring (static mode)

**Day 24-25: Breaking Change Detection** - Export change analysis - Import impact analysis - Signature change detection - Severity classification

**Day 26-28: Context & Cache** - Dependency cache manager (O(1) lookups) - File modification tracking - Cache invalidation - Graceful fallback

### Phase 5: AI Handoff & CLI (Week 5)

**Day 29-30: AI Agent Handoff** - Prompt generator - Conflict ID generation - File output (Markdown + JSON) - Audit logging

**Day 31-32: CLI Commands** - `install` command - `status` command - `preview` command - `resolve` command - `cache` command - `sync-fork` command

**Day 33-35: Configuration** - Config file parsing (`.git-semantic.json`) - Default configuration - Config validation - Runtime config updates

### Phase 6: Testing & Polish (Week 6)

**Day 36-38: Test Suite** - Port 897 test cases from TypeScript - Add Go-specific tests - Benchmark critical paths - Integration tests

**Day 39-40: Documentation** - Update README for Go - Add Go-specific installation instructions - Document CGO requirements - Add build instructions

**Day 41-42: Build & Release** - Cross-compilation setup - goreleaser configuration - CI/CD pipeline (GitHub Actions) - Binary packaging

-----

## Success Criteria

A successful Go conversion should:

1. ✅ **Feature Parity**: All TypeScript features working in Go
1. ✅ **Performance**: 2-3x faster parsing and merging
1. ✅ **Binary Size**: Single binary < 15MB (compressed)
1. ✅ **Cross-Platform**: Builds for Linux, macOS, Windows
1. ✅ **Test Coverage**: 897 tests ported and passing
1. ✅ **Documentation**: Complete Go-specific docs
1. ✅ **Build System**: Automated cross-compilation
1. ✅ **Installation**: Simple binary installation (no runtime deps)

-----

## Conclusion

This complete guide provides everything needed to:

1. **Understand** git-semantic’s architecture and design
1. **Implement** it from scratch in TypeScript
1. **Convert** the TypeScript implementation to Go

**Estimated Effort**: TypeScript Implementation: 2 weeks (1 developer, full-time) - Go Conversion: 6 weeks (1 developer, full-time)

**Key Success Factors**:

1. Read this guide thoroughly
1. Start with parser + 1 language (validate approach)
1. Add languages incrementally
1. Test continuously (897 test cases provided)
1. Reference examples when stuck
1. Follow the phase-based implementation plan

Good luck, and happy coding! 🚀

-----

**Document Version**: 1.0.0 **Last Updated**: May 22, 2026 **License**: MIT