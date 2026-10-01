# ADR 0011: Actor attribution and machine-managed attention

- Status: Accepted
- Date: 2026-10-01
- Decisions: BUILD-DECISIONS B20 and B21

## Context

Autonomous agents need to update operational Dossiers, but some work decisions remain human-only. Team Sync's `author` is a machine identity used to shard append-only audit files; it cannot also say which agent caused a write. The principal's attention state must be cheap to scan and consistent without asking the human to maintain another field.

## Decision

Represent the actor on each mutation as `human:<identity>`, `agent:<stable-slug>`, or `system:<name>`, and retain the machine-local author independently in audit. MCP reads `DOSSIER_AGENT` for an explicit autonomous agent; absent that, existing interactive behavior is preserved. Core authorization is an accident-prevention rule, not an authentication boundary. Agents may not mark a Dossier `done` or accept a Delegation Contract. An agent's decided Acceptance field is retained as an `agent_proposal` and is not applied live.

Add an optional frontmatter `attention` value (`none|fyi|decide|blocked`, summary ≤140 characters, timestamp and actor). Only agents and systems may set or clear it. It is separate from lifecycle and next action; interactive surfaces display it read-only and list filtering can select a level.

## Consequences

- Audit can attribute an agent action while retaining the machine identity that owns the single-writer shard.
- Existing calls that omit actors retain compatibility through a human identity derived from configured author.
- Actor propagation must cover every write adapter and Service use case; missing attribution is a correctness gap, not silently inferred authorization.
- Attention updates go through the ordinary Save/revision path, so the field participates in integrity and concurrency checks.
