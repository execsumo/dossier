# Harness Capabilities

Dossier v1 supports **Claude Code and Pi**. This document records Claude Code's integration capabilities and Pi's, as verified against each harness.

Other harnesses (Codex, Antigravity) remain out of scope for v1. The `Harness` interface and registry remain extensible.

## 1. Capability Matrix (Claude Code)

| Feature | Claude Code |
|:---|:---|
| **Config File Path** | `~/.claude.json`<br>`~/.claude/settings.json` |
| **MCP Registration Path** | `~/.claude.json` -> `mcpServers` |
| **Hook Configuration** | `"hooks"` in `settings.json` |
| **Hook Payload Format** | JSON on `stdin` (includes `session_id`, `hook_event_name`) |
| **SessionStart Hook** | Yes (`SessionStart`) |
| **SessionEnd Hook** | Yes (`SessionEnd`) |
| **Pre-Compaction Hook** | Yes (`PreCompact`) |
| **Raw Transcript Access** | Yes (via session UUID matching) |
| **Stable Session ID** | Yes (UUID string in payload) |
| **MCP Session Env Var** | Yes (`CLAUDE_CODE_SESSION_ID`, verified) |
| **Caller-chosen Session ID** | Yes (`claude --session-id <uuid>`, verified 2026-09-02) |
| **Context Injection** | Yes (Stdout from `SessionStart` hook) |
| **Install/Notice Surfacing** | Yes (During init & session start) |

All capabilities are available, so Claude Code supports Dossier's full deterministic happy path. Even so, if a capability is missing in a given session (e.g. transcript access), Dossier must degrade visibly — warn rather than silently skip.

## 2. Capability Matrix (Pi)

> **Verified against Pi (`@earendil-works/pi-coding-agent`) 0.83.0 — source-read, not assumed.**
> This section **corrects** the earlier assumption (2026-08-03) that a user's
> Claude-like hooks extension would supply the whole contract through
> `PI_SESSION_ID`/`PI_SESSION_FILE` in the process environment. Two of those
> assumptions do not hold; see *Findings* below.

| Feature | Pi |
|:---|:---|
| **Config dir** | `~/.pi/agent` (override: `PI_CODING_AGENT_DIR`) |
| **Extension discovery** | `<agent dir>/extensions/*.ts` and `<agent dir>/extensions/*/index.ts`; project-local `.pi/extensions/…` |
| **Extension model** | In-process TypeScript modules loaded via jiti; no build step |
| **MCP Registration Path** | **None built in.** MCP arrives only through a third-party adapter extension |
| **Hook Configuration** | **None.** Pi has extension *events*, not out-of-process hooks |
| **SessionStart / SessionEnd / Pre-compaction** | Extension events (`session_start`, `session_shutdown`, `session_before_compact`) — **not bridged by Dossier yet** |
| **Raw Transcript Access** | Yes — session JSONL at `<agent dir>/sessions/--<cwd>--/<ts>_<uuid>.jsonl` |
| **Stable Session ID** | Yes (UUID; `ctx.sessionManager.getSessionId()`) |
| **Session env vars** | `PI_SESSION_ID` / `PI_SESSION_FILE` — **bash-tool children only** |
| **Session identity for other processes** | Via the bundled Dossier extension (below) |
| **Install/Notice Surfacing** | Yes (`dossier init`, `dossier harness list`, `dossier doctor`) |

### Findings that contradicted the earlier assumptions

1. **`PI_SESSION_ID`/`PI_SESSION_FILE` are not process-wide.** Pi builds them per
   bash-tool invocation (`core/tools/bash.js` deletes them from the inherited
   environment and re-adds them from the live session). A Dossier process Pi
   starts any other way — an MCP server, a helper — never sees them, and a
   process spawned once keeps a stale snapshot across `/new`, `/resume` and
   `/fork`.
2. **Pi ships no MCP client.** MCP support is a third-party adapter extension, so
   Dossier must not claim the MCP capability under Pi.

Consequence: Dossier claims neither MCP nor lifecycle hooks for Pi, and supplies
session identity itself through a bundled extension.

### The Dossier Pi extension (`assets/pi-extension.ts`)

Installed to `<agent dir>/extensions/dossier/index.ts` by `dossier init` or
`dossier harness install pi` — idempotent, backed up before replacement, and
confirmed before writing (B7/B8). On every `session_start` it:

- writes a **session pointer** to `<agent dir>/dossier/sessions/<pi-pid>.json`
  (override: `DOSSIER_PI_SESSION_DIR`) recording `session_id`, `session_file`,
  `cwd` and the Pi pid, write-then-rename so a reader never sees a partial file;
- mirrors `PI_SESSION_ID`/`PI_SESSION_FILE` into the Pi process environment, so
  every child Pi spawns from then on inherits the identity directly;
- prunes pointers whose Pi process is gone, and removes its own on quit.

Dossier resolves the pointer by walking its own process ancestry (procfs, `ps`
fallback) until it finds the pointer of the Pi process that owns it — so two
concurrent Pi sessions can never read each other's binding. `Service.Switch`
records the resolved source, so a Pi session's binding is filed under `pi` with
Pi's capabilities even on a machine that also has Claude Code configured.

Verified end-to-end against Pi 0.83.0: a real session published its UUID and
JSONL path, a child process with no session environment resolved that id through
the ancestry walk, bound a Dossier, and read the binding back.

### Out of scope in this pass (named, not forgotten)

