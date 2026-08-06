# Dossier — Vision

> **Purpose of this document.** To describe what Dossier is for, who it serves,
> and the working patterns it supports — so business leaders and team members
> can tell us whether the vision is right before more of it gets built.
>
> This is deliberately not a technical document. It describes what the product
> does and how it feels to use, not how it is built. Where something does not
> exist yet, we say so (see *Where we are today*).
>
> **What we want from you:** disagreement. Specific questions are at the end.

---

## 1. The vision

**Dossier is the working memory of a distributed team.**

Every serious piece of work is carried by people who each hold part of the
picture in their heads — what was decided and why, what is still owed and by
whom, what a colleague already understands and what they will need explained.
That knowledge lives in Slack threads, in a document someone made once, in the
gap between two meetings, and mostly in memory. It survives right up until
someone takes leave, changes role, works across an eight-hour offset, or simply
comes back on Monday.

Dossier makes that knowledge durable and shared, without turning it into a job
somebody has to do. You do your work through the AI agent you already use. The
agent keeps the record: what this topic is, what was decided, what is needed
from whom, what happens next. When you come back — tomorrow, or in three weeks, or as a
different person entirely — the state of the work is there, and it is honest
about where it came from.

The measure of success is simple: **nobody should ever lose a day to a question
that was already answered.**

---

## 2. Who this is for, and what breaks today

Dossier is built for **business leaders and their teams who drive work through
AI agents** — people running pricing, growth, operations, or strategy, not
people writing software. The tool happens to live in the same terminal as the
agent, but the work it holds is business work.

One thing about that work shapes everything below. Very little of it is
delivered by the people on your team alone. A growth lead with five reports gets
things done through Finance, Legal, Product, an agency, a peer's roadmap and an
executive's calendar — none of whom they manage, most of whom they cannot chase.
So when this document says *team*, read it as the whole set of people a piece of
work actually runs through. The org chart is one useful fact among many; it is
not the shape of the problem.

Five things break for these teams today, and each has a real cost:

| What breaks | What it costs |
|---|---|
| Context dies with the conversation | You re-explain the same background every time you pick a topic back up |
| Only one person holds the thread | When they are unavailable, the work stops rather than continuing |
| "Waiting on someone" is invisible | Things sit for a week before anyone notices they were never actually asked — or never actually agreed to |
| Handoffs are underspecified | A colleague eight hours away hits an ambiguity, and loses their whole day waiting for an answer |
| Meeting prep is rebuilt from scratch | Twenty minutes before every recurring meeting, reconstructing what you already knew |

None of these are exotic. They are Tuesday. And every one of them is a
knowledge problem, not an effort problem — which is why working harder does not
fix them.

---

## 3. What Dossier is, in three ideas

**A topic is a thing.** Not a folder, not a chat log — a durable object with a
name, an owner, a status, and a next step. You have twenty of them. Dossier
calls each one a *dossier*, and it holds one topic of work from start to
finish, however many conversations and people that takes.

**The record is curated, and the source is kept.** A dossier holds the *state*
of the work — the situation, the decisions and who made them, what was ruled
out and why, what is true right now, what happens next. Everything noisy is
stripped. But nothing is thrown away: the raw material sits behind the record,
and every meaningful claim links back to where it came from. You can always ask
"who decided that, and on what basis?" and get an answer rather than a memory.

**The agent maintains it; you do the work.** There is no form to fill in and no
weekly hygiene ritual. You have the conversation you were going to have anyway,
and the record updates. Dossier asks two things of you, and only two. The first
is **judgement** — who owns this, what is it worth, what do you need and from
whom — because those are the things it cannot infer. The second is a **word at
the boundary**: Dossier sees your conversations with your agent, and nothing
else. It cannot know that you finally sent Alex that message in Slack, or that
he replied yes. So when work crosses out of its view, it asks — *"mark this as
raised?"* — and you answer in three words, or paste the reply back. That is the
whole maintenance burden. What it will never do is guess, and then present the
guess as fact.

---

## 4. What makes it trustworthy

