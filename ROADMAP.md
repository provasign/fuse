# Fuse Roadmap

Fuse is a semantic Git merge driver. MIT licensed. Embeds Grove for
cross-file context. See [NEXT_STEPS.md](NEXT_STEPS.md) for the working
plan beyond the checklists below.

---

## v0.1.0 — Parser & Grove Integration ✅ shipped

- [x] Grove client with auto-start logic (same startup contract as Prism)
- [x] Tree-sitter parser for in-memory merge: parses base/ours/theirs as strings (not files on disk)
- [x] Per-language extractors: Go, TypeScript, TSX, JavaScript, Python, Java, Rust
- [x] Config file strategies: JSON, YAML, TOML structural merge
- [x] Symbol extractor: functions, classes, methods, interfaces, exports, imports per language

---

## v0.2.0 — IntelliMerge Pipeline ✅ shipped

- [x] IntelliMerge 7-phase orchestrator: context building → symbol extraction → recency analysis → graph context → breaking change detection → classification → strategy selection
- [x] 5 merge strategies: Symbol (≥ 85% confidence), Import (≥ 90%), Config (≥ 80%), Line (60–70%), Handoff (< 30%)
- [x] Symbol-level three-way merge algorithm
- [x] Import statement merge: union + deduplication + style preservation
- [x] Config deep merge: JSON/YAML/TOML structural merge
- [x] Breaking change detection: `removed_export`, `signature_changed`, `broken_import` via Grove blast radius

---

## v0.3.0 — Classification & AI Handoff ✅ shipped

- [x] Conflict classification: INCREMENTAL / STRUCTURAL / ARCHITECTURAL / CONFIGURATIONAL / COMPLEX
- [x] AI handoff prompt generation: writes `.git/fuse/conflict-<sha>.md` with three-way comparison + Grove context + suggested resolution approach
- [x] Audit log: `.git/fuse/audit.json` — every merge decision recorded with timestamp, file, class, strategy, confidence, outcome

---

## v0.4.0 — Git Integration & CLI ✅ shipped

- [x] Git merge driver registration: `fuse install` writes `~/.gitconfig`
- [x] Git driver interface: reads `%O %A %B %P` args, writes result in-place, exits 0 (clean) or 1 (conflict)
- [x] CLI: `fuse merge`, `fuse install`, `fuse uninstall`, `fuse status`, `fuse audit`, `fuse config`
- [x] HTTP API: `POST /merge` endpoint at `:9999`
- [x] 7 source languages + 3 config formats: Go, TS, JS, Python, Java, Rust, C, JSON, YAML, TOML

---

## v0.5.0 — Trust & Measurement ✅ shipped

- [x] Escalation-ladder pipeline: git-parity line merge → fine-grained LCS line merge → symbol merge; fuse is never worse than git
- [x] Post-merge validation gate: every auto-merge re-parsed (tree-sitter + stdlib go/parser for Go); failures surface as conflicts, never silent corruption
- [x] `fuse bench`: replay real merge history, score resolution and human-match rates (measured on gin: 100% git parity, 4/16 git conflicts resolved, 3 byte-identical to human)
- [x] `fuse resolve --agent <cmd> --apply`: pipe handoff prompt into any agent CLI, validate output, write resolution
- [x] Grove auto-index: empty index built automatically, delta refresh before merges (`merge.auto_index`)
- [x] Reconstruction fidelity: original-line passthrough for unchanged symbols, doc-comment carrying, neighbor-anchored insertion of theirs-added symbols, import style preservation, Go method/type key disambiguation
- [x] `curl | sh` / PowerShell installers, uninstall scripts
- [x] Embedded Grove (no daemon, no grove_url)

## v0.6.0 — Distribution & Agent Wiring

- [x] GitHub Action: server-side conflict resolution for agent PR branches (shipped in v0.5.0)
- [ ] `fuse mcp`: stdio MCP server — fuse_merge_check, fuse_preview, fuse_resolve, fuse_impact (see NEXT_STEPS.md §3)
- [ ] Homebrew: join the family tap — `brew install provasign/shale/fuse`
- [ ] Benchmark corpus expansion: TS/Python-heavy repos, publish per-language results in README

## v1.0.0 — Production Hardening

- [ ] Shale integration: merge events and agent resolutions as PR-card evidence; Grove CertifyDiff as advisory evidence generator — never a gate (see NEXT_STEPS.md §2)
- [ ] Raise symbol-path resolution rate (rename detection, container-aware nested merge, import-removal semantics) — measured by `fuse bench`, gated by zero mis-merge regressions
- [ ] Formatting-preserving structural config merge (today: line merge first, re-serializing deep merge on conflict)
- [ ] Windows shell support for `fuse resolve --agent`