- **Lifecycle bridging.** The extension does not yet call `dossier hook
  session-start|session-end|pre-compaction`. Until it does, Pi sessions get no
  session-start context injection, no end-of-session capture, and no
  pre-compaction save. `Detect` reports those capabilities as unavailable rather
  than implying them.
- **MCP registration.** Users who run an MCP adapter extension register
  `dossier mcp serve` with that adapter themselves; Dossier does not write
  `mcp.json` for Pi.
- Pi's JSONL is archived as provided — Dossier never mutates or reinterprets the
  source transcript.

---

## 3. Claude Code Integration Details

- **MCP Path:** Stdio-based server registered globally in `~/.claude.json` under `"mcpServers"` or locally in a project's `.mcp.json`.
- **Hooks:** Lifecycle hooks trigger commands. The standard output of the `SessionStart` hook is directly injected into Claude Code's active context window. The `PreCompact` hook triggers just before history truncation.
- **Hook payloads do not carry distilled state (verified).** `SessionEnd`/`PreCompact` deliver `session_id`, `hook_event_name`, and `transcript_path` — there is no field through which the agent's curated state could arrive, and a hook cannot invoke the agent to produce one. `Service.SessionEnd` therefore archives the transcript and leaves the Distilled State untouched unless the session already saved it. An earlier reading of this table assumed the boundary could perform "a final `Save` of the session's active Dossier context"; it cannot, and the code no longer implies it does. The `distilled_state` field the hook handler decodes remains for harnesses that can supply one; none in the registry does today. Consequence: eager `dossier_save` during the session is the only path to the Distilled State, which is why `assets/instructions.md` states that as a load-bearing rule, and why the boundary emits a visible warning when a session persisted nothing.
- **Session ID:** A stable UUID is passed in the JSON payload on `stdin` to any hook handler. (Note: Previously, this session ID was only available to hooks and was not automatically resolved by MCP adapters; the addition of env-var resolution closes this gap).

### MCP Session Identity

The stdio MCP server (`dossier mcp serve`) is launched per session with `CLAUDE_CODE_SESSION_ID` set in its environment. This UUID is identical to:
- The session ID in the hook stdin JSON payload,
- The transcript filename (e.g., `~/.claude/projects/.../<uuid>.jsonl`),
- The `~/.claude/session-env/<uuid>` entry.

Therefore, an MCP tool can resolve the active session ID directly from the environment without the agent supplying it.

### Caller-chosen session ids (`--session-id`)

`claude --session-id <uuid>` makes the **caller**, not Claude, choose the session id
("Use a specific session ID for the conversation", verified against the installed CLI on
2026-09-02). This is what makes the "open in Claude" handoff deterministic (ADR 0006):
Dossier mints a UUIDv4, writes the session binding for it, and then launches Claude with
that id, so the `SessionStart` hook fires already bound and injects the Distilled State.

**Assumption, not a guarantee.** It is an external CLI contract that could change. If the
flag is rejected, the launch fails and the error surfaces (`m.err` in the TUI, a non-zero
exit from `dossier open`) — a visible failure, never a silent no-op. The documented fallback
is prompt-only (no `--session-id`, no pre-binding), which depends on the model calling
`dossier_session` itself.

#### Observed Quirks
A single Claude Code session may spawn two concurrent `dossier mcp serve` processes. Both processes carry the identical `CLAUDE_CODE_SESSION_ID` in their environment, so reading the environment variable remains unambiguous and safe.

---

### 3.1 Hook Schema and Installation Caveats

### Hook Schema Format
To ensure hooks are not ignored by the Claude Code hook executor, they must be registered in the correct array-of-matchers schema.

#### Claude Code (`~/.claude/settings.json`)
Requires the `"matcher"` key:
```json
"hooks": {
  "SessionStart": [
    {
      "matcher": "*",
      "hooks": [
        {
          "type": "command",
          "command": "/absolute/path/to/dossier hook session-start"
        }
      ]
    }
  ]
}
```

### 3.2 Stable Binary-Path Installation and MCP Configuration

To prevent dangling hook paths and ensure a reliable, persistent connection, Dossier uses a stable, self-managed path for all harness integrations.

#### Stable Path Installation (`dossier install`)
Users can install the Dossier binary to a stable PATH location using the `dossier install` command.
- **Default Path:** `~/.local/bin/dossier`
- **Override Flag:** `--dir` (e.g. `dossier install --dir /usr/local/bin`)
- **Self-Install on `init`:** Running `dossier init` from a volatile directory (such as a build cache, temporary directory, or repository workspace) will detect the environment and prompt the user to install to the stable location first.

#### MCP Config Schema and Location (Claude Code)
- **Location:** `~/.claude.json` (user scope). This is distinct from hooks, which live in `~/.claude/settings.json`. Claude Code reads user-scope MCP servers only from `~/.claude.json`, so the two writes must not be conflated.
- **Migration:** Older builds mistakenly wrote the `dossier` MCP entry into `~/.claude/settings.json` (where Claude Code ignores it). `init` now strips any stale `dossier` entry from `settings.json` and registers it in `~/.claude.json`, healing an already-polluted config idempotently.
- **Configuration Block:**
```json
{
  "mcpServers": {
    "dossier": {
      "type": "stdio",
      "command": "/Users/hgill/.local/bin/dossier",
      "args": [
        "mcp",
        "serve"
      ]
    }
  }
}
```
