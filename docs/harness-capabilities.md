# Harness Capabilities

Dossier v1 supports **Claude Code and Pi**. This document records Claude Code's integration capabilities and the Pi contract provided by a compatible hooks extension.

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
| **Context Injection** | Yes (Stdout from `SessionStart` hook) |
| **Install/Notice Surfacing** | Yes (During init & session start) |

All capabilities are available, so Claude Code supports Dossier's full deterministic happy path. Even so, if a capability is missing in a given session (e.g. transcript access), Dossier must degrade visibly — warn rather than silently skip.

## 2. Capability Contract (Pi)

Pi support is provided by the user's existing Claude-like hooks extension. Dossier detects the contract from the process environment; it does not install Pi hooks.

| Feature | Pi requirement |
|:---|:---|
| Session identity | `PI_SESSION_ID` |
| Transcript path | `PI_SESSION_FILE` |
| Session-start hook | Extension invokes `dossier hook session-start` |
| Session-end hook | Extension invokes `dossier hook session-end` |
| Pre-compaction hook | Extension invokes `dossier hook pre-compaction` |
| Transcript fallback | Dossier reads `PI_SESSION_FILE` when stdin has no `transcript` |

Pi's JSONL file is archived as provided. Dossier does not mutate or reinterpret the source transcript.

---

## 3. Claude Code Integration Details

- **MCP Path:** Stdio-based server registered globally in `~/.claude.json` under `"mcpServers"` or locally in a project's `.mcp.json`.
- **Hooks:** Lifecycle hooks trigger commands. The standard output of the `SessionStart` hook is directly injected into Claude Code's active context window. The `PreCompact` hook triggers just before history truncation, enabling a final `Save` of the session's active Dossier context.
- **Session ID:** A stable UUID is passed in the JSON payload on `stdin` to any hook handler. (Note: Previously, this session ID was only available to hooks and was not automatically resolved by MCP adapters; the addition of env-var resolution closes this gap).

### MCP Session Identity

The stdio MCP server (`dossier mcp serve`) is launched per session with `CLAUDE_CODE_SESSION_ID` set in its environment. This UUID is identical to:
- The session ID in the hook stdin JSON payload,
- The transcript filename (e.g., `~/.claude/projects/.../<uuid>.jsonl`),
- The `~/.claude/session-env/<uuid>` entry.

Therefore, an MCP tool can resolve the active session ID directly from the environment without the agent supplying it.

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
