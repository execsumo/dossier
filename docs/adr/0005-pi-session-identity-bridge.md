# ADR 0005: Pi Session Identity via a Bundled Extension and a PID-Keyed Pointer

## Status
Accepted (2026-08-04). Extends ADR 0003's precedence ladder; corrects the Pi
assumptions recorded in `HANDOFF.md` (2026-08-03) and `BUILD-DECISIONS.md` B2.

## Context
Dossier's Pi support assumed the user's own Claude-like hooks extension would
supply the whole contract, with `PI_SESSION_ID` and `PI_SESSION_FILE` present in
the process environment. Reading Pi 0.83.0
(`@earendil-works/pi-coding-agent`) contradicted two parts of that assumption:

1. **Session environment variables are bash-tool-only.** Pi constructs
   `PI_SESSION_ID`/`PI_SESSION_FILE` per bash-tool invocation — it deletes them
   from the inherited environment and re-adds them from the live session
   (`core/tools/bash.js`). Any Dossier process Pi starts another way (an MCP
   server started by an MCP adapter extension) never sees them. Worse, a process
   spawned once holds a *stale* snapshot: `/new`, `/resume` and `/fork` change
   the session id, and an environment variable cannot change with it.
2. **Pi has no built-in MCP client.** MCP arrives only via a third-party adapter
   extension, so Dossier must not claim the MCP capability for Pi.

Without a session id, `ResolveSessionID` returns `ErrNoSessionID` and the MCP
path refuses to bind (ADR 0003, correctly — the alternative is concurrent
sessions sharing the `sess_default` bucket). So under Pi the agent could not
bind or switch a Dossier at all.

Pi's real extension point is an in-process TypeScript module auto-discovered
from `<agent dir>/extensions/`, with `ctx.sessionManager.getSessionId()` and
`getSessionFile()` available on every session event.

## Decision
Dossier **ships and installs its own Pi extension** (`assets/pi-extension.ts`,
embedded in the binary and written to
`<agent dir>/extensions/dossier/index.ts`). On every `session_start` it:

1. writes a **session pointer** — `<agent dir>/dossier/sessions/<pi-pid>.json`
   (override `DOSSIER_PI_SESSION_DIR`) holding `session_id`, `session_file`,
   `cwd`, the Pi pid and a timestamp — write-then-rename;
2. mirrors `PI_SESSION_ID`/`PI_SESSION_FILE` into the Pi **process** environment,
   so every child Pi spawns from then on inherits the identity with no lookup;
3. prunes pointers whose Pi process no longer exists, and deletes its own on quit.

Dossier resolves the pointer by walking its own **process ancestry** (procfs,
`ps` fallback, depth-bounded) until it finds the pointer belonging to the Pi
process that owns it. The session-id ladder becomes:

1. explicit → 2. `CLAUDE_CODE_SESSION_ID` → 3. `PI_SESSION_ID` →
**4. Pi session pointer** → 5. `DOSSIER_SESSION` → 6. `sess_default`
(CLI/TUI only; never MCP).

Resolution also **names its source**, and `Service.Switch` records that harness
on the binding, so a Pi session is filed under `pi` — with Pi's capabilities —
rather than under whichever harness is merely configured on the machine.

Capability reporting is honest: Pi reports `Installed` and `SessionIdentity`,
and reports MCP and the lifecycle hooks as unavailable, because Dossier does not
provide them for Pi yet. `dossier init`, `dossier harness list` and `dossier
doctor` surface "Pi is installed but cannot give Dossier a session id yet; run
`dossier harness install pi`" instead of failing silently later.

### Alternatives rejected
- **A single "current session" pointer file.** Simplest, but two concurrent Pi
  sessions would overwrite each other and bind each other's Dossiers, breaking
  the per-session-binding invariant.
- **Environment mirroring alone.** Free, but only reaches processes spawned
  *after* the mirror and cannot follow `/new`, `/resume` or `/fork` in an
  already-running MCP server. Kept as a fast path, not as the mechanism.
- **Requiring the user to pass `session_id` explicitly.** Reintroduces exactly
  the failure ADR 0003 removed: the agent has no way to learn its own id.
- **Asking the user's own hooks extension to export the variables.** Not
  something Dossier can install, verify, or degrade visibly about — and it
  cannot fix the stale-snapshot problem either.

## Consequences
- An agent inside Pi can call `dossier_session` with just a slug, exactly as in
  Claude Code, including from an MCP server that Pi did not spawn via bash.
- Concurrent Pi sessions stay isolated: only an *ancestor's* pointer resolves.
- Pointer files are per-Pi-pid and self-cleaning; a crashed Pi leaves at most one
  stale file, pruned on the next Pi start.
- `dossier harness install pi` covers the "added Pi after init" path;
  `dossier harness list` shows what each harness actually provides.
- Pi sessions still get **no** lifecycle bridging (no session-start injection, no
  end-of-session capture, no pre-compaction save). This is reported as
  unavailable rather than implied, and is the next piece of Pi work.
- macOS/Linux only, as elsewhere in v1: ancestry uses procfs with a `ps`
  fallback.
