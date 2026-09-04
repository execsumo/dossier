# Dossier — Build Decisions

> Date: 2026-06-14 · Resolves SPEC §16 "Open Implementation Questions" and the HANDOFF "Questions To Bring Back To The User."
> These are now **settled**. The dev agent should not relitigate them; if reality contradicts one during discovery, flag it and stop, don't silently diverge.

This document closes the implementation-discovery questions so the build can start without stalling. Product strategy is settled in `PRD.md`/`PRFAQ.md`; this file settles the *build* choices the spec deliberately left open. Architecture detail for each lives in `ARCHITECTURE.md`.

---

## Settled decisions

| # | Question (SPEC §16 / HANDOFF) | Decision | Why |
|---|---|---|---|
| **B1** | Language: Go or Rust? | **Go.** | Fastest reliable path to a cross-platform single binary; mature MCP-over-stdio SDK; the app is I/O- and file-bound, not CPU-bound, so Rust's performance edge is unused. Simpler concurrency for the optimistic-locking / atomic-write needs. |
| **B2** | Harness target? | **Claude Code and Pi (v1).** | Claude Code provides the full deterministic capability set directly. **Pi does not** — the original rationale here (a third-party hooks extension exporting `PI_SESSION_ID`/`PI_SESSION_FILE` plus lifecycle calls) was verified false against Pi 0.83.0: those variables reach bash-tool children only, and Pi ships no MCP client. Dossier therefore installs its own Pi extension, which supplies session identity today; lifecycle bridging is still owed (ADR 0005, `docs/harness-capabilities.md` §2). The *decision* — support Pi in v1 — stands. Codex and Antigravity remain out of scope. |
| **B3** | TUI extent in v1? | **Rich TUI** (Bubble Tea). | Full-screen open-work dashboard, status/priority editing, list, and merge/link conflict resolution. Includes native markdown rendering and fsnotify-based reactive hot-refreshing. CLI flags remain the scriptable surface; the TUI is the interactive surface. |
| **B4** | Tokenizer choice (SPEC §11.4)? | **Embedded BPE vocab** (o200k/cl100k-class), compiled into the binary, benchmarked against **Opus 4.8** as the reference per PRD §6. Lives behind a `Tokenizer` port so it can be swapped. | Keeps single-binary purity; precision per target model is explicitly not required. Caveat documented in `README.md`. |
| **B5** | `ripgrep` soft-dep vs native search (SPEC §11.3, Q8)? | **Native Go recursive scan is the default**; `ripgrep` is auto-detected and used as a fast path when present. Both behind a `Searcher` port. | Preserves single-binary purity (no hard dependency) while taking the speed win when `rg` exists. Best of both. |
| **B6** | Provenance syntax (Q9)? | Adopt the SPEC §4.2 form verbatim: `[src:art_<id>#L<a>-L<b>]`, or `[src:art_<id>]` when no line range. `doctor` validates by regex + reference resolution. | One syntax, easy for agents to emit and for `doctor` to parse and resolve. |
| **B7** | Safe install behavior for modifying harness configs (Q5)? | **Never clobber.** Read → merge → write, with a timestamped backup of any file touched, fully **idempotent** (re-running `init` is a no-op if already correct). | Harness config files are the user's; corrupting them is the worst failure mode. |
| **B8** | Hook-install confirmation (Q6)? | **Yes** — `dossier init` prompts before modifying Claude Code's config, unless `--yes`/non-interactive. | Modifying another tool's config without consent is hostile; explicit opt-in. |
| **B9** | TUI vs plain CLI split (Q10)? | Both. Every operation is reachable via **CLI flags** (scriptable, `--json`) *and* the relevant ones via the **TUI**. The MCP/CLI/TUI all call one core service (see `ARCHITECTURE.md`) so behavior is identical. **Exception:** the per-session active binding (`Switch`/`Active`) is intentionally *not* exposed in the TUI — it has no meaningful session target from the TUI's standalone launch context. See [ADR 0004](docs/adr/0004-tui-no-session.md). **Carve-out:** the TUI's `c` key ("open in Claude") *does* call `Switch`, but only for a session id it mints itself while launching that Claude Code session — see [ADR 0006](docs/adr/0006-tui-open-in-claude.md); the same handoff is `dossier open` on the CLI. | Satisfies the "single command" capture constraint (PRD §4.10) and the agent-led happy path simultaneously. |
| **B10** | Self-Install Path | **Idempotent stable-path copy.** Add `dossier install` (default `~/.local/bin/dossier`), and have `init` offer self-install if run from a volatile/build dir. | Keeps harness configs pointing to a stable location that won't break when rebuilding or moving the volatile build binary. |
| **B11** | MCP Registration | **Harness config auto-wiring.** Register both the MCP stdio server (command = stable path, args = `[mcp, serve]`) and lifecycle hooks during `init` after confirmation. Write each to the location Claude Code actually reads — hooks → `~/.claude/settings.json`, MCP → `~/.claude.json`. `init` migrates stale entries an older build wrote to the wrong file. | Connects both MCP and lifecycle hooks automatically; conflating the two destinations (e.g. MCP in `settings.json`) silently fails because Claude Code ignores it. |
| **B13** | What a team store discloses (2026-09-03) | **The raw session stash never syncs; `thinking` turns are excluded from the compiled transcript.** `<slug>/sessions/<author>/` is gitignored (`*/sessions/`) and stays machine-local; the compiled transcript artifact — line-stable and citable — remains the shared depth layer and still syncs, as does per-author `audit/<author>.log` (attribution is wanted provenance). Excluded `thinking` is **tallied in the compiled header**, never silently dropped. The exclusion set is written at `init`, not at first sync. | The stash is write-only — nothing in the codebase reads it, and a `[src:]` range into raw JSONL lands mid-record and cites nothing — so syncing it shipped the rawest content (unedited prompts, thinking, every tool result) for zero reachable depth. Git history is append-only across every clone: default-on cannot be walked back without rewriting history on every teammate's machine, while default-off can be flipped on in an afternoon. The asymmetry decides it, not privacy. |
| **B14** | Where agent working files go (2026-09-03) | **`<slug>/artifacts/` is a frontmatter-only namespace; loose work goes to `<slug>/files/`.** `artifacts/` is parsed, not scanned, so a hand-written file there is skipped by the evidence index, uncitable, and outside the revision hash. `files/` is created with every dossier; `dossier link --from-file` is the promotion path to real evidence. `Store.ValidateArtifactFiles` makes `doctor` report a non-artifact file in `artifacts/` as an issue. | `context/instructions.md` previously told agents to default scratch and attachments into `artifacts/`, which silently destroyed them — `doctor` reported "healthy, 0 artifacts" over a file sitting right there, while `dossier search` still found it, so it read as captured evidence while being none. Silent evidence loss is the one failure this system exists to prevent. |
| **B15** | Slug mutability (2026-09-04) | **The `dos_` ID is immutable identity; canonical slugs may be renamed only through a first-class operation.** Rename moves the complete directory atomically, persists former slugs as resolving aliases, requires optimistic concurrency, and is identity-aware under Team Sync. Generic metadata Save rejects slug/alias mutation. | A display-oriented slug sometimes becomes unclear, but changing it by hand can split frontmatter from its directory and break references. A dedicated operation preserves path integrity while aliases keep old commands and links valid. |
| **B12** | Team portability (2026-07-15) | **Team Sync via an embedded git remote (GitHub), fully hidden behind the binary.** Amends the "single-user" premise to **local-first, optionally team-synced**. Embedded go-git (no git CLI dependency); single-writer per-author file layout (`<slug>/sessions/<author>/`, `<slug>/audit/<author>.log`); sync conflicts on `dossier.md` route into the existing `conflicts/*.md` machinery (`kind: sync_concurrent_edit`), remote wins the working tree, nothing lost; machine-local files (`config.yaml`, root `sessions/`, per-slug `sessions/`, `context/`) never sync (see **B13**); local saves never block on network. Full rationale + rejected alternatives (shared-drive live store, mailbox, hosted service): [ADR 0005](docs/adr/0005-team-sync-via-github.md). Plan: `docs/team-sync-plan.md`. | Git solves transport/auth/atomicity/history and refuses silent overwrites — the exact guarantees Dossier already promises locally; a shared drive's sync client would bypass revision checks with last-write-wins, violating two hard rules. |

