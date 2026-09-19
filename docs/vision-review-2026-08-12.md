# VISION.md review — 2026-08-12

> Scope: is `VISION.md` complete, internally consistent, consistent with shipped
> reality, and consistent with `PLANv02.md` — and what is still outstanding to
> decide. Assessment only; no edits were made to `VISION.md`.

**Verdict.** As a piece of product argument it is complete and unusually
coherent — the five layers, the four trust commitments, and the six questions in
§10 all line up, and the internal cross-references all resolve. Two things are
wrong, and both are in the parts of the doc that promise honesty:

1. **§9 "Where we are today" is factually wrong about the biggest thing that
   shipped.** Team Sync — the shared-store transport — is built, merged to
   `main`, and dogfooded locally. The doc says the opposite.
2. **§4 promises a provenance guarantee that does not exist anywhere** — not in
   code, not in the Distillation Guide, not in `PLANv02.md`.

Everything else is drift between `VISION.md` and the older `PLANv02.md`, which
the vision has silently overtaken.

---

## A. VISION vs. shipped reality

### A1. Team Sync is shipped; the vision says it does not exist — **highest severity**

`VISION.md` contains **zero** occurrences of "sync", "git", "GitHub", or "team
sync" (grepped). Meanwhile:

- `internal/sync/` is on `main` (19 files: `gitsync.go`, `merge.go`,
  `credentials.go`, adapter, state, status, plus tests).
- `HANDOFF.md:122` — "Team Sync is FEATURE-COMPLETE for local/offline use…
  `go test -race ./...` green."
- `SPEC.md:1015` — "implemented and integrated; all criteria are covered by
  automated tests… EXCEPT the… PAT onboarding, which remains pending the Phase 4
  real-GitHub pilot."

Against that, [VISION.md:559](VISION.md:559) says *"It runs locally on one
person's machine"* and [VISION.md:568](VISION.md:568) files the whole of Layer 5
under **Further out** — *"deliberately last, so the model is validated by real
use before distribution is added on top of it."*

**Be precise about the correction.** What shipped is *transport*: one shared
store, replicated through a private GitHub repo, with conflict-honest merges.
What Layer 5 *promises* — "a requirement you record as needed from Ryan appears
on Ryan's own list" ([VISION.md:446](VISION.md:446)) — needs the requirements
model and identity, neither of which is built. So the honest statement is:
**the hard half (distribution) is built and merged; the experience it enables is
not.** The doc currently says neither exists.

This matters beyond tidiness: §9 exists so reviewers "can opine on the right
things," and the sequencing claim ("deliberately last… validated by real use
before distribution") is being used as an argument for the roadmap while the
opposite already happened.

### A2. §5's privacy commitment is not enforced by the code that shipped — **the finding with teeth**

[VISION.md:468](VISION.md:468): *"until it is answered convincingly,
compensation and performance topics stay out of shared memory entirely."*

Team Sync as built has **no privacy model**: everything in the store syncs
except machine-local files (`config.yaml`, credentials, root `sessions/`,
`context/` — `SPEC.md:1019`). There is no per-dossier visibility, no exclusion
path, no `visibility:` field (and `PLANv02.md:1041` rejects that field on sight).

So the commitment currently rests entirely on the user's discipline not to
promote a sensitive topic into a synced store. That is a real gap between a
stated safety property and an enforced one, and it is the reason the audience
question (§B4 below) is now urgent rather than future work.

### A3. Three-way provenance is claimed but does not exist anywhere

[VISION.md:120-123](VISION.md:120) — *"It also says how Dossier came to believe
it: something it **observed** in the work, something **you told it**, or
something it **inferred**. The three are never blurred together."*

Verified absent:

- `core.Provenance` is `{Origin, URL, CapturedBy, Harness}`
  ([internal/core/artifact.go:49](internal/core/artifact.go:49)) — no belief-kind
  field.
