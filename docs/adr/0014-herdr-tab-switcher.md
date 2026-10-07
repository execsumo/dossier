# ADR 0014: Dossier as a session switcher over herdr tabs

## Status
Accepted (2026-10-06). **Implemented 2026-10-06** (`internal/harness/herdr.go`, `internal/tui/agents.go`). Amends [ADR 0006](0006-tui-open-in-claude.md)
and replaces the herdr right-split launch (HANDOFF, 2026-09-30). Behaviour outside herdr is
unchanged.

## Context
ADR 0006 gave the TUI a `c` key that mints a session id, binds it, and launches an agent.
Inside herdr (`HERDR_ENV=1`), the agent opens in a split to the right of the TUI. Each
press starts a **new** session. Dossier has no idea whether a Dossier already has a live
agent and cannot take you back to one. Splits also stop working beyond one or two agents,
because every split narrows the panes.

The goal is several agent sessions running at once, one per Dossier, with the Dossier TUI as
the place you return to, see which agents need you, and jump between them.

We considered three alternatives and rejected them:

- **Running the agent inside Dossier** (Dossier drives `claude -p --input/output-format
  stream-json` or the API and draws the chat). It means rebuilding Claude Code's UI
  (permissions, diffs, interrupts, compaction) for every harness. PRD defers an "in-app LLM
  wrapper". Rejected.
- **A terminal emulator inside the TUI** (a pty plus a vt emulator library). That means
  writing a small multiplexer and putting a full-screen TUI inside another full-screen TUI,
  the same class of problem as the existing "…" overlay rendering glitch inside herdr.
  Rejected.
- **One herdr workspace per Dossier, holding a TUI and an agent side by side.** It is
  appealing because the brief stays visible, but it creates two switchers (herdr's sidebar
  and Dossier's). It also needs N TUI processes that each run health and sync ticks and
  compete for the sync lock, which would require a pinned, read-only, no-sync TUI mode. Agent
  panes would be half-width, and herdr's sidebar would mix one workspace per project with one
  per Dossier. **Deferred, not rejected.** Because matching is by session id (below), this
  can later be added as an `open_layout: workspace` option without changing how switching
  works.

## Verified herdr behaviour (2026-10-06, herdr 0.9.3, on the owner's machine)
Recorded in `docs/harness-capabilities.md` §4. In short:

1. `herdr agent list` returns every live agent on the server (all workspaces) with
   `pane_id`, `tab_id`, `agent_status` (`idle|working|blocked|done|unknown`),
   `state_change_seq`, and `agent_session {kind, value}`.
2. For Claude Code, `agent_session.value` **is the id passed with `--session-id`**. Verified
   both through `herdr agent start` and through `herdr pane run "exec … claude --session-id
   <uuid>"`, which is the existing `HandoffPlan.ShellLine()` path. The id appears within a
   few seconds; the status starts as `unknown` and then becomes `idle`.
3. For Pi, `agent_session` is `{kind: "path", value: ".../<timestamp>_<pi-session-id>.jsonl"}`.
   Dossier keys Pi bindings by Pi's own session id (ADR 0009), which is the uuid suffix of
   that file name.
4. `herdr tab create --workspace <ws> --cwd <dir> --label <text> [--env K=V] --focus|--no-focus`
   returns `.result.tab.tab_id` and `.result.root_pane.pane_id`. `--env K=` sets an empty value.
5. `herdr agent focus <pane_id>` **switches tabs** to the agent's pane and marks it seen.
6. Lifecycle: an agent started through `pane run "exec …"` **closes its tab when it exits**.
   An agent started with `herdr agent start` leaves an empty shell tab behind. Either way the
   agent leaves `agent list` immediately.
7. `herdr agent start` runs the agent by its kind name (`argv: ["claude", …]`), so it would
   ignore `$DOSSIER_CLAUDE_BIN`.

## Decision

### Opening: `c` focuses the existing session or opens a new one
In the TUI with `HERDR_ENV=1`, `c` on a Dossier:

1. **Finds live sessions.** It runs `herdr agent list` and derives a session key for each
   agent: the value of `agent_session` when `kind == "id"`, or the uuid suffix of the file
   name when `kind == "path"` (Pi). It looks each key up with `Store.GetSessionBinding`; a
   binding whose `DossierID` is the target is a match. It also checks the TUI's in-memory
   **launched map** (below).
2. **If there is a match, it focuses it** with `herdr agent focus <pane_id>`. With several
   matches it picks the highest `state_change_seq` (the most recently active). No new
   binding is written.
3. **Otherwise it opens a new session**, using ADR 0006's ordering (resolve the binary, mint
   the id, `Switch`, then launch):
   `herdr tab create --workspace $HERDR_WORKSPACE_ID --cwd <launch dir> --label <slug> --focus`,
   then `herdr pane run <root_pane> "<plan.ShellLine()>"`. Keep `exec` in the line so the tab
   closes when the agent exits (fact 6). Do **not** use `herdr agent start`, because of
   fact 7 and the empty tab it leaves.
