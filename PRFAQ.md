# Dossier — PR/FAQ

> Codename: chainlink. Amazon-style working-backwards document.
> Status: **v1 shipped** (all milestones complete). v02 — the team dimension —
> is planned; see `VISION.md` for the forward narrative and `PLANv02.md` for the
> plan. Originally drafted 2026-06-14 · Rewritten against shipped reality 2026-08-05.
>
> **Scope of this document:** what exists today and why it was built that way.
> Where it describes something not yet built, it says so explicitly.

---

## Press Release

**Dossier keeps a topic of work alive across agent sessions — and across the
people working on it.**

*A local, durable memory layer for business leaders who run their work through
CLI coding agents.*

**SAN FRANCISCO** — Dossier is a local memory layer that lets a business
operator carry a topic of work across many agent sessions without re-explaining
it each time. It is fully integrated with Claude Code; Pi is supported for
session identity through an extension Dossier installs itself.

People who drive their work through coding agents hit the same wall: a serious
topic doesn't finish in one session. They come back days later and the context
is gone, scattered across `/resume` histories that mix throwaway chats with work
that matters. Resumed sessions are bloated with false starts and small talk. And
the moment they want to switch agents, the thread breaks entirely. The
`handoff.md` pattern proved people *want* durable, portable context — but
maintaining one file per topic, across twenty topics a day, doesn't scale.

Dossier makes a topic a first-class, durable object. You promote any session
into a **Dossier**: the critical information on the topic — the situation, the
decisions made and by whom, what was ruled out and why, open questions, and the
next action — with the noise stripped out, backed by an archive of the raw
material that supports it. Its distilled state is plain Markdown you can open in
any reader, with artifacts and audit history beside it. When you start an agent
session, your open Dossiers are surfaced automatically, ordered by priority. You
pick one up and the agent resumes with exactly the distilled context it needs —
with a clear token target and a warning if a topic is sprawling — with the full
archive one search away.

**What shipped beyond the core.** Each Dossier carries a **lead** — the person
accountable — and can be tagged with the recurring **meeting forums** where it
belongs (1:1, steering committee, weekly business review, and so on). The
terminal UI opens on a lead picker, so preparing for a meeting starts by
choosing a person and narrowing to a forum, rather than by searching. A bundled
**delegation skill** turns a Dossier into a handoff for a human colleague,
tuned for teams working across an offset: it asks where the reader will stall
rather than walking a checklist, prioritises decision rights and escalation
above all else, and persists the agreed contract so completion can be checked
later against what was actually agreed.

"The thing I kept losing wasn't the chat — it was the *state of the work*: what
we decided, why, and what's left," said the first user. "Dossier is the
difference between starting cold and starting where I left off."

Dossier is built around three deliberate choices. **One:** the distilled state
and the captured source archive are kept separate, so context stays focused and
citable at the same time — every material claim links back to the source that
justifies it. **Two:** the agent decides what's worth keeping and writes the
update itself — no review step to slow you down — and because captured raw
material is never deleted, any call it makes stays recoverable. **Three:** it
meets the agent where it already lives, and unsupported capabilities degrade
visibly instead of silently.

Install with `brew tap execsumo/tap && brew install dossier`, then `dossier
init`. After that, when you open a Claude Code session, the agent sees your
Dossier library and can continue an existing topic or promote the current
conversation into a new one. The CLI and terminal UI remain available when you
want direct control.

**What comes next** is the team dimension: structured tracking of what is
required and from whom, escalation along the reporting line, colleague
calibration profiles, timezone-aware handoffs, and eventually a shared team
memory. `VISION.md` describes that end state for a business audience.

---

## Customer FAQ

**Who is this for?**
Business leaders and operators — pricing, growth, operations, strategy — who run
many topics through CLI coding agents and need durable, portable context. The
tool lives in a terminal; the work it holds is business work, not code.

**What exactly is a Dossier?**
One distinct topic of work. Its **Distilled State** is a single Markdown file:
the topic's critical information with noise removed — situation, decisions,
findings, current state, next action — *not* a chat recap. Beside it is an
**Archive** of supporting artifacts: transcripts where available, source
snapshots, files, queries, and links. Multiple sources don't make multiple
Dossiers; they are all artifacts under the one topic they support.

**How is this different from `/resume`?**
`/resume` replays a whole session, mixing durable work with throwaway chatter
and carrying every false path forward. Dossier carries only the curated state of
a *topic*, warns against a clear token target, and is not tied to the session or
the agent that created it.

