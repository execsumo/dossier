# Dossier — PR/FAQ

> Codename: chainlink. Amazon-style working-backwards document.
> Status: draft for v1 (objective-critical core). Date: 2026-06-14.

---

## Press Release

**Dossier keeps your long-running agent work alive across Claude Code sessions — without the bloat.**

*A durable memory layer for people who run many parallel threads of work through CLI coding agents.*

**SAN FRANCISCO — June 14, 2026** — Today we're introducing **Dossier**, a local, single-user memory layer that lets technically-savvy business users carry a topic of work across many agent sessions. v1 supports Claude Code and Pi; Pi requires a compatible Claude-like hooks extension. Dossier shows visible capability notices when a session cannot provide the full hook or transcript-capture experience.

People who drive their work through coding agents hit the same wall: a serious topic doesn't finish in one session. They come back days later and the context is gone, scattered across `/resume` histories that mix throwaway chats with work that matters. Resumed sessions are bloated with false starts and small talk. And the moment they want to switch from one agent to another, the thread breaks entirely. The `handoff.md` pattern proved people *want* a durable, portable context file — but maintaining one by hand, per topic, across 20 topics a day, doesn't scale.

Dossier makes a topic a first-class, durable object. You promote any session into a **Dossier**: the critical information on the topic — the situation, the decisions made (and by whom), open questions, and the next action — with the noise (niceties, small talk, dead ends) stripped out, backed by an archive of the raw material that supports it: transcripts when the harness exposes them, source snapshots, text files, queries, and links. Its distilled state is stored as plain Markdown you can open in any reader, with artifacts and audit history beside it. When you start any agent session, your open Dossiers and what each one needs next are surfaced automatically where hooks are available, or through MCP/context-file fallback where they are not. You pick one up and the agent resumes with exactly the distilled context it needs — with a clear token target and warning if the Dossier is getting too large — with the full archive one search away when it's needed.

"The thing I kept losing wasn't the chat — it was the *state of the work*: what we decided, why, and what's left," said the first user. "Dossier is the difference between starting cold and starting where I left off, in whatever tool I happen to open."

Dossier is built around three deliberate choices. **One:** the distilled critical information and the captured source archive are kept separate, so context stays focused and citable at the same time — every material claim links back to the source that justifies it. **Two:** the agent decides what's worth keeping and writes the update itself — no review step to slow you down — and because captured raw material is never deleted, any call it makes stays recoverable from the Archive. **Three:** it meets the agent where it already lives, through MCP, hooks where available, and a plain context file fallback, so unsupported capabilities degrade visibly instead of silently.

v1 is local and single-user, available as a CLI/TUI and as an MCP server that Claude Code uses through its hooks and MCP extension points. Pi is supported through a compatible Claude-like hooks extension. Support for other harnesses (Codex, Antigravity), sharing with colleagues, a web app, an in-app chat model, native binary attachment management, and automatic Slack/email/Drive ingestion are on the roadmap and intentionally out of v1.

To get started, run `dossier init`. After that, when you open a supported agent session, the agent sees your Dossier library, tells you any capability limitations, and can help you continue an existing Dossier or promote the current conversation into a new one. CLI commands remain available when you want direct control.

---

## Customer FAQ

**Who is this for?**
Technically-savvy business users who run many topics through CLI coding agents and need durable, portable context — not just code.

**What exactly is a Dossier?**
One distinct topic of work. Its **Distilled State** is stored as a single Markdown file: the topic's critical information with noise removed — situation, material claims, decisions, open questions, next action — *not* a chat recap. Beside that file is an **Archive** of supporting artifacts: transcripts when available, source snapshots, Markdown/text files, queries, and links. Multiple sources don't make multiple Dossiers; they're all artifacts under the one topic they support.

**How is this different from `/resume`?**
`/resume` replays a whole session, mixing durable work with throwaway chatter and carrying every false path forward. Dossier carries only the curated state of a *topic*, warns against a clear token target, and works across supported agents — not just the one that created the session.

