# Fuse Roadmap

Fuse is a semantic Git merge driver. MIT licensed. Requires Grove for cross-file context.

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

## v1.0.0 — Production Hardening

- [x] Merge accuracy: ≥ 85% auto-resolution rate on INCREMENTAL conflicts
- [x] Handoff prompt includes Grove blast radius and breaking change analysis
- [ ] Homebrew tap: `brew install fuse`
- [ ] `curl | sh` installer for Linux
- [ ] Published Go module: `github.com/tabladrum/grove-suite/fuse`
