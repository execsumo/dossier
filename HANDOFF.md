# Dossier / chainlink — Handoff

> Updated: 2026-09-02
> Purpose: the entry point for the dev agent picking up implementation. Read this first.

## Start here (reading order)

1. **This file** — status, decisions, how to start, watchouts.
2. **`BUILD-DECISIONS.md`** — the implementation choices that were open in the SPEC are now settled. Do not relitigate them.
3. **`ARCHITECTURE.md`** — how the code is structured (Go, ports-and-adapters). Seeded; you maintain it as you build.
4. **`CLAUDE.md`** (= `AGENTS.md`) — repo working rules, build/test commands, hard rules, definition of done.
5. **`SPEC.md`** — the contract: data model, CLI (§7), MCP (§8), algorithms (§11), acceptance criteria (§14), milestones (§15).
6. **`PRD.md`** / **`PRFAQ.md`** — the product *why*, when a decision's intent is unclear.
7. **`VISION.md`** / **`PLANv02.md`** — where the product is going next (the team dimension) and the plan for getting there. Written for a business audience; read them before starting v02 work.

Precedence when docs disagree: `BUILD-DECISIONS.md` > `SPEC.md` (mechanics) > `PRD.md`/`PRFAQ.md`.

## Current state

> **Stage: All Milestones Completed (Project Fully Finished) + Lead/Interface Meeting Prep.** The entire Dossier durable memory layer (codename *chainlink*) is fully implemented, verified, and integrated across all surfaces (CLI, MCP, and the Bubble Tea TUI).
> - **Milestone 1–5:** Core file store, CLI, recall, warnings, lexical search/suggestions, promote/link flow, and the MCP stdio server are implemented.
> - **Milestone 6:** Active session binding, hook installation for Claude Code, confirmation prompts, capability detection, non-clobbering configurations, and the interactive **Rich TUI** (dashboard, detail recall view with native markdown rendering and fsnotify hot-refreshing, status/priority/next-action inline editing, ambiguity link resolution, and syntax-highlighted merge conflict resolving) are fully completed.
> - **Lead Tracking & Accountability:** A newly added `Lead` field allows assigning team members to specific dossiers. Supported end-to-end through `dossier promote --lead`, `dossier lead`, `dossier_update` MCP tool, and the TUI's inline `l` (lead) editor.
> - **Milestone 7:** Optimistic concurrency control, non-overlapping frontmatter auto-merging, DP LCS unified diff body conflict generation (writing to `conflicts/`), and `dossier merge` CLI/Service commands are verified.
> - **Milestone 8:** The final Distillation Guide is authored in `assets/guide.md` and embedded in the binary to be written to `~/.dossier/context/guide.md` upon initialization. It has been upgraded to employ rigorous linguistic compression (syntactic pruning, lexical density, and negative space framing). All dogfooding validations, test sweeps, and PRD success metrics have been fully met.
> - **Stable Install & Auto-MCP Configuration:** Implemented stable binary self-install path command (`dossier install`, default `~/.local/bin/dossier`), volatile path detection on `init`, and auto-registration of both the MCP stdio server and lifecycle hooks in Claude Code's user/global configuration files (preserving existing third-party configs and backing up changed files).
> - **v1 harness support (2026-08-03, revised 2026-08-04):** Claude Code remains fully integrated. Pi is supported through a `PiHarness` plus the Dossier Pi extension Dossier now installs itself (session identity today; lifecycle bridging still owed — see "Pi integration" below). Codex and Antigravity remain out of scope. The `Harness` interface/registry remain extensible.
> - **In-session Dossier switching fixed (2026-06-16, ADR 0003):** Previously the MCP `dossier_session` tool (consolidating switch/active) required a `session_id` the agent had no way to obtain, so an agent could not change (or even read) its active Dossier from inside Claude Code. Root cause + fix: Claude Code sets `CLAUDE_CODE_SESSION_ID` in the MCP server's env (verified identical to the transcript UUID and hook stdin `session_id`). A shared `harness.ResolveSessionID` now resolves the session at the adapter edge (precedence: explicit → `CLAUDE_CODE_SESSION_ID` → `PI_SESSION_ID` → `DOSSIER_SESSION` → `sess_default`); MCP degrades visibly instead of falling back to the shared bucket. An agent can now call `dossier_session` with just a slug; per-session isolation is preserved. See `docs/harness-capabilities.md` (§ MCP Session Identity) and ADR 0003.
> - **Hardening (2026-06-17):** Addressed critical system stability and performance risks. Prevented OOM crashes by streaming artifact frontmatter instead of loading large bodies. Fixed severe O(N) performance degradation by bypassing full YAML parsing with fast-path string lookups during directory scans. Ensured strict history preservation, bounded LCS diff matrices for massive conflict files, resolved symlink-destructive writes in the harness adapter, and fortified non-interactive stdin blocking. Enhanced `Doctor` with comprehensive provenance validation, artifact origin checks, and unresolved conflict reporting. Improved `SessionEnd` safety by adding explicit audit logging for missing payload scenarios.
> - **TUI de-sessioned (2026-06-17, ADR 0004):** The TUI no longer resolves or carries a session identity and no longer exposes the per-session active binding. Removed the `a` ("make active") key, the `★` active marker/column, the `Session:`/`Active:` header fields, and the standalone-session footer warning — these only ever acted on the `sess_default` bucket that no live agent session reads (the "make-active does nothing for me" / "fixed Session value" confusion). The TUI is now purely a reactive browse/edit viewer (list, markdown recall with live hot-refreshing, status/priority/next-action inline editing, link, syntax-highlighted merge). `Service.Switch`/`Active` are unchanged and remain the CLI/MCP surface for per-session binding. Narrows B9 (see ADR 0004); supersedes ADR 0002 and obsoletes `docs/tui-plan.md`'s catch-up section.

