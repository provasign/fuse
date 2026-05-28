# Fuse Roadmap

Fuse is a semantic git merge driver. Requires Grove for cross-file context.

## v0.1.0 — Parser & Grove Integration
_Target: Phase 1–2 of Implementation Plan_

- [ ] Grove client with hard-requirement startup check (same auto-start pattern as Prism)
- [ ] Tree-sitter parser for in-memory merge: parses base/ours/theirs as strings (distinct from Grove's file-indexing use of Tree-sitter)
- [ ] Per-language extractors: Go, TypeScript, JavaScript, Python, Java, Rust, plus JSON/YAML/TOML config strategies
- [ ] Symbol extractor: functions, classes, methods, interfaces, exports, imports per language

## v0.2.0 — IntelliMerge Pipeline
_Target: Phase 3–4 of Implementation Plan_

- [ ] IntelliMerge 7-phase orchestrator: context building → symbol extraction → recency analysis → graph context → breaking change detection → classification → strategy selection
- [ ] 5 merge strategies: Symbol (85% confidence), Import (90%), Config (80%), Line (60–70%), Handoff (<30%)
- [ ] Symbol-level three-way merge algorithm
- [ ] Import statement merge: union + deduplication + style preservation
- [ ] Config deep merge: JSON/YAML/TOML structural merge
- [ ] Breaking change detection: `removed_export`, `signature_changed`, `broken_import` via Grove blast radius

## v0.3.0 — Classification & AI Handoff
_Target: Phase 5–6 of Implementation Plan_

- [ ] Conflict classification engine: INCREMENTAL / STRUCTURAL / ARCHITECTURAL / CONFIGURATIONAL / COMPLEX
- [ ] Dynamic confidence model: 5-factor log-odds adjustment (opt-in)
- [ ] AI handoff prompt generation: writes `.git/fuse/conflict-<sha>.md` with three-way comparison + Grove context
- [ ] Audit log: `.git/fuse/audit.json` — every merge decision recorded

## v0.4.0 — Git Integration & CLI
_Target: Phase 7–8 of Implementation Plan_

- [ ] Git merge driver registration: `fuse install` writes `~/.gitconfig` + `.gitattributes`
- [ ] Git driver interface: reads `%O %A %B %P` args, writes result in-place, exits 0 or 1
- [ ] CLI: `fuse merge`, `fuse install`, `fuse uninstall`, `fuse status`, `fuse audit`, `fuse config`
- [ ] Supports all 9 file types: Go, TS, JS, Python, Java, Rust, JSON, YAML, TOML

## v1.0.0 — Production Ready

- [ ] Merge accuracy benchmark: ≥ 85% auto-resolution rate on INCREMENTAL conflicts
- [ ] 897 tests passing across all language strategies
- [ ] Single binary distribution: `brew install fuse`, GitHub Releases
- [ ] Zero external API calls — AI handoff is local prompt generation only
