# Dossier — Architecture

> Updated: 2026-08-05 · Language: Go (see `BUILD-DECISIONS.md` B1)
> Status: **implemented**. This documents the current structure and load-bearing decisions.

This document describes **how** to build what `SPEC.md` specifies. The SPEC defines the seams (data model, tool/CLI contracts, file layout, acceptance criteria); this defines the internal shape behind those seams.

---

## 1. Guiding principle: one core, many adapters

Dossier is a single binary that wears three faces — CLI, MCP-over-stdio server, and harness hook handler — plus a TUI. The cardinal rule:

> **All four entry points are thin adapters over one pure domain core. None of them contains business logic.**

This is ports-and-adapters (hexagonal). It buys us the property the SPEC implicitly requires but never states: **CLI, MCP, and TUI must behave identically** (acceptance criteria are written once but must hold across surfaces). They behave identically because they call the same `core.Service` and render the same `Result` values.

```
                 driving adapters (call into core)
        ┌──────────┬──────────┬──────────┬──────────┐
        │   CLI    │   MCP    │  Hooks   │   TUI    │
        │ (cobra)  │ (stdio)  │ handler  │(bubbletea)│
        └────┬─────┴────┬─────┴────┬─────┴────┬─────┘
             └──────────┴──────────┴──────────┘
                            │ calls
                   ┌────────▼─────────┐
                   │   core.Service   │   use-cases: Promote, Save,
                   │  (orchestration) │   Link, Merge, Recall, Switch,
                   └────────┬─────────┘   List, Archive, ...
                            │ depends on PORTS (interfaces)
        ┌──────────┬────────┼─────────┬──────────┬──────────┐
        │  Store   │ Searcher │ Tokenizer │ Harness │  Clock  │
        └────┬─────┴────┬───┴─────┬────┴────┬─────┴────┬────┘
             │          │         │         │          │
        driven adapters (implement ports)
        fsstore     rg/native   bpe       claudecode    wall
```

Core depends on **nothing** outside the standard library and its own port interfaces. Everything else depends on core. This makes the domain testable in isolation and lets us swap search backends, tokenizers, and harnesses without touching logic.

---

## 2. Package layout (Go)

```
dossier/
  go.mod
  cmd/dossier/
    main.go              # entrypoint: routes to `cli` (default) or `mcp serve`
  internal/
    core/                # PURE DOMAIN — no I/O, no third-party deps
      dossier.go         # Dossier, Frontmatter, meeting-interface enums, section model + invariants
      artifact.go        # Artifact, types, size/format validation
      audit.go           # AuditEvent types (§4.4) + JSONL marshaling
      revision.go        # canonical hashing + optimistic-concurrency check (see §6)
      priority.go        # priority scoring (SPEC §11.1)
      suggest.go         # lexical suggestion ranking (SPEC §11.2)
      result.go          # Result/Warning/NextAction value types (the §8.2 envelope, surface-agnostic)
      errors.go          # typed domain errors ↔ the §8.2 error codes
      ports.go           # Store, Searcher, Tokenizer, HarnessRegistry, Clock, Syncer interfaces
      service.go         # Service: orchestrates use-cases over the ports (incl. Sync/SyncStatus)
    store/               # driven adapter: filesystem (implements core.Store)
      fsstore.go         # layout, read/write, atomic write protocol (§5)
      auditlog.go        # append-only JSONL with O_APPEND + lock
      lock.go            # per-dossier advisory file lock
      ids.go             # ULID generation; slug generation + collision suffix
    search/              # driven adapter (implements core.Searcher)
      native.go          # pure-Go recursive scan (default)
      ripgrep.go         # rg fast-path when detected
    tokenizer/           # driven adapter (implements core.Tokenizer)
      bpe.go             # embedded vocab; estimate()
    harness/             # driven adapters (implement core.HarnessRegistry / Harness)
      harness.go         # Registry + shared hook-merge helpers
      claudecode.go      # Claude Code: hooks + MCP registration + skill install (B2)
      pi.go              # Pi: detection + installs the bundled Pi extension (B2, ADR 0005)
      pisession.go       # Pi session pointer: location, record, process-ancestry walk
      session.go         # session-id resolution ladder shared by CLI/MCP (ADR 0003/0005)
    sync/                # driven adapter: Team Sync go-git engine (implements core.Syncer)
      gitsync.go         # GitSync + Config/report types; sync.go: pull→resolve→commit→push
      merge.go/tree.go   # remote-wins 3-way merge (no git markers ever); DiffTree + MergeBase
      credentials.go     # PAT resolution (~/.dossier/credentials 0600, `gh auth token` fallback)
      gitignore.go       # machine-local exclusion set (config.yaml, root sessions/, context/)
      adapter.go         # maps GitSync's internal types → core.Sync* DTOs (keeps core pure)
    config/              # config.yaml load/save/defaults (incl. team.remote / team.branch)
    hooks/               # hook PAYLOAD builders + session-start/end handlers (call core)
    cli/                 # cobra commands → core.Service → render (text/--json)
    mcp/                 # stdio MCP server → core.Service → §8.2 envelope
    tui/                 # bubbletea models/views (fsnotify hot-refresh, glamour markdown) → core.Service
  assets/                # go:embed — Distillation Guide, context templates, installables
    guide.md
    library.tmpl.md
    dossier-delegate-skill.md  # installed into Claude Code's skills dir
    pi-extension.ts            # installed into Pi's extensions dir (session identity)
  docs/
    harness-capabilities.md   # PRODUCED by dev agent (Milestone 1)
```