**Does it work with agents other than Claude Code?**
Yes, with limits. Dossier v1 fully integrates with Claude Code. Pi is supported when the user's existing Claude-like hooks extension provides the lifecycle calls, session identity, and transcript path described in `HANDOFF.md`. Codex and Antigravity remain out of scope. If a capability is unavailable in a given session, Dossier makes that limitation visible during install and again when the Dossier library is loaded at session start.

**How do I start a Dossier mid-conversation, when I realize a chat matters?**
Ask the agent to make this conversation a Dossier. Under the hood, `dossier promote` turns the current session into a new Dossier. If it actually belongs to an existing topic, `dossier link` connects it — and Dossier suggests likely matches so you don't have to hunt. When the match is ambiguous, the agent asks which thread to use instead of silently guessing.

**What is the normal day-one workflow?**
Install once with `dossier init`. On a new agent session, the agent receives your Dossier library, tells you whether transcript capture and save hooks are available in that harness, and asks whether you want to continue an existing topic or start fresh. If the conversation becomes durable, you ask the agent to promote or link it. From then on, that session has one active Dossier; the agent keeps it current and saves at supported session boundaries.

**How do I see what's on my plate without leaving the agent?**
Run **`/dossier`** in-session to list your Dossiers grouped by status (or `/dossier active`, `/dossier blocked` to filter). The list is ordered by priority — not just recency — using each Dossier's importance, urgency, and due date, so an overdue, high-importance topic sits at the top and a fresh low-priority one drops down. You set those fields once and surfacing does the rest.

**Won't the context get huge over time?**
Dossier targets **100k tokens** for the Distilled State context. That is a warning threshold, not a hard stop: the Distilled State loads in full by default, and if it is over target the agent tells you clearly and helps you decide whether to reorganize, archive resolved material, split the topic, or keep going. Raw Archive artifacts are not loaded by default; they are pulled in on demand via search.

**If it summarizes my work, won't it drop something I needed?**
The agent decides what to keep in ordinary saves — no confirmation step, because that friction is exactly what we're trying to avoid. The safety net isn't a review gate, it's that distillation never *deletes*: raw material stays in the Archive where available, fully searchable and linked. If the distilled state omits something, you pull it back from the Archive — nothing is lost, just not surfaced by default. Dossier still asks for human input when an action is ambiguous or contradiction-prone, such as choosing which existing thread to link or resolving a merge conflict.

**Can I trust a decision it records?**
Each material claim in the Distilled State links to the artifact that justifies it — the transcript moment when available, the data, the snapshot of the thread where it was settled. Provenance is built into the model, not bolted on.

**What happens when two threads turn out to be the same topic?**
You merge them. Dossier asks which Dossier should survive as the target, produces one converged Distilled State, and surfaces any conflicts for you to resolve, rather than guessing.

**Where does my data live, and what format?**
On your machine. Each Dossier has a directory with a plain Markdown distilled-state file, a YAML frontmatter header for status and next action, text-first artifacts, and an audit log. No database. You can open, read, and search the Markdown in any reader (e.g. Obsidian) without Dossier running. v1 is local and single-user; nothing is shared or sent anywhere unless you later opt into roadmap features.

**Does Dossier pull in my Slack threads, emails, and docs automatically?**
No. Dossier *stores* the material you or your agent bring to it — it doesn't fetch from external sources itself. Your agent already has its integrations; when it pulls in a Slack thread or doc, Dossier saves that as a citable snapshot. Sourcing is the agent's job; retaining and organizing it is Dossier's.

**Does Dossier capture the whole agent transcript?**
When Claude Code exposes a transcript, yes: Dossier captures it deterministically as an Archive artifact. When transcript access is unavailable in a session, Dossier says so during installation and again when your Dossier library is shown at session start, so you know exactly what is and is not being retained.

