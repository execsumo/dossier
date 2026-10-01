# Agent Work Ecosystem Conventions

This document records conventions shared by Dossier, Rolodex, and Sit Room. It is intentionally about boundaries and wire conventions, not shared Go packages: the products keep independent release cycles and may duplicate small implementations. The family design system is [`docs/design.md`](docs/design.md); Sit Room maintains its own copy.

## Product boundary

- **Dossier** owns durable work state, provenance, revisions, and audit. Files are truth; there is no daemon or database.
- **Rolodex** owns agent identities and measured improvement through real use.
- **Sit Room** owns runtime behavior: credentials, network access, clocks, model calls, routing, and orchestration.
- A product uses another's documented CLI/MCP surface, not its private store or filesystem layout.

## Shared API conventions

- Mutations and reads return a structured result envelope: `ok`, `data`, `warnings`, and `next_actions`; errors use `ok: false` plus a stable machine-readable `error.code`, human-readable `error.message`, and optional `error.details`.
- Warnings are observable and actionable. A missing capability or failed best-effort operation is not a successful silent no-op.
- Actor identity is explicit and distinct from machine author identity. Use `human:<identity>`, `agent:<stable-agent-slug>`, or `system:<name>` for attribution; an actor label is provenance, not an authentication boundary.
- Service/core owns policy. CLI, MCP, and TUI adapters validate transport shape, call the same service operation, and render its result; do not fork business rules per surface.

## Files, writes, and concurrency

- Read/merge/write user configuration; never clobber unrelated entries. Before replacing a user-owned file, make a recoverable backup and make installation idempotent. Ask before changing harness configuration.
- Write files atomically where practical: temporary file in the destination directory, flush, then rename. Serialize mutations with the narrowest stable lock; do not hold a network or UI wait under a content lock.
- Preserve source and history. Do not silently overwrite concurrent authored state; retain both proposals and surface a resolvable conflict. Append-only audit is attributed and sharded by a stable single-writer identity where needed.
- Machine-local credentials, runtime state, and private intake are excluded before publication, not after. A published history cannot be assumed retractable.

## Dependency and degradation rules

- Domain logic stays independent of filesystem, network, model, and harness implementation. Effects live behind ports/adapters.
- Network/model/harness behavior belongs at the runtime boundary; durable state belongs in Dossier.
- Degrade visibly: explain which capability or operation failed, what state remains safe, and the next action. Never promise transcript capture or lifecycle behavior without verification against the actual harness.
- These conventions improve compatibility; they are not a security specification. Each product documents its own concrete threat model and authorization rules.