> - **Cleanup & hardening (2026-06-24):** Code-review follow-ups. (1) **Unified write path** — removed `Service.SetStatus`/`SetLead`; CLI, MCP, and TUI now route all metadata edits (status, lead, …) through `Save`, eliminating an adapter fork that violated the "thin adapters over one core.Service" rule. `Save`'s audit entry now records a field-level `field "old"→"new"` summary, and still emits a `status_changed` event (not a generic `save`) when the lifecycle status changes, preserving the agent-facing provenance the dedicated commands had and satisfying SPEC §300. (2) **Quality gate wired up** — added `.github/workflows/ci.yml` (gofmt + `go vet` + `go test` + the long-documented `internal/core` dependency-direction guard); removed a stray `test_glamour.go` scratch file and a gofmt-dirty commit that the gate now catches. (3) **TUI polish** — markdown renderer cached (rebuilt only on width change); hot-refresh now watches every dossier directory so the dashboard live-updates, not just the detail view; `store.Lock` simplified to a single blocking `flock.Lock`. (4) Renaming via `dossier_update` keeps the slug stable (documented in SPEC §8.1).
> - **Simplification of Priority and Dashboard (2026-06-25):** Dropped the cumulative points-based priority scoring in favor of a clean 2x2 Eisenhower matrix (1-4: Do, Plan, Delegate, Delete) strictly mapping importance (high/low) and urgency (high/low). In the TUI dashboard, replaced the Staleness column with a 'Due' column formatted as MM/DD, and expanded the Priority column width to 12 characters to display the full Eisenhower matrix labels.
> - **Programmatic Context Injection (2026-06-25, expanded 2026-06-26):** Migrated from passive injection (`guide.md` printed in `session-start` or requiring an explicit file read) to active interception via the `dossier_session` MCP tool payload. When an LLM calls `dossier_session` to bind or retrieve a dossier, the MCP handler intercepts the `Service.Switch`/`Active` response and wraps it in a struct containing the state AND the full string contents of `guide.md`. This zero-tax, seamless programmatic pattern guarantees strict distillation adherence without polluting global Claude Code prompts (`skill.md`) with a 1500-token overhead on generic tasks. On 2026-06-26, this interception pattern was extended to include runtime operating instructions (`instructions.md`), vastly shrinking the global `skill.md` context bloat down to just a trigger instruction.

> - **TUI Lead landing screen (2026-06-29):** The TUI now opens on a `ViewLeadSelector` landing screen for meeting prep: pick a Lead to scope the dashboard to, with "All" and "Unassigned" pinned first and search-as-you-type narrowing the list (each row shows its dossier count). Selecting a lead drops into the dashboard filtered to that owner; `f` reopens the selector, and the dashboard subtitle shows the active scope. Implementation notes: (1) the filter is a typed `leadFilter{kind, name}` value (not a sentinel string), so a lead literally named "All"/"Unassigned" can't collide with the pinned modes; (2) a single `Model.visibleItems` (the full `items` narrowed by the filter) is the source of truth every table-cursor lookup indexes into — eliminating the row↔item desync that would otherwise hit `enter`/`m`/`getTargetDossier`; (3) lead enumeration/filtering are pure, table-tested helpers (`deriveLeadOptions`, `filterLeadOptions`, `leadFilter.matches`). The dashboard list now fetches **all** statuses (a lead's resolved/archived dossiers must be on hand for prep), and a `statusTier` sort keeps live work (active/waiting/blocked) above terminal work (resolved/archived) so the all-status fetch never buries open work under a high-priority archived item. All logic stays in `internal/tui`; `core`/CLI/MCP are untouched. The selector list is **windowed** (`leadWindow`/`leadVisibleRows`): only a height-bounded slice around the cursor renders, with `↑ N more above` / `↓ N more below` indicators, so a long lead list scrolls instead of overflowing the screen.

> - **Homebrew distribution (2026-07-08):** Added `Formula/dossier.rb` to a shared personal tap, `execsumo/homebrew-tap` (renamed from the Heard-only `homebrew-heard`, so it can host formulas/casks for any of the user's projects going forward). `dossier install` via `~/.local/bin` still works and takes PATH precedence if present, but `brew tap execsumo/tap && brew install dossier` is now the recommended path for keeping multiple devices current. `.github/workflows/release.yml` gained an `update-tap` job that runs after every tagged release: it reads that release's `checksums.txt` and regenerates `Formula/dossier.rb` (version + per-platform `sha256`/`url` for darwin/linux × arm64/amd64) and pushes it to the tap via a `GH_PAT` repo secret (fine-grained PAT, contents:read/write scoped to `homebrew-tap`). From then on `brew upgrade dossier` tracks latest with no manual step. README's Quickstart now leads with this option.
> - **Discussion interfaces (2026-08-05, release v0.1.14):** Added optional multi-select `Frontmatter.Interfaces` with the canonical values `Pricing WBR`, `1:1`, `OLG Standup`, `Growth Standup`, `Steerco`, `Solutioning`, and `OpsRev`. The shared `Service.List` filter uses **ANY** semantics. CLI supports `dossier promote --interface`, `dossier interface`, and `dossier ls --interface`; MCP exposes `interfaces` on `dossier_promote`, `dossier_update`, and `dossier_list`; the TUI cycles interface scope with `i` and combines it with the Lead filter. The field is serialized in YAML, included in revisions, validated, surfaced in list/recall/detail output, and backward-compatible when absent.

> - **SessionStart slimmed to a one-line nudge (2026-07-15):** Dogfooding found the unbound-session branch of `Service.SessionStart` — a full open-dossier bulletlist (name/status/slug/priority per line) plus a 3-step "check before creating" instruction block — was steering the agent toward thinking about Dossier on every single session, including ones with nothing to do with it, since the hook fires unconditionally on session start. Replaced it with a single-line nudge (`N open dossier(s): <names>. Use dossier_list ... dossier_session ... dossier_promote ...`). This isn't a capability loss: `dossier_promote` already runs its own similarity check server-side and returns `next_actions` guiding the agent through ambiguous matches (see `Service.Promote`), so the old instructional prose was purely redundant with behavior the tool already has. The **bound**-session branch (active dossier's full Distilled State + Distillation Guide inlined) is unchanged — a session with an explicit binding has earned the heavier payload. This is the same "active interception, zero-tax" principle documented under **Programmatic Context Injection** above, now applied to the hook's default-empty path too.

