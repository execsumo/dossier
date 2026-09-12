# Architecture Review and Sequenced Plan

> Reviewed: 2026-09-11
> Scope: whole-repo architecture — docs vs. code consistency, robustness, and
> structural simplicity. TUI internals are explicitly **out of scope here**;
> they have their own completed review and remediation plan in `docs/tui.md`.
>
> **Status: completed 2026-09-11.** F1–F8 were resolved and F9 remains on its
> named triggers. Two proposed implementation details were deliberately
> changed after validation:
> - F1 did not gain a `Confirmer` port. Promote now always returns its existing
>   ambiguity result and next actions; user interaction belongs entirely to
>   the driving adapter.
> - F5's frontmatter shortlist was rejected because body-only matches can reach
>   medium confidence, and `FSStore.List` already reads bodies. An optional
>   streaming `DossierScanner` removes the N+1 second reads while preserving
>   exact `ScoreDossier` semantics.

## Review summary

The architecture is fundamentally sound. The one-core/many-adapters rule is
real in the code, the port seams are in the right places, compatibility
handling is honest (lazy, lossless, strict on unknown keys), conflict handling
is genuinely one mechanism with two triggers, and the Team Sync process model
(D7) is clean. Tests are broad, and the TUI recently went through a full
remediation (`docs/tui.md`).

The findings are concentrated in four themes:

1. **The core-purity rule is enforced weaker than documented** — and there is
   one genuine violation inside `core.Service`.
2. **`ARCHITECTURE.md` has drifted** from the code it claims to describe.
3. **A handful of robustness gaps** in the `Promote` path and around the
   Service's implicit concurrency contract.
4. **Size concentration** in four files that will tax the next contributor.

## Findings

**F1 (P0) — `Service.Promote` runs a GUI dialog from inside core.**
`internal/core/service.go:827-851`: when promote finds ambiguous candidates
and `darwin` is the OS, core executes `osascript` (a Finder dialog) and keys
off the user's button press. Problems, in order of severity:
- It violates the #1 structural rule: core imports `os/exec` and performs
  I/O, platform behavior, and user interaction inline in a use-case.
- It is a cross-surface behavior fork: on macOS an MCP agent session can
  **block on a human clicking a dialog box** (no timeout on `cmd.Output()`),
  while on Linux the dialog silently never runs.
- It embeds a test-detection hack in domain logic
  (`!strings.HasSuffix(os.Args[0], ".test")`).

The fix shape already exists in the codebase's own idiom: `Result` carries
`Data: candidates` + `NextActions` for exactly this situation. The dialog is a
bolt-on that preemptively answers "confirm or reject" on behalf of the
surface.

