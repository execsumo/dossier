# ADR 0010: Distinct Team Sync authors for headless agents

- Status: Accepted
- Date: 2026-10-01
- Decision: BUILD-DECISION B19

## Context

Team Sync's audit shards are single-writer per author. An always-on headless agent on a VM cannot safely share a principal's username with the principal's laptop: both machines would write the same `audit/<author>.log` shard. Team Sync credentials and onboarding must also work without an interactive terminal or a general-purpose human GitHub login.

## Decision

A headless agent is a distinct Team Sync author configured on its machine (for example, `author: sitroom`). The synced roster records a member kind, `human` by default and `agent` when explicitly configured. Agent identities are visible as agents and cannot be assigned as Dossier leads; agents may own work only through Delegation Contracts. Each operation's actor attribution is a separate concern and will identify the acting agent (D3). Fine-grained token credentials remain supported as the headless authentication path.

Add per-Dossier inbox exclusions before any store is first published. Inbox entries contain private routed excerpts and are machine-local by default.

## Consequences

- B17's one-machine-per-author invariant remains intact; no multi-device audit-shard policy is introduced.
- Existing roster YAML remains compatible: absent kinds mean `human`.
- Headless operation does not imply Dossier becomes a daemon or gains network/model responsibilities.
- Live GitHub validation remains required before Sit Room relies on Team Sync in production.