> - **Pi harness support (2026-08-03):** Added a Dossier-side `PiHarness`. It detects the user's existing Pi Claude-like hooks extension through `PI_SESSION_ID`/`PI_SESSION_FILE`, resolves `PI_SESSION_ID` for MCP and hooks, reads `PI_SESSION_FILE` as the transcript fallback when hook stdin does not include one, and records Pi-specific transcript provenance. Dossier does not install or modify Pi hooks; the existing extension owns lifecycle invocation. **Partly superseded 2026-08-04 — see the next entry: the environment-variable assumption did not survive contact with Pi.**
> - **Dossier Pi extension + session identity (2026-08-04, ADR 0005):** Verified against Pi 0.83.0 that `PI_SESSION_ID`/`PI_SESSION_FILE` are set **only for bash-tool children** (Pi strips and re-adds them per invocation), and that Pi has **no built-in MCP client**. So a Dossier MCP server under Pi had no session id at all — `ErrNoSessionID`, no binding, no switching — and any process spawned once kept a stale id across `/new`, `/resume`, `/fork`. Dossier now **ships its own Pi extension** (`assets/pi-extension.ts`, embedded), installed by `dossier init` when Pi is detected on the device, or by `dossier harness install pi` when Pi is added later. It publishes a per-Pi-pid **session pointer** and mirrors the identity into Pi's process environment; Dossier resolves it by walking its own process ancestry, which keeps concurrent Pi sessions isolated. Session-id precedence gains the pointer at step 4, resolution now **names its source**, and `Service.Switch` records that harness on the binding (a Pi session is no longer filed as Claude Code with Claude Code's capabilities). Capability reporting turned honest: Pi reports `Installed`/`SessionIdentity` and reports MCP + lifecycle hooks as unavailable, because Dossier does not provide them for Pi yet — surfaced by `init`, `dossier harness list`, and a non-fatal `doctor` advisory. `init` also stopped letting the last harness scanned answer for all of them (`data["harnesses"]` is now per-harness). **Lifecycle bridging for Pi remains out of scope** (see "Pi integration" below). Verified end-to-end against a real Pi session: the extension published the true session UUID and JSONL path, and a child process with no session environment resolved it, bound a Dossier, and read the binding back.
> - **Resolvable citations & the Archive as a full view (2026-09-02):** Dogfooding found the Distilled State coming out *thin* — technically well-cited, practically unrecoverable. Root cause was structural, not prompt quality: `guide.md` instructed destructive write-time compression ("Do not log the mechanics of the work; log the result") into a single fixed view, while the `[src:art_x#L42-L68]` pointers it mandated were **dead ends**. `Store.ReadArtifact` existed but was never surfaced through `Service`, MCP, or the CLI; the `#L42-L68` fragment was matched by a non-capturing group and silently discarded; and `RecallResult` returned the curated view with no artifact list, leaving the Archive invisible. An author who cannot follow a pointer rationally inlines everything or loses it. Fixes, informed by the VCC trace-compiler paper (arXiv 2603.29678, "one lossless IR, many views"): (1) **`Service.ReadArtifact`/`ListArtifacts`**, surfaced as `dossier_artifact`/`dossier_artifacts` (MCP) and `dossier artifact <dossier> [<artifact>] [-L a-b]` (CLI) — a citation now resolves to line-numbered verbatim source. (2) **`core.CompileTranscript`** lowers raw harness JSONL into role-tagged nodes (`user`/`assistant`/`thinking`/`tool_call`/`tool_result`) before archiving, so a cited range lands on a turn rather than mid-JSON; the raw trace still goes to the session stash, and bookkeeping records are *counted* in the compiled header rather than dropped silently. (3) **Citation grammar validated** — `internal/core/provenance.go` parses the line-range fragment and `doctor` flags a range that does not fit the artifact. (4) **A floor to match the ceiling** — `uncitedArtifactWarning` surfaces archived evidence the Distilled State never points at, via `Save`, `Recall`, and a `doctor` advisory. (5) **`splitContentLines`** is now the one canonical line split shared by citations, search hits, and fetches; it fixed a bug where a range ending on a blank line silently returned fewer lines than `EndLine` claimed. (6) **`guide.md` rewritten** — compression reframed as a rendering decision, a never-compress list for values (ids, paths, numbers, error text, commands), deferred compression for recent material, `[observed]`/`[assumed]`/`[decided]`/`[rejected]` role tags, a `## Evidence` index section, guidance to reach for `decision_evidence`/`file_snapshot`/`query`/`link` artifacts instead of citing a whole transcript, and a worked "dense but thin" counter-example. See `ARCHITECTURE.md` §9b.
> - **Resolvable-citations follow-up fixes (2026-09-02):** A code review of the previous entry's change (12 findings) landed in three tiers; tier 1 (release blockers) is merged, tiers 2–3 are in review. **Tier 1:** (1) `Service.ReadArtifact` rejects `start_line > end_line` before slicing — the `#L`-range path already guarded against a reversed range via `ParseProvenanceRef`, but the explicit `start_line`/`end_line` request parameters did not, so `dossier_artifact {"start_line": 8, "end_line": 3}` panicked the MCP stdio server (`slice bounds out of range`). (2) `Artifact.Lines` is now persisted in frontmatter at write time (`FSStore.WriteArtifact`) and read by `evidenceIndex`, so `Recall`/`ListArtifacts` no longer falls back to reading every artifact body just to count its lines — the frontmatter-only list path was always returning a zero count, silently defeating the "Archive artifacts are not loaded by default" guarantee (SPEC §14.2). (3) `bodyContentLines` now exempts the `## Evidence` section heading and `[assumed]`-tagged lines from the provenance validator, so `assets/guide.md`'s own worked examples pass `dossier doctor` instead of tripping "missing provenance" on the format the guide itself mandates.
> - **`dossier-delegate` skill bundled (2026-08-04):** The skill is now embedded (`assets/dossier-delegate-skill.md`) and installed by `dossier init`/`ClaudeCodeHarness.Install` into `~/.claude/skills/dossier-delegate/SKILL.md`, following the same idempotent/backed-up/single-confirmation contract as the rest of `Install`. It remains explicit-invocation-only and is never wired into `customInstructions`/session-start auto-injection.

All features (CLI, MCP, and Rich TUI) are fully operational, tested, and integrated.

## Pi integration (Dossier ships its own extension as of 2026-08-04)

> **Supersedes the "Pi Extension Contract" written on 2026-08-03**, which assumed
> the user's own Claude-like hooks extension would supply everything through
> `PI_SESSION_ID`/`PI_SESSION_FILE` in the process environment. Reading Pi 0.83.0
> showed those variables reach **bash-tool children only**, and that Pi has **no
> built-in MCP**. See ADR 0005 and `docs/harness-capabilities.md` §2.