**Does it work with agents other than Claude Code?**
Claude Code is fully integrated: hooks, MCP, and transcript capture. **Pi is
partially supported.** Dossier installs its own Pi extension, which gives Pi
sessions a reliable identity so a Dossier can be bound and switched. Pi does not
yet get session-start surfacing, end-of-session saves, or pre-compaction saves,
and Pi has no built-in MCP client — `dossier harness list` reports all of that
as unavailable rather than pretending otherwise. Codex and Antigravity are out
of scope. If a capability is missing in a session, Dossier says so at install
and again at session start.

**How do I start a Dossier mid-conversation, when I realise a chat matters?**
Ask the agent to make this conversation a Dossier. If it actually belongs to an
existing topic, the agent links it instead — and Dossier proposes likely matches
so you don't have to hunt. When the match is ambiguous, it asks which thread to
use rather than silently guessing.

**How do I see what's on my plate?**
Ask the agent ("what's open?"), run `dossier ls`, or open the terminal UI by
running `dossier` with no arguments. The UI opens on a lead picker — choose a
person, and the dashboard scopes to their topics; press `i` to cycle to a
specific meeting forum. There is no `/dossier` slash command; the agent surfaces
your library at session start and answers in conversation.

**How is the list ordered?**
By a 2×2 of importance and urgency (the Eisenhower quadrants), then by due date,
then by how long a topic has gone untouched. **An overdue item does not
currently escalate above its quadrant** — this is a known limitation, and
changing it is one of the first things in v02: importance becomes the primary
ordering with time pressure computed inside it, plus a tier above everything for
work that is blocking another person.

**Who owns a topic?**
Each Dossier has a `lead`. It is set when you promote a topic or changed at any
time, from the CLI, from the agent, or inline in the terminal UI. Ownership is
what makes the meeting-prep and (in v02) escalation views possible.

**Can it help me hand work to a colleague?**
Yes — that's the `dossier-delegate` skill, installed with `dossier init`. Invoke
it explicitly (`/dossier-delegate`, or just ask for help defining a piece of
delegated work) and it reads the Dossier as the person receiving it would: "they
wake up with no way to reach you for hours — where do they stall?" It asks about
the one or two things genuinely missing, usually decision rights and escalation,
then persists the agreed contract onto the Dossier and renders a paste-able note
from it. It never produces a completeness score, and it never runs on its own —
structure is available on demand, never imposed.

**Won't the context get huge over time?**
Dossier targets 100k tokens for the Distilled State. That is a warning
threshold, not a hard stop: the state loads in full, and if it is over target the
agent tells you and helps you decide whether to reorganise, archive resolved
material, split the topic, or keep going. Archive artifacts are not loaded by
default; they are pulled in on demand.

**If it summarises my work, won't it drop something I needed?**
The agent decides what to keep in ordinary saves — no confirmation step, because
that friction is exactly what we are avoiding. The safety net isn't a review
gate; it's that distillation never *deletes*. Superseded content moves to history
and the Archive, fully searchable. Dossier still asks for human input where an
action is ambiguous or contradiction-prone: choosing which existing thread to
link, or resolving a merge conflict.

**Can I trust a decision it records?**
Each material claim links to the artifact that justifies it — the transcript
moment where available, the data, the snapshot of the thread where it was
settled. Provenance is part of the model, not a feature bolted on.

**What happens when two threads turn out to be the same topic?**
You merge them. Dossier asks which should survive, produces one converged state,
and surfaces conflicts for you to resolve rather than guessing. Sources are
archived, never discarded.

**Where does my data live, and what format?**
On your machine, under `~/.dossier`. Each Dossier is a directory with a plain
Markdown file, YAML frontmatter for status and ownership, text-first artifacts,
and an audit log. No database. You can read and search it in any Markdown reader
without Dossier running. Nothing is shared or sent anywhere.

**Does Dossier pull in my Slack threads, emails, and docs automatically?**
No. Dossier *stores* the material you or your agent bring to it — it doesn't
fetch from external sources itself. Your agent already has its integrations;
when it pulls in a thread or a document, Dossier saves that as a citable
snapshot. Sourcing is the agent's job; retaining and organising it is Dossier's.

**Does it capture the whole agent transcript?**
When Claude Code exposes one, yes — deterministically, as an Archive artifact.
When transcript access is unavailable, Dossier says so at installation and again
when your library is shown at session start, so you know what is and is not
being retained.

