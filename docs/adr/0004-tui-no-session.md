# ADR 0004: The TUI carries no session identity (no active-binding affordance)

## Status
Accepted (2026-06-17). Supersedes [ADR 0002](0002-tui-session-id.md).
Amended (not superseded) by [ADR 0006](0006-tui-open-in-claude.md): the TUI still resolves
no session identity of its own, but may mint one for a Claude Code session it is about to
launch and bind a Dossier to it first. Everything below stands.

## Context
The "active" Dossier is bound **per session**, not globally (a hard rule: "No global
active Dossier"). A session is identified by `harness.ResolveSessionID` (ADR 0003),
whose only real source in Claude Code is `CLAUDE_CODE_SESSION_ID`, set in the env of a
live agent session.

The TUI is launched interactively by a human from a normal terminal (`dossier` or
`dossier tui`). That terminal is not a Claude Code agent session, so no
`CLAUDE_CODE_SESSION_ID` is present and session resolution falls back to the constant
`sess_default` bucket. Consequences of the previous design (ADR 0002, which had the TUI
reuse `cli.resolveSessionID`):

- The `Session:` header was always the constant `sess_default`, rendered as
  "(local default — no active Claude session)" — a fixed, information-free value.
- The `a` ("make active") key and the `★` marker did bind a Dossier, but only inside the
  `sess_default` bucket, which no live agent session ever reads. So "make active" had no
  effect on any agent's session — the recurring "why does this do nothing for me?"
  confusion. `docs/tui-plan.md` catch-up items 1–2 tried to make this *honest* (banner +
  footer warning) but left the non-functional affordance in place.

## Decision
The TUI's role is **browsing and editing** what Dossiers exist — not toggling a
per-session active binding it has no meaningful target for. Therefore:

- The TUI does **not** call `Service.Switch` or `Service.Active`, and resolves **no**
  session identity. `tui.Run(ctx, svc)` no longer takes a `sessionID`/`isRealSession`.
- The `a` key, the `★` active marker / active column, the `Session:` header field, the
  `Active:` header field, and the standalone-session warning footer are all removed.
- The TUI keeps every non-session operation: list/recall (browse), status/priority/
  next-action editing, link (with ambiguity resolution), and merge (with conflict
  resolution).

This is a deliberate scope decision, **not** a bug fix. `Service.Switch`/`Active` remain
fully supported and unchanged — the CLI (`dossier switch`/`active`) and MCP
(`dossier_switch`/`dossier_active`) are where per-session binding belongs, because they
*do* run inside a real agent session.

## Consequences
- Resolves the "fixed Session value" and "make-active does nothing" confusion by removing
  the affordances rather than papering over them.
- Narrows **B9** ("every operation reachable via CLI *and* TUI"): the per-session active
  binding is intentionally CLI/MCP-only. B9 is annotated with this exception and points
  here. The "thin shim, no logic in adapters" hard rule is **not** affected — no logic is
  forked; one Service method is simply not surfaced in one adapter.
- ADR 0002 is superseded (its entire premise — how the TUI resolves a session — is gone).
- ADR 0003's MCP-vs-TUI `allowDefault` divergence is moot for the TUI (the TUI no longer
  resolves a session); it still describes the MCP-vs-CLI distinction, which stands.
- `docs/tui-plan.md` catch-up items 1–3 are obsoleted by this decision.