---

## Spec ambiguities resolved

These are smaller inconsistencies a dev agent *will* hit. Resolved here; mechanics in `ARCHITECTURE.md` §"Concurrency & Revisions".

1. **`base_revision` does not belong in Dossier frontmatter.** SPEC §4.1 lists it as a (optional) frontmatter field, but a revision is a *session-side* fact, not a property of the Dossier. **Remove it from frontmatter.** Instead: `dossier_recall` and `dossier_session` **return** the current revision; the agent passes it back as `base_revision` on `dossier_save`. The session binding persists it as `last_seen_revision`.

2. **Recall output must include the revision.** SPEC §14.2 / §8 don't show recall returning a revision, but the agent needs it to save later. Recall returns `{distilled_state, frontmatter, revision, token_estimate, warnings}`.

3. **"Canonical content" for the revision hash is undefined.** Defined in `ARCHITECTURE.md`: SHA-256 over canonicalized frontmatter (sorted keys, normalized scalars) + body (normalized line endings) + an artifact manifest (sorted `art_id:content_hash` list). Prefix `rev_`.

4. **Slug collision suffix vs ULID ids.** SPEC §12.2 says "append short id suffix" but §12.3 uses ULIDs (26 chars). Use the **last 6 chars of the ULID** (Crockford base32) as the suffix, e.g. `pricing-model-refresh-7p3k2a`.

5. **`conflicts/<conflict-id>.md` format is unspecified.** Defined in `ARCHITECTURE.md` §"Conflict artifacts": frontmatter (`base_revision`, `attempted_revision`, `kind`, `session`, `ts`) + the rejected proposed body + a unified diff against current. `doctor` reports unresolved conflicts.

6. **Concurrent `audit.log` appends.** Multiple sessions may write one Dossier's log. Use `O_APPEND` single-line writes (atomic on POSIX for lines under `PIPE_BUF`) guarded by a short-held advisory lock on the dossier dir. Last-write-wins for audit entries is acceptable (SPEC §4.4) — ordering by timestamp on read.

7. **Storage layout drift.** PRD §3.6 omits `conflicts/`, `context/`, `sessions/` that SPEC §3.2 adds. **SPEC §3.2 is authoritative.**

---

## Still genuinely deferred (do NOT resolve by guessing — these are discovery output)

These can only be answered by touching the real harness; they are the deliverable of Milestone 1, captured in `docs/harness-capabilities.md`:

- Exact config-file paths and hook formats for Claude Code (SPEC §16 Q2).
- Whether/how Claude Code exposes raw transcripts deterministically (Q3).
- Whether Claude Code provides a stable session id (Q4).

---

## Inputs still owed by the user (not blocking the first milestones)

- **10–20 real dogfood topics** to seed (HANDOFF / SPEC §17). Needed for Milestone 8, not before. The dev agent should ask for these when reaching the dogfood phase.