**Can two sessions work on different Dossiers at once?**
Yes. The active Dossier is bound per agent session, not globally. Two sessions
can follow two different topics simultaneously without stepping on each other.

**Can I share a Dossier with a colleague?**
Not yet. Today Dossier is single-user and local. A shared team memory — where a
request you record against a colleague appears on *their* list, and their answer
appears on yours — is the committed end state of the roadmap, deliberately
sequenced last so the model is validated by real use before distribution is
added on top. See `VISION.md` §Layer 5.

**Can I delete a Dossier?**
Not through Dossier. You can archive it, which hides it from default views while
keeping it searchable and recoverable. If you truly want deletion, delete the
folder directly. Non-destruction is the trust mechanism that lets ordinary saves
skip a confirmation step, so we don't offer a delete that would undermine it.

---

## Internal FAQ

**Why a flat set of distinct Dossiers instead of a graph of linked topics?**
Because the user's mental model is that a topic is self-contained: extra sources
are *artifacts of one topic*, not evidence of many. A persistent inter-topic
graph adds navigation and maintenance cost for a relationship that in practice
is resolved by **merging** two Dossiers into one. Flat plus merge is simpler and
matches how the work is actually reasoned about.

**Why separate Distilled State from Archive instead of one evolving document?**
Two requirements pull opposite ways: "keep only the critical information" wants a
focused working document; "cite the actual thread verbatim" wants raw fidelity.
One document can't be both. Splitting them lets the Distilled State carry the
full substance of the topic while the Archive stays citable. The provenance links
across the two layers are what make citation a *property of the model* rather
than a feature to add later. Note: "distilled" means *noise removed*, not *made
short* — but there is still a token target so the product can warn when a topic
is sprawling.

**Why no review step — isn't "the agent decides" too loose?**
A confirm-on-every-write gate adds friction on every save, directly counter to
low-overhead capture across twenty topics a day. But "the agent freely decides"
would be too loose, so we tighten both axes without a gate. **What** to keep is
steered by a shipped **Distillation Guide** the agent loads. **When** to write is
enforced by hooks: best-effort each turn, with deterministic backstops forcing a
save on session end (including `/clear` and `/exit`) and before compaction. The
trust mechanism for content is **non-destruction** plus after-the-fact edits.
Guided, enforced-cadence, non-destructive — never gated for ordinary saves.

**Why is the Distillation Guide injected through the tool response rather than
the system prompt?**
Putting a 1,500-token guide into global instructions taxes every unrelated
coding task. Instead, the moment an agent binds a Dossier, the response carries
the guide and the operating instructions with it. Zero cost when you're not
using Dossier; full steering the moment you are. The same principle trimmed the
session-start hook down to a one-line nudge, because the hook fires on *every*
session, including ones with nothing to do with Dossier.

**What did we get wrong about Pi?**
We assumed the user's own hooks extension would supply session identity through
environment variables. Reading Pi 0.83.0 showed those variables reach bash-tool
children only, and that Pi has no built-in MCP client — so a Dossier MCP server
under Pi had no session identity at all. Dossier now ships its own Pi extension
that publishes a per-session pointer. Lifecycle bridging remains genuinely
missing and is reported as unavailable rather than implied. This is the clearest
case so far of the standing rule: when reality contradicts a document, change
the document.

**What's the riskiest part of the product?**
It was distillation quality and the merge engine; both have held up. The current
risk is different: **the model of other people is thinner than the product's
positioning.** Ownership is a free-text name, "waiting" is a passive status with
no record of who or since when, there is no notion of who is looking at the
tool, and nothing knows what time it is where a colleague works. That gap is
what v02 exists to close.

**What is explicitly not built?**
Sharing and multi-user, a web app, an in-app chat model, native binary
attachments, automated ingestion (Slack/email/Drive), and semantic search. Of
these, only sharing has moved from "deferred" to "committed, sequenced last."

**How do we know it's working?**
Someone resumes a real topic in a different session than created it and reaches
productive work without re-explaining context — repeatedly, across days.
`VISION.md` §8 restates the measures in team terms; the PRD carries the v1
metrics.

---

## Related documents

- **`VISION.md`** — where this is going, written for business leaders and team
  members. Start there for the *what* and *why* of the team dimension.
- **`PLANv02.md`** — the plan for building it, with sequencing and open decisions.
- **`PRD.md`** — the v1 product requirements.
- **`HANDOFF.md`** — implementation status and reading order for engineers.
