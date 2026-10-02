# Dossier

**Durable memory for long-running work in Claude Code.**

Agent sessions forget. Dossier doesn't. Promote a session into a **Dossier** and it keeps the state of that topic — situation, decisions and who made them, open questions, next action — with the noise stripped out. Every claim cites its source, and the raw material is archived and one search away. Next session, you pick up exactly where you left off.

- **Resume instantly.** Open Dossiers are surfaced at session start, sorted by priority.
- **Nothing is lost.** Transcripts are archived automatically; nothing is ever deleted.
- **Share with your team.** Optional Team Sync gives everyone one shared brain.
- **Yours.** Plain Markdown in `~/.dossier/` — no database, no cloud, no account. Open it in any editor, including Obsidian.

## Install

Requires Claude Code on macOS, Linux, or Windows (Windows is experimental).

**macOS / Linux**

```bash
brew tap execsumo/tap
brew install dossier
dossier init
```

No Homebrew? Grab a binary from the [Releases page](https://github.com/execsumo/dossier/releases), make it executable, and run `./dossier init`.

**Windows (PowerShell)**

```powershell
# use dossier-windows-arm64.exe on ARM machines
curl.exe -L https://github.com/execsumo/dossier/releases/latest/download/dossier-windows-amd64.exe -o dossier.exe
.\dossier.exe init
```

`init` copies Dossier to `%USERPROFILE%\.local\bin`. If `dossier` isn't found afterward, add that folder to your `PATH` and open a new terminal.

`init` sets everything up: your workspace at `~/.dossier`, plus Dossier's MCP server and session hooks in Claude Code. It asks before changing anything, backs up every file it touches, and never overwrites your existing setup. Re-run it anytime; check on things with `dossier doctor`.

Update with `brew upgrade dossier`, or on Windows download the new `.exe` and run `.\dossier.exe init` again.

## Use it

### In Claude Code

Once installed, it just works:

- **Session start:** your open Dossiers appear in the conversation. Pick one up or start a new one.
- **During the session:** the agent recalls, saves, searches, and switches Dossiers for you. Each session follows its own Dossier, so parallel sessions never collide.
- **Session end:** the transcript is archived into the Dossier automatically.

Capture a new idea in one line:

```text
/spark The vendor changed the API contract and I need to work out migration,
backward compatibility, and who needs to review it.
```

Dossier names it, checks for duplicates, and creates it at medium priority.

### In the terminal UI

Run `dossier` with no arguments for a full-screen dashboard of your Dossiers, sorted by priority.

![Dossier TUI table view](docs/screenshots/tui-table.png)

- **Table or board:** press `v` to switch between a table and stage columns (spark → define → execute → review → blocked → done).
- **Filter** by Lead or discussion interface with `f` — handy for meeting prep.
- **Edit inline:** Lead, stage, priority, due date, next action.
- **Link** sources and **merge** Dossiers, with conflicts shown side by side.
- **Launch an agent** with `c`: a fresh session, already bound to the selected Dossier with its state loaded. Inside [herdr](https://herdr.dev), it opens in a new split pane.

![Dossier TUI board view](docs/screenshots/tui-board.png)

Press `?` for all shortcuts.

### From the command line

```bash
dossier promote "payments-migration" --lead "Alice"
dossier ls                              # open Dossiers, by priority
dossier show payments-migration         # distilled state + metadata
dossier search "webhook"                # search state and archives
dossier next payments-migration "Write the cutover runbook"
dossier status payments-migration execute
dossier open payments-migration         # launch an agent on it
```

Run `dossier --help` for the full list.

## Team Sync

Share a store with your colleagues through a private GitHub repo — Dossier handles the plumbing.

- **Local-first.** Saves never wait on the network. Your work lands on your machine first, then syncs.
- **Conflict-honest.** If two people edit the same Dossier at once, nothing is overwritten. The clash becomes a conflict file for you to resolve.
- **Easy to join.** One command and a GitHub sign-in — no developer tools needed.
- **Agents welcome.** Headless agents can join as their own team members.

### Turn your local store into a team store

1. Create an **empty, private** GitHub repo (no README or license).
2. Run `dossier team create <repo-url> --name "Your Name"`.
3. Sign in to GitHub if asked, review the list of Dossiers it will publish, and type `y`.
4. Give each teammate repo access, then add them to the roster: `dossier team add <username> "Their Name"` and `dossier sync`. Only the manager (you) can change the roster. Send them the repo URL; they run `dossier team join <repo-url>`.
5. Assign work with `dossier lead <slug> <username>`, or `dossier promote … --lead <username>`. Teammates find it with `dossier ls --mine`, which lists dossiers they lead **and** ones where they own a Delegation Contract (the `MATCHED AS` column says which).

Your existing Dossiers become the shared ones, and everything in the store syncs, including archived Dossiers. Anyone with repo access can read it all. Machine-local files (`config.yaml`, session bindings, raw session captures) stay on your machine.

```bash
dossier team create <url> --name "Your Name"   # turn your store into a team store
dossier team join <url>                        # join one
```

Full setup, joining and day-to-day guide: [`docs/team-sync-onboarding.md`](docs/team-sync-onboarding.md).

## How it works

Each Dossier is a folder under `~/.dossier/<slug>/`, moved to `~/.dossier/archive/` when done:

- **Distilled State** — the operational brief: Objective, Done When, Validation, Constraints, and the context needed to act.
- **Archive** — the source material the brief cites.
- **Audit log** — an append-only record of every change.

One Go binary runs the CLI, the MCP server, the hooks, and the TUI. They all share one core, so they behave identically.

## Configuration

Settings live in `~/.dossier/config.yaml` (machine-local, never synced):

```yaml
open_with: claude-code   # claude-code, cursor, codex, or antigravity
interfaces: [Pricing WBR, "1:1", Steerco]
leads: [Alice, Bob]
token_limit: 100000
```

## Uninstall

```bash
dossier harness uninstall claude-code   # remove the Claude Code integration
brew uninstall dossier
```

Your data in `~/.dossier/` is left untouched.

## License

[MIT](LICENSE) © 2026 Herwin Gill