**What Dossier now installs.** `dossier init` (and `dossier harness install pi`
for a Pi added later) writes the bundled extension — `assets/pi-extension.ts`,
embedded in the binary — to `<pi agent dir>/extensions/dossier/index.ts`,
idempotently, backing up anything it replaces and confirming first (B7/B8).

**What it does.** On every `session_start` it publishes a session pointer at
`<pi agent dir>/dossier/sessions/<pi-pid>.json` (`session_id`, `session_file`,
`cwd`, pid) and mirrors the identity into the Pi process environment for
children spawned afterwards. Dossier walks its own process ancestry to find the
pointer of the Pi process that owns it, so concurrent Pi sessions stay isolated.
Session-id precedence: explicit → `CLAUDE_CODE_SESSION_ID` → `PI_SESSION_ID` →
**Pi pointer** → `DOSSIER_SESSION` → `sess_default` (never for MCP).

**Still owed for Pi (deliberately out of scope this pass):** lifecycle bridging.
The extension does not yet invoke `dossier hook session-start|session-end|
pre-compaction`, so Pi sessions get no session-start injection, no end-of-session
capture and no pre-compaction save. Detection reports those as unavailable
rather than implying them. When that work happens, the bridge must: fire
pre-compaction *before* Pi discards context; fire session-end before the process
exits; pass JSON on stdin with `session_id`/`transcript`/`distilled_state`
(Dossier falls back to the pointer's `session_file` when `transcript` is
omitted); preserve exit status rather than swallowing failures; and avoid
duplicate calls for the same boundary. The Pi session file is archived as Pi
JSONL — never reinterpreted or mutated.

## Current initiative: Team Sync (started 2026-07-15, branch `team-sync`) — IN PROGRESS

**What/why:** v1's "single-user" premise is amended (owner decision, 2026-07-15): multiple colleagues' sessions will contribute to one shared store, with session captures stashed per-dossier so nothing is lost. Transport is a **private GitHub repo fully hidden behind the binary** (embedded go-git; colleagues never see git). Chosen over a shared-drive live store because drive sync clients last-write-wins behind our backs, violating two hard rules. Decision record: **[ADR 0005](docs/adr/0005-team-sync-via-github.md)** + **B12** in `BUILD-DECISIONS.md`.

**Plan:** `docs/team-sync-plan.md` — 4 phases, strictly ordered, one PR each off the `team-sync` integration branch (merge to `main` only when the whole feature dogfoods clean):
1. **Phase 1 — Identity & store layout v2** (no networking): `author` config field; author-stamped audit events; per-author audit shards `<slug>/audit/<author>.log` (legacy `audit.log` stays readable, never rewritten); per-dossier session stash `<slug>/sessions/<author>/<session-id>.md`; schema bump + additive `Migrate`; SPEC §3.2 + ARCHITECTURE updates in the same PR.
2. **Phase 2 — `Syncer` port + go-git adapter + manual `dossier sync`**: pull→resolve→commit→push; `dossier.md` sync conflicts route into existing `WriteConflict` (`kind: sync_concurrent_edit`, remote wins working tree); PAT auth `0600`; >100 MB artifacts excluded from sync with visible warning; tested against local bare repos.
3. **Phase 3 — Onboarding + auto-sync**: `dossier team create|join`; sync piggybacks on Recall/Save/session hooks (async/debounced, never blocks); TUI/doctor/MCP surfacing. No daemon.
4. **Phase 4 — Docs, dogfood, hardening**: README for the least technical teammate, PRD/SPEC amendments, failure drills, two-colleague pilot.

**Status (2026-07-16): Phases 1–3 all DONE & integrated into `team-sync` (HEAD `3f16559`). Team Sync is feature-complete for local/offline use, `go test -race ./...` green, dogfooded end-to-end through the real binary. Only Phase 4 remains, and its core items (real-GitHub PAT flow + two-colleague pilot) need the owner — see the bottom of this initiative for the exact resume point.** Detail below, newest phase last.

