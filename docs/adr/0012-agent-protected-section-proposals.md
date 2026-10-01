# ADR 0012: Agent proposals for protected Dossier sections

- Status: Accepted
- Date: 2026-10-01
- Decision: BUILD-DECISION B22

## Context

Case officers need to maintain current situation and findings, but the canonical Objective, Done When, Validation, Constraints, and Decisions cannot be silently rewritten by an autonomous agent. Splitting one Save into accepted and unaccepted pieces would make one call's effects difficult to reason about.

## Decision

When a Save by an `agent:` actor changes any protected section, reject the entire body update and preserve the complete proposed body plus diff as a conflict of kind `agent_proposal`. No part of the proposal is applied. Agent callers keep routine updates separate from protected-section proposals. The existing conflict detail/resolution surfaces are reused; resolving an `agent_proposal` requires a human actor.

## Consequences

- Proposal provenance and conflict retention use the established conflict artifact path; no parallel proposal storage model is created.
- The response names protected sections and returns the proposal id, while retaining the existing revision in the live Dossier.
- Section comparison is a structural Markdown heading-level-two comparison, not semantic interpretation. Future finer Decision Rights can evolve policy without silently changing the current whole-save contract.
