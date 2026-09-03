# ADR 0006: The TUI may mint a session id to hand a Dossier to Claude Code

## Status
Accepted (2026-09-02). Amends [ADR 0004](0004-tui-no-session.md) — it does not supersede it.

## Context
[ADR 0004](0004-tui-no-session.md) removed every session affordance from the TUI: the TUI
resolves **no** session identity and calls neither `Service.Switch` nor `Service.Active`.
The reasoning was specific: the TUI's own terminal is not an agent session, so session
resolution fell back to the constant `sess_default` bucket that no live agent ever reads.
"Make active" therefore did nothing for anyone.

That reasoning is about the TUI's *own* identity. It says nothing about a session the TUI
**creates**. Browsing a Dossier and then wanting to work on it with an agent is the common
next step, and today it means quitting the TUI, opening a terminal, starting `claude`, and
hoping the agent calls `dossier_session` with the right slug.

Two facts make a deterministic handoff possible:

- `claude` accepts `--session-id <uuid>`, so the caller — not Claude — chooses the id.
- `Service.SessionStart` inlines the Distillation Guide and the full Distilled State into
  the session-start payload **iff** a binding already exists for that session id, and the
  Claude Code installer registers `dossier hook session-start` as the `SessionStart` hook.

So the id can be minted, bound, and handed to Claude in one step: the hook fires already
bound and the session opens with the Dossier loaded, with no reliance on the model choosing
to call a tool.

## Decision
The TUI gets a `c` key ("open in Claude"), active in the dashboard and detail views, which:

1. resolves the `claude` binary (`$DOSSIER_CLAUDE_BIN`, else PATH),
2. mints a fresh UUIDv4,
3. calls `Service.Switch` to bind the focused Dossier to that id,
4. launches `claude --session-id <uuid> "<prompt>"` in the Dossier's directory via
   `tea.ExecProcess`, suspending the TUI until the session exits.

The TUI still resolves **no session identity of its own**. It never asks "what session am
I in?" — the question ADR 0004 answered with "none, and that is the point". It answers a
different question: "what session am I about to start?", for which it holds the id by
construction. ADR 0004's decision stands unchanged for every other purpose: no `a` key, no
`★` marker, no `Session:` header, no `Active` call.

Ordering matters and is part of the decision: **resolve the binary before writing the
binding**. A missing `claude` must fail before anything is persisted.

The shared launch logic lives in `internal/harness/launch.go` (`ClaudeBin`,
`NewClaudeSessionID`, `PlanClaudeHandoff`, `HandoffPlan.Command`). `internal/core` is
untouched — `Switch` and `Path` already exist, and spawning a process is I/O that must stay
outside core.

**Parity:** the same handoff is exposed as `dossier open <slug-or-id>`, calling the identical
helpers, per B9 and the "never fork logic into an adapter" hard rule. **MCP is deliberately
excluded**: an agent calling an MCP tool is *already inside* a Claude Code session and has
`dossier_session` for binding — spawning a second session from inside the first has no user.
This is the same reasoning shape as ADR 0004's carve-out.

## Consequences
- **B9 is re-widened, narrowly.** ADR 0004 made `Switch` CLI/MCP-only. The TUI may now call
  `Switch`, but only for a session id it minted itself and only as part of launching that
  session. B9 records both the exception and this carve-out.
- **A new external CLI contract.** `claude --session-id <uuid>` is a harness assumption of
  exactly the kind `CLAUDE.md` requires recording; it is in `docs/harness-capabilities.md`.
  If the flag ever disappears, `ExecProcess` returns an error that surfaces as `m.err` — a
  visible failure, not a silent no-op.
- **Orphan bindings are possible.** If `claude` fails *after* the binding is written, a
  binding persists for a session that never existed. It is keyed by an id nothing will ever
  present, so it is inert; looking the binary up first bounds the common case. Not worth
  cleanup machinery.
- **The launch also passes a prompt.** The pre-written binding only injects context when the
  session-start hook is installed; the prompt makes the handoff work on an MCP-only or
  hook-less install. It costs one turn and duplicates context when both paths are present.
  It is one field on `HandoffPlan`, so dropping it is a one-line change.
- **Working directory.** Frontmatter carries no repo pointer, so the Dossier directory
  (`~/.dossier/<slug>/`) is the only defensible cwd. Claude Code will show its trust prompt
  the first time; it also makes the prompt's `./dossier.md` fallback correct.

## Alternatives considered
- **Prompt only** (`claude "<prompt>"`, no `--session-id`, no `Switch`): touches no ADR, but
  the Dossier loads only if the model decides to call the tool. Rejected — the whole point
  is determinism. It remains the fallback if the flag is ever withdrawn.
- **Leave it to the CLI** (`dossier open` with no TUI key): the TUI is where a user is
  already looking at the Dossier they want to work on. Rejected as the primary path, but
  `dossier open` ships anyway for parity.
