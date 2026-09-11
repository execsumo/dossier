# TUI Review and Sequenced Remediation Plan

> Reviewed: 2026-09-11
> Scope: `internal/tui/`, its tests, the CLI launch path, and the documented
> TUI contract. This is the current remediation plan; `docs/tui-plan.md` is the
> historical implementation plan.

## Review result

The TUI is useful and coherent at a normal terminal size. The main dashboard,
search, lead/interface filters, Kanban board, detail recall, Markdown scrolling,
editing, links, artifacts, merge selection, and external refresh paths are
implemented. The shared `core.Service` boundary is also the right foundation.

It is not yet robust enough to call finished:

- At `40x10`, the dashboard and board lose their title and table/board context,
  leaving only a row and the footer.
- At `60x20`, the filter overlay is clipped before all interface options and its
  footer are visible.
- At `60x20`, the combined editor wraps and clips its right-hand columns and
  hides its save hint.
- If a dossier changes externally while the Links overlay is open, the recall
  refresh changes `currentView` back to `ViewDetail` but leaves `overlayStack`
  populated. The overlay remains visible and Escape no longer closes it.
- Several asynchronous TUI commands discard `core.Result.Warnings`; no TUI
  state renders `core.Result.NextActions`.
- The README promises a detail token estimate, while the detail view and its
  test explicitly omit `Tokens:`.

### Checks performed

- `go test ./internal/tui/... -count=1`: 97 passed.
- `go test -race ./internal/tui/... -count=1`: 97 passed.
- Live `tuistory` run against a throwaway `DOSSIER_HOME`, including:
  dashboard, search, filters, Kanban, detail/scrolling, links, artifacts,
  editing, merge selection, external hot refresh, and resize checks at
  `120x36`, `80x24`, `60x20`, and `40x10`.
- A screenshot of the refreshed Links overlay was captured at
  `/tmp/dossier-tui-overlay-refresh.png`. It shows the overlay staying on
  screen after the underlying detail was refreshed.

## Sequenced plan

### 1. Stabilize navigation and asynchronous state

**Priority: P0. Do this before visual polish.**

1. Make the overlay stack the single source of truth:
   `currentView` must always equal the top overlay, or the base view when the
   stack is empty. Centralize push, pop, dismiss, and refresh behavior.
2. Fix hot refresh while any overlay is open. A refresh must update the
   underlying dossier without changing the active overlay, and every overlay
   must remain closable with Escape. Add a regression test for Links,
   Artifacts, Contracts, Edit, and Merge overlays.
3. Add request generations or target IDs to asynchronous messages. A late
   list/recall/mutation response must not overwrite a newer selection, dossier,
   filter, or overlay.
4. Preserve the selected dossier when a refresh reorders or filters the list.
   Use the immutable ID, not only the table row index.
5. Make dashboard/board edits concurrency-safe. List rows currently become a
   `targetDossier` with an empty base revision
   (`internal/tui/tui.go:757-771`), so editing from a home surface bypasses
   optimistic revision checking. Either carry the revision in list data or
   recall immediately before opening the editor.

### 2. Preserve the complete `core.Result` envelope

**Priority: P0. The TUI must not be a lossy adapter.**

1. Carry `Warnings` and `NextActions` through every async message, not only
   recall and artifact-content reads. In particular cover list, artifact-index,
   link confirmation, merge, save, rename, and agent handoff results.
2. Render warnings and next actions in one shared status area with bounded
   wrapping/scrolling. Do not let a long warning push the action footer off
   screen.
3. Keep warning state associated with the operation that produced it. A list
   refresh must not erase a mutation warning before the user can read it.
4. Add tests that inject warnings and next actions for each TUI operation and
   assert that they are visible and survive the relevant navigation.

Relevant current gaps include `listDossiersCmd` and `listArtifactsCmd`
(`internal/tui/tui.go:629-677`), `confirmLinkCmd`
(`internal/tui/tui.go:708-716`), and `saveEditCmd`
(`internal/tui/editor.go:331-367`).

### 3. Establish a responsive layout contract

**Priority: P0 for the normal supported range, P1 for extreme sizes.**

1. Decide and document the supported minimum size. Prefer a responsive
   `80x24` layout, with a clear “terminal too small” screen below the absolute
   minimum, rather than rendering clipped controls.
