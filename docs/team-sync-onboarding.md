# Team Sync — Onboarding for Teammates

> Audience: the least-technical teammate joining a shared Dossier store.
> You do **not** need any developer tools for this. If you can run one command and sign in once, you're in.

> **Status (2026-09-19, Team MVP):** joining now signs you in to GitHub through the GitHub CLI (`gh`) with a browser click, and shows the name teammates will see. Checked in a sandbox and in CI on macOS, Windows and Linux; **not yet validated against live GitHub or on a real Windows PC** (see [`team-sync-validation.md`](team-sync-validation.md) Part D and `docs/harness-capabilities.md` "Windows and macOS"). Until then, the person who set up the store should be available during the first join. Claims that are still untrue stay struck through, with the actual behavior next to them. Background: [`team-adoption-plan-review.md`](team-adoption-plan-review.md) §6, BUILD-DECISIONS B17.

## What a shared Dossier store is

A **Dossier** is your agent's memory of a topic: the situation, the decisions made, the findings, and the next step — kept in one durable place instead of scattered across chats.

A **shared team store** is that memory, shared with your colleagues. Everyone's sessions contribute to the same set of topics, so the team builds up one shared brain rather than each person starting fresh on every topic.

Your work is saved on your own machine first, and only then shared. Nothing you do is ever lost.

## Before you start (once)

1. **Accept the GitHub invitation.** Whoever set up the team store adds you to its private GitHub repository. GitHub emails you an invitation; accept it.
2. **Install two programs:**
   - **Dossier**, from the link the person who set up the store gives you.
   - **The GitHub CLI (`gh`)**: on a Mac, `brew install gh`; on Windows, `winget install --id GitHub.cli`; or download it from https://cli.github.com.
3. **Get the link** to the team store from whoever set it up.

## Joining the team store

```text
dossier team join <link>
```

Replace `<link>` with the link you were given. The command:

1. **Signs you in to GitHub** if you aren't already. It asks `Sign in to GitHub now? Your browser will open. [Y/n]`. Press Enter. `gh` shows a one-time code and opens github.com; paste the code and click **Authorize**. That's the only sign-in. Dossier doesn't store a password or token itself; it asks `gh` each time.
2. **Checks that your GitHub account can see the team repository.** If it can't, it tells you to ask the person who invited you to add you, and to accept the invitation email. Nothing on your computer changes.
3. **Downloads the team's topics** to your computer.
4. **Tells you how you'll appear**, for example `You'll appear to teammates as Priya Shah (psmith).` Your name comes from the team roster that the manager keeps, matched to your computer's login name. If you see `You're not in the team roster yet`, send the manager the command it prints; you can keep working meanwhile.

~~**Ask for your name.**~~ Dossier doesn't ask for your name; it uses your work login name (for example `psmith`) and the display name the manager entered. To change how your name is shown, ask the manager.

> **If `join` fails** (wrong link, no access, no network), it says why and changes nothing you need to clean up. Fix the cause and run the same command again. Anything the failed attempt created is moved to a folder next to your store, named like `~/.dossier.failed-join-<time>`. It is kept, not deleted, and you can remove it once you've joined.

**If `gh` isn't installed**, `join` stops and prints how to install it. **If your GitHub sign-in later expires** (Dossier or Claude says `sync_auth_failed`), run `dossier signin`.

### Manager checklist for a new teammate

1. Add them to the GitHub repository (Settings → Collaborators).
2. Add them to the roster: `dossier team add <their username> "<Their Name>"`, then `dossier sync`. Their username is their work login name (what `id -un` shows on a Mac, or the part after the `\` in `whoami` on Windows).
3. Send them the store link and this page.
4. After they join, have them open Claude in their usual work folder and ask "what's assigned to me?". Check that it finds their first assignment and binds it.

### Starting work on an assignment (primary path)

You don't need any Dossier commands or topic IDs.

1. Open Claude the way you normally do, in the folder where the work files are.
2. Ask for your work in plain words, for example "What's assigned to me?" or "Let's continue the pricing review." Claude knows who you are from your computer's login name. Claude finds the Dossier, loads its brief and starts from there. If more than one topic matches, it will ask you which one.
3. As decisions and results come in, ask Claude to save them to the Dossier. The end of a session does not save anything on its own.

Prefer a list? Run `dossier sync`, then `dossier tui`. Press `f` to show only your topics, and `c` to open one in Claude. The list shows only what has already reached your machine, which is why you sync first.

### About sign-in

- **How it works:** Dossier uses the GitHub CLI's sign-in. `gh` keeps the credential, normally in your system's secure storage (Keychain on a Mac, Credential Manager on Windows); Dossier asks `gh` for it when it syncs and never writes it anywhere itself.
- **What it can reach:** it is your normal GitHub sign-in, so it is not limited to the team repository. Your organization allows the GitHub CLI.
- **Alternative (no `gh`):** a fine-grained token with Contents read/write on the team repository, saved to `~/.dossier/credentials` (on a Mac, `chmod 600` it). Ask the person who set up the store if you need this.

## Day-to-day: it mostly just works

Once you've joined, your work on a Dossier is **always saved on your machine first** — even with no internet.

Syncing starts simple: a manual step you run when you want to share your latest work or catch up to your colleagues:

```text
dossier sync
```

~~A later phase makes syncing happen **automatically** around your saves and lookups, so you won't have to think about it (currently in pilot testing).~~ **Actual:** automatic sync is partly built. It runs when a Claude session starts and ends (`internal/core/service_session.go:215-219`, `:498-502`), and in the background after the agent reads, saves or renames a Dossier (`internal/mcp/tools.go:324,419,611`). It does **not** run after changes you make with `dossier` commands or in the dashboard. Run `dossier sync` after those.

Either way, a flaky connection never loses your work. If a sync can't reach the team store right now, Dossier tells you plainly and keeps your changes safe until the next sync. A failed `dossier sync` says "Sync failed" and why. When an automatic sync fails, Claude tells you at the start of your next session, or during the session after it saves. The dashboard (`dossier tui`) also shows a health line at the bottom, for example `Team sync · last sync failed 18m ago · work is safe locally`. It updates on its own about once a minute. Press `H` for the full report.

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
