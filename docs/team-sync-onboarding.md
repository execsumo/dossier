# Team Sync — Onboarding for Teammates

> Audience: the least-technical teammate joining a shared Dossier store.
> You do **not** need any developer tools for this. If you can run one command and sign in once, you're in.

> **Status (Pilot): The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub.**
> Treat this as an experimental feature.

> **Status: operational but NOT ready for self-service (review 2026-09-18).** Do not hand this page to a colleague to follow alone. Several steps below describe behavior that is not built. Those claims are struck through, with the actual behavior next to them. The person who set up the store should do this setup *with* the colleague (see the "Concierge setup" section below). Evidence and fix list: [`team-adoption-plan-review.md`](team-adoption-plan-review.md).

## What a shared Dossier store is

A **Dossier** is your agent's memory of a topic: the situation, the decisions made, the findings, and the next step — kept in one durable place instead of scattered across chats.

A **shared team store** is that memory, shared with your colleagues. Everyone's sessions contribute to the same set of topics, so the team builds up one shared brain rather than each person starting fresh on every topic.

Your work is saved on your own machine first, and only then shared. Nothing you do is ever lost.

## One-time setup: joining the team store

You'll get a link to the team store from whoever set it up.

```text
dossier team join <url>
```

Replace `<url>` with the link you were given. The command will:

1. ~~**Ask for your name.** This is how your contributions are attributed to you across the team.~~ **Actual:** it does not ask. Your name is your computer's login name (`internal/config/config.go:62`). To change it, edit `author:` in `~/.dossier/config.yaml`.
2. ~~**Ask you to sign in once** with a **personal access token (PAT)** — a private, password-like code from GitHub that lets Dossier talk to the shared store on your behalf.~~ **Actual:** it never prompts (`internal/cli/cli.go:1606-1650`). Sign-in must be set up *before* running `join`: either a token saved at `~/.dossier/credentials` with permissions `0600`, or GitHub's `gh` tool already signed in. If neither is present, Dossier tries without credentials and does not say so (`internal/sync/credentials.go:59`).

> **Superseded warning.** If `join` fails (wrong link, no sign-in, no network), do **not** run it again yet. A failed join leaves a partial setup behind, and the retry is refused with "target directory is not empty". Ask the person who set up the store to clean it up (runbook §6).

### Concierge setup (use this for now)

The store owner should do this with the colleague, screen-shared:

1. Install Dossier. Confirm `~/.dossier` does not exist yet. Joining requires an empty store location.
2. Create a fine-grained GitHub token that can read and write contents on the team repo. Save it to `~/.dossier/credentials` and set its permissions to `0600`.
3. Run `dossier team join <url>`, then `dossier ls` and `dossier doctor`. Check that the team's Dossiers are listed.
4. Send the colleague the exact `dossier open <slug>` line for their first assignment.

### About the sign-in token

- **Where it goes:** a private file on your machine at `~/.dossier/credentials`. It never leaves your computer, and it is never shared with anyone.
- **Why it's needed:** it's how Dossier proves to GitHub that you're allowed to read and write the shared store — so you don't have to sign in every time.
- **Keep it private:** treat it like a password. Dossier stores it so that only you can read it.
- **Convenience path:** if you already use GitHub's command-line tool and are signed in, Dossier can reuse that sign-in automatically (`gh auth token`) — no token to paste.

## Day-to-day: it mostly just works

Once you've joined, your work on a Dossier is **always saved on your machine first** — even with no internet.

Syncing starts simple: a manual step you run when you want to share your latest work or catch up to your colleagues:

```text
dossier sync
```

~~A later phase makes syncing happen **automatically** around your saves and lookups, so you won't have to think about it (currently in pilot testing).~~ **Actual:** automatic sync is partly built. It runs when a Claude session starts and ends (`internal/core/service_session.go:215-219`, `:498-502`), and in the background after the agent reads, saves or renames a Dossier (`internal/mcp/tools.go:324,419,611`). It does **not** run after changes you make with `dossier` commands or in the dashboard. Run `dossier sync` after those.

~~Either way, a flaky connection never loses your work. If a sync can't reach the team store right now, Dossier tells you plainly and keeps your changes safe until the next sync.~~ **Actual:** your work is kept safe on your machine; that part is true. But Dossier does **not** tell you plainly. Automatic syncs report nothing, and a failed `dossier sync` prints a `Warning:` line followed by "Sync successful" (`internal/cli/cli.go:1541`). Read the `Warning:` lines.

## If two of us edited the same thing

Sometimes you and a colleague both edit the same topic. That's fine.

- **Nothing is lost**, and there are never any messy conflict markers in your files.
- The version that's already in the shared store stays in the topic file.
- **Your version is saved right alongside it** as a short note in a `conflicts/` folder, for you to reconcile.
- ~~You'll get a friendly heads-up that there's something to reconcile, and Dossier's dashboard walks you through it step by step — keep whichever parts you want.~~ **Actual:** you get a heads-up only when you run `dossier sync` yourself, and there is no step-by-step reconciliation yet. The dashboard's resolver handles only `dossier merge` conflicts (`internal/tui/tui.go:2075-2086`). Tell the person who set up the store. They will reconcile it.

Both perspectives are preserved — neither is silently overwritten. If you and a colleague disagree, the disagreement is recorded openly rather than smoothed over.

## What never leaves your machine

Some things are yours alone and **never travel** to the shared store:

- **Your local settings** (`config.yaml`) — your machine's install details.
- **Your local session list** — which topic each of your sessions is looking at.
- **Your locally generated context** — the helper notes Dossier writes for your machine.

These are specific to your computer, so sharing them would overwrite someone else's setup. They stay put, by design.

~~Everything that's **about the topics themselves** — your distilled notes, the captured source material, your per-topic session captures, and the audit trail — does sync, so the team sees it.~~ **Actual:** your raw per-topic session captures do **not** sync (`*/sessions/`, `internal/sync/gitignore.go:36`). What does sync:

- your distilled notes;
- the audit trail;
- your working files (`files/`);
- all captured source material (`artifacts/`). This includes the readable **session transcripts** Dossier saves when a session ends: what you typed, and whatever the agent read or ran, such as file contents and command output. The agent's private reasoning is excluded.

Everyone with access to the team repo can read these, permanently. **Don't work on anything in a team Dossier that you wouldn't show the whole team.**

---

*Questions about joining? Ask the person who set up your team store. For the technical plan behind this feature, see `docs/team-sync-plan.md`.*