**Can two sessions work on different Dossiers at the same time?**
Yes. The active Dossier is per agent session, not global. Two sessions can work on two different Dossiers, even in the same harness. You can switch a session's active Dossier with `/dossier`, by asking the agent naturally, by starting a new session, or after `/clear`. Clearing a session removes the Dossier from that session's context; it does not delete the Dossier.

**Can I share a Dossier with a colleague?**
Not in v1. Sharing, a web app, an in-app chat model, and automatic Slack/email/Drive ingestion are deferred.

**Can I delete a Dossier?**
Not through Dossier in v1. You can ask the agent to archive it, which hides it from the default open-work view while keeping it searchable. If you truly want deletion, you delete the Dossier folder directly.

---

## Internal FAQ

**Why a flat set of distinct Dossiers instead of a graph of linked topics?**
Because the user's mental model is that a topic is self-contained: extra sources are *artifacts of one topic*, not evidence of many. Modeling inter-topic relationships as a persistent graph adds navigation and maintenance cost for a relationship that, in practice, is resolved by **merging** two Dossiers into one. Flat + merge is simpler and matches how the work is actually reasoned about.

**Why separate Distilled State from Archive instead of one evolving document?**
Two requirements pull opposite ways: "keep only the critical information, strip the noise" wants a focused working document; "cite the actual Slack thread verbatim" wants raw fidelity. One document can't be both. Splitting them lets the Distilled State carry the full substance of the topic (noise removed, but nothing important compressed away) while the Archive stays citable. The provenance links across the two layers are what make citation a *property of the model* rather than a feature to add later. Note: "distilled" means *noise removed*, not *made short*, but the Distilled State does have a token target so the product can warn when a topic is sprawling.

**Why no review step — isn't "the agent decides" too loose?**
A confirm-on-every-write gate adds friction on every single save — directly counter to low-overhead capture across ~20 topics a day. But "the agent freely decides whether and what to write" *would* be too loose, so we tighten it on both axes without a gate. **What** to keep is steered by a shipped, rigorously developed **Distillation Guide** (a skill the agent loads) — explicit rules on what's critical vs. noise. **When** to write is enforced where hooks are available: the agent keeps the session's active Dossier current best-effort each turn, with **deterministic hook backstops** that force a final save on session end (including `/clear` and `/exit`) and before context compaction. The trust mechanism for content is **non-destruction** — the Archive keeps source where the harness can provide it, so a wrong call is recoverable, not fatal — plus optional after-the-fact edits. Guided + enforced-cadence + non-destructive, never gated for ordinary saves; ambiguous link and merge conflict resolution still ask the human.

**Why MCP *and* a context file?**
MCP is the cleanest "agent recalls topics from inside the harness" path and is increasingly common, but not universal. A generated context file is the universal fallback so degradation is graceful. The hard requirement is visible degradation: if a harness cannot support deterministic load, save hooks, or transcript capture, Dossier says so instead of pretending.

**What's the riskiest part of v1?**
Distillation quality and the suggestion/merge engine. If distilled state is wrong or merges mangle history, users stop trusting it. Since we've removed the confirm gate (it added friction), the safety net is structural: never destroy captured source artifacts, keep an audit trail, allow optional after-the-fact edits, and surface merge conflicts explicitly. A bad distillation degrades surfacing, not retention.

**What is explicitly out of scope for v1?**
Sharing/multi-user, web app, in-app LLM wrapper, native binary attachment storage, and automated ingestion integrations (Slack/email/Drive OAuth). v1 = local single-user core: Dossier store, MCP + CLI/TUI, lifecycle/status, search, promote/link/merge.

**How do we know it's working?**
The user resumes a real topic in a *different* supported agent than created it and reaches productive work without re-explaining context, with clear token-target warnings when needed — repeatedly, across days. See Success Metrics in the PRD.
