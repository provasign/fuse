# Fuse

> **Symbol-aware Git merge driver. Auto-resolves the conflicts that shouldn't exist — and never does worse than git.**

> **Embedded Grove:** Fuse links Grove directly and opens the on-disk index in-process. No `grove serve` daemon, no `grove_url`, no token — if old docs mention them, you're on a pre-embedded build.

---

You're running multiple AI agents in parallel. Or an agent and your human developers. They all commit to the same files.

Git sees lines. It doesn't know your agent changed `Login()` while your human changed `validatePassword()` in the same file. They're different functions — structurally independent — but they happen to occupy adjacent lines. Git declares a conflict. A developer stops, opens a merge tool, manually resolves something that was never actually conflicting, and goes back to work.

Multiply that by a hundred agent PRs a week.

Fuse replaces git's line-level merge with an escalation ladder. Anything git can merge cleanly, fuse merges byte-for-byte identically. Where git conflicts, fuse escalates: first a finer-grained line merge, then a Tree-sitter symbol merge that understands that two changes to different symbols never conflict, regardless of where they appear in the file. Every auto-merged result is re-parsed before it ships — a merge that doesn't parse is discarded and surfaced as a conflict instead. The conflicts that survive all of that get git markers plus an AI-ready handoff prompt with Grove blast-radius context, and `fuse resolve --agent` can hand them to an agent in one command.

---

## How It Works

```
git merge <branch>
     │  .gitattributes: *.go merge=fuse
     ▼
fuse merge %O %A %B %P
     │
     ▼
┌──────────────────────────────────────────────────────────────┐
│  Escalation ladder                                           │
│                                                              │
│  1. git-equivalent line merge   clean + parses? → ship       │
│     (byte-for-byte git semantics — fuse is never worse       │
│      than git)                                               │
│  2. fine-grained LCS line merge clean + parses? → ship       │
│     (resolves adjacent-but-disjoint edits git rejects)       │
│  3. symbol merge (Tree-sitter)  clean + parses? → ship       │
│     (resolves same-location additions, distinct-symbol       │
│      edits, independent methods of one class)                │
│                                                              │
│  validation gate: every clean result is re-parsed            │
│  (tree-sitter + stdlib go/parser for Go); failures are       │
│  surfaced as conflicts, never silently shipped               │
│                                                              │
│  alongside: Grove blast radius + breaking-change detection   │
│  (cross-file impact of changed exports, both sides)          │
└──────────────────────────────┬───────────────────────────────┘
                               │
                  ┌────────────┴─────────────┐
                  ▼                          ▼
          Auto-resolved               Unresolvable
          Write merged file     Conflict markers +
          Exit 0                .git/fuse/conflict-<sha>.md
                                Exit 1
```

---

## Measured Accuracy

Fuse ships with its own benchmark: `fuse bench <repo>` replays every merge
commit in a repository's history, re-merges each file both branches modified,
and scores the result against the resolution the humans actually committed.

