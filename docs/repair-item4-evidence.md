# Item 4: non-destructive archive and session cleanup

## Design and confirmed cause

The existing `dossier archive <slug-or-id>` / `dossier done <slug-or-id>` commands call `Service.Archive` (`internal/core/service_session.go:186`). Previously it wrote `status: done`, moved the entire directory into `archive/` through `FSStore.Write`, and recorded an audit event, but never enumerated or cleared session bindings. Manual folder removal likewise bypasses all service operations.

Bindings are written/read/cleared in `internal/store/fsstore.go` (`SaveSessionBinding`, `GetSessionBinding`, `ClearSessionBinding`). The new optional `SessionBindingStore` port exposes bulk reading for archive and doctor. Filename identity is authoritative for cleanup, not the JSON's self-reported ID. Corrupt files stop archive before the status change. Cleanup errors after archive are visible warnings with a retry command; the operation is deliberately not claimed to be transactional with bindings.

No `rm` command: SPEC §2.7 and CLAUDE.md prohibit native delete. Use the existing archive command; it preserves all state, artifacts, audit, and history, and removes bindings pointing at that immutable ID. Other/unbound sessions are untouched. Repeating archive is safe.

`internal/store/fsstore.go:113-121` hashes immutable IDs to `.locks/<hash>.lock`; `internal/store/lock.go:14-24` acquires/releases advisory locks. Empty lock files are not held locks. They remain intentionally: deleting an unlocked pathname is unsafe while a waiting writer can still hold its old file descriptor, because a new writer can create and lock a different inode. This does not satisfy a literal request to remove lock files; retaining them preserves the existing concurrency contract. No real locks or store files were inspected or removed.

## Regression

Before the core cleanup change:

```text
go test ./internal/cli -run TestArchiveClearsOnlyMatchingBindingsAndPreservesHistory -count=1
--- FAIL
archive retained session one
archive retained session two
```

After: the same command passed. The test also checks unrelated bindings survive, state remains in archive, stable lock identities remain, and a second archive succeeds.

## Exact manual commands executed

Baseline was built before editing `Service.Archive`:

```sh
cd /tmp/dossier-repairs-858363ad
go build -o /tmp/dossier-before-item4 ./cmd/dossier
# after the repair:
go build -o /tmp/dossier-after-item4 ./cmd/dossier
T=$(mktemp -d /tmp/dossier-item4.XXXXXX)
printf 'TEMP_ROOT=%s\n' "$T"
for phase in before after; do
  BIN=/tmp/dossier-$phase-item4
  H="$T/$phase"
  mkdir -p "$H/user"
  HOME="$H/user" DOSSIER_HOME="$H/store" "$BIN" promote "Archive cleanup" --distilled '# Keep history' --force
  HOME="$H/user" DOSSIER_HOME="$H/store" "$BIN" switch archive-cleanup --session bound-one
  HOME="$H/user" DOSSIER_HOME="$H/store" "$BIN" switch archive-cleanup --session bound-two
  HOME="$H/user" DOSSIER_HOME="$H/store" "$BIN" archive archive-cleanup
  printf '%s binding files: ' "$phase"
  find "$H/store/sessions" -name '*.json' | wc -l
  printf '%s retained archive: ' "$phase"
  test -f "$H/store/archive/archive-cleanup/dossier.md" && printf 'yes\n'
  HOME="$H/user" DOSSIER_HOME="$H/store" "$BIN" archive archive-cleanup
done
```

Observed temp root `/tmp/dossier-item4.Xi5GTR`: before retained **2** bindings; after retained **0**; both retained the archived Dossier and both repeated archives succeeded.

Doctor orphan detection is covered alongside item 3/6 in the corresponding evidence note. Already-orphaned bindings are reported, not automatically removed; restore the missing Dossier or remove only the named machine-local binding after inspection. No private data or real remote was used.
