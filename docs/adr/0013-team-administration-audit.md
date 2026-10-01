# ADR 0013: Team administration actors and audit

- Status: Accepted
- Date: 2026-10-01
- Decision: BUILD-DECISION B24; D3

## Context

Dossier audit entries are normally owned by a Dossier, but roster changes, team creation, and team joining mutate shared-store administration rather than any one Dossier. The configured Team Sync author identifies the writer shard, but does not say whether a human or autonomous agent initiated the operation.

## Decision

Team roster add/remove/kind changes, confirmed team creation, team joining, and resolving a root team-roster conflict require a human actor in core. CLI adapters use `agent:<DOSSIER_AGENT>` when set, otherwise `human:<configured-author>`. The machine-local configured author remains the separate audit-shard writer identity.

Record successful team actions in append-only `team-audit/<author>.log` files at the store root. These files sync with `team.yaml`; per-author files avoid multiple writers appending to one shard. Routine sync is transport and does not itself produce a team-admin event. Sync-generated roster conflicts are attributed to `system:team-sync` in the root team audit shard, and human resolutions are recorded there as well.

## Consequences

- Team audit rows use `dossier_id: __team_roster__` as an explicit root-level target sentinel; they are not attached to an arbitrary Dossier.
- Roster add/remove rolls back the roster write if the matching audit append fails.
- Team create/join surface an audit-write or follow-up-sync failure as a warning after the external operation has already succeeded.
- This is provenance and accident prevention, not authentication; actor identity is caller-supplied.