- `grep -i "inferred|observed|asserted|told it"` over `assets/guide.md`,
  `assets/instructions.md`, and `SPEC.md` returns **nothing** — so it is not even
  a soft convention the Distillation Guide steers agents toward.
- `PLANv02.md` never mentions it.

And §9 lists it under neither "Working now" nor "Being built next," so the doc's
own escape hatch ("Where something does not exist yet, we say so") does not
cover it. This came in with the R2 fold (commit `5a99c0b`) and was written in the
present tense without a corresponding plan entry. Either move it to §9's
next-up list or add it to `PLANv02.md`; as written it is the one place the
trust section overclaims.

### A4. Everything else in §9 checks out

"Working now" — durable topics, curated state with source retained, ownership
(`lead`), priorities, search, interface tagging with filtered views, the
`dossier-delegate` skill: all verified present. "Being built next" correctly
describes the requirements model, three-level importance, identity, and
colleague/interface notes as *not* built (no `requirements` in `internal/core`;
`Importance` is still binary `high`/`low` at
[internal/core/dossier.go:45](internal/core/dossier.go:45); `Interface` is still
a hardcoded 7-value enum at
[internal/core/dossier.go:97](internal/core/dossier.go:97); no `people/` or
`interfaces/` roster).

---

## B. Internal consistency of VISION.md

Cross-references all resolve (§3→§4, §4→§3, §4b→§4, §7→§3, §10→Layer 5); the
counts are right (four commitments, five layers, five requirement states, six
questions). Two substantive tensions:

### B1. "Only two things" ignores the setup data the product needs — **moderate**

[VISION.md:86-94](VISION.md:86): *"Dossier asks two things of you, and only
two"* — judgement, and a word at the boundary. Reinforced by
[VISION.md:511](VISION.md:511): *"there is no field anywhere that exists to be
filled in."*

But the layers require a third input class that is neither: **authored reference
data about people and forums.** Colleague profiles ([VISION.md:380](VISION.md:380)
— "co-authored with the person they describe, and ideally written by them"),
interface notes ([VISION.md:400](VISION.md:400) — objective, scope, what does
*not* belong), and the timezone/working-hours data Layer 4b runs on
([VISION.md:425](VISION.md:425)) are all things a human writes, once, up front.
They are also the highest-leverage content in the product.

This is defensible — it is setup, not per-topic logging, and §3b's "improves as a
by-product of use" carries much of it after the first pass. But the doc currently
does not say so, and the absolutism ("only two," "no field anywhere") is the kind
of claim a reviewer will bounce off the moment they are asked to write seven
colleague profiles. Worth one sentence naming setup as a distinct, one-time ask.

### B2. "Dossier sees your conversations with your agent, and nothing else"

[VISION.md:89](VISION.md:89). Same root as B1 — it also sees a roster you
maintain. Minor, but it is the sentence the boundary argument rests on.

---

## C. VISION vs. PLANv02 — the plan has not followed the vision

`PLANv02.md` is dated 2026-08-05 and marked "proposal, not yet settled";
`VISION.md` was substantially rewritten later the same day (`5a99c0b`) and moved
on four points without the plan being updated. These are **plan defects, not
vision defects** — but they mean nobody can implement from `PLANv02.md` as it
stands.

| # | VISION says | PLANv02 says | Action |
|---|---|---|---|
| C1 | Five requirement states incl. **accepted**, plus a **committed-by** date held separately from needed-by ([VISION.md:229-252](VISION.md:229)) | Four states `needed → requested → answered\|dropped`, one date ([PLANv02.md:282](PLANv02.md:282)) | Schema change to Phase 2 **before** it is built |
| C2 | Escalation routes by **relationship**: the interface where it gets raised and where it goes next ([VISION.md:270-277](VISION.md:270)) | Escalation is `f(me, reports_to, time)` — pure org chart ([PLANv02.md:516](PLANv02.md:516)) | New per-topic escalation-route field; not in the Person/Interface schemas |
| C3 | Profile entries **expire** — shown as old, reconfirmed or retired ([VISION.md:376](VISION.md:376)); agent **proposes, never silently learns** | §4.2 has the learning loop but no staleness or dating mechanism | Add to Phase 4 |
| C4 | (n/a — vision is silent) | Phase 6 shared store "shells out to the `git` binary — **it does not vendor a Go git library**" ([PLANv02.md:794](PLANv02.md:794)) | **Already contradicted by shipped code**: embedded go-git, per ADR 0005. PLANv02 §10 describes a design that was overtaken |
| C5 | Handoff is "only a contract once they say so" — accepted / clarify / propose-change ([VISION.md:336](VISION.md:336)) | Not in the plan or in the `dossier-delegate` design in `HANDOFF.md` | Add to Phase 4 |