A memory system is only useful if you believe it. Four commitments, and each
one is a design constraint rather than an aspiration:

**Nothing is ever deleted.** When the record is updated, what was there before
moves into history — it does not disappear. If the agent's judgement about what
mattered was wrong, the material is recoverable. This is why there is no
"approve this update?" step slowing you down: safety comes from nothing being
destroyed, not from you checking every write.

This is a different thing from the confirmations in §3, and the difference is
worth holding onto. Dossier never asks your permission to record something it
watched happen — that is the gate we removed, and it stays removed. It asks only
when it needs you to **assert** something it could not see, because the
alternative is guessing about the world outside the conversation. One is a
bureaucratic checkpoint on the tool's own work; the other is the tool declining
to invent facts.

**Every claim carries its source — including how it got there.** A decision in a
dossier says who made it, when, why, and links to the conversation or document
that settled it. It also says how Dossier came to believe it: something it
*observed* in the work, something *you told it*, or something it *inferred*. The
three are never blurred together, because "Dossier thinks Alex agreed" and "Alex
agreed" are different facts and only one of them is evidence. This is what makes
the record defensible months later, to someone who was not there.

**It never guesses when it should ask.** Two topics that might be the same
thing, two people who edited the same record — Dossier surfaces the ambiguity
and asks. It does not quietly pick one.

**It informs; it does not nag.** This one is a deliberate stance. A tool that
tracks deadlines across five timezones could very easily become a machine for
generating low-grade dread — red counters, overdue tallies, a running score of
how far behind you are. We are explicitly not building that. Dossier tells you
what is true and what is next. It does not editorialise about your performance,
and it does not manufacture urgency. A tool that makes you anxious gets
abandoned, and an abandoned memory is worse than none.

---

## 5. What it feels like to use

Most of the time there is no screen at all. You work with your agent as you
already do; Dossier is what the agent knows.

**Opening a session.** The agent already knows what is open and what needs you.
You say "let's pick up the pricing migration" and it has the state — no
re-explaining, no hunting for the document.

**During the work.** You talk. Decisions get recorded with their reasoning. If
you say "I need the vendor contacts from Alex before Friday," that becomes a
tracked item, not a sentence that scrolls away.

**Stepping back.** There is a dashboard when you want to see everything at
once, arranged by what actually needs you. You can also just ask: "what is
waiting on Priya?" — and get an answer.

**Handing over.** When you delegate something, the agent helps you write a
handoff that is actually complete, and you paste it wherever your colleague
lives — Slack, email, a ticket.

The design principle underneath: **the fastest way in is a conversation, and the
second fastest is a screen you did not have to configure.** Anything that
requires setting up a view before it becomes useful has already failed.

---

## 6. The patterns

This is the substance of the vision. Five layers, in roughly the order you
would experience them.

---

### Layer 1 — Memory that survives the conversation

**The pattern:** a topic outlives the session it started in, the tool it
started in, and eventually the person who started it.

You work on something for an hour. That conversation ends. Three days later,
you or a colleague opens a new one and the topic is intact — not the chat
transcript, but the *state*: where things stand, what was settled, what is
open. The dead ends are recorded too, compressed into a line, so nobody
re-runs an experiment that already failed.

**Why it matters:** this is the foundation everything else sits on. Without
durable state there is nothing to delegate from, nothing to escalate, and
nothing to prepare a meeting from.

---

### Layer 2 — Accountability you can see

Two patterns, and together they are the biggest change for a leader.

#### 2a. "What's required, and from whom"

Most tools model this as *waiting* — a passive status that tells you something
has stalled but not what would unstall it. We think that is the wrong shape.

Instead, a dossier carries a short list of **requirements**: specific things
needed from specific people, each with the date it is needed by and the
**interface** where it gets raised — Dossier's word for a recurring meeting,
your 1:1s and standups and steering committees. Not a task list — a list of the
inputs that are blocking progress, and who holds them.

