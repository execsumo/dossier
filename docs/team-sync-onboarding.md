# Team Sync — Onboarding for Teammates

> Audience: the least-technical teammate joining a shared Dossier store.
> You do **not** need any developer tools for this. If you can run one command and sign in once, you're in.

> **Status (Pilot): The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub.**
> Treat this as an experimental feature.

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

1. **Ask for your name.** This is how your contributions are attributed to you across the team.
2. **Ask you to sign in once** with a **personal access token (PAT)** — a private, password-like code from GitHub that lets Dossier talk to the shared store on your behalf.

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

A later phase makes syncing happen **automatically** around your saves and lookups, so you won't have to think about it (currently in pilot testing).

Either way, a flaky connection never loses your work. If a sync can't reach the team store right now, Dossier tells you plainly and keeps your changes safe until the next sync.

## If two of us edited the same thing

Sometimes you and a colleague both edit the same topic. That's fine.

- **Nothing is lost**, and there are never any messy conflict markers in your files.
- The version that's already in the shared store stays in the topic file.
- **Your version is saved right alongside it** as a short note in a `conflicts/` folder, for you to reconcile.
- You'll get a friendly heads-up that there's something to reconcile, and Dossier's dashboard walks you through it step by step — keep whichever parts you want.

Both perspectives are preserved — neither is silently overwritten. If you and a colleague disagree, the disagreement is recorded openly rather than smoothed over.

## What never leaves your machine

Some things are yours alone and **never travel** to the shared store:

- **Your local settings** (`config.yaml`) — your machine's install details.
- **Your local session list** — which topic each of your sessions is looking at.
- **Your locally generated context** — the helper notes Dossier writes for your machine.

These are specific to your computer, so sharing them would overwrite someone else's setup. They stay put, by design.

Everything that's **about the topics themselves** — your distilled notes, the captured source material, your per-topic session captures, and the audit trail — does sync, so the team sees it.

---

*Questions about joining? Ask the person who set up your team store. For the technical plan behind this feature, see `docs/team-sync-plan.md`.*
