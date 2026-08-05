# Dossier

**A local, single-user durable memory layer for long-running work in Claude Code.**

Dossier keeps a topic of work alive across Claude Code sessions. You *promote* a session into a **Dossier** — the critical state of the topic (situation, decisions and who made them, open questions, next action) with the noise stripped out — backed by an **Archive** of the raw material it came from. Every claim cites its source. Next session you resume with exactly the distilled context you need, and the full archive is one search away.

No database, no cloud, no account. Your data is plain Markdown under `~/.dossier/` that you can open in any editor (e.g. Obsidian).

## Quickstart

Requires **Claude Code or Pi** on macOS or Linux. Claude Code is fully integrated; Pi support currently covers session identity (see [Using it with Pi](#using-it-with-pi)).

**Option A — Homebrew (recommended)**

```bash
brew tap execsumo/tap
brew install dossier
dossier init        # wires up Claude Code
```

To update later, on any device: `brew upgrade dossier` (or just `brew upgrade`). The tap's formula is republished automatically on every release, so this always tracks latest.

**Option B — prebuilt binary**

Download the latest release for your platform from the [Releases page](https://github.com/execsumo/dossier/releases), make it executable, and run `init`:

```bash
# example for macOS Apple Silicon
curl -L https://github.com/execsumo/dossier/releases/latest/download/dossier-darwin-arm64 -o dossier
chmod +x dossier
./dossier init        # installs to a stable PATH, then wires up Claude Code
```

To update later, repeat the download step, then re-run `./dossier install && ./dossier init` to re-bind the stable path.

**Option C — build from source** (requires Go 1.26+)

```bash
git clone https://github.com/execsumo/dossier.git
cd dossier
go build ./cmd/dossier
./dossier init
```

That single `init` does everything:

- copies the binary to a stable location on your `PATH` (`~/.local/bin/dossier`) so its path never changes,
- creates your workspace at `~/.dossier`,
- registers Dossier's **MCP server** and **session hooks** in Claude Code (after a confirmation prompt — pass `-y` to skip it), and
- installs the **Dossier Pi extension** if Pi is on the machine.

It's idempotent and non-clobbering: your existing MCP servers, hooks, and extensions are preserved, and every file is backed up before editing. Re-run it anytime, and check things with `dossier doctor` or `dossier harness list`.

## Using it

### Inside Claude Code (the main way)

Once `init` has run, Dossier works on its own:

- **At session start**, your open Dossiers are surfaced into the conversation, sorted by priority — so you and the agent can pick up where you left off.
- **During the session**, the agent recalls, saves, searches, promotes, and switches Dossiers through MCP tools — nothing for you to remember. Switching binds *this* session (the MCP server resolves your Claude Code session automatically), so concurrent sessions can each follow a different Dossier without stepping on each other.
- **At session end and before compaction**, hooks save the active Dossier so context isn't lost.

A shipped **Distillation Guide** tells the agent *what* to keep; the hooks decide *when* to save. To save tokens on your generic coding tasks, the guide isn't injected globally. Instead, Dossier uses **programmatic context injection**: the moment the agent binds a dossier via the MCP server, the server dynamically wraps its response payload with the full guide. A **Resumption Protocol Skill** injected into Claude Code ensures the agent polls **Active Monitors** (live external links like Slack threads) upon resuming a session. There's no confirmation gate — trust comes from the fact that nothing is ever deleted (superseded content moves to the Archive and audit log) and every claim carries a source link.

### Using it with Pi

Install Pi after running `init`? Wire it up with one command:

```bash
dossier harness install pi     # installs the Dossier Pi extension
dossier harness list           # what each harness gives Dossier
```

This writes `~/.pi/agent/extensions/dossier/index.ts` (backing up anything it
replaces, and asking first unless you pass `-y`). Restart Pi to load it.

The extension exists because Pi hands `PI_SESSION_ID` only to processes its bash
tool spawns — so without it, Dossier running any other way under Pi cannot tell
which session it belongs to, and refuses to bind a Dossier rather than risk two
sessions sharing one binding. The extension publishes the live session id for
every Dossier process the Pi session owns; `/dossier-session` inside Pi shows
what Dossier will resolve.

**What Pi does not have yet:** lifecycle bridging. Pi sessions get no
session-start surfacing, no end-of-session save, and no pre-compaction save —
`dossier harness list` reports those as unavailable rather than pretending. Pi
also has no built-in MCP client; if you run an MCP adapter extension, register
`dossier mcp serve` with it yourself. Use the CLI in the meantime.

### From the command line

Everything is scriptable too. The CLI, MCP, and hooks are thin layers over one core, so they behave identically.

```bash
# Promote a topic into a Dossier (optionally seed its distilled state, lead, and meeting interfaces)
dossier promote "payments-migration" --lead "Alice" --interface "1:1" --interface "Steerco" --distilled "## Situation
Migrating billing off the legacy gateway.
## Next action
Confirm webhook signing keys with the vendor."

dossier ls                        # open Dossiers, by priority
dossier show payments-migration   # full distilled state + metadata
dossier search "webhook"          # search distilled state + archives
dossier status payments-migration active
dossier lead payments-migration "Bob"
dossier interface payments-migration "1:1" "Pricing WBR"
dossier ls --interface "1:1" --json   # topics to discuss in a 1:1
dossier next payments-migration "Write the cutover runbook"
dossier priority payments-migration --importance h --urgency h
dossier link payments-migration --from-file ./notes.md   # attach a source to the archive
dossier merge old-slug payments-migration                # fold one Dossier into another
dossier archive payments-migration                       # archive (never deletes)
```

Full reference: `dossier --help`.

### In the terminal UI

For interactive browsing and editing, launch the full-screen TUI — run `dossier` with no arguments, or `dossier tui`:

```bash
dossier        # or: dossier tui
```

It opens a priority-sorted dashboard of your Dossiers, with Lead and discussion-interface filters for meeting prep. From there you can:

- **open** a Dossier to read its distilled state (with a live token estimate and over-target warning). The distilled state is rendered natively as rich, syntax-highlighted Markdown. The view automatically live-refreshes when Claude Code updates the dossier in the background.
- **filter** by Lead with `f`, then cycle through `All` and the seven discussion interfaces with `i` (for example, Marcus + `1:1`),
- **edit** the Lead, status, priority (importance/urgency/due date), and next action inline without leaving the dashboard,
- **link** a source, resolving ambiguous matches by picking from ranked candidates, and
- **merge** one Dossier into another, resolving any conflicts in a syntax-highlighted side-by-side view (sources are archived, never deleted).

The TUI is a thin layer over the same core as the CLI and MCP, so it behaves identically — `q` quits, `?` toggles help.

## How it works

Each Dossier is a directory under `~/.dossier/<slug>/`:

- **Distilled State** — one curated Markdown file: the topic with noise removed, not a lossy summary.
- **Archive** — the captured source artifacts that the distilled claims cite.
- **audit.log** — an append-only record of every change.

One Go binary serves the CLI, the MCP-over-stdio server, and the session hooks. There's no daemon — it runs on demand, invoked by you, by the hooks, or by the MCP server. **Nothing is ever deleted:** superseded content moves to the Archive and audit log.

## Good to know

- **Claude Code and Pi.** Claude Code is fully integrated. Pi has session identity via the bundled extension; its lifecycle hooks and MCP are not wired yet. Other harnesses remain out of scope. If a capability is missing, Dossier says so — at install, in `dossier harness list`, and in `dossier doctor` — rather than failing silently.
- **Config lives in two files.** Hooks go in `~/.claude/settings.json`; the MCP server goes in `~/.claude.json` (the only place Claude Code reads user-scope MCP servers). Both store the absolute path of the stable binary — if you rebuild, rename, or move it, re-run `dossier install` then `dossier init` to re-bind, idempotently.
- **Token counts are estimates.** Dossier uses a BPE tokenizer benchmarked against Opus 4.8; it won't match every model exactly. The 100k-token figure is a configurable warning threshold, not a hard cap — Dossier warns, it never silently truncates.
- **Wiring it up by hand.** If you'd rather not let `init` edit your config: register the MCP server with `claude mcp add dossier -- dossier mcp serve`, and run `dossier hook session-start` to see what the start hook emits.
- **Switching install methods.** `dossier install` (Option B) puts the binary at `~/.local/bin/dossier`; Homebrew (Option A) puts it under its own prefix (e.g. `/opt/homebrew/bin/dossier`). If both are present, whichever comes first on your `PATH` wins — `brew` will warn you ("shadowed by...") if that happens. Pick one method per machine to avoid confusion about which binary is actually running.

## License

[MIT](LICENSE) © 2026 Herwin Gill