Replaying [gin](https://github.com/gin-gonic/gin)'s full merge history
(148 merge commits, 89 dual-modified files):

|                  | files | fuse resolved | matches human resolution |
|------------------|------:|--------------:|-------------------------:|
| git conflicted   |    16 |       4 (25%) |                  3 (75%) |
| git auto-merged  |    73 |     73 (100%) |                73 (100%) |

Two properties matter more than the headline rate:

- **Parity:** on every file git can merge, fuse produces the identical bytes.
- **No silent corruption:** every auto-merge is re-parsed; anything suspect
  becomes an explicit conflict.

Run it on your own history: `fuse bench . --limit 200`. Numbers vary by
codebase and conflict style; measure, don't trust.

---

## Conflict Classification

Fuse classifies every conflict before choosing a resolution strategy:

| Class | Description |
|-------|-------------|
| `INCREMENTAL` | Additive changes to different parts of a symbol |
| `STRUCTURAL` | One branch renamed or moved a symbol |
| `CONFIGURATIONAL` | Changes to config/dependency files |
| `ARCHITECTURAL` | Cross-file interface or API change |
| `COMPLEX` | Interleaved logic changes |

The classification drives handoff prompt content and audit records.

---

## AI Handoff

When Fuse cannot resolve a conflict, it writes a structured prompt to `.git/fuse/conflict-<hash>.md`:

- All three versions of the conflicting symbols (base, ours, theirs)
- Symbol signatures from all three versions
- Grove blast radius: what other files reference the changed symbols
- Grove breaking-change analysis: what callers would break under each version
- A resolution task with explicit output contract

Close the loop in one command:

```bash
# Pipe the prompt into any agent CLI; validate; write the resolution.
fuse resolve .git/fuse/conflict-abc123.md --agent "claude -p" --apply
```

The agent contract is plain: prompt on stdin, complete resolved file on
stdout. Fuse validates the output before it touches your working tree —
non-empty, no conflict markers, parses cleanly — and rejects anything else.
Set `resolve.agent_cmd` in `fuse.yaml` to make `--agent` the default.

---

## GitHub Action

Merge drivers are local-only, but agent PR conflicts live on the server. The
bundled composite action keeps agent branches mergeable: it installs fuse,
registers the driver (via `.git/info/attributes` — nothing in your tree),
merges the base branch into the PR branch, and optionally pushes the result.

```yaml
name: fuse-merge
on: pull_request

jobs:
  merge:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.head_ref }}
          fetch-depth: 0
      - uses: provasign/fuse@main
        with:
          push: 'true'
```

When fuse can't resolve everything, the job fails with the list of conflicted
files and AI handoff prompts under `.git/fuse/` — pipe those into
`fuse resolve --agent` from a follow-up step if you want full automation.

---

## Why not mergiraf?

[Mergiraf](https://mergiraf.org) is an excellent structured merge driver, and
if you only want syntax-aware single-file merging you should consider it.
Fuse exists for the layer above:

- **Cross-file awareness.** Fuse queries Grove's code graph for blast radius
  and breaking-change analysis — "this merge removes an export that 6 files
  call" is information no single-file merger can produce.
- **AI handoff.** Unresolvable conflicts become agent-ready prompts with all
  three versions plus graph context, and `fuse resolve --agent` applies the
  agent's validated resolution.
- **Audit trail.** Every decision is recorded in `.git/fuse/audit.json` —
  what was merged, by which strategy, at what confidence.
- **Graph drift evidence.** Every auto-merge also records the structural
  code-graph delta it produced in `.git/fuse/drift.json`: which symbols were
  added, removed, or changed, and which changes break the exported surface
  (old vs. new signature). Drift is matched by stable symbol identity via
  Grove's graph diff, so line shifts don't register — only real structural
  change does. This is the raw material for telling a concurrent agent
  *"the ground shifted under you: `Login()` changed"* mid-task. Advisory and
  fail-open; `fuse status` summarizes it.

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

Register as a Git merge driver in the current repository:

```bash
fuse install
```

This sets `merge.fuse.name` / `merge.fuse.driver` in the repo's git config
and appends the supported patterns to `.gitattributes`:

```
*.go   merge=fuse
*.ts   merge=fuse
*.py   merge=fuse
*.java merge=fuse
*.rs   merge=fuse
...
```

To register for all repositories instead, set the driver globally yourself:

```bash
git config --global merge.fuse.name "Fuse semantic merge driver"
git config --global merge.fuse.driver "fuse merge %O %A %B %P"
```

---

## CLI Reference

```bash
fuse install                    # register git driver + .gitattributes in this repo
fuse uninstall                  # remove git driver registration
fuse merge <base> <ours> <theirs> [path]    # manual invocation (normally called by git)
fuse preview <base> <ours> <theirs>         # print merged result without writing
fuse resolve <conflict-file> [--agent <cmd>] [--apply]   # AI-resolve a conflict
fuse bench [repo] [--limit N] [--json]      # replay merge history, score accuracy
fuse check <file>               # breaking changes vs HEAD
fuse impact <file-or-symbol>    # blast radius via Grove
fuse deps <file>                # dependency edges via Grove
fuse status                     # show recent merge decisions from the audit log
fuse config                     # print resolved configuration
fuse mcp [dir]                  # stdio MCP server (agent integration)
```

---

## Configuration

`fuse.yaml` in the project root:

```yaml
merge:
  handoff_threshold: 0.30        # below this, emit a handoff prompt
  enable_breaking_change: true   # Grove-backed breaking-change detection
  enable_context: true           # Grove context in handoff prompts
  enable_drift: true             # record per-merge graph drift evidence
  grove_required: true           # fail if the Grove index can't be opened
  auto_index: true               # build/refresh the Grove index before merges

resolve:
  agent_cmd: "claude -p"         # default agent for `fuse resolve`
```

---

## Grove Integration

Fuse embeds [Grove](https://github.com/provasign/grove) as a library and
opens the repository's `.grove/` index in-process — no daemon. Grove provides
the cross-file blast radius and breaking-change detection that make handoff
prompts meaningful.

On first use in a fresh clone the index is empty; with `merge.auto_index`
(default on) fuse builds it automatically and delta-refreshes it before
merges so impact data reflects the working tree. Without Grove data, fuse
still merges — it just loses cross-file analysis.

---

## Tree-sitter Usage

Fuse parses the three in-memory merge versions (base, ours, theirs) as
strings with Tree-sitter, independent of Grove's on-disk indexing — a merge
needs the same file in three states simultaneously. Merged output is
re-parsed before shipping; for Go, the stdlib parser is also consulted
because tree-sitter's Go grammar tolerates constructs `gofmt` rejects.

---

## Language Support

Symbol-level merge: Go, TypeScript, TSX, JavaScript, Python, Java, Rust.
Config formats (JSON, YAML, TOML): line merge first, structural deep merge
on conflict. Everything else: git-equivalent line merge.

---

## Audit Log

Every merge decision is appended to `.git/fuse/audit.json`:

```json
{
  "timestamp": "2026-06-11T14:23:01Z",
  "file": "internal/auth/login.go",
  "language": "go",
  "strategy": "symbol",
  "conflictType": "INCREMENTAL",
  "severity": "LOW",
  "confidence": 0.92,
  "autoMerged": true,
  "breakingChanges": 0
}
```

`fuse status` prints the recent entries.

---

## Quick Start

```bash
# Build
make build

# Register fuse as the merge driver in this repo
./bin/fuse install

# Show resolved config
./bin/fuse config

# Test a three-way merge directly (no Git required)
./bin/fuse merge base.go ours.go theirs.go path/in/repo.go
# Exit 0 = clean; Exit 1 = conflict markers written to ours.go

# Score fuse against your own merge history
./bin/fuse bench . --limit 200

# Agent integration: stdio MCP server with four tools
./bin/fuse mcp .
```

## MCP Tools

`fuse mcp` exposes fuse to agents the same way `grove mcp` and `prism mcp`
do: stdio JSON-RPC, no ports, no tokens. Four tools, deliberately few and
terse — results are summary-first (verdict + counts; detail only where
conflicts exist) and compact JSON, because they land in an agent's context
window:

| Tool | Purpose |
|------|---------|
| `fuse_merge_check` | Dry-run semantic merge of HEAD vs a ref — "will my work merge cleanly?" before pushing. The killer tool: merge conflicts become pre-push feedback instead of failures. |
| `fuse_preview` | Three-way merge of in-memory content; returns merged text + conflict flag. |
| `fuse_resolve` | Fetch a handoff prompt for in-context resolution; validate and optionally apply the resolution. |
| `fuse_impact` | Grove blast radius for a file/symbol — self-check before finishing a task. |

There is no HTTP server. PR conflict resolution in CI is the GitHub
Action's job; agents use MCP; git invokes the merge driver; humans use the
CLI.

---

## Status

The merge pipeline is benchmarked against real merge history (`fuse bench`)
with two hard invariants: byte-parity with git wherever git succeeds, and no
auto-merge ships without re-parsing cleanly. Symbol-level merge covers 7
languages plus structural config merge for JSON/YAML/TOML. Grove-backed
breaking-change detection, AI handoff with agent integration
(`fuse resolve --agent`), and an append-only audit log round out the loop.

Run `make test` for the test suite.