Notes:
- `internal/` so nothing is importable as a library — this is an application, not a SDK.
- The dependency rule is enforced by direction: `core` imports none of its sibling packages. This is guarded in CI (`.github/workflows/ci.yml`, "Dependency direction" step) via a `go list` assertion over `./internal/core`'s imports, alongside `gofmt`/`go vet`/`go test`.

---

## 3. The Service facade

`core.Service` is the only thing adapters talk to. One method per use-case, each taking a typed request and returning `(core.Result, error)` where `error` is always one of the typed domain errors in `errors.go`.

```go
type Service struct {
    store  Store
    search Searcher
    tok    Tokenizer
    hreg   HarnessRegistry
    clock  Clock
    cfg    Config
}

func (s *Service) Promote(ctx, PromoteReq) (Result, error)
func (s *Service) Save(ctx, SaveReq) (Result, error)        // optimistic concurrency, §6
func (s *Service) Link(ctx, LinkReq) (Result, error)        // candidates if id omitted
func (s *Service) Merge(ctx, MergeReq) (Result, error)      // conflict detection
func (s *Service) Recall(ctx, RecallReq) (Result, error)    // returns revision + token estimate
func (s *Service) List(ctx, ListReq) (Result, error) // status + interface filters
func (s *Service) Search(ctx, SearchReq) (Result, error)
func (s *Service) Switch(ctx, SwitchReq) (Result, error)
func (s *Service) Active(ctx, ActiveReq) (Result, error)
func (s *Service) Save(ctx, SaveReq) (Result, error)        // single write path: distilled state + frontmatter (name/status/lead/interfaces/next_action/priority/...) + artifacts, with optimistic-concurrency conflict handling. All metadata edits (status, lead, interfaces, etc.) route through here so CLI/MCP/TUI stay identical.
func (s *Service) Archive(ctx, ArchiveReq) (Result, error)
func (s *Service) Path(ctx, PathReq) (Result, error)
func (s *Service) Doctor(ctx) (Result, error)
func (s *Service) Init(ctx, InitReq) (Result, error)
func (s *Service) HarnessStatus(ctx) (Result, error)                  // per-harness detection, read-only
func (s *Service) InstallHarness(ctx, InstallHarnessReq) (Result, error) // one harness added after init
func (s *Service) TeamCreate(ctx, TeamCreateReq) (Result, error)
func (s *Service) TeamJoin(ctx, TeamJoinReq) (Result, error)
```