4. **No workspace rename.** `labelHerdr` currently renames the caller's workspace and tab to
   the slug. That stops: the workspace belongs to the Dossier TUI, and the new tab is
   labelled through `--label`.
5. **Failure falls back visibly.** Any herdr failure sets a warning and falls back to the
   in-place `tea.ExecProcess` launch, as the split launch does today.

`C` always opens a new session in a new tab, for a second session on the same Dossier.

### No persistent registry
The mapping from Dossier to pane is **derived** on every use by joining herdr's live agent
list against the session bindings that already exist. Nothing new is written to disk, so
nothing goes stale: an exited agent simply stops matching. Agents started by hand and bound
with `dossier_session` match too.

**The launched map** is an in-memory `minted session id → pane id` table held by the TUI
process for agents it launched itself. It covers profiles whose harness reports a session id
different from the one Dossier minted:

- **Pi:** the minted binding is orphaned and the real binding appears only after the model
  calls `dossier_session`.
- **Cursor, Codex and Antigravity:** these carry the minted id in `DOSSIER_SESSION`, but herdr
  reports the harness's own id, which may never match.

An entry counts only while its pane is in `agent list`. It is pruned once the pane has been
absent for longer than a 30 s detection grace period, because a freshly launched agent takes a
few seconds to appear. It also covers **Claude before its folder-trust prompt is answered**:
until then herdr reports no `agent_session` (observed 2026-10-06), so the binding join cannot
match. The map is discarded when the
TUI exits. That is acceptable: after a restart, a non-Claude session started by the previous
TUI matches again once it has bound itself, and otherwise the next `c` opens a new one.

### Status badges
While the TUI runs with `HERDR_ENV=1`, it polls `herdr agent list` every ~2 s (one process
call, 5 s timeout). herdr does not tell a pane whether it is visible, so the TUI polls even
from a background tab and marks each Dossier that has a matched agent:

- `●` working
- `▲` needs you (`blocked`)
- `✓` done (finished, not yet viewed)
- `○` idle
- `?` unknown

A Dossier with several agents shows the most urgent status (`blocked` > `done` > `working` >
`idle`). A failed poll keeps the last known badges and shows one warning that they may be stale.
It never blanks them silently. Table cells are truncated before styling, so badges are
plain glyphs and are not dimmed.

### Cycling
`]` and `[` focus the next or previous Dossier that has a live agent, in the current list
order. Both keys are unbound today.

### Outside herdr
Nothing changes: `c` takes over the terminal through `tea.ExecProcess`. The extended help
says that switching between sessions needs herdr. The TUI does not nag about it.

### Parity (B9)
The herdr calls and the join logic live in `internal/harness/herdr.go` (pure join functions
plus `herdrRun` behind the existing test seam). `dossier open` keeps its current behaviour
in this change, as SPEC already exempts it for the split launch. A later
`dossier open --focus-existing` can reuse the same functions. MCP stays excluded, for ADR
0006's reason.

## Consequences
- SPEC §(config, `open_with` paragraph) must change from "new focused right-hand split via
  `herdr pane split …`" to the tab flow above when this ships. `docs/tui.md` gains `C`, `]`
  and `[`, plus the badge legend.
- herdr's JSON shapes become an external contract alongside `claude --session-id`. Parse
  defensively; an unexpected shape is a visible warning plus the in-place fallback.
- Polling adds one subprocess every ~2 s while the TUI is visible inside herdr.
- The "…" overlay rendering glitch inside herdr (HANDOFF open question) matters more once
  the TUI is home base. Fix it in the same pass or immediately after.
- The tabs open in the TUI's workspace. Where the agent's *working directory* should be
  (repo vs Dossier folder) is [ADR 0015](0015-repo-identity-and-local-resolution.md); this
  ADR uses whatever launch directory that resolves.

## Acceptance (checkable when built)
1. Unit: the join function maps a fixture `agent list` (Claude id, Pi path, unknown kind) plus
   fixture bindings to the right Dossier, and picks the highest `state_change_seq`.
2. Unit, with `herdrRun` stubbed: `c` with a matched agent issues only `agent focus`; with no
   match it issues `tab create … --label <slug> --focus` then `pane run <root_pane> "exec …"`
   and writes exactly one binding; a herdr error falls back to the exec path with a warning.
3. Unit: `C` always creates; `]`/`[` order follows the visible list; badges follow the
   precedence rule; a poll failure keeps dimmed badges and shows one warning.
4. Manual, in real herdr: open two Dossiers, return to the TUI tab, see two badges, press `c`
   on the first and land in its tab, `/exit` it, and see the tab close and the badge clear.
