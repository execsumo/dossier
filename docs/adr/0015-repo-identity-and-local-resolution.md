# ADR 0015: Repo identity in the Dossier, path resolved per machine

## Status
Accepted (2026-10-06). **Not yet implemented.** Amends [ADR 0006](0006-tui-open-in-claude.md)'s
"Working directory" consequence. Used by [ADR 0014](0014-herdr-tab-switcher.md) when it
opens a new tab.

## Context
ADR 0006 starts every agent in the Dossier's own folder (`~/.dossier/<slug>/`), because the
frontmatter has no pointer to a repo. That fits a brief-only Dossier, but for coding work the
agent should start in the code repo. This matters more under ADR 0014, where agents stay
running in their own tabs.

Team Sync (B12) shares Dossiers across machines, and every teammate keeps repos somewhere
different (`~/projects/billing-api`, `C:\src\billing`). A path stored in the Dossier would be
wrong for everyone but its author. Machine-local files never sync (B13).

## Decision

### 1. The Dossier stores which repo, not where it is
Add an optional synced frontmatter field:

```yaml
repos:
  - github.com/acme/billing-api   # first entry = primary (launch directory)
  - github.com/acme/billing-web
```

- **Normalized form:** lowercase `host/owner/name`, with the scheme, user, port, a trailing
  `.git` and a trailing `/` removed. `git@github.com:acme/x.git`, `ssh://git@github.com/acme/x`
  and `https://github.com/acme/x` all become `github.com/acme/x`.
- **Validation:** the core service normalizes input and rejects values that don't parse.
  Order is significant; duplicates are rejected.
- **Not a protected section.** `repos` is metadata, not work definition (B22 does not apply).
  Humans and agents may edit it through `dossier_update` / `dossier update` and the
  `dossier repo add|remove` commands, and every change is audited like other frontmatter edits.

### 2. Each machine maps repo identity to a local path
- **Learned map:** `<home>/local/repo-paths.json` holds `{ "github.com/acme/x": {"path": "...",
  "learned_at": "...", "via": "session|scan|manual"} }`. The new `local/` directory holds
  machine-local state that is not configuration. Add `/local/` to
  `internal/sync/gitignore.go` `defaultGitignoreEntries` and to the B13 list.
- **Search roots:** an optional `repo_roots: [~/projects, ~/src]` setting in `config.yaml`,
  which is user configuration and machine-local already.

### 3. Resolution, at launch time
For the primary repo, then for each other repo:

1. **Learned map.** Use the path only if it still exists and its `origin` still normalizes to
   the same identity. A stale entry is dropped and the warning says so.
2. **Search `repo_roots`** one level deep (`<root>/*/.git`, including `.git` files for
   worktrees) for a matching `origin`. A hit is written to the map (`via: scan`). If there
   are several hits, don't guess: warn and list them, and ask for `dossier repo locate`.
3. **Not found:** launch in the Dossier folder with a visible warning:
   `billing-api (github.com/acme/billing-api) isn't on this machine — run dossier repo locate <slug> <path>`.

Read remotes with the embedded go-git (`PlainOpenWithOptions{DetectDotGit: true}`), not the
git CLI (B12: no git CLI dependency). This is I/O, so it lives behind a new `RepoLocator` port
with a filesystem adapter; core only sees identities and resolved paths.

### 4. Learning from use
When a session is bound to a Dossier (MCP `dossier_session` / `dossier_link` /
`dossier_promote`, or a SessionStart hook that finds a binding), Dossier looks at the
session's working directory:

- **Which working directory:** the MCP server process's `os.Getwd()`. The harness starts the
  MCP server, which inherits the session's directory, so this works for every harness. The
  Claude Code SessionStart hook payload also carries `cwd`. Today
  `internal/cli/cli.go`'s hook payload struct ignores it; add the field.
- **If that directory is inside a git repo whose normalized `origin` is in the Dossier's
  `repos`:** record the path in the learned map (`via: session`). This is silent, because it
  only updates a cache.
- **If the Dossier has no `repos`, or the remote isn't listed:** suggest, don't write. The
  response carries a `next_action`: "This session is in github.com/acme/x; add it to the
  Dossier's repos?" This follows the "no silent link" rule: linking a repo to a Dossier is the
  user's call.

### 5. Launch and prompt changes
- **Launch directory** = the resolved primary repo, otherwise the Dossier folder.
- **The prompt names the Dossier by absolute path.** ADR 0006's prompt says `./dossier.md`,
  which is wrong once the agent works in a repo. It becomes `<dossier dir>/dossier.md`. The
  prompt also lists the other resolved repos with their local paths, and names the
  unresolved ones.
