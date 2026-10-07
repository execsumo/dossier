# ADR 0017: `dossier export` writes one self-contained brief for a non-Dossier reader

## Status
Implemented (2026-10-07). Files: `internal/core/export.go` (`Service.Export`, `RecordExport`), `internal/core/workfiles.go` (`FileStore.ReadWorkingFile`, `IsTextContent`), `internal/core/audit.go` (`exported` event fields), `internal/store/fsstore.go` (`ReadWorkingFile`), `internal/store/fake.go`, `internal/exportout/exportout.go` (shared path resolution, atomic write, audit call), `internal/cli/cli.go` (`dossier export`), `internal/mcp/tools.go` (`dossier_export`). Tests: `internal/core/export_test.go`, `internal/store/workfile_read_test.go`, `internal/cli/export_test.go`, `internal/mcp/export_test.go`.

## Context
A Dossier often has to reach someone who does not run Dossier. The motivating case: before a meeting,
the owner hands an executive the Dossier and says "go wild with questions". The formal write-up is
itself kept in the Dossier, but the executive's likely questions are answered elsewhere in it: in
Constraints, Decisions, Open Questions, and the plans and evidence behind them. The executive will
usually paste the material into an AI assistant (Claude, ChatGPT) and question it there.

Sending `dossier.md` alone is not enough for that:
- it carries machine frontmatter that means nothing to the reader;
- the evidence and the write-up live outside it, in `artifacts/` and `files/`;
- the store also holds things that must not leave: raw session transcripts, routed inbox excerpts,
  history, audit, conflicts.

The command is named `export`, not `share`. "Share" suggests a live, human-shared object (compare
Team Sync). This is a point-in-time copy that leaves the system.

## Decision

### 1. One Markdown file, no options for shape
`dossier export <slug-or-id>` writes one self-contained Markdown file. It has no zip mode and no
include/exclude flags. A single file can be pasted or attached into any chat assistant or email,
and one fixed shape means the owner never has to make a packaging decision before a meeting.

### 2. Contents, in order
1. **Title and header line.** `# <name>`, then the export date, last updated (`updated_at`),
   status, lead, and next action. The YAML frontmatter and the revision hash are not emitted:
   both are noise to the reader.
2. **Reader preamble.** This exact text, which tells the reader's AI how to use the file:

   > **About this document.** This is a working Dossier exported on <date> (last updated <updated_at>): the
   > author's full working context on one outcome, broader than any formal write-up it contains.
   > If you are an AI assistant helping the reader: answer from this document; say which section or
   > supporting item an answer comes from; when the document does not cover something, say so
   > plainly rather than inferring. Citations of the form `[src:art_…]` refer to the supporting
   > items below by ID; items listed under "Not included" were deliberately left out.

3. **The Distilled State body, verbatim.** It is not rewritten, reordered, or trimmed.
4. **`## Supporting material`.** Each included item gets a `### <title>` heading, then a metadata
   line (artifact ID or `files/` path, type, captured/modified date, origin/URL), then its content
   in a fenced block. The fence must be longer than any backtick run inside the content.
5. **`## Not included`.** Lists every excluded item and the reason, so the reader's AI knows what
   it cannot see and never guesses at it.

### 3. Inclusion rules
| Source | Rule |
|---|---|
| Artifacts of type `source_snapshot`, `file_snapshot`, `link`, `query`, `decision_evidence` | Included in full. |
| Artifacts of type `transcript` | **Excluded**, listed under Not included. Raw sessions never leave. |
| Superseded snapshots | Only the newest of each `(type, title, provenance.url)` group is included, by `refreshed_at` and then `captured_at`. Its heading lists the superseded IDs (`supersedes art_a, art_b`), so a citation of an older snapshot still resolves to the current version. |
| Text working files under `files/` | Included in full. This is where the formal write-up usually lives. |
| Binary working files under `files/` (e.g. `.pptx`, `.pdf`, `.docx`) | Not inlined. Listed under Not included with path and size, plus a warning. Dossier stores no binaries natively (SPEC §4.3). |
| `## Files` entries pointing outside the Dossier (repo paths, absolute paths) | Not resolved or inlined. Listed under Not included, plus a warning. |
| `inbox/`, `history/`, `audit/`, `conflicts/`, session stashes, `config.yaml` | Never read for export. Not listed. |