The wording here is deliberate, and it is not decoration. Something is **needed
from** a person; it is not *owed by* them, and they are not *the blocker*. Much
of this work runs through people nobody in the room manages — Finance, Legal, an
agency, a peer's team — and a record that reads as an accusation is a record you
cannot let those people see. Dossier states what progress depends on. It does
not assign fault, and it does not imply an authority you do not have.

The important distinction, and the one nothing else surfaces:

> **Has it actually been asked yet?**

*Not yet raised* means the ask exists in your head and nowhere else — **you**
are the one holding it up. *Raised* means it is genuinely with them, and the
clock starts there. In practice, a surprising share of "I'm waiting on Alex"
turns out to be "I never actually asked Alex," and that is a category of failure
no tool currently shows anyone.

There is a second gap behind it, and it bites hardest with the people you have
no authority over:

> **And did they actually agree?**

Raising something is not the same as getting a commitment. You can ask Legal for
sign-off by Friday and hear nothing; Friday arriving does not mean Legal is
late, it means nobody ever agreed to Friday. So a requirement can also be
*accepted* — and if the other person named their own date, that date is recorded
next to yours rather than replacing it. **Your needed-by is what you want.
Their committed-by is what you have.** Most of the disappointment in
cross-functional work lives in the gap between those two, and the gap is
invisible when a tool stores only one date.

This changes what deserves your attention. Something surfaces because it went
quiet with no acknowledgement, or because a commitment somebody actually made
has passed — not merely because a date you set alone went by. That distinction
is the difference between a signal and a nag.

**How it should feel:**

```
Pricing migration                                    Alex · due Fri
  ⌁ Vendor contacts             not yet raised — 1:1
  ⌁ Legal sign-off on tiering   raised 4d ago, no reply — Steerco
  ⌁ Q3 forecast numbers         Priya committed Thu 14th — WBR
```

That is the whole model: *not raised*, *raised*, *accepted*, *answered*, and
*dropped* for the ones that stop mattering — closed rather than removed, because
nothing here is ever deleted. What we are deliberately refusing is the next
twenty states: declined, replanned, in progress, at risk. Each is individually
reasonable and collectively they are a ticket tracker, which is the thing you
are using Dossier to escape.

#### 2b. Escalation that follows the real route

Ownership is recorded, and so is what to do when something stalls. That makes
three things possible that are usually left to memory — and only one of them
has anything to do with the org chart:

**Upward — your team's exceptions become your list.** When something a direct
report owns goes past its date, it appears on the leader's list, marked with
whose it is and why it surfaced. Not their whole plate — that would be noise
you learn to ignore within a fortnight — only the exceptions. A leader's view
is their own work, plus their team's problems.

**Sideways — the route is the relationship, not the org chart.** Most of what
actually blocks this work sits with people outside your line: Legal, Finance,
a partner team, an agency, an executive. A reporting structure says nothing
useful about any of them, so Dossier does not pretend otherwise. What it records
instead is the route you would actually use — the interface where this gets
raised, and where it goes next if that room does not settle it. *"Raise in the
launch steerco; if it is still open on Thursday, it goes to the VP."* That is a
fact about a relationship, and you already know it; it has simply never been
written down anywhere your agent can read.

**Downward — what you owe your team goes to the very top.** This is the half
that usually goes untracked. If a colleague is blocked on something *you* owe
them, and their day starts in four hours, that is the most expensive item on
your list — more expensive than most of your own work, because it is costing
someone else their day, not just delaying yours.

**Why it matters:** a leader's real job is unblocking, and the things needing
unblocking are exactly the things that do not show up on a personal to-do list.

---

### Layer 3 — Handoffs that do not cost a day

Two patterns that compound: one makes each handoff good, the other makes every
subsequent handoff shorter.

#### 3a. Delegation as a written contract

When you hand work to a colleague, especially one who will read it while you
sleep, the agent helps you write it properly. Not by making you fill in a
template — by asking the right question:

> *Read this as the person receiving it. They wake up, they have no way to
> reach you for eight hours. Where do they get stuck?*

That framing does the prioritising. The two things that actually cost a full
day on an offset team are almost always the ones missing:

- **Decision rights** — can they decide this themselves, or must they wait?
- **Escalation** — if they are blocked, what should they do *instead of waiting
  silently*? Including the crucial part: what to work on while they wait, so
  one open question does not cost them the whole day.

Everything else — the objective, the context, the constraints, what "done"
means and how it will be checked — the agent fills in from the dossier, because
it is usually already there. It asks about those only where the dossier is
genuinely silent or says two different things, which happens most often when the
work crosses into a function that was never part of the original conversation.
What it does not do is walk you through all seven every time to prove it
checked.

Two deliberate choices here:

**It only runs when you ask.** It never fires because a dossier looks thin or
because an owner was set. Structure is available on demand; it never becomes
overhead on the fast, informal path that works fine most of the time.

**It never gives you a score.** No "4 of 7 sections complete." Either it names
the specific thing that is missing, or it tells you it is ready. A completeness
score turns a judgement into a form, and forms get abandoned.

The contract is then stored on the dossier, and the message you paste to your
colleague is generated from it. That matters for the check-back later: you can
ask "did this meet what we agreed?" and get an answer against what was
*actually* agreed. If the goalposts moved in between, they moved — but you will
be told, rather than shown the new target as though it were the original.

**And it is only a contract once they say so.** The note tells your colleague
that three replies are useful — *accepted*, *I need one thing clarified*, or *I'd
propose changing this* — and whichever comes back gets recorded against the
handoff. This is the same honesty as Layer 2a, one level up: a brief you sent is
not a brief they took on. Until someone accepts, what you have is a proposal, and
Dossier will say so rather than let you spend a week believing the work is
underway.

#### 3b. The colleague profile that learns — in both directions

The most expensive part of delegating is calibration: explaining things they
already know, and failing to explain the one thing they do not.

So each person in the team has a short profile. Not a bio, not an assessment —
a **calibration note**, and specifically about the *gap* between what work
usually assumes and what this person actually has.

> *Alex already understands cash-flow discounting — don't explain it. Alex
> doesn't have contacts at the website-builder vendor — supply them explicitly
> every time.*

That single pair of facts saves both people time on every handoff: you skip
what is redundant, and you pre-empt the blocker.

**It has a second direction, and for a leader it may be the more valuable one.**
Nobody delegates *to* the leader — but everyone reports *to* them, and every
report is guessing at the same three things: what will they ask, what shape of
update do they want, what do they consider finished. Written down once, those
questions get answered in the update rather than in a round-trip after it.

So a profile is not "how to manage this person." It is:

> **How to exchange work with this person without a wasted round-trip** — in
> whichever direction the work is moving.

**And it improves as a by-product of use.** After a handoff completes, the agent
notices what you had to explain and offers to update the profile: *"Alex now
has the vendor contacts — move them to 'already has'?"* Nobody sits down to
maintain it; it gets better because you used it. Two guardrails on that: the
agent **proposes, it never silently learns** — a note about a person changes
only when a person agrees to the change. And entries get stale, because people
grow. What was true about Alex a year ago is a liability now, so old entries are
shown as old and can be reconfirmed or retired rather than quietly accumulating.

**Who writes it matters.** These notes should be co-authored with the person
they describe, and ideally written by them. Two reasons, and both are real:
Alex knows what Alex does not know far better than you do — and a note about a
colleague that they have never seen is a liability, whereas one they wrote is
simply useful. The discipline is straightforward: strictly factual, strictly
about the work, never about performance or personality, and written as though
they will read it. Because they should.

---

### Layer 4 — A rhythm that prepares itself

#### 4a. Meetings that arrive pre-assembled

Your week has a shape: 1:1s, a standup, a weekly business review, a steering
committee. Each of these is an interface, each has its own purpose, and each
currently costs twenty minutes of reconstruction beforehand.