- **The repo's own instructions govern repo work.** Started in the repo, Claude Code loads
  the repo's `CLAUDE.md` (and those of parent folders) by itself; Codex, Cursor and Pi read
  `AGENTS.md`. Dossier does not copy those rules into its prompt. *Unverified:* Claude Code
  is believed **not** to read `AGENTS.md` natively (repos usually import it from `CLAUDE.md`
  with `@AGENTS.md`). Check this with a headless run before relying on it, and record the
  result in `docs/harness-capabilities.md`.
- **Deliverables go where they belong** (replaces ADR 0006's "save files to `./files/`"):
  - Work that belongs to the project (code, docs, assets the repo would keep) goes **in the
    repo**, following its `CLAUDE.md`/`AGENTS.md`.
  - Work that serves the outcome but isn't part of the codebase (stakeholder decks, analyses,
    briefs) goes in `<dossier dir>/files/`.
  - Either way it is listed under `## Files`. Repo files are recorded by identity plus
    repo-relative path (`github.com/acme/billing-api:docs/pricing.md`), never by absolute
    path, because a shared Dossier must not carry one machine's paths. Dossier `files/`
    entries stay relative to the Dossier.
  - With no resolved repo, everything goes to `files/`, as today.
- **Claude Code** also gets `--add-dir <dossier dir>` (the flag is present in the installed
  CLI, 2026-10-06), so writing to `files/` doesn't stop for a permission prompt. Other
  profiles get only the prompt text. Record per-profile support in
  `docs/harness-capabilities.md` when built.
- **Context loading is unaffected.** The binding drives the SessionStart injection, not the
  working directory. Dossier's MCP server and hooks are installed at user scope
  (`~/.claude.json` / `~/.claude/settings.json`), so they still load inside any repo.

### 6. Rollout: one release
The frontmatter reader rejects unknown keys (SPEC §4.1), so a synced Dossier carrying `repos`
would be unreadable on an older binary. As of 2026-10-06 the owner is the only user, and
colleagues install only when the owner says so, after this work ships. **`repos` therefore
ships in one release, with reader and writer together.** No staged rollout is needed.

**Standing rule after colleagues install:** any later synced frontmatter field must ship
reader-first. One release accepts and round-trips it; a later release writes it, once every
team member has upgraded.

## Alternatives considered
- **A path in the Dossier.** Wrong on every machine but one. Rejected.
- **A path template with `$HOME`.** This works only if everyone uses the same layout, which
  the owner explicitly said is not the case. Rejected.
- **Storing the map in `config.yaml`.** Config is user-edited and has a strict schema, and a
  learned cache would cause config writes on every bind. Only `repo_roots` (a user choice)
  goes there.
- **Auto-adding a repo to `repos` when a bound session runs in it.** Convenient, but it
  silently links, and a session started in the wrong repo would pollute a shared Dossier for
  the whole team. Rejected in favour of a suggestion.

## Consequences
- SPEC §4.1 gains `repos`. §3.2 and B13 gain `local/`. §7 gains `dossier repo add|remove|locate|status`.
  §8 `dossier_update` gains `repos`. The `open_with` paragraph changes from "the Dossier
  directory" to "the resolved primary repo, else the Dossier directory".
- `doctor` reports `repos` entries that don't resolve on this machine. This is informational,
  not an error.
- Brief-only Dossiers (no `repos`) behave exactly as today.

## Acceptance (checkable when built)
1. Table test: normalization of SSH, scp-style, HTTPS, `.git`, a trailing slash, mixed case
   and a port, plus rejection of garbage.
2. Temp-dir test: resolution order (map hit; stale map entry dropped; search hit written to
   the map; several search hits produce a warning and no guess; a miss falls back to the
   Dossier folder with a warning).
3. Test: binding from a directory inside a listed repo writes the map; binding from an
   unlisted repo returns the suggestion `next_action` and leaves frontmatter unchanged.
4. Test: with a resolved repo, the prompt contains `<dossier dir>/dossier.md`, contains no
   `./dossier.md`, states the deliverables rule (project work in the repo under its
   instructions, other deliverables in `<dossier dir>/files/`, `## Files` entries for repo
   files as `<identity>:<relative path>`), and Claude plans include `--add-dir <dossier dir>`.
   Without a repo, the prompt sends everything to `files/`.
5. Test: `repos` round-trips through read and save unchanged.
6. Sync test: `local/` never appears in a commit.
7. Manual: a Claude session launched in a repo with a `CLAUDE.md` follows it. Check once
   whether Claude Code reads a bare `AGENTS.md`, and record the result.
