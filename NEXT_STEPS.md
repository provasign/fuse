# Fuse — Next Steps

Working notes for where fuse goes after v0.5.0. Ordering inside each section
is priority order. Provasign-the-product is paused; nothing below depends on
it. Where fuse integrates with the rest of the family, the integration target
is **Shale** (the product being taken to market) and **Grove** (the shared
library) — never the provasign gating workflow.

---

> **✅ STATUS MARKER — done up to here (2026-06-11):**
> The Grove-side foundation for §1 is built and the evidence half of the
> stale-context loop ships:
>
> - **Grove hardened** (grove commits af32c02..35102d0): all critical/high
>   findings from `grove/docs/grove-assessment-2026-06-11.md` fixed —
>   symbol-ID collisions, ICR fallback, per-repo module-cache pollution,
>   CertifyDiff context-line over-reporting + `index_stale` gate, test-edge
>   scoping, quadratic edge building (39.7s → 0.5s), no-change reindex
>   (3.7s → 35ms), real MCP schemas + `grove_certify`, nested .gitignore +
>   `**`, parallel parsing, ranked search.
> - **Grove graph-diff exists** (the §1 prerequisite): `SnapshotSymbols` /
>   `Diff` / `DiffSince` / `DiffAgainstFileContent` — stable-identity
>   diffing with `BreakingChanges`, immune to line-shift/ID churn.
> - **Fuse records drift** (fuse commit 6302674): every auto-merge appends
>   the structural delta (added/removed/changed/breaking symbols with
>   old→new signatures) to `.git/fuse/drift.json` — advisory, fail-open,
>   gated by `merge.enable_drift`. `fuse status` summarizes it.
>
> **✅ Delivery half shipped too (2026-06-12, Grove v0.6.1 / prism 3515a7e):**
> the **stale-context loop from §1 is now closed end to end.** Prism's
> `prism_drift` tool re-verifies the agent's delivered working set against
> the working tree and reports symbol-level drift — origin `merge` with
> Fuse's old→new signatures when `drift.json` shows a merge caused it —
> and every context-bearing Prism response carries a one-line stale-context
> warning the moment a delivered file changes on disk. Grove also gained
> rename detection (GraphDiff `renamed` + drift `renamedFrom`).
>
> **Still open:** `fuse mcp` and the serve-vs-Action decision (§3 — two
> explicit pending decisions: retire `fuse serve` in favor of the GitHub
> Action, and weigh MCP vs CLI for fuse's agent surface), merge-quality
> items (§4: bench corpus, rename-aware *merge* resolution,
> container-aware nested merge, import semantics), distribution (§5:
> Homebrew, Action hardening, Windows resolve), and Shale evidence
> emission (§2).

---

## 1. The four-piece story (hypothesis)

Grove, Prism, Fuse, and Shale each remove a different bottleneck on running
many coding agents against **one repository**:

| Piece | Bottleneck it removes | In database terms |
|-------|----------------------|-------------------|
| Prism | context windows can't hold the repo (read path) | the query planner / working set |
| Fuse  | parallel branches collide (write path) | the commit protocol |
| Grove | nobody knows what a change breaks | the consistency checker |
| Shale | humans can't review the volume | the transaction log |

Individually each is a nice tool. Together they are something nobody ships
today: **optimistic concurrency control for coding agents**. Git was designed
for human concurrency at human cadence — pessimistic, serialized, review
-gated. This stack turns a repository into something closer to a database
with MVCC: N agents work simultaneously, conflicts are resolved at symbol
granularity instead of blocking, every integration is checked against the
code graph, and the human reads a stream of evidence instead of a queue of
diffs.

The concrete piece nobody has imagined — and the thing worth building to
prove the story — is the **stale-context loop**:

> When Fuse merges agent A's branch, Grove can diff the code graph
> before/after the merge. Prism knows which symbols are in agent B's working
> set. Compose them and agent B gets told, *mid-task*: "the ground shifted
> under you — `Login()` and `SessionStore` changed, blast radius touches 2
> files you're editing; here is the 40-line delta that matters to you."

Today every agent discovers drift at merge time (or worse, at runtime). With
this loop, drift is delivered as a push notification with a minimal context
patch. That is optimistic concurrency *with conflict pre-warning* — the thing
that makes a 10-agent fleet on one repo feel like one fast developer instead
of ten colliding ones. No orchestrator framework has this because none of
them own the merge layer, the graph, and the context delivery at once.

Pitch-sized version: **"Git made human collaboration scale. Shale+Grove+
Prism+Fuse make agent collaboration scale — same repo, same git, no new
platform."**

## 2. Shale integration (the family tie that matters)

Shale's card answers "what did the agent do and can I trust it?" Fuse owns a
piece of evidence nobody renders today: **integration evidence**.

- **Merge events as Shale evidence.** Fuse already writes
  `.git/fuse/audit.json` (strategy, confidence, validation result, breaking
  changes per merge). Emit the same record in a Shale-consumable form so a
  card can say: *"merged alongside 2 concurrent agent branches — 5 conflicts
  auto-resolved at symbol level (all re-parsed clean), 1 escalated to agent
  resolution (prompt attached), 0 breaking changes to exported symbols."*
  Mechanics: either Shale's session recorder picks up fuse's audit file, or
  fuse writes an event into Shale's store directly when a session is active.
  Match Shale's posture exactly: advisory, fail-open, local-first, no server.
- **`fuse resolve --agent` runs are sessions.** An AI resolving a conflict is
  exactly the kind of agent action Shale exists to make visible: intent (the
  handoff prompt), evidence (the validated resolution), gaps (none — the
  validation gate is machine-checkable). Record it.
- **CertifyDiff, reframed.** Grove v0.5 exposes `CertifyDiff` /
  `CertificationReport`. Use it as an *evidence generator for Shale cards*
  (machine-checked findings about a merge result), not as an admission gate.
  Same API, opposite posture: advisory, never blocking.

## 3. Agent wiring: MCP vs HTTP

`fuse serve` (HTTP) already exists. The question is what agents should talk
to. Recommendation:

- **Ship `fuse mcp` (stdio MCP server)** following the house pattern
  (`prism mcp`, grove embedded). Agents don't want ports, tokens, or
  daemons; stdio MCP is zero-config and per-workspace. Tools:
  - `fuse_merge_check {branch|ref}` — "will my work merge cleanly against
    main / against agent-B's branch?" — dry-run, returns per-file strategy,
    conflicts, breaking changes. This is the killer tool: agents check
    *before* pushing, turning merge conflicts from failures into feedback.
  - `fuse_preview {base, ours, theirs, path}` — three-way merge of in-memory
    content (what `POST /merge` does today).
  - `fuse_resolve {prompt_path}` — return the handoff prompt content +
    structured conflict JSON so the agent can resolve in-context; an
    `apply` parameter writes the validated resolution.
  - `fuse_impact {file|symbol}` / `fuse_check {file}` — Grove blast radius
    and breaking-changes-vs-HEAD, so agents can self-check before
    finishing a task.
- **Keep HTTP for machines that aren't agents** — CI jobs, editor plugins,
  the GitHub Action. It's already there; don't grow it beyond `/merge` +
  `/health` until something needs it.
- **Later (stale-context loop, §1):** the MCP server is also the natural
  delivery channel for drift notifications once Grove graph-diff exists.
  Don't build the notification plumbing until the simple tools above are
  used in anger.

## 4. Merge quality (measured by `fuse bench`, gated on zero regressions)

Current honest numbers on gin's history: 100% parity with git's auto-merges;
4/16 git-conflicted files resolved, 3 byte-identical to human. Headroom, in
expected-value order:

1. **Bench corpus expansion** — TS/Python-heavy repos with merge-commit
   history; publish per-language tables in the README. Do this *first* so
   every later change is measured against more than Go.
2. **Rename detection** — symbol with identical/near-identical body under a
   new name on one side + edits on the other is currently a conflict;
   body-similarity matching turns it into a clean merge (`STRUCTURAL` class
   actually earning its name).
3. **Container-aware nested merge** — today a class whose methods diverge on
   both sides is resolved by symbol-body LCS; a true nested symbol merge
   (per-method three-way inside the class shell) lifts TS/Python/Java
   resolution further.
4. **Import semantics** — one-side import removal currently survives via
   union; honor removal when the other side didn't touch it.

## 5. Distribution

- **Homebrew**: join the existing tap — `brew install provasign/shale/fuse`
  (one tap for the family; shale already trained users to trust it).
- **GitHub Action hardening**: the action shipped in v0.5.0; add a follow-up
  step example that runs `fuse resolve --agent` on the handoff prompts so
  the README shows the fully-automated path end to end.
- **Windows**: `fuse resolve --agent` shells through `$SHELL`/`/bin/sh`;
  needs a cmd/PowerShell equivalent.

## 6. Explicitly not doing

- Anything that couples fuse to provasign's certified-admission workflow
  (paused product; advisory-evidence posture only, via Shale).
- Growing the HTTP API into a daemon agents depend on.
- Auto-resolving with lower confidence to chase the resolution-rate number —
  the two invariants (git parity, validated output) outrank the headline
  rate.