`Result` carries `data any`, `warnings []Warning`, `next_actions []NextAction` — the exact §8.2 envelope, but surface-agnostic. The MCP adapter serializes it as JSON; the CLI adapter prints text or `--json`; the TUI renders it. **Warnings (e.g. over-token-target, transcript-unavailable) are produced once in core** and flow to every surface — never re-implemented per adapter.

---

## 4. Ports (the seams)

```go
type Store interface {
    // CRUD over dossiers, artifacts, audit, sessions, conflicts, config.
    // Returns current revision on reads; enforces atomic writes (§5).
    Read(slugOrID string) (*Dossier, Revision, error)
    List(statusFilter string) ([]Frontmatter, error)   // frontmatter scan only; service applies interface filters
    Write(d *Dossier, base Revision) (Revision, error) // optimistic; see §6
    WriteArtifact(dossierID string, a *Artifact) error
    AppendAudit(dossierID string, e AuditEvent) error
    // ... session bindings, conflicts, init/layout
}

type Searcher interface {
    Search(query string, scope SearchScope) ([]Hit, error)
}

type Tokenizer interface {
    Estimate(text string) int
}

type Harness interface {
    Name() string
    Detect() (Capabilities, error)   // reads harness config, returns capability booleans
    Install(InstallOpts) error        // idempotent, non-clobbering, backs up (B7/B8)
}
type HarnessRegistry interface{ All() []Harness }

type Clock interface{ Now() time.Time }

// Team Sync (Phase 2). core owns the Sync* DTOs; internal/sync maps to them so
// core never imports the go-git adapter. Sync is local-first: the local commit
// always lands; a nil Syncer means team sync is not configured (Service.Sync
// returns a clear error, never panics).
type Syncer interface {
    Sync(ctx context.Context) (SyncReport, error)     // pull→resolve(remote-wins)→commit→push
    Status(ctx context.Context) (SyncStatus, error)   // ahead/behind/last-sync, no mutation
    Create(ctx context.Context) error                 // initialize team store and push
    Clone(ctx context.Context, url, dir string, depth int) error // join team store
}
```