**F2 (P0) — the purity guard does not enforce "no I/O".**
`internal/core/dependency_test.go` only rejects imports containing a dot
(siblings and third-party). The entire standard library passes — including
`os`, `os/exec`, and `net`. The documented rule ("no I/O; filesystem logic
goes behind a port") is therefore aspirational. After F1, core production code
may not need `os` at all, at which point the full rule can be machine-checked.

**F3 (P1) — duplicate ADR 0005.** `docs/adr/0005-team-sync-via-github.md`
(2026-07-15) and `docs/adr/0005-pi-session-identity-bridge.md` (2026-08-04)
share a number; HANDOFF, B2, ARCHITECTURE, and `docs/harness-capabilities.md`
all cite "ADR 0005" with two different meanings. The Pi ADR, being later,
should become 0009.

**F4 (P1) — `ARCHITECTURE.md` drift.** The doc is "updated 2026-08-05" but
several structures postdate it:
- §2 documents an `internal/hooks/` package; it does not exist. Hook handling
  is a cobra subcommand inside `internal/cli/cli.go` (`hook <...>`).
- §4's `Store` listing is far narrower than the real port (`ports.go`): missing
  `ReadRevision`, `List` returning `[]ListedFrontmatter` (not `[]Frontmatter`),
  `ReadArtifact`/`ListArtifacts`, audit-shard methods, `ValidateArtifactFiles`,
  session-binding/conflict/context-asset methods, and the optional `Renamer`
  and `PostInstallAdvisor` ports.
- §3's facade listing duplicates the `Save` line and omits `Rename`,
  `RenameSlug`, `ContextRefresh`, and the guide-delivery methods
  (`GetGuide`/`GuideForSession`/`EnsureContextAssets`).

**F5 (P1) — Promote's ambiguity scan is N+1 full reads.**
`service.go` `Promote` does `store.List("all")` then `store.Read(fm.ID)` for
every dossier to score candidates. Each `Read` parses the full body; every
non-forced promote re-parses the whole store. This undoes, for this path, the
frontmatter-only fast path that the 2026-06-17 hardening added to scans.
`ScoreDossier` does read the body (SPEC §11.2), so bodies are needed — but
only for dossiers that survive a cheap frontmatter prefilter.

**F6 (P2) — Promote silently skips ambiguity checking on list error.**
The whole candidate block sits inside `if err == nil` from `store.List`. A
transient store error degrades to "no ambiguity check ran, no warning said
so" — against the spirit of both "no silent link" and "degrade visibly".

**F7 (P2) — the Service concurrency contract is implicit.** Core has no
mutex. The long-lived `mcp serve` process runs the debounced sync goroutine
concurrently with tool handlers, so `Service` is accessed from multiple
goroutines by design. Today the safety argument rests on "handlers only read
`cfg`" — but `AddLead`/`AddInterface` mutate `s.cfg` slices in place, and
nothing structural prevents a future MCP tool from calling them. One honest
fix (an `RWMutex` around the vocabulary state, or a documented and
asserted single-writer rule) should exist before the pattern is trusted.

**F8 (P2) — size concentration.** `internal/core/service.go` (2745 lines,
~40 methods across ~8 concerns), `internal/tui/tui.go` (3157), `internal/cli/cli.go`
(2054), `internal/store/fsstore.go` (1487). None is wrong, but the Service
file now mixes lifecycle, dossier CRUD, promote/transcript capture, session
binding, guide delivery, harness reporting, and team sync in one file.

**F9 (P3) — watch items (no action now, named so they are not forgotten).**
- **D8 (silent auto-sync failures)** is logged as revisitable; the trigger is
  dogfood evidence that users miss failures. Keep it that way until then.
- **Dual Lip Gloss majors** (`lipgloss` v1 + `lipgloss/v2` in `go.mod`) are a
  documented, deliberate pin pending a Bubbles upgrade; consolidate then.
- **Store port width** (25+ methods): if a second store implementation ever
  exists, consider splitting by capability; one real implementation plus a
  fake does not justify it today.

---

## Implementation record

1. **F1/F2:** Removed the macOS dialog and test-process detection from core.
   Promote now always returns candidates and next actions on ambiguity. The
   purity guard rejects production imports of `os`, `os/*`, `net`, and
   `net/*`, in addition to sibling and third-party packages.
2. **F3/F4:** Renumbered the Pi identity decision to ADR 0009 and reconciled
   all references. `ARCHITECTURE.md` §2–§4 now reflects the actual package,
   Service, Store, and optional-port surfaces. HANDOFF requires a same-PR docs
   audit whenever ports or subcommands move.
3. **F5/F6:** Added optional `DossierScanner`. FSStore streams each full
   Dossier once, while older Store implementations retain a `List`+`Read`
   fallback. Exact body-aware scoring is preserved, only the top three
   candidates are retained, and any unavailable or partial ambiguity scan is
   surfaced as a warning. Promote receives the created immutable ID directly
   from the internal Save path rather than rescanning the store.
4. **F7:** Added an `RWMutex` around the only mutable Service-owned state,
   configured leads/interfaces, and documented the remaining concurrency
   contract. A race regression runs vocabulary mutation and Save while the MCP
   debouncer is inside Sync.
5. **F8:** Split `service.go` by dossier, promote, session, harness, and team
   concerns without changing the public Service API. CLI remains deferred
   until its next functional change; TUI remains governed by `docs/tui.md`.
6. **F9:** No code change. Revisit D8 on dogfood evidence, Lip Gloss on the
   next Bubbles upgrade, and Store capability splitting when a second real
   store arrives.

Validation completed with `go build ./cmd/dossier`, `go vet ./...`,
`test -z "$(gofmt -l .)"`, `git diff --check`, and `go test -race ./...`.
The Promote scale test covers 500 filesystem Dossiers under the two-second
budget.