C4 is the one to act on first: `PLANv02.md` §10 (and the §8 sequencing table's
Phase 6 row) now specify a *different implementation of a feature that already
exists*. Reading the two documents together, someone would rebuild Team Sync.

### C6. ADR debt is real and one number is duplicated

`PLANv02.md:934-958` requires ADRs **0006–0011** before implementation. `docs/adr/`
contains 0001–0005 and **none of 0006–0011 exist**. ADR **0011 is a declared hard
gate** — "Phase 3 does not start until this ADR is merged"
([PLANv02.md:1024](PLANv02.md:1024)).

Separately: **two files both numbered 0005** —
`0005-pi-session-identity-bridge.md` and `0005-team-sync-via-github.md`. Small,
but ADR numbers are referenced by number across `HANDOFF.md` and `CLAUDE.md`
(`CLAUDE.md` says "B12 / ADR 0005" meaning the team-sync one; `HANDOFF.md:44`
says "ADR 0005" meaning the Pi one). Renumber one.

---

## D. Outstanding decisions, ranked

1. **Audience boundaries in a shared store** (VISION Q3 / `PLANv02.md` §13.1,
   ADR 0011). Was "future"; is now **live**, because sync shipped with no privacy
   model while §5 states a commitment the code does not enforce. The three options
   are already framed — (a) `visibility:` field (rejected), (b) two stores
   (leaning), (c) private topics stay out of Dossier. Four sign-off items are
   listed at `PLANv02.md:1058`. This also gates Phase 3.
2. **The upward roll-up** (VISION Q4). The doc names it the piece it is least
   sure of, and routing-by-relationship removed its original justification without
   removing the feature. Decide whether a leader sees a report's exceptions at all.
3. **`accepted` + `committed_by`** (C1). Cheap now, a schema migration later —
   settle before Phase 2 writes the requirements model.
4. **Where requirements live** — frontmatter array vs. `requirements/<id>.md`
   (`PLANv02.md:1117`, open by decision, leaning frontmatter). Note the shipped
   go-git merge path changes the calculus: `PLANv02.md`'s "free item-wise merge"
   argument assumed a git merge driver that was never built.
5. **Escalation-route schema** (C2) — what the "where it goes next" field
   actually is on a dossier.
6. **Real-GitHub PAT dogfood + two-colleague pilot** — the two owner-gated items
   still open from Team Sync Phase 4 (`HANDOFF.md:126-131`). These are also what
   would let §9 say something true about Layer 5.

---

## E. Smallest set of edits that would make VISION.md correct

Not applied — listed so the fix is cheap to action:

1. Rewrite §9's third paragraph: distribution transport is **built and merged**,
   validated locally, awaiting a real-GitHub pilot; the Layer 5 *experience*
   (requirements landing on someone else's list) still needs the requirements
   model and identity. Drop or requalify "It runs locally on one person's machine."
2. Requalify §5's privacy sentence: it is a **discipline** today, not an enforced
   boundary — the store syncs whole.
3. Either move the observed/told/inferred provenance claim in §4 into §9's
   next-up list, or add it to `PLANv02.md`.
4. One sentence in §3 acknowledging the one-time setup ask (roster, profiles,
   interface notes) as distinct from ongoing maintenance.
