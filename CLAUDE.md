# CLAUDE.md — Dossier repo working rules

Guidance for any agent (Claude Code, Codex, etc.) building Dossier. Read `HANDOFF.md` first for the reading order and current status.

## What this is

Dossier (codename *chainlink*) is a local, single-user durable memory layer for agent-driven work in Claude Code and Pi. Pi support depends on the compatible hooks-extension contract documented in `HANDOFF.md`. A Dossier is a flat durable topic with a curated Markdown **Distilled State**, a source-retaining **Archive**, and an append-only audit log. One self-contained **Go** binary serves CLI + MCP-over-stdio + hooks + TUI. No database; files are the source of truth.

## Reading order (do not skip)

1. `HANDOFF.md` — status + where to start.
2. `BUILD-DECISIONS.md` — settled build choices; **do not relitigate**.
3. `ARCHITECTURE.md` — how the code is structured (Go, ports-and-adapters). Maintain it as you build.
4. `SPEC.md` — the contract (data model, CLI, MCP, algorithms, acceptance criteria §14, milestones §15).
5. `PRD.md` / `PRFAQ.md` — the why, when a *why* is unclear.

If `SPEC.md` and `PRD.md` disagree, `SPEC.md` wins on mechanics; `BUILD-DECISIONS.md` wins over both where it speaks.

## Build / test / run (once scaffolded)

```bash
go build ./cmd/dossier        # compile the single binary
go test ./...                 # all tests
go test ./internal/core/...   # fast pure-domain tests
go vet ./... && test -z "$(gofmt -l .)"  # lint / format check
DOSSIER_HOME=$(mktemp -d) ./dossier init   # exercise init against a throwaway store
./dossier mcp serve           # run the MCP server over stdio
```

Match the surrounding code's idioms once they exist. Standard Go: `gofmt`, table-driven tests, errors wrapped with `%w`, no panics in library code.

## Hard rules (structural — these are the trust mechanism)

- **Files are truth. No database, no derived index in v1.** An index, if ever needed, is a pure derived cache added later.
- **Core stays pure.** `internal/core` imports no sibling packages and no I/O. Logic that touches the filesystem/harness/network goes behind a port. A CI check guards the dependency direction.
- **CLI, MCP, and TUI must behave identically** — they are thin adapters over one `core.Service`. Never fork logic into an adapter.
- **No native delete.** Archive only.
- **No last-write-wins for Distilled State.** Concurrent edits become `conflicts/*.md` artifacts, surfaced.
- **No silent truncation** to hit the 100k token target — warn, never cut.
- **No silent link/merge** of ambiguous targets — ask the user.
- **No global active Dossier** — binding is per session.
- **Non-destructive always** — superseded content moves to Archive/audit, never deleted. This replaces a human confirm gate.
- **Degrade visibly** — a missing harness capability is a surfaced warning, never a silent no-op. Don't promise transcript capture universally.
- **Never clobber a user's harness config** — read/merge/write, back up, idempotent, confirmation before modifying (BUILD-DECISIONS B7/B8).

## Test expectations

Every milestone ships with tests for its acceptance criteria (SPEC §14). Minimum bar: `revision.go`/`priority.go`/`suggest.go` table tests; `store` temp-dir integration incl. the 500-Dossier <2s scan; MCP envelope + error-code mapping; harness `Detect`/idempotent-`Install`; `doctor` corrupt-store fixtures; Distillation Guide golden-file fixtures. See `ARCHITECTURE.md` §10.

## Definition of done for a change

Compiles, `go vet` + `gofmt` clean, tests pass, the relevant SPEC §14 acceptance criteria are demonstrably met, and `ARCHITECTURE.md` is updated if structure changed. Update `HANDOFF.md`'s status section as milestones complete.

## When blocked or when reality contradicts a doc

The capability assumptions (transcript access, session ids, hooks) are *assumptions* until verified against the real Claude Code harness. If Claude Code can't do what a doc assumes, **stop and flag it**; record findings in `docs/harness-capabilities.md`. Don't silently work around a contradicted decision.
