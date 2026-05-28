# Fuse

Fuse is a semantic Git merge driver. It operates at symbol granularity instead of line granularity, resolves the majority of conflicts automatically, and produces AI-ready handoff prompts for the ones it cannot.

## Why Line-Level Merge Fails

`git merge` operates on lines of text. It cannot distinguish between a line that is part of a function signature and a line that is part of an unrelated comment one hundred lines away. This produces two failure modes:

1. **False conflicts.** Two developers modified different functions in the same file. Their changes don't interact, but they touched lines close enough together that the diff algorithm declares a conflict. A developer now spends time manually resolving something that should have been automatic.

2. **Silent data corruption.** A function was moved to a different file. On the source branch it was deleted from the original location; on the target branch it was modified in place. Line-level merge sees a deletion and a modification to overlapping lines and may silently drop the modification.

Fuse parses all three versions of a file (base, ours, theirs) into symbol-level representations, then merges those. Two changes to different symbols are structurally independent even if their source lines are adjacent.

## Architecture

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

## Conflict Classification

Fuse classifies every conflict before choosing a resolution strategy:

| Class | Description | Typical auto-resolution rate |
|-------|-------------|------------------------------|
| `INCREMENTAL` | Additive changes to different parts of a symbol | ~85% |
| `STRUCTURAL` | One branch renamed or moved a symbol | ~60% |
| `CONFIGURATIONAL` | Changes to config/dependency files | ~80% |
| `ARCHITECTURAL` | Cross-file interface or API change | Handoff |
| `COMPLEX` | Interleaved logic changes | Handoff |

## Merge Strategies

| Strategy | Confidence | When used |
|----------|-----------|-----------|
| Symbol | ≥ 85% | Distinct symbol changes |
| Import | ≥ 90% | Import statement differences |
| Config | ≥ 80% | JSON/YAML/TOML structure merge |
| Line | 60–70% | Structural changes (fallback) |
| Handoff | < 30% | Complex/architectural conflicts |

## AI Handoff

When Fuse cannot resolve a conflict confidently, it writes a structured prompt to `.git/fuse/conflict-<hash>.md`. The prompt includes:

- All three versions of the conflicting region (base, ours, theirs)
- Symbol signatures from all three versions
- Grove blast radius: what other symbols reference the changed symbol
- Grove breaking change analysis: what callers would break under each version
- A suggested resolution approach based on conflict classification

Feed this file to an AI agent to resolve the conflict in context.

## Installation

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

## CLI Reference

```bash
fuse install                    # register git driver globally
fuse uninstall                  # remove git driver registration
fuse merge <base> <ours> <theirs> <path>   # manual invocation (normally called by git)
fuse status [dir]               # show merge driver config and Grove connection
fuse audit [dir]                # show recent merge decisions
fuse config [dir]               # show or edit fuse.yaml
```

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

## Grove Dependency

Fuse requires a running Grove instance. It checks `$GROVE_URL/health` on startup and auto-starts `grove serve` if unreachable. Grove provides the cross-file blast radius and breaking change detection that make the ARCHITECTURAL and COMPLEX classifications meaningful — without it, Fuse falls back to line-level merge for those cases.

## Tree-sitter Usage

Fuse uses Tree-sitter independently of Grove — it parses the three in-memory merge versions (base, ours, theirs) as strings, not files on disk. This is distinct from Grove's file-on-disk indexing. Fuse needs to parse the same file in three states simultaneously within a single merge invocation; going through Grove's indexer would require writing all three versions to disk and reindexing.

## Language Support

Same languages as Grove's parser: Go, TypeScript, TSX, JavaScript, Python, Java, Rust, C, C++, C#, PHP. Config file formats (JSON, YAML, TOML) use a structural diff instead of Tree-sitter.

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
