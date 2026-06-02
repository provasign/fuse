# Fuse

> **Symbol-aware Git merge driver. Auto-resolves the conflicts that shouldn't exist.**

> **Embedded Grove:** Fuse now links Grove directly and opens the on-disk index in-process. No `grove serve` daemon, no `grove_url`, no token — if old docs mention them, you're on a pre-embedded build.

---

You're running multiple AI agents in parallel. Or an agent and your human developers. They all commit to the same files.

Git sees lines. It doesn't know your agent changed `Login()` while your human changed `validatePassword()` in the same file. They're different functions — structurally independent — but they happen to occupy adjacent lines. Git declares a conflict. A developer stops, opens a merge tool, manually resolves something that was never actually conflicting, and goes back to work.

Multiply that by a hundred agent PRs a week.

Fuse replaces git's line-level merge with a symbol-aware one. It parses all three versions of a file with Tree-sitter, extracts symbols, queries Grove for cross-file blast radius, and merges at symbol granularity. Two changes to different symbols never conflict, regardless of where they appear in the file. The ones that are genuinely ambiguous get conflict markers plus an AI-ready handoff prompt — all the context an agent needs to resolve them in one pass.

---

## How It Works

```
git merge <branch>
     │  .gitattributes: *.go merge=fuse
     ▼
fuse merge %O %A %B %P
     │
     ├──► Grove: /impact  (blast radius of changed symbols)
     ├──► Grove: /deps    (cross-file dependencies)
     │
     ▼
┌─────────────────────────────────────────────────────────┐
│  IntelliMerge — 7-phase pipeline                        │
│                                                         │
│  Phase 1: Context building        (Grove API calls)     │
│  Phase 2: Symbol extraction       (Tree-sitter, in-mem) │
│  Phase 3: Recency analysis        (git log weighting)   │
│  Phase 4: Project graph context   (cross-file edges)    │
│  Phase 5: Breaking change detect  (signature diff)      │
│  Phase 6: Conflict classification (5 categories)        │
│  Phase 7: Strategy selection      (5 strategies)        │
└──────────────────────────────────┬──────────────────────┘
                                   │
                      ┌────────────┴─────────────┐
                      ▼                          ▼
              Auto-resolved               Unresolvable
              Write merged file     Conflict markers +
              Exit 0               .git/fuse/conflict-<sha>.md
                                   Exit 1
```

---

## Conflict Classification

Fuse classifies every conflict before choosing a resolution strategy:

| Class | Description | Typical auto-resolution rate |
|-------|-------------|------------------------------|
| `INCREMENTAL` | Additive changes to different parts of a symbol | ~85% |
| `STRUCTURAL` | One branch renamed or moved a symbol | ~60% |
| `CONFIGURATIONAL` | Changes to config/dependency files | ~80% |
| `ARCHITECTURAL` | Cross-file interface or API change | Handoff |
| `COMPLEX` | Interleaved logic changes | Handoff |

---

## Merge Strategies

| Strategy | Confidence | When used |
|----------|-----------|-----------|
| Symbol | ≥ 85% | Distinct symbol changes |
| Import | ≥ 90% | Import statement differences |
| Config | ≥ 80% | JSON/YAML/TOML structure merge |
| Line | 60–70% | Structural changes (fallback) |
| Handoff | < 30% | Complex/architectural conflicts |

---

## AI Handoff

When Fuse cannot resolve a conflict confidently, it writes a structured prompt to `.git/fuse/conflict-<hash>.md`. The prompt includes:

- All three versions of the conflicting region (base, ours, theirs)
- Symbol signatures from all three versions
- Grove blast radius: what other symbols reference the changed symbol
- Grove breaking change analysis: what callers would break under each version
- A suggested resolution approach based on conflict classification

Feed this file to an AI agent to resolve the conflict in context.

---

## Installation

**Binary install (fastest):**

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/provasign/fuse/main/install.sh | bash

# Windows (PowerShell)
irm https://raw.githubusercontent.com/provasign/fuse/main/install.ps1 | iex

