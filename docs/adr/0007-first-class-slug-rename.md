# ADR 0007: Dossier IDs are immutable; canonical slugs are renameable

## Status
Accepted (2026-09-04). Supersedes the statement that a slug/directory never changes when a display name changes; display-name edits still do not implicitly rename the slug.

## Context
A slug is useful as a human-readable command and directory name, but the name chosen when a Dossier is created can become misleading. Editing `slug:` by hand is unsafe: the frontmatter can diverge from the directory, while artifacts, history, audit shards, conflicts, working files, and machine-local session stashes all live beneath that directory. Slug references may also exist in scripts or prior conversations.

The existing `dos_` ID is already immutable and session bindings store that ID. It is therefore the correct durable identity; the slug should be a mutable locator with compatibility aliases.

## Decision
Slug changes use a dedicated `Service.RenameSlug` operation, exposed as:

- `dossier rename <slug-or-id> <new-slug> [--base-revision <rev>]`;
- MCP `dossier_rename` with `id`, `new_slug`, and `base_revision`;
- `s` from the TUI detail view.

Generic `Save`, `dossier_update`, and direct `Store.Write` reject slug or alias mutation.

A successful rename:

1. validates a canonical lowercase ASCII slug and rejects reserved, occupied, canonical, or alias collisions;
2. preserves the immutable Dossier ID;
3. archives the pre-rename revision;
4. adds the old canonical slug to sorted frontmatter `aliases`;
5. atomically replaces `dossier.md` and performs one same-parent `os.Rename` of the complete directory;
6. writes a `slug_renamed` audit event.

Reads resolve the immutable ID, current canonical slug, or any alias. Renaming back to an old slug rotates the displaced canonical slug into aliases, so every historical locator remains valid. Newly created Dossiers cannot claim another Dossier's alias.

The filesystem adapter uses locks outside the movable directory: a root namespace lock and stable `<home>/.locks/<id-hash>.lock`. Rename also takes `.sync.lock`, preventing a directory move from racing a Team Sync checkout.

Team Sync indexes Dossiers by immutable ID before its path-based merge. A rename and concurrent edit reconcile to one selected directory; remote wins concurrent different rename targets, and losing slugs are unioned into aliases. Fast-forward checkout stashes ignored per-Dossier session data by ID and restores it beneath the resulting canonical directory.

## Consequences

- Old slug references remain valid, but canonical output and paths use the newest slug.
- Aliases become revision-bearing canonical frontmatter and participate in list search.
- The complete nested directory moves without copying, so unknown user files and every Dossier namespace move together.
- A stale base revision, failed move, or collision leaves the original Dossier usable; a move failure restores the old frontmatter.
- Clients predating this schema cannot parse `aliases` because Dossier intentionally uses strict frontmatter. Team stores that use slug rename therefore require clients containing this ADR's schema support.

## Alternatives considered

- **Display-name change only:** safe and still supported, but does not improve a misleading command/path slug.
- **Symlink or root alias registry:** keeps references but is less portable and can drift separately from the Dossier under Team Sync.
- **Store directories by immutable ID:** cleanest eventual separation of identity and presentation, but requires a store-wide migration and changes user-facing path semantics.
- **Manual frontmatter/directory edits:** rejected because they bypass concurrency, history, audit, collision checks, aliases, and Team Sync reconciliation.