"Text" means valid UTF-8 with no NUL bytes in the first 8 KB. No such check exists yet; add it to
core as a pure helper. Reading a working file's content needs a new `ReadWorkingFile` method on the
optional `FileStore` port, which today only lists. Order: artifacts cited by the Distilled State first, in order of first citation; then
uncited artifacts by `captured_at`; then working files by path.

### 4. Guardrails: warn, never edit
Warnings go in the result envelope (CLI stderr, MCP `warnings`). None of them blocks the export,
and none rewrites content:
- **Local paths.** Each absolute path under the user's home directory (`/home/<user>/`,
  `/Users/<user>/`, `~/`) found in the output is named once, with where it occurs.
  Paths are not rewritten: changing a constraint's wording is a decision for the owner.
- **Unresolved conflicts** on this Dossier: the brief may not be settled.
- **Size.** If the token estimate of the whole file exceeds `token_limit`, warn with the estimate.
  Never trim to fit (hard rule: no silent truncation).
- **Exclusions.** One summary warning: "N transcripts, M binary files, K external file references
  not included."

### 5. Output location and safety
- CLI: `dossier export <slug-or-id> [-o <path>|-o -] [--force] [--json]`. `-o -` writes to
  stdout and sends the summary to stderr.
- **Default location: `~/Downloads/<slug>-export-<YYYY-MM-DD>.md`.** If there is no
  `~/Downloads`, the home directory is used. This default is the same for CLI and MCP and does not
  depend on the working directory. Two reasons:
  - The MCP server's working directory is usually the agent's project repo, where a stray export
    would end up committed.
  - The file is going to be attached or sent, and that is where people look for files to send.
- If the default path already exists (a second export the same day), `-2`, `-3`, … is appended.
  Nothing is overwritten.
- An explicit `-o`/`output_path` that already exists is refused unless `--force` is given.
- The command never writes inside the Dossier store. An export under `~/.dossier` would sync to
  the team and be mistaken for evidence. A path that resolves inside `DOSSIER_HOME` is rejected.
- On success the CLI prints a summary: output path, revision, items included/excluded, token
  estimate, warnings.

### 6. Audit
A successful export appends an `exported` audit event carrying `revision`, `artifacts_included`
(IDs), `files_included` (paths), and `output` (the basename, not the full path). The owner can
later answer "which version did the executive see?". The event is append-only and changes no
revision. Writing to stdout records `output: "-"`.

This adds no versioning scheme. `revision` is the content hash every Dossier already carries for
optimistic concurrency, and superseded revisions are already kept under `history/rev_<hash>.md`.
The event's timestamp records *when*. The revision is the direct key to *what*: the exact text that
went out, recoverable even after later edits. A timestamp alone would mean reconstructing that from
the edit sequence.

### 7. One core operation
`core.Service.Export(ctx, ExportReq) (Result, error)` assembles the document, as a pure function
over store reads, and returns the Markdown plus the summary and warnings. Writing the file is the
adapter's job: CLI writes to `-o`; MCP `dossier_export` takes optional `output_path`/`force` and
writes the file the same way, returning the path and summary. MCP never returns the full document
inline unless `inline: true` is passed. **Export is CLI and MCP only for now** (a known surface
gap, recorded in `ARCHITECTURE.md`). A TUI binding is out of scope for this ADR; when added it
must call `Service.Export`.

## Consequences
- An owner can hand anyone a Dossier with one command, without exposing sessions.
- The document goes out as it is. The export never improves or redacts it, so the Distilled State's
  quality is what the reader gets, and local paths or sensitive notes are the owner's call, warned
  about but not fixed.
- Binary write-ups (`.docx`, `.pptx`) do not travel inside the export. An owner who wants the write-up
  inside must keep a Markdown version in `files/`.
- An export is a copy. It does not update when the Dossier changes; the audit event records which
  revision went out.

## Alternatives considered
- **Send `dossier.md` as is:** carries frontmatter, misses the evidence and write-up, and has no
  instruction for the reader's AI.
- **Zip bundle (brief + artifacts):** awkward to give to a chat assistant, and the reader has to
  assemble context themselves. One file does that work for them.
- **Options for shape (`--bundle`, `--include-transcripts`):** rejected. Each option is a decision
  the owner has to make at the worst moment, and transcripts are never appropriate to send.
- **Redaction list:** deferred. Warn-and-review covers the known risk (local paths); add redaction
  only if real exports show a recurring pattern.
- **Live sharing link:** that is Team Sync's job, and it requires the reader to run Dossier.