**Status:** Plan docs committed (this entry, ADR 0005, B12, plan doc, CLAUDE.md premise update). **Phase 1 (Identity & store layout v2) implemented, reviewed, and merged into `team-sync` (2026-07-15)** — delivered by an antigravity delegate, verified against the DoD (build/vet/gofmt clean, 94 tests incl. two-author isolation + legacy-audit-untouched; transcript discovery lives in `harness.ResolveTranscript`, preferring the hook payload's `transcript_path`).

**Spec/docs alignment merged (2026-07-15):** PRD/PRFAQ/SPEC single-user premise amended (dated, history preserved); SPEC §7 gained the `dossier sync` / `dossier team create|join` command surface (marked specified-ahead-of-implementation); SPEC §14.10 Team Sync acceptance criteria added; §4.4 corrected to per-author audit shards; guide.md gained a Multi-author Dossiers etiquette note. Delivered by an antigravity delegate, DoD-verified, merged.

**Phase 2 de-risk spike (go-git sync engine) — DONE & reviewed (2026-07-16).** Delivered by a cline delegate on `delegate/cline-gitsync-spike` (commit `e461156`), scope-fenced to `internal/sync/**` + `go.mod`/`go.sum` + `docs/spikes/gitsync-findings.md`. Prototypes pull→resolve→commit→push against local bare repos, remote-wins conflict capture on `dossier.md` (`ConflictRecord`, zero git merge markers), >100 MB exclusion, store-wide `.sync.lock` — NOT wired into `core.Service`. **Reviewed & independently verified** (build/vet/gofmt clean, 8/8 tests). Findings: `docs/spikes/gitsync-findings.md` (excellent — honestly names its own gaps and gives an 11-point Phase 2 recommendation list). Key go-git finding: `Worktree.Pull` has no per-file remote-wins policy and writes merge markers on divergence, so the spike bypasses it with a custom 3-way remote-wins merge (`DiffTree` + `MergeBase` + per-file blob checkout + a manual 2-parent merge commit).

**Spike reconciled onto a Phase 2 base (2026-07-16):** the spike branched 9 commits behind `team-sync` (base `8b84978`, before Phase 1 + docs-alignment landed). Rebased the single spike commit onto current `team-sync` → branch **`feat/teamsync-p2`** (spike commit `970baaa`). Rebase was conflict-free (Phase 1 never touched `go.mod`/`go.sum`, so the go-git dep diff replayed cleanly). Verified the reconciled base: full-repo `go build`/`go vet`/`gofmt` clean, **102 tests pass**. `feat/teamsync-p2` is the known-good base the Phase 2 wiring delegate builds on.

### Decisions log — owner away 2026-07-16 (decision authority delegated; discuss on return)

- **D1 — Do NOT merge the spike into `team-sync` standalone.** An unwired `internal/sync` package on the integration branch is off-plan ("one PR per phase, each a wired milestone"). Instead the Phase 2 PR lands `internal/sync` *already wired* via `core.Syncer`. Base = `feat/teamsync-p2` (spike rebased onto team-sync).
- **D2 — Phase 2 stays one coherent PR/one delegate; do not fragment it** across agy+cline (fragmenting a single phase creates intra-phase merge seams — the thing that causes rework). agy owns all of Phase 2 including PAT auth.
- **D3 — Delegate routing:** Phase 2 wiring → **agy** (owner's stated default for sequential work). Parallel non-colliding track → **cline** (reserved for genuine parallelism). See memory `prefer-agy-delegate-default`.
- **D4 — cline's parallel track = docs-only, brand-new files** (the only guaranteed zero-collision surface while Phase 2 rewrites code): Phase 4 teammate-facing onboarding + failure-drill runbook as new `docs/` files, describing the already-SPEC'd §7 command surface. Light post-Phase-2 reconciliation is cheap; code collision is impossible. **[DONE — integrated `b114ede`.]**
- **D5 — Post-integration dogfood of `dossier sync` (advisor-prompted) caught a real bug the tests missed; fixed inline** (`bee5507`). Driving the actual cobra→`Service.Sync` path against two throwaway local stores showed the normal onboarding flow (each colleague `dossier init`s their own store → no shared git ancestor) flagged the identical, managed `.gitignore` as a phantom sync conflict. Fixed in `remoteWinsMerge`: only capture a conflict for `<slug>/dossier.md` paths with genuinely differing content; managed/single-writer files take remote-wins silently. Regression test added. I fixed this myself rather than round-tripping a delegate — it was a ~10-line, precisely-root-caused change (delegate-skill rule: don't delegate a quick edit you'd finish before writing the spec). Genuine dossier.md conflicts unaffected (e2e test still green).
- **D6 — Phase 3 split by risk profile; run agy solo, no forced cline parallelism** (advisor-aligned). 3a = `dossier team create|join` (clean/testable → delegate now). 3b = auto-sync on hooks ("async/debounced/never-blocks", behavioral) → spec only after 3a's surface is real, with concrete non-blocking/debounce/failure checks. This is *sequencing two risk profiles*, not fragmenting one phase (respects D2). **No cline parallel track this phase:** Phase 3a spans CLI + config + `internal/sync` (Clone/create), leaving no collision-free surface for a second writer — forcing cline on would manufacture collisions or docs-about-unbuilt-commands (churn). Honest call over busywork; cline resumes at 3a-land with README/onboarding polish against the *real* command surface. (Owner may override if throughput is preferred over churn-avoidance.)
- **D7 — Phase 3b process model: three INDEPENDENT best-effort syncs, no IPC** (agy escalated a real flaw in my spec; the escalation valve working). SessionEnd (short-lived hook process) CANNOT flush the MCP server's in-memory debouncer — they're separate OS processes. Resolution: (1) MCP debouncer drains on its OWN stdin-EOF shutdown; (2) SessionEnd hook does its own push of on-disk state; (3) SessionStart hook does its own pull. They can't race destructively because Phase 2's store-wide `.sync.lock` serializes every `Sync()`. No coordination needed. **On-disk store is the only shared state between processes.**
- **D8 — Auto-sync failures are intentionally silent (`_, _ = s.Sync`), surfaced via `dossier doctor`, not per-failure warnings.** Best-effort + local-first = no data loss; `doctor` shows ahead/behind/last-sync/conflicts, and the runbook points users there. Per-failure warnings on every transient background sync would be noise. **Logged as revisitable** if dogfooding shows users miss sync failures.
- **D9 — Two more real bugs caught by real-binary dogfood of the 3b flow, fixed inline** (`3f16559`): (a) the CLONE-based onboarding path (`team join` → shared history → fast-forward pull) hit `ffToRemote`'s `wt.Checkout(Force:true)`, which **deletes gitignored machine-local files** — so a joined colleague's first fast-forward pull silently DELETED their `config.yaml`, un-teaming them (syncer→nil, auto-sync + doctor-sync-section silently died). Fix: stash machine-local paths across the Force checkout + restore. (b) same root cause made `doctor`'s sync section vanish. Phase 2's tests never hit this because they used independently-init'd stores (divergent-merge path, never fast-forward). Regression test `TestSync_FastForwardPreservesMachineLocal`. **Pattern worth noting for Phase 4: the CLONE onboarding path and the fast-forward pull path are under-tested vs. the divergent-merge path — real-GitHub dogfood should exercise them.**

**Phase 2 wiring: DONE & INTEGRATED into `team-sync` (`6d3e559`, 2026-07-16)** — delivered by an agy (antigravity, Gemini 3.1 Pro) delegate, verified + integrated by the orchestrator. `core.Syncer` port in `internal/core/ports.go` (core stays pure — dependency-direction guard passes), core-owned `SyncReport`/`SyncConflict`/`SyncStatus`/`SyncExcluded` DTOs, `internal/sync/adapter.go` mapping `GitSync`→port, `Service.Sync`/`SyncStatus` orchestration, thin `dossier sync` CLI (`--status`/`--json`), sync-conflict routing into the existing `WriteConflict` machinery (`kind: sync_concurrent_edit`, one mechanism/two triggers), `internal/sync/credentials.go` (PAT `~/.dossier/credentials` exact-0600 + `gh auth token` fallback, injectable runner), `team.remote`/`team.branch` config, gitignore machine-local set (`/config.yaml`, root `/sessions/`, `/context/`). End-to-end test `internal/cli/cli_sync_test.go` (two clones converge; concurrent `dossier.md` edits → distinct `conflicts/*.md` remote-wins, zero markers; >100 MB excluded; nil-syncer errors cleanly). **Build/vet/gofmt clean, 105 tests, purity guard passes.** ARCHITECTURE.md §2/§4 document the port. Review caught + fixed: a `confID` collision (time-only ID overwrote a 2nd same-sync conflict → now slug+index unique, with a 2-conflict test); a gofmt nit; agy's missed ARCHITECTURE update (added by orchestrator). Auth is unit-tested only (real-GitHub PAT flow is a Phase 4 dogfood item — local bare repos need no auth). The now-stale `feat/teamsync-p2` base branch and `delegate/*` branches can be pruned.

**Phase 2 is now genuinely closed** — validated end-to-end through the real binary (build → `dossier init`/`promote`/`sync` on two local stores → convergence, remote-wins, zero phantom conflicts), plus the `bee5507` fix + regression test above. `dossier sync` HEAD on `team-sync` is `bee5507`.

**Phase 3a (`dossier team create|join`): DONE & INTEGRATED into `team-sync` (`aa815dd`, 2026-07-16)** — delivered by an agy delegate, verified + integrated by the orchestrator. `team create <url>` inits the store as a git repo (HEAD `main`) + origin remote + pushes existing content; `team join <url>` shallow-clones (depth 50) the remote into the store then runs `Service.Init` (harness/layout), guarding a non-empty target. `core.Syncer` port extended with `Create(ctx)` + `Clone(ctx,url,dir,depth)` (core stays pure); thin CLI (`team create`/`join`) loads→sets→saves `config.Team`→wires→calls `Service.TeamCreate`/`TeamJoin`. **Verified via real-binary dogfood** (create pushes A's store; join clones into B with zero phantom conflicts; join-into-non-empty and unreachable-remote both error cleanly, no panic) + a committed round-trip integration test (`internal/cli/cli_team_test.go`, written by the orchestrator — agy had only run a manual script). Build/vet/gofmt clean, tests pass, purity guard passes. ARCHITECTURE §3/§4 updated.

**Phase 3b (auto-sync on hooks): DONE & INTEGRATED into `team-sync` (`88eb13e` feature + `3f16559` fix, 2026-07-16)** — delivered by an agy delegate, verified + integrated by the orchestrator. Auto-sync at three trigger points (all best-effort, non-blocking, no daemon): `SessionStart`→bounded pull, `SessionEnd`→bounded push (short-lived hook processes); the long-lived `mcp serve` runs a **debounced background sync goroutine** (2s coalesce) triggered by `dossier_save`/`dossier_recall`, drained on stdin EOF. CLI stays non-blocking (no debounce). `dossier doctor` surfaces sync health via `Syncer.Status`. See **D7** (process model) + **D8** (silent-failure decision). **`go test -race ./...` passes** (the concurrency gate) + real-binary dogfood (session-start hook pulls, doctor shows sync). **D9: dogfood caught + fixed a serious Phase-2 bug the clone-onboarding path exposed** — fast-forward pull was deleting machine-local `config.yaml` (un-teaming joiners); fixed + regression test.

**Team Sync is FEATURE-COMPLETE for local/offline use.** Full flow validated end-to-end through the real binary: `team create` → `team join` → concurrent edits → auto-sync via session hooks → convergence, remote-wins conflicts, zero data loss, machine-local files preserved, `doctor` sync status. All committed to `team-sync`. `go test -race ./...` green.

**Phase 4 doc reconciliation + test-gap closure: DONE & INTEGRATED into `team-sync` (`f0fce10`, 2026-07-16)** — delivered by an agy (Gemini 3.1 Pro) delegate, verified + integrated by the orchestrator. Two buckets landed: (1) **doc reconciliation with an audience-split honesty rule** (advisor-prompted, to avoid overclaiming the never-run PAT path) — contract docs (SPEC §7's three `sync`/`team` command entries; a §14.10 status line) now state Team Sync is *implemented and integrated*, while user-facing docs (README + `docs/team-sync-onboarding.md` + `docs/team-sync-runbook.md`) moved to honest **"in pilot — built and works locally, not yet validated against live GitHub"** framing rather than flipping the stale "coming in a later release" tags to "ready"; README gained a Team Sync (pilot) pointer to the onboarding doc. (2) **D9 coverage gap closed** — two new regression tests in `internal/sync/ff_preserve_test.go`: `TestSync_FastForwardConverges` (ff pull *converges* on-disk content to remote, byte-equal — distinct from the existing machine-local-*preservation* guard) and `TestSync_CloneReceivesExistingRemoteContent` (a fresh clone *delivers* existing remote content on join). DoD verified in the delegate's worktree by the orchestrator: `go build`/`go vet`/`gofmt` clean, `go test -race ./internal/sync/... ./internal/cli/...` + full `go test ./...` green, doc-string grep assertions pass, diff scope was docs + test-file only (zero product `.go` changes). PRD was left untouched — its Team Sync premise was already amended earlier and read correctly.

**Remaining Phase 4 = NEEDS THE OWNER (autonomous run stops here):**
- **Real-GitHub PAT dogfood** — everything above is local bare repos; the actual GitHub HTTPS+PAT path (`~/.dossier/credentials` 0600, `gh auth token` fallback) is unit-tested only, never run against real GitHub. Needs a real private repo + a PAT. (This is the one Team Sync criterion the docs still flag as pending — see the §14.10 status line.)
- **Two-colleague pilot** — needs actual colleagues on real machines.
- The clone-onboarding + fast-forward-pull paths now have unit coverage (above), but the *real-GitHub* exercise of them is still owed by the PAT dogfood.

**Resume here if interrupted:** current `team-sync` HEAD is `f0fce10` and carries **Phase 2 + 3a + 3b + the Phase 4 doc/test items, all integrated, dogfooded, `-race` green**. No delegates in flight (all torn down; `delegate/agy-p4docs` worktree + branch removed). Read `docs/spikes/gitsync-findings.md`, `docs/team-sync-plan.md`, and the **D1–D9 decisions log** above. The plan's "Non-negotiables" + CLAUDE.md's Team Sync hard rule bind all phases. Stale branches safe to prune: `feat/teamsync-p2`, `delegate/agy-teamsync-p2`, `delegate/agy-teamsync-p3a`, `delegate/agy-teamsync-p3b`, `delegate/cline-*`, `delegate/cline-gitsync-spike`. **Next action: the two owner-gated items above (real-GitHub PAT dogfood, then the two-colleague pilot). All work that could land without the owner is now done.**

> **Active Monitors & Agent Skill (Completed):** Added `## Active Monitors` to the Dossier schema (`assets/guide.md`) for tracking live, mutable context (e.g., Slack threads, Jira tickets) separately from static archived references. We have also created a "Resumption Protocol" Skill (`assets/skill.md`) that is embedded and written out during `dossier init` to `~/.dossier/context/skill.md`. `init` also configures Claude Code's `customInstructions` to point to this skill so agents automatically know to poll these monitors upon resuming a dossier.
> **TUI catch-up to the session-id fix (2026-06-16) — later superseded:** The TUI's session/active presentation was updated to reflect real-session vs fallback (honest header banner + standalone footer warning). This was **superseded on 2026-06-17 by ADR 0004**, which removed the session/active concept from the TUI entirely (see the de-sessioned entry above). The honest-banner/footer work no longer exists.
> **Frontmatter backward compatibility (2026-06-25):** A store written by an older build (e.g. a three-level `importance: medium` from before the binary high/low schema) no longer hard-fails on load or edit. The invariant is *map toward attention, not away*: `Frontmatter.Normalize` (`internal/core/dossier.go`) coerces any value that is no longer a valid enum member, or a field absent from an older file, to its highest-attention valid value (`importance`/`urgency` → `high`, `status` → `active`). It is wired at three layers: (1) **read** — `CalculatePriorityScore` normalizes its copy so legacy values sort toward attention immediately; (2) **write** — `Service.Save` heals + persists + emits a `Warning` for every coercion; (3) **startup** — a version-gated one-time sweep (`Service.Migrate`, `config.schema_version` vs `core.CurrentSchemaVersion`) runs from `wire`, rewriting every stale Dossier once and logging to **stderr** (never stdout, so `mcp serve` stays clean). `doctor` reports legacy values as healable rather than fatal. The write-side source of `medium` was also closed (MCP tool schema, CLI `--importance/--urgency` mapping, SPEC §357). To extend for a future field: add the field's `Normalize` + one block in `Frontmatter.Normalize`, and bump `CurrentSchemaVersion`.



## `dossier-delegate` skill (design captured 2026-06-30; bundled into `dossier init` 2026-08-04)

**Part of the shipped install path as of 2026-08-04.** The `SKILL.md` content
is embedded (`assets/dossier-delegate-skill.md`) and `ClaudeCodeHarness.Install`
writes it to `~/.claude/skills/dossier-delegate/SKILL.md` (testable via
`/dossier-delegate`) as part of `dossier init`'s Claude Code integration step,
following the same idempotent/non-clobbering/single-confirmation contract
(B7/B8) as the rest of `Install`. The design below is otherwise unchanged.

**Problem:** the user delegates work captured in Dossiers to an overseas team
on an ~8-hour offset. An undefined success criterion or escalation path
doesn't cost a Slack reply there — it costs a full lost day. The user wants
best-practice delegation structure (clear output, validation criteria,
escalation path, decision rights) *available* on a Dossier, but explicitly
**not enforced** — it must never become required overhead on the
zero-friction "dossiers start organically and quickly" flow that's working
today.

**Design (see the skill file for full detail):**
- **Pull-only, explicit invocation** — a slash command / trigger phrases
  ("help me define this exercise," "let's clarify so we can delegate"),
  never triggered by dossier state (no auto-fire off a thin `next_action` or
  a `lead` being set).
- **Method is a stall-simulation, not a fixed rubric:** "read this as the
  teammate, waking up with no way to reach the sender for hours — where do
  they stall?" Biases toward **Decision Rights** and **Escalation** first —
  the two categories that actually cause a full-day loss on an offset team —
  before Objective/Context/Constraints, which are usually already implicit.
  Output is binary/qualitative (name the missing fact, or say "ready") —
  **never a completeness score**, which would recreate the enforcement feel
  the user is avoiding.
- **Persisted contract vs. rendered note (resolves a real tension):** the
  outbound note is verbose by design (frontloading is the point); the
  Distilled State is terse by design (the Distillation Guide fights bloat).
  Resolution: persist a *compressed* delegation contract (Objective /
  Success Criteria / Validation / Constraints / Decision Rights / Escalation)
  into the Dossier body via the existing `Save` path — so it's
  optimistic-concurrency protected and every change lands in the audit log
  with a field-level diff. That answers the user's "how do I check success
  criteria was met without moving the goalpost on my team" concern: the
  goalpost can still move, but never silently — a later re-render must flag
  drift between the persisted contract and current dossier state rather than
  silently rendering the new state as original. The paste-able Slack/Jira
  note is then a pure rendering of that persisted contract, regenerated on
  demand, never inventing new criteria at render time.
- **Return contract:** the teammate has no MCP access (human, async, Slack/
  Jira) — the note defines the exact shape of their expected reply so it can
  be pasted back into the Dossier later, rather than assuming they can write
  to the store themselves.
- Reuses existing frontmatter fields (`next_action`, `open_questions`,
  `lead`) as the pre-delegation gap-tracking home wherever possible — no
  core/SPEC schema change for v1.

**Explicitly deferred (named, not forgotten):**
- A new `delegation_note` artifact type — the existing SPEC §4.3 artifact
  taxonomy (transcript/source_snapshot/file_snapshot/link/query/
  decision_evidence) is entirely about *captured* source material, not
  synthesized outbound documents, so a generated note doesn't cleanly fit
  today's artifact model. Revisit only if rendered-note-only proves
  insufficient in practice.
- Per-teammate formatting/escalation preferences.
- Scoping a contract to a single Next Step vs. the whole Dossier (multiple
  concurrent delegated threads on one topic).

**Grounded in:** Anthropic's agent-design/context-engineering writing,
Karpathy's autoresearch loop, and this environment's own `delegate` skill's
7-block Spec Contract (`~/.claude/skills/delegate/SKILL.md`) — adapted here
for a human, async, offset delegate rather than a supervised agent.

## Resolved decisions (the foundation)

Product (from `PRD.md` §0):
- v1 supports **Claude Code and Pi**; local, single-user, file-based, **no database**. Pi integration is carried by the Dossier Pi extension Dossier installs (see "Pi integration" above).
- A Dossier is a **flat durable topic** (no graph/tree). Each has `dossier.md` (frontmatter + Distilled State), `artifacts/`, `audit.log`.
- **One active Dossier per session**, not global. Ordinary saves have **no human gate** (trust = non-destruction + the Distillation Guide). Ambiguous links and merge conflicts **do** ask the user.
- **No native deletion** (archive only). Text-first artifacts; reject single artifacts > 1 GB. Every material claim carries provenance. 100k tokens is a **warning threshold**, not a hard limit. Missing harness capabilities are warned about, never silent.

Build (from `BUILD-DECISIONS.md`):
- **Language: Go.** **Supported harnesses: Claude Code and Pi.** **Rich TUI** (Bubble Tea).
- Tokenizer: embedded BPE benchmarked vs Opus 4.8, behind a port. Search: native Go default + ripgrep fast-path. Harness installs: non-clobbering, idempotent, confirmation before modifying. Several SPEC ambiguities resolved (notably: `base_revision` is session-side, not frontmatter; recall returns the revision; revision-hash canonicalization defined).

## What remains

Implementation is complete. The remaining work is product validation and minor maintenance:

1. Keep the quality gate green: `go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, and `git diff --check`.
2. Dogfood with 10–20 real topics, including resume, ambiguity, unavailable-transcript, concurrency, and merge-conflict drills.
3. Review distilled states and tune `assets/guide.md` based on those drills rather than adding confirmation overhead.
4. Track the PRD success metrics: cross-session resume success, provenance misses, over-target warnings, and time-to-find-next-topic.

The explicitly deferred items remain listed in `BUILD-DECISIONS.md`, `PRD.md`, and the delegation design above. No implementation milestone is currently outstanding.

## Historical implementation starting point

Milestone 1 (SPEC §15, sequenced in `ARCHITECTURE.md` §12):

1. Scaffold the Go module and `cmd/dossier` + `internal/{core,store,config}` per `ARCHITECTURE.md` §2. Add the CI guard that keeps `internal/core` pure.
2. Define `core` types + `ports.go` + a fake `Store`; stub the `fsstore`.
3. Implement `dossier init` and `dossier doctor` baselines.
4. **Validate Claude Code for real** — MCP registration, SessionStart/session-end/pre-compaction hooks (incl. `/clear`, `/exit`), raw transcript access, stable session id, context injection, install/notice surfacing. Record findings in **`docs/harness-capabilities.md`**.

Get the Store contract and the revision/concurrency logic (`ARCHITECTURE.md` §5–6) right early — everything writes through them.

## Workflow & validation

- **Branch off `main`; one focused PR per milestone** (SPEC §15, sequenced in `ARCHITECTURE.md` §12). Keep the initial commit as the pristine baseline.
- **Conformance check per PR:** in each PR description, include a table mapping the relevant SPEC §14 acceptance criteria (and any `BUILD-DECISIONS.md` items touched) to the tests/behavior that satisfy them, and explicitly note anything not yet met. This is how the work is validated against the docs.
- **Definition of done** (`CLAUDE.md`): compiles; `go vet` + `gofmt` clean; tests pass; relevant §14 criteria demonstrably met; `ARCHITECTURE.md` updated if structure changed.
- **Flag, don't diverge:** if Claude Code can't do what a doc assumes, or a contract is ambiguous or proves wrong, stop and surface it for human review. Record the finding (capability gaps → `docs/harness-capabilities.md`; new decisions → an ADR) before changing course. Do not silently work around a settled decision.

## Docs to build and maintain (the dev agent owns these)

- **`docs/harness-capabilities.md`** — required Milestone 1 deliverable; the real capability matrix for Claude Code (see "How to start" step 4).
- **`docs/adr/NNNN-title.md`** — one Architecture Decision Record per *new* decision not already settled in `BUILD-DECISIONS.md` (library choices, contract refinements). Keeps divergence auditable.
- **`ARCHITECTURE.md`** — keep current; update it in the same PR as any structural change.
- **`HANDOFF.md`** (this file) — update the status section as each milestone lands so any agent can resume cleanly. (You're building a durable-handoff tool — dogfood the pattern.)
- **`assets/guide.md`** — author the shipped Distillation Guide per SPEC §10, with good/bad examples; back it with golden-file fixtures.
- **`README.md`** — fill in real install/usage/caveats once the binary and commands exist.
- **Package-level godoc** on each `internal` package describing its role and the port(s) it implements or depends on.

## Watchouts (hard rules — also in CLAUDE.md)

- Do not reintroduce a hard 100k limit. It is a warning threshold.
- Do not silently link or merge ambiguous topics.
- Do not promise transcript capture universally; degrade visibly.
- Do not add a database or persistent topic graph in v1.
- Do not add native deletion.
- Do not last-write-wins the Distilled State; concurrent edits become conflict artifacts.
- Do not put logic in CLI/MCP/TUI adapters — they are thin shims over one `core.Service`.
- Do not treat the Archive as universally lossless; capture depends on harness/source availability.
- Keep the Distillation Guide central. Quality improves through guide iteration and dogfooding, not confirmation gates.

## Partnering plan (dogfood rhythm)

The tool becomes effective fastest through real use, not abstract polish. Once the core is working (Milestone 8):

1. **Harness reality pass** — map hooks, transcript access, config files, session ids across your actual Claude Code setup.
2. **Seed set** — create 10–20 real Dossiers from current work (messy, clean, blocked, resolved). _The dev agent should ask the user for these topics when it reaches this phase._
3. **Distillation reviews** — compare generated Distilled State against your memory; tune the guide, not toward more gates.
4. **Resume drills** — resume a topic in a *different* agent than created it; record what was missing or bloated.
5. **Ambiguity drills** — test promote/link against similar topics to tune suggestion confidence.
6. **Failure drills** — force unavailable transcript, over-target state, concurrent edits, merge conflicts; verify warnings feel clear, not noisy.
7. **Weekly metric check** — cross-session resume success, provenance misses, over-target warnings, time-to-find-next-topic (PRD §8).

## Inputs still owed by the user

- 10–20 real dogfood topics (needed at Milestone 8, not before).

## Definition of a good first session

Either: scaffold the Go skeleton + `init`/`doctor` baseline from `ARCHITECTURE.md`, **or** produce `docs/harness-capabilities.md` by validating Claude Code for real. Keep the work grounded in real agent sessions — that is where Dossier proves useful or gets exposed quickly.