2. Create one layout-budget helper for title, subtitle, scope rows, body,
   warnings, and footer. Replace scattered magic values such as `height-4`,
   `height-17`, and the final `clipScreenHeight` crop.
3. Ensure every overlay fits within the terminal width and height. The filter
   overlay currently renders both option columns without windowing; long
   interface lists therefore disappear below the screen. Window or scroll
   lead/interface options, merge candidates, links, artifacts, and contract
   fields consistently.
4. Make the editor responsive. Stack the four enum columns or turn them into a
   scrollable/focused section at narrow widths; always keep the active field and
   Save/Cancel guidance visible.
5. Wrap or truncate long labels, descriptions, URLs, warning text, and
   Markdown without allowing styled content to exceed the terminal width.
6. Keep the title, current surface, selected item, active filter, and exit/save
   action visible at every supported size.

Add a snapshot and screenshot matrix for `120x36`, `100x30`, `80x24`, and
`60x20`, plus a graceful below-minimum case at `40x10`. Assert line width,
screen height, cursor visibility, and reachable exit/save actions.

### 4. Align information architecture and interaction language

**Priority: P1. Do after state and layout behavior are stable.**

1. Choose one metadata order and use it in the dashboard, detail, editor,
   README, and tests. The dashboard is
   `Dossier, Priority, Stage, Lead, Due`; detail currently renders
   `Dossier, Stage, Priority, Lead, Interfaces, Due, Next`
   (`internal/tui/tui.go:2491-2507`). Prefer the dashboard order, then append
   `Interfaces` and `Next`.
2. Resolve the token-estimate contract. Either render `TokenEstimate` in detail
   and retain the README promise, or remove the promise everywhere. The
   preferred outcome is to show the estimate beside the existing warning.
3. Replace `Unassigned (Me)` with `Unassigned`. The de-sessioned TUI has no
   current-session owner, so “Me” is misleading
   (`internal/tui/tui.go:2486-2489`).
4. Fix the search/extras contradiction. While a query is active, archived rows
   are visible and the toggle says `Hide Extras...`, but the subtitle still
   says `resolved/archived hidden`. Derive the subtitle and toggle label from
   the same `extrasVisible` state.
5. Make modal context come from the active operation. For example, a merge
   opened directly from the dashboard should identify its source rather than
   depending on a possibly stale `recallResult`.
6. Make navigation help honest and consistent. `?` is only handled on selected
   parent views; most overlays do not expose it, and text-input overlays do not
   honor `q` as quit. Decide on a common rule for `q`, `ctrl+c`, `esc`, and `?`,
   then make both dispatch and help follow it. Include a visible Back/Cancel
   action in every modal.
7. Document intentional differences between dashboard and board, especially
   collapsed terminal work versus the board’s always-visible Done column.

### 5. Harden watcher and resource lifecycle

**Priority: P1.**

1. Surface `fsnotify` errors and watch add/remove failures instead of dropping
   them (`internal/tui/tui.go:497-511` and `588-620`).
2. Coalesce bursts of file events and make refresh cancellation/replacement
   explicit. A burst must not fill the channel or produce stale responses.
3. Give model-owned watchers a clear lifecycle. `NewModel` creates a watcher
   and goroutine, while only `Run` closes it; headless tests cannot cleanly
   own that resource. Inject the watcher or provide a model `Close`/start
   boundary.
4. Add a manual refresh command as a visible fallback when watching is
   unavailable.
5. Test rename, delete/recreate, rapid writes, watch failure, and refresh while
   each overlay/editor is open.

### 6. Simplify and retire drift

**Priority: P2.**

1. Consolidate repeated view rendering and modal chrome behind the shared layout
   and status primitives rather than maintaining per-view height arithmetic.
2. Remove or update dead state and stale comments, including the unused
   `suppressFooter` path and the commented-out legacy help-overlay tests.
3. Keep `docs/tui.md` as the current plan, clearly mark
   `docs/tui-plan.md` as historical, and update README/help text only after the
   interaction contract is settled.

## Definition of done

- The overlay invariant, async response ordering, and dashboard/board revision
  behavior have model tests.
- Warnings and next actions from every TUI service call are visible and tested.
- Supported terminal sizes do not clip context or actionable controls.
- Hot refresh works from every overlay and never traps the user.
- Dashboard, board, detail, editor, help, README, and tests use the same
  labels, field order, and key semantics.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt`, and
  `git diff --check` pass, followed by the live `tuistory` matrix above.
