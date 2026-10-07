# ADR 0002: TUI Session Identity Resolution

## Status
**Superseded by [ADR 0004](0004-tui-no-session.md)** (2026-06-17): the TUI no longer
resolves or carries any session identity, so the decision below (the TUI reuses the CLI's
`resolveSessionID()`) no longer applies. Retained for history.

Previously: Accepted — precedence updated by [ADR 0003](0003-mcp-session-id-from-env.md).

> Update (2026-06-16, ADR 0003): the shared resolver now inserts `CLAUDE_CODE_SESSION_ID`
> ahead of `DOSSIER_SESSION` in the precedence ladder. The decision below (the TUI reuses the
> CLI's `resolveSessionID()` rather than minting its own) is unchanged; only the ladder grew a
> higher-priority entry, which the TUI inherits automatically. See `docs/history/tui-plan.md`
> ("Catch-up after the MCP session-id fix") for the remaining TUI presentation follow-ups.

## Context
For the interactive local TUI, we need to determine how the session identity is established. The CLI and MCP adapters resolve the active session ID by checking the `--session` flag, falling back to the `DOSSIER_SESSION` environment variable, and ultimately defaulting to `"sess_default"`.

## Decision
We chose to reuse the exact same session resolution logic as the CLI (reusing `resolveSessionID()`, which checks the flag, the `DOSSIER_SESSION` environment variable, and defaults to `"sess_default"`). 

## Consequences
- The TUI shares the same active dossier binding as concurrent CLI or MCP commands in the same shell session.
- Users can switch or query the active dossier in the TUI, and the CLI/MCP will immediately observe the same active dossier under the same session ID.
- Keeps the system simple, consistent, and predictable across all three interfaces (CLI, MCP, TUI).
