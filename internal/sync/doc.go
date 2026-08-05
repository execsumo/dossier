// Package sync is a Phase-2 SPIKE adapter proving out go-git as the transport
// for Dossier Team Sync (see docs/team-sync-plan.md and ADR 0005).
//
// It implements a pull → resolve(remote-wins) → commit → push engine plus a
// store-wide sync lock, oversized-file exclusion, .gitignore management, and
// conflict capture. Conflicts are detected, never merged into the working tree:
// no git merge markers ever land in the store; the remote version wins the
// working copy and the local version is returned in a [ConflictRecord] for the
// caller (the future Phase-2 wiring) to route into core's conflicts/*.md
// machinery.
//
// This package is deliberately self-contained: it is NOT wired into
// [core.Service], the CLI, or MCP. The real Phase 2 wiring will live behind a
// future core.Syncer port; this spike validates the approach and records
// findings in docs/spikes/gitsync-findings.md. All tests run against local bare
// repos — no network, no GitHub.
package sync