Why each is a port:
- **Store** — the whole "no database, files are truth" decision lives behind one interface; tests use a temp dir, and an in-memory fake makes core tests fast.
- **Searcher** — lets native/ripgrep swap per B5 without core knowing.
- **Tokenizer** — B4; swappable, mockable (tests assert behavior, not exact counts).
- **Harness** — isolates the **riskiest, most fragile code** (mutating other tools' config files) behind one interface, and makes the capability matrix a set of table tests against fixture config dirs.
- **Syncer** — isolates the go-git networking/merge engine (B12 Team Sync) behind one interface. `Service.Sync` orchestrates: it calls the port, then routes any both-modified `dossier.md` into the **existing** `conflicts/*.md` machinery via `store.WriteConflict` (`kind: sync_concurrent_edit`) — one conflict mechanism, two triggers (local edit + cross-machine sync). Remote wins the working tree; the local version is preserved; never last-write-wins, never git merge markers.

**Auto-sync (Phase 3b) — lifecycle, not a daemon.** Sync becomes automatic at three trigger points, all best-effort and non-blocking: (1) `Service.SessionStart`/`SessionEnd` (short-lived hook processes) do a bounded pull/push, gated on a configured syncer; (2) the long-lived `mcp serve` process runs a **debounced background sync goroutine** (`internal/mcp/server.go`) triggered after `dossier_save`/`dossier_recall`, coalescing rapid edits and **drained (bounded) on stdin EOF** so the last change is never stranded; (3) short-lived CLI commands do NOT debounce — they rely on the session-boundary hooks and explicit `dossier sync`. There is no daemon and nothing survives the process. Concurrent pushes from these independent paths are safe because the Phase 2 store-wide `.sync.lock` serializes every `Sync()`. `dossier doctor` surfaces sync health (configured / ahead / behind / last-sync / unresolved conflicts) via `Syncer.Status`. **The fast-forward pull path stashes machine-local files (`config.yaml`, root `sessions/`, `context/`, sync state) across go-git's Force checkout**, which would otherwise delete these gitignored files and silently un-team a joined colleague.

### Structured meeting interfaces

`Frontmatter.Interfaces` is an optional, ordered multi-select field. The canonical
values are `Pricing WBR`, `1:1`, `OLG Standup`, `Growth Standup`, `Steerco`,
`Solutioning`, and `OpsRev`. `Service.List` applies interface filters with **ANY**
semantics: a dossier is returned when it contains at least one requested value.
This shared service behavior is exposed by the CLI (`ls --interface`), MCP
(`dossier_list.interfaces`), and the TUI (`i`), while `Lead` remains the person
filter used to answer queries such as “Marcus in my 1:1.”

---

## 5. File operations & atomic writes

Implements SPEC §12. The non-negotiables:

**`dossier.md` write protocol** (in `fsstore.go`):
1. Acquire the per-dossier advisory lock (`<dossier>/.lock`, `flock`).
2. Read current content → compute current `Revision`.
3. Optimistic-concurrency check against the caller's `base_revision` (§6).
4. Write to a temp file **in the same directory** (so rename is atomic on the same filesystem).
5. `fsync` the temp file.
6. `rename` temp over `dossier.md` (atomic replace).
7. Append the audit event.
8. Release lock.

**Artifacts**: generate id first → write file atomically (temp+rename) → append audit. Reject any single artifact > 1 GB (`artifact_too_large`). Validate format ∈ {markdown, json, txt}; binary → `binary_artifact_unsupported`, store metadata/path/provenance only.

**Audit Log**: Sharded by sanitized author slug (`audit/<author>.log`). `O_APPEND` single-line JSONL writes (atomic for lines < `PIPE_BUF` on POSIX), under a short-held dir lock. Read = parse all shards line-by-line plus legacy `audit.log`, sort globally by `ts`.

**Session Stash**: Transcripts are written to `sessions/<author>/<session-id>.md` upon session end to provide an uncompressed author-specific history independent of distillation.

**IDs / slugs** (`ids.go`): ULID with prefixes `dos_ art_ sess_ rev_ conf_`. Slug per SPEC §12.2; on collision append `-` + last 6 chars of the ULID (Crockford base32).

---

## 6. Concurrency & revisions (resolves the spec ambiguity)

> See `BUILD-DECISIONS.md` items 1–3, 6. **`base_revision` is NOT stored in frontmatter** — it's a session-side token returned by reads and passed back on writes.

**Revision** = `rev_` + SHA-256 (hex, truncated to 32 chars) over the canonical form:

```
canonical(frontmatter)         # keys sorted; scalars normalized; lists in declared order
+ "\n---\n"
+ normalize_newlines(body)     # CRLF→LF, trailing-whitespace trimmed per line, single trailing \n
+ "\n---artifacts---\n"
+ join(sorted("<art_id>:<sha256(content)>"), "\n")
```

Canonicalization must be **deterministic** — same logical content always yields the same revision regardless of YAML key order or line endings. Put `canonicalize()` and `Revision()` in `revision.go` with exhaustive table tests; they are load-bearing.

**Optimistic concurrency on `Save`** (SPEC §11.5):
1. Read current revision.
2. If `current == base_revision` → write (atomic protocol §5).
3. If different:
   - If **only non-overlapping frontmatter** changed (e.g. one session set `next_action`, another set `status`) → auto-merge, audit, succeed.
   - Otherwise → write the rejected proposal to `conflicts/<conf_id>.md`, audit `conflict_created`, return `concurrent_edit`. **Never** last-write-wins for Distilled State body.

The read use-cases (`Recall`, `Switch`, `Active`) return the revision so the agent can round-trip it.

**Conflict artifact format** (`conflicts/<conf_id>.md`):
```yaml
---
id: conf_...
dossier_id: dos_...
kind: distilled_state_concurrent_edit   # or merge_conflict
base_revision: rev_...
attempted_revision: rev_...
session: sess_...
ts: 2026-06-14T16:10:00-07:00
---
## Rejected proposal
<the body the caller tried to write>

## Diff against current
<unified diff>
```
`doctor` reports any `conflicts/*.md` whose status isn't resolved.

---

## 7. Entry-point wiring

`cmd/dossier/main.go`:
- `dossier mcp serve` → builds the Service, runs the MCP stdio server (`internal/mcp`).
- `dossier hook <session-start|session-end|pre-compaction>` → `internal/hooks` handler (reused by harness hook configs; same binary, same Service).
- `dossier install [--dir <dir>]` → copies the binary to a stable PATH location (default `~/.local/bin/dossier`), ensuring idempotence and executable permissions.
- everything else → cobra CLI (`internal/cli`); `--tui` or the bare `dossier` with no subcommand can launch the TUI.

All three construct the Service identically via a small `wire()` that picks adapters (native vs ripgrep search, real vs fake store) — keep composition in one place.

**Session resolution (adapter-side, see ADR 0003 and ADR 0005).** `core` stays pure: every session-scoped method takes an explicit `SessionID`. *Which* session an adapter is acting for is discovered at the edge by `harness.ResolveSessionID(explicit, allowDefault)` — or `harness.ResolveSession(...)`, which also names the harness the id came from — with precedence `explicit param/flag → CLAUDE_CODE_SESSION_ID (set by Claude Code in each session's env) → PI_SESSION_ID (Pi sets this for bash-tool children only) → Pi session pointer (published by the bundled Pi extension for the owning Pi process, found by walking this process's ancestry) → DOSSIER_SESSION → sess_default`. Adapters pass the resolved harness name into `SwitchReq.HarnessName` so a binding records the harness the session actually ran under rather than the first one configured on the machine. The MCP adapter calls it with `allowDefault=false` and **degrades visibly** (`harness_capability_unavailable`) rather than silently sharing the `sess_default` bucket across concurrent sessions; the CLI calls it with `allowDefault=true` for manual use. This is the bridge that lets an in-session agent call `dossier_session` with only a slug. The **TUI carries no session identity at all** (it does not expose `Switch`/`Active`/`Session` binding) — it is a read/edit viewer over the store; see [ADR 0004](docs/adr/0004-tui-no-session.md).

**MCP**: use the official Go MCP SDK over stdio. Each `dossier_*` tool (SPEC §8.1) is a ~10-line handler: parse input → call one Service method → marshal `Result` into the §8.2 envelope. Map typed errors → §8.2 codes in **one** place (`mcp/errors.go`).

**Hooks** (SPEC §9): `session-start` builds the library payload (frontmatter scan + capability warnings + guide pointer + active Dossier's distilled state if bound). `session-end`/`pre-compaction` force a `Save` of the active binding. Hook handlers call core; they don't reimplement save.

---

## 8. Harness adapters (the fragile edge)

The harness implements `Detect()` and `Install()`. `Install` is **idempotent, non-clobbering, and backs up** every file it touches (B7), and is **gated by confirmation** in `init` (B8). It registers both the lifecycle hooks and the MCP server (under name `"dossier"`) using the stable binary path passed via `InstallOpts.StableBinaryPath`. Claude Code keeps these in **distinct files**: hooks go to `~/.claude/settings.json` while the MCP server must go to `~/.claude.json` (the only location Claude Code reads user-scope MCP servers from). `Install` also injects the Dossier Resumption Protocol into Claude Code's `customInstructions` array in `settings.json`. `Install` also migrates stale entries an older build wrote to the wrong file (e.g. a `dossier` MCP entry left in `settings.json`). Capability detection produces the booleans in SPEC §5.1.

v1 supports **Claude Code and Pi** (B2). Claude Code provides the full capability set directly. **Pi does not**: its `PI_SESSION_ID`/`PI_SESSION_FILE` reach bash-tool children only, and it has no built-in MCP client, so Dossier ships a Pi extension of its own (ADR 0005). The `Harness` interface and `Registry` remain extensible, while Codex and Antigravity remain out of scope for v1. The product must still **degrade visibly** — a capability missing in a given session is a warning surfaced through `Result`, never a silent no-op.

`Capabilities` (`internal/core/session.go`) therefore separates *what a harness offers* from *what it is*: `Installed` (present on this device, so its integration is installable ahead of first use) and `SessionIdentity` (Dossier can resolve a per-session id for it) sit alongside the MCP/hook/transcript booleans of SPEC §5.1. `LiveSession()` and `Present()` are the two predicates the service uses; `Present()` is what makes `init` install into a harness that is installed but idle.

**`PiHarness`** (`internal/harness/pi.go`) detects Pi from its agent directory (`PI_CODING_AGENT_DIR`, default `~/.pi/agent`), a `pi` on PATH, the Pi process environment, or a live session pointer. `Install` writes the embedded `assets/pi-extension.ts` to `<agent dir>/extensions/dossier/index.ts` under the same idempotent/backed-up/confirmed contract (B7/B8) as the Claude Code installer. It claims **neither** MCP nor lifecycle hooks — Dossier does not provide those for Pi yet, and overclaiming them would be a silent no-op instead of a visible gap. `internal/harness/pisession.go` owns the pointer record, its location, and the depth-bounded process-ancestry walk (procfs, `ps` fallback) that finds the owning Pi process.

Per-harness reporting lives in `core`: `HarnessReport` + `Service.HarnessStatus` (read-only detection) and `Service.InstallHarness` (install one integration by name, the "user added Pi after `init`" path, exposed as `dossier harness list|install`). `harnessAdvisories` produces the actionable line that `init`, `harness list` and `doctor` all surface when a harness is installed but its bridge is not. Doctor treats such an advisory as a warning, not an issue — `Doctor.OK` now tracks `report.Issues`, so a missing integration never masquerades as store damage.

---

## 9. Assets & Programmatic Context Injection

The Distillation Guide, the Dossier Protocol skill, and the `library.md` template are **embedded** via `go:embed` (`assets/`). `init` writes both the guide and the `skill.md` file to `~/.dossier/context/`; `context refresh` regenerates `~/.dossier/context/library.md` from the template + a live frontmatter scan. This keeps the single-binary promise — no external asset files to ship.

`assets/dossier-delegate-skill.md` is also embedded, but is installed to a different destination: `ClaudeCodeHarness.Install` (`internal/harness/claudecode.go`) writes it to `~/.claude/skills/dossier-delegate/SKILL.md` — Claude Code's own skills directory, not `~/.dossier/context/` — because it must resolve as a real `/dossier-delegate` slash-command Skill rather than be pulled programmatically via MCP interception like the guide/instructions files. The write follows the same idempotent/backed-up/single-confirmation contract (B7/B8) as the rest of `Install`: byte-identical content is a no-op, differing content gets a timestamped `.bak` before being overwritten.

**Programmatic Injection (Zero-Tax Architecture):** Instead of injecting the 1500-token `guide.md` into global prompts (`skill.md`) or passive lifecycle hooks where it wastes tokens on generic coding tasks, Dossier uses **active interception**. When an LLM invokes the `dossier_session` MCP tool to bind a topic, the MCP server dynamically wraps the `Service.Switch` or `Service.Active` state response in a payload that explicitly includes the full string contents of the Distillation Guide. This guarantees the LLM receives strict schema instructions *exactly* when it enters a dossier context, while maintaining zero overhead during non-dossier operations. Future iterations will apply this deterministic pattern to other operational instructions currently housed in `skill.md` to further compress global bloat.

The `session-start` hook is the one injection point that fires unconditionally on *every* session, whether or not it has anything to do with Dossier — so it deliberately does not follow the "inline the heavy payload" pattern above for a session with no active binding. `Service.SessionStart` (`internal/core/service.go`) reduces that case to a single-line nudge (open-dossier names + a pointer to the three MCP tools that would act on them); the Distillation Guide and a bound Dossier's full Distilled State are still inlined in full when a session *does* have an active binding, since that binding is the explicit signal that this session is about Dossier work.

---

## 10. Testing strategy (how the acceptance criteria get met)

- **core**: pure → table-driven unit tests. `revision.go`, `priority.go`, `suggest.go`, concurrency branches, frontmatter validation. Use a fake `Store`/`Clock`/`Tokenizer`.
- **store**: integration tests against a temp `DOSSIER_HOME`. Assert atomic-write durability, the 500-Dossier frontmatter scan < 2s (SPEC §14.1), append-only audit, slug collisions.
- **Distillation Guide**: golden-file fixtures — sample transcript in, assert the distilled output's *structure and provenance presence* (not verbatim prose). This is how guide quality stays regression-safe.
- **MCP**: drive the server over in-memory pipes; assert the §8.2 envelope and error-code mapping for each tool.
- **harness**: fixture Claude Code and Pi config dirs; assert `Detect()` capabilities (including that Pi does **not** claim MCP or lifecycle hooks) and that `Install()` is idempotent, backs up, and writes nothing without confirmation. Pointer resolution is tested with an injected ancestry function: ancestor pointers resolve, a concurrent session's pointer does not, and malformed/newer-schema pointers are rejected.
- **doctor**: corrupt-store fixtures (bad YAML, dangling provenance, unparseable audit, stale context) → assert each is reported.

---

## 11. What NOT to build (architectural guardrails)

These mirror the HANDOFF watchouts and are structural, not stylistic:

- **No database, no derived index** in v1. Files are truth. If listing/search ever needs an index, it's a pure derived cache added later — not now.
- **No persistent cross-Dossier graph/links.** Relatedness is resolved by merge into one Distilled State.
- **No global active Dossier.** Binding is per session (`sessions/<id>.json`).
- **No native delete** command on CLI or MCP. Archive only.
- **No last-write-wins for Distilled State.** Conflicts are artifacts, surfaced.
- **No silent truncation** to hit the token target. Warn, never cut.
- **No silent linking/merging** of ambiguous targets. Ask.
- **Core stays pure.** If you're tempted to import `os`/a harness/the filesystem into `internal/core`, you've put logic in the wrong layer — move it behind a port.

---

## 12. Build order (maps to SPEC §15 milestones)

The SPEC milestones are the plan; this is the architectural sequencing that makes them land cleanly:

1. **M1** — `core` types + `ports.go` + fake Store; `fsstore` skeleton; `config`; `init`/`doctor` baseline; **`docs/harness-capabilities.md` for Claude Code** (B2). Establish the dependency-direction CI guard now.
2. **M2** — full `fsstore` (atomic writes, audit, ids/slugs); core create/read/update; `list`/`show`/`path`/`archive`.
3. **M3** — `recall` (+ revision + token estimate), `tokenizer`, over-target warning, `search` (native first, rg fast-path), generated `library.md`.
4. **M4** — MCP stdio server + all `dossier_*` tools over the existing Service.
5. **M5** — `promote`, `suggest`, `link` + ambiguity, transcript-unavailable warnings.
6. **M6** — session binding, `switch`/`active`, Claude Code hooks (session-start/end, pre-compaction), TUI dashboard.
7. **M7** — optimistic concurrency + conflict artifacts + `merge` with conflict reporting.
8. **M8** — ship Distillation Guide with examples; dogfood across all three harnesses; tune.

Revisions and the Store contract are foundational — get §5 and §6 right early; everything writes through them.
