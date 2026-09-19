# Team Sync — Onboarding for Teammates

> Audience: the least-technical teammate joining a shared Dossier store.
> You do **not** need any developer tools for this. If you can run one command and sign in once, you're in.

> **Status (Pilot): The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub.**
> Treat this as an experimental feature.

> **Status: operational but NOT ready for self-service (review 2026-09-18; updated after the P0 fixes, same day).** Do not hand this page to a colleague to follow alone. There is still no sign-in prompt, so the person who set up the store should do this setup *with* the colleague (see "Concierge setup" below). Claims that were untrue at review time and are now fixed have been restored. Claims that are still untrue stay struck through, with the actual behavior next to them. The fixes are checked in a sandbox, not yet against live GitHub. Evidence and fix list: [`team-adoption-plan-review.md`](team-adoption-plan-review.md) §6; validation: [`team-sync-validation.md`](team-sync-validation.md).
>
> **Coming next (Team MVP, BUILD-DECISIONS B17):** join will offer GitHub sign-in through the `gh` tool (a browser click, no token to create), and teammates will appear under names from a shared roster the manager keeps. This page describes today's flow until then.

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
2. ~~**Ask you to sign in once** with a **personal access token (PAT)** — a private, password-like code from GitHub that lets Dossier talk to the shared store on your behalf.~~ **Actual:** it never prompts (`internal/cli/cli.go:1606-1650`). Sign-in must be set up *before* running `join`: either a token saved at `~/.dossier/credentials` with permissions `0600`, or GitHub's `gh` tool already signed in. If neither is present, every `dossier` command warns "no credentials found", and `dossier sync` fails with `sync_auth_failed` and tells you what to do.

> **If `join` fails** (wrong link, no sign-in, no network), it says why and changes nothing you need to clean up. Fix the cause and run the same command again. Anything the failed attempt created is moved to a folder next to your store, named like `~/.dossier.failed-join-<time>`. It is kept, not deleted, and you can remove it once you've joined.

### Concierge setup (use this for now)

The store owner should do this with the colleague, screen-shared:

1. Install Dossier. Confirm `~/.dossier` does not exist yet, or holds nothing but the token file from step 2. Joining into a store that already has Dossiers is not supported.
2. Create a fine-grained GitHub token that can read and write contents on the team repo. Save it to `~/.dossier/credentials` and set its permissions to `0600`.
3. Run `dossier team join <url>`, then `dossier ls` and `dossier doctor`. Check that the team's Dossiers are listed.
4. Have the colleague open Claude in their usual work folder and ask "what's assigned to <their name>?". Check that it finds their first assignment and binds it.

### Starting work on an assignment (primary path)

You don't need any Dossier commands or topic IDs.

1. Open Claude the way you normally do, in the folder where the work files are.
2. Ask for your work in plain words, for example "What's assigned to Priya?" or "Let's continue the pricing review." Claude finds the Dossier, loads its brief and starts from there. If more than one topic matches, it will ask you which one.
3. As decisions and results come in, ask Claude to save them to the Dossier. The end of a session does not save anything on its own.

Prefer a list? Run `dossier sync`, then `dossier tui`. Press `f` to show only your topics, and `c` to open one in Claude. The list shows only what has already reached your machine, which is why you sync first.

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

Either way, a flaky connection never loses your work. If a sync can't reach the team store right now, Dossier tells you plainly and keeps your changes safe until the next sync. A failed `dossier sync` says "Sync failed" and why. Automatic syncs don't print anything, so the dashboard (`dossier tui`) shows a health line at the bottom, for example `Team sync · last sync failed 18m ago · work is safe locally`. It updates on its own about once a minute. Press `H` for the full report.

## If two of us edited the same thing

Sometimes you and a colleague both edit the same topic. That's fine.

- **Nothing is lost**, and there are never any messy conflict markers in your files.
- The version that's already in the shared store stays in the topic file.
- **Your version is saved right alongside it** as a short note in a `conflicts/` folder, for you to reconcile.
- You'll get a friendly heads-up that there's something to reconcile: the dashboard's health line shows `1 conflict`, and `dossier sync` tells you too. Dossier's dashboard walks you through it: press `x` and pick the conflict. The shared version and yours appear side by side, and `d` shows exactly what differs. Then keep the shared version, restore yours, or keep both so the topic's lead can merge them. You can also ask Claude to show you the conflict and resolve it. You can also just ask Claude to resolve it. In the pilot, the topic's lead makes that call, so if it isn't your topic, tell them.

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
- all captured source material (`artifacts/`). This includes the readable **session transcripts** Dossier saves when a session ends: what you typed, and whatever the agent read or ran, such as file contents and command output. The agent's private reasoning is excluded, and so is the raw, unedited copy of a transcript used to start a new topic, which stays on your machine.

Everyone with access to the team repo can read these, permanently. **Don't work on anything in a team Dossier that you wouldn't show the whole team.**

---

*Questions about joining? Ask the person who set up your team store. For the technical plan behind this feature, see `docs/team-sync-plan.md`.*
