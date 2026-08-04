# Dossier — Build Decisions

> Date: 2026-06-14 · Resolves SPEC §16 "Open Implementation Questions" and the HANDOFF "Questions To Bring Back To The User."
> These are now **settled**. The dev agent should not relitigate them; if reality contradicts one during discovery, flag it and stop, don't silently diverge.

This document closes the implementation-discovery questions so the build can start without stalling. Product strategy is settled in `PRD.md`/`PRFAQ.md`; this file settles the *build* choices the spec deliberately left open. Architecture detail for each lives in `ARCHITECTURE.md`.

---

## Settled decisions

| # | Question (SPEC §16 / HANDOFF) | Decision | Why |
|---|---|---|---|
| **B1** | Language: Go or Rust? | **Go.** | Fastest reliable path to a cross-platform single binary; mature MCP-over-stdio SDK; the app is I/O- and file-bound, not CPU-bound, so Rust's performance edge is unused. Simpler concurrency for the optimistic-locking / atomic-write needs. |
| **B2** | Harness target? | **Claude Code and Pi (v1).** | Claude Code provides the full deterministic capability set directly. Pi provides the same contract through a compatible Claude-like hooks extension (`PI_SESSION_ID`, `PI_SESSION_FILE`, and lifecycle calls). Codex and Antigravity remain out of scope. |
| **B3** | TUI extent in v1? | **Rich TUI** (Bubble Tea). | Full-screen open-work dashboard, status/priority editing, list, and merge/link conflict resolution. Includes native markdown rendering and fsnotify-based reactive hot-refreshing. CLI flags remain the scriptable surface; the TUI is the interactive surface. |
| **B4** | Tokenizer choice (SPEC §11.4)? | **Embedded BPE vocab** (o200k/cl100k-class), compiled into the binary, benchmarked against **Opus 4.8** as the reference per PRD §6. Lives behind a `Tokenizer` port so it can be swapped. | Keeps single-binary purity; precision per target model is explicitly not required. Caveat documented in `README.md`. |
| **B5** | `ripgrep` soft-dep vs native search (SPEC §11.3, Q8)? | **Native Go recursive scan is the default**; `ripgrep` is auto-detected and used as a fast path when present. Both behind a `Searcher` port. | Preserves single-binary purity (no hard dependency) while taking the speed win when `rg` exists. Best of both. |
| **B6** | Provenance syntax (Q9)? | Adopt the SPEC §4.2 form verbatim: `[src:art_<id>#L<a>-L<b>]`, or `[src:art_<id>]` when no line range. `doctor` validates by regex + reference resolution. | One syntax, easy for agents to emit and for `doctor` to parse and resolve. |
| **B7** | Safe install behavior for modifying harness configs (Q5)? | **Never clobber.** Read → merge → write, with a timestamped backup of any file touched, fully **idempotent** (re-running `init` is a no-op if already correct). | Harness config files are the user's; corrupting them is the worst failure mode. |
| **B8** | Hook-install confirmation (Q6)? | **Yes** — `dossier init` prompts before modifying Claude Code's config, unless `--yes`/non-interactive. | Modifying another tool's config without consent is hostile; explicit opt-in. |
| **B9** | TUI vs plain CLI split (Q10)? | Both. Every operation is reachable via **CLI flags** (scriptable, `--json`) *and* the relevant ones via the **TUI**. The MCP/CLI/TUI all call one core service (see `ARCHITECTURE.md`) so behavior is identical. **Exception:** the per-session active binding (`Switch`/`Active`) is intentionally *not* exposed in the TUI — it has no meaningful session target from the TUI's standalone launch context. See [ADR 0004](docs/adr/0004-tui-no-session.md). | Satisfies the "single command" capture constraint (PRD §4.10) and the agent-led happy path simultaneously. |
| **B10** | Self-Install Path | **Idempotent stable-path copy.** Add `dossier install` (default `~/.local/bin/dossier`), and have `init` offer self-install if run from a volatile/build dir. | Keeps harness configs pointing to a stable location that won't break when rebuilding or moving the volatile build binary. |
| **B11** | MCP Registration | **Harness config auto-wiring.** Register both the MCP stdio server (command = stable path, args = `[mcp, serve]`) and lifecycle hooks during `init` after confirmation. Write each to the location Claude Code actually reads — hooks → `~/.claude/settings.json`, MCP → `~/.claude.json`. `init` migrates stale entries an older build wrote to the wrong file. | Connects both MCP and lifecycle hooks automatically; conflating the two destinations (e.g. MCP in `settings.json`) silently fails because Claude Code ignores it. |

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