# Pin a specific version
VERSION=v0.4.0 curl -fsSL https://raw.githubusercontent.com/provasign/fuse/main/install.sh | bash
```

Installs to `~/bin` by default. Set `INSTALL_DIR=/usr/local/bin` to override.

**Build from source:**

```bash
make build    # compile ./bin/fuse
make install  # install to $GOPATH/bin
```

Register as a Git merge driver globally:

```bash
fuse install
```

This writes to `~/.gitconfig`:

```
[merge "fuse"]
    name = Fuse semantic merge driver
    driver = fuse merge %O %A %B %P
```

Per-repository, add `.gitattributes`:

```
*.go   merge=fuse
*.ts   merge=fuse
*.py   merge=fuse
*.java merge=fuse
*.rs   merge=fuse
*.cs   merge=fuse
```

---

## CLI Reference

```bash
fuse install                    # register git driver globally
fuse uninstall                  # remove git driver registration
fuse merge <base> <ours> <theirs> <path>   # manual invocation (normally called by git)
fuse status [dir]               # show merge driver config and Grove connection
fuse audit [dir]                # show recent merge decisions
fuse config [dir]               # show or edit fuse.yaml
```

---

## Configuration

`fuse.yaml` in the project root:

```yaml
grove_url: http://localhost:7777     # Grove instance
languages:                           # which file types to handle
  - go
  - typescript
  - javascript
  - python
  - java
  - rust
  - csharp
confidence_threshold: 0.70          # below this, produce handoff prompt
audit_log: true                      # write .git/fuse/audit.json
```

Environment override: `GROVE_URL`.

---

## Grove Dependency

Fuse requires a running Grove instance. It checks `$GROVE_URL/health` on startup and auto-starts `grove serve` if unreachable. Grove provides the cross-file blast radius and breaking change detection that make the `ARCHITECTURAL` and `COMPLEX` classifications meaningful — without it, Fuse falls back to line-level merge for those cases.

---

## Tree-sitter Usage

Fuse uses Tree-sitter independently of Grove — it parses the three in-memory merge versions (base, ours, theirs) as strings, not files on disk. This is distinct from Grove's file-on-disk indexing. Fuse needs to parse the same file in three states simultaneously within a single merge invocation; going through Grove's indexer would require writing all three versions to disk and reindexing.

---

## Language Support

Same languages as Grove's parser: Go, TypeScript, TSX, JavaScript, Python, Java, Rust, C, C++, C#, PHP. Config file formats (JSON, YAML, TOML) use a structural diff instead of Tree-sitter.

---

## Audit Log

Every merge decision is appended to `.git/fuse/audit.json`:

```json
{
  "timestamp": "2026-05-28T14:23:01Z",
  "file": "internal/auth/login.go",
  "class": "INCREMENTAL",
  "strategy": "Symbol",
  "confidence": 0.92,
  "resolved": true,
  "symbols_merged": ["Login", "validatePassword"]
}
```

---

## Quick Start

```bash
# Build
make build

# Register fuse as the merge driver
./bin/fuse install

# Show resolved config
./bin/fuse config

# Test a three-way merge directly (no Git required)
./bin/fuse merge base.go ours.go theirs.go path/in/repo.go
# Exit 0 = clean; Exit 1 = conflict markers written to ours.go

# Start HTTP API
./bin/fuse serve --port 9999
curl -X POST http://localhost:9999/merge \
  -H 'Content-Type: application/json' \
  -d '{"base":"...","ours":"...","theirs":"...","path":"x.go"}'
```

---

## Status

Phase 1 complete: parsing, three-way merge for 7 languages (Go, TypeScript, TSX,
JavaScript, Python, Java, Rust) plus config merge for JSON/YAML/TOML. Tree-sitter
backed symbol extraction, LCS-based line fallback, classification, Grove-backed
breaking change detection, AI handoff prompt generation, audit log.

Run `make test` for the test suite.