In Dossier, a topic is tagged with the interfaces it belongs to, and each
interface has a short note of its own — what it decides, what belongs there, and
importantly **what does not** (so the agent can say "this belongs in the
solutioning session, not the steerco").

Then preparing is one request:

```
prep my 1:1 with Alex
```

and you get the interface's purpose, Alex's live topics, what you are waiting on
from him, **what he is waiting on from you**, anything of his that has gone past
its date, and the parts of his profile that matter for what you are about to
discuss. Group interfaces work the same way, organised by person instead of
scoped to one.

**The refinement that makes it usable:** an agenda that lists everything
in-flight teaches you to skim it, and then you miss the one item that mattered.
So prep shows what you still need to **raise**, plus what needs a **chase** —
raised and gone quiet with no acknowledgement, or accepted and now past the date
they themselves named. Things asked and still within their window stay out of
the way. You walk in with the list of what needs saying, not an inventory of
everything outstanding.

#### 4b. Time that respects whose day it is

A distributed team runs on other people's clocks. Dossier knows each person's
timezone and working hours, and uses them for things that are otherwise guesswork:

- **When their day starts.** *"Alex's day begins in three hours — two open
  requests are routed to him."*
- **When to send.** *"Send by 16:40 for Priya to have a full working day on
  this."*
- **How long something has really been waiting.** "Asked four days ago" is
  wrong if two of them were their weekend. Aging counts their working days.

Consistent with the stance in §4: these are **boundaries, not countdowns**.
Passing 16:40 means Priya starts tomorrow, which is usually fine. It is stated
so you can choose, not so you feel late.

---

### Layer 5 — One memory, shared by the team

Everything above works for one person. The end state is a team that shares one
memory.

When it lands: a requirement you record as needed from Ryan appears **on Ryan's
own list**, as something he has been asked for, without anyone pasting anything.
His answer appears on yours. A colleague picks up a topic you started and gets the same
state you had, not a summary of it. Someone returns from leave and reads what
happened instead of asking.

Two things we are being deliberate about:

**Not everything can be shared.** A leader has topics that cannot go into a
team-visible space — compensation, performance, a reorganisation, a hire. That
is not an edge case, it is a normal week, and a shared memory that cannot hold
that boundary safely is one we should not ship.

We have also learned that "mine" and "the team's" is probably too coarse a
split. A single growth topic can carry a number the whole team should see, a
board commitment that stops at the leadership group, an agency contract, and a
vendor conversation that stops with one person. Whether that means named
audiences per dossier, a smaller set of levels, or something else, we do not
know yet. One requirement is already clear whatever we choose: if a claim is
drawn from a restricted source, showing the claim must never expose the source.
A summary that quietly leaks what it summarised is worse than no sharing at all.
How this separation works is the open question we most want your view on, and
until it is answered convincingly, compensation and performance topics stay out
of shared memory entirely.

**Sharing raises the bar on colleague profiles — which is why we set it early.**
In a shared team memory, Alex can read Priya's profile. A note that has been
factual, task-scoped and co-authored from the beginning needs no cleanup when
that day comes. One written as private commentary becomes a problem the moment
there is a second reader. The discipline described in Layer 3 is not
politeness — it is what makes the shared memory safe to have.

---

## 7. What Dossier deliberately is not

Being clear about this is as useful as the vision itself, and each one is a
choice rather than a gap:

**Not a project management tool.** No sprints, no boards, no burndown, no
percent-complete. Dossier holds the *knowledge* of the work, not its
choreography. If it grows subtasks and reminders, we have built a worse Jira.

The line is worth drawing precisely, because a team that keeps two versions of
the truth will eventually trust neither. Dossier holds the things no execution
tool has ever held well: what is going on and why, what was decided and by whom,
what was ruled out, what this work depends on from outside it, what was agreed
in a handoff, and where every claim came from. If you run a board or a tracker,
that keeps what it is good at — tasks, delivery dates, execution state — and
Dossier points at it rather than shadowing it. If you do not run one, which is
the normal case for the teams we are building for, Dossier is not going to
become one. Its answer to "what is the state of this work" will always be a
paragraph you can read, not a percentage.

**Not a system of record for HR.** Colleague profiles are about how to exchange
work — what someone knows, needs, and prefers. They are never about
performance, and the product should make writing that kind of note feel wrong.

**Not a chase-bot.** Dossier will compose the message; it will not send it.
Automated nudges to colleagues from a manager's tool is a different product with
a different relationship to the team.

**Not a place your team logs their work.** Nobody is asked to write a status
update, and there is no field anywhere that exists to be filled in. The record
is a by-product of conversations people were having anyway. The confirmations in
§3 are the exception that proves it: they are one word, at the moment you were
already thinking about the thing. The moment Dossier requires more than that, it
stops being maintained.

**Not a replacement for talking to people.** The point of removing wasted
round-trips is to spend the conversations you do have on things worth
discussing.

---

## 8. How we will know it is working

Deliberately measured in outcomes, not usage:

1. **Resuming is free.** Someone picks up a topic they did not start and reaches
   useful work without asking anyone to re-explain it.
2. **Handoffs stop losing days.** The count of "blocked overnight on something
   that could have been written down" trends toward zero.
3. **Briefings get shorter.** The same colleague needs less context each time,
   because the profile learned what they already have.
4. **The asked/unasked gap closes.** Fewer things sit unasked while everyone
   believes they are waiting on someone — and fewer dates turn out to have been
   wished for rather than agreed.
5. **Meetings start assembled.** Prep time before recurring meetings drops to
   near zero, and fewer items get remembered on the walk back.
6. **It stays calm.** Opening the dashboard feels like orientation, not like a
   list of ways you are behind. If it starts to feel like the latter, that is a
   defect, and we will treat it as one.

These are worth measuring rather than asserting, and most of them can be:
how long it takes someone else to resume a topic, how many clarifications they
need first, how often work stalls overnight for want of something writable, how
long things sit unraised, how much of the state we surface turns out to be wrong
or stale. We would rather baseline a few of these on a real team before turning
Dossier on than declare victory from the fact that people are using it — usage
is not the outcome, and a tool people open every day while their work still
stalls has failed.

---

## 9. Where we are today

Honesty about state, so you can opine on the right things.

**Working now:** durable topics that survive across sessions, curated state with
full source retained, ownership, priorities, search, interface tagging with
filtered views, and the delegation-contract skill described in Layer 3a. It runs
locally on one person's machine.

**Being built next:** proper timezone handling; the requirements model in
Layer 2a, including acceptance and the committed-by date; three-level importance
with time pressure computed rather than hand-maintained; identity, so the tool
knows who is looking; escalation up, across and down, routed by relationship
rather than reporting line; colleague and interface notes; assembled meeting
prep.
Recipient acceptance on a handoff contract lands with that work.

**Further out:** the shared team memory in Layer 5 — deliberately last, so the
model is validated by real use before distribution is added on top of it.

---

## 10. What we would like your view on

The reason this document exists. Please disagree with any of it, but these six
in particular:

1. **Is "what's required, and from whom" the right shape for accountability?**
   Would you actually record these, or is it one more thing to maintain?

2. **Would you write — or let someone write — a calibration profile of you?**
   The upward half is meant to save your reports a round-trip. Does it read as
   useful, or as uncomfortable? This is the idea we are least certain about.

3. **Where should the audience boundaries fall?** We are convinced "mine versus
   the team's" is too coarse (Layer 5), and unconvinced by every alternative so
   far. In your week, how many genuinely different audiences does one topic have
   — and is per-topic visibility enough, or does it have to reach individual
   pieces of evidence inside a topic?

4. **Should a leader see a report's exceptions at all?** Routing now follows the
   relationship rather than the org chart, but the upward roll-up survived, and
   it is the piece we are least sure of. Is "your report's overdue items appear
   on your list" a leader doing their job, or surveillance that changes how the
   team writes things down?

5. **Does prep suppress the right things?** Hiding requests that are asked and
   still within their window keeps agendas short. Is that a relief, or would you
   rather see everything and decide yourself?

6. **Is tracking what someone actually committed to honest, or bureaucratic?**
   Recording that Priya said Thursday, separately from your wanting Tuesday, is
   meant to stop you from believing in a date nobody agreed to. Does that read
   as the tool being straight with you, or as one more thing to keep current?
