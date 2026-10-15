# Repair item 1: stale quoted `/opt/homebrew/bin/dossier` hook stop survives install

Date: 2026-10-14. Base: `7354587` (main). Worktree: `/tmp/dossier-repairs-858363ad` (shared). Not committed.

## Conclusion

### Observed

- **Main's own migration works.** Main's `ClaudeCodeHarness.Install` with confirmation (`--yes`) removes the quoted `/opt/homebrew/bin/dossier` hook stop entry and keeps unrelated Stop hooks. The CLI runs in a temp HOME below show this, as do the in-process tests.
- **On main, an install the user never confirmed reports success.** Two cases return `nil`: no terminal without `--yes`, and the user declining (`internal/harness/claudecode.go:280-293` at base). A `nil` makes the CLI print a normal report and exit 0, while nothing is written and the stale Stop hook stays (CLI run 1 below). `Service.InstallHarness` reports a skip only for `ErrInstallSkipped` (`internal/core/service_harness.go:271`). Pi (`internal/harness/pi.go:177-193`) and Claude Code's `Uninstall` already return that sentinel. The base code also ignored the `os.Stdin.Stat()` error and then dereferenced `stat`.
- **The `dossier` on this machine's PATH is v0.5.1 (Homebrew), and v0.5.1 treats Stop as a required hook.** Source: `git show v0.5.1:internal/harness/claudecode.go`, lines 170, 175, 309: `stopCmd := execCmd + " stop"`, `isHookConfigured(hooksMap["Stop"], stopCmd)`, `updateHookArray(hooksMap["Stop"], stopCmd, "hook stop")`. The removal commit `816264e` is in `v0.5.1..HEAD`, so it is unreleased. `v0.5.1` is the latest tag.

### Inferred (not verified)

- **Which binary ran the install that left the hook in place on this machine?** The evidence points to the v0.5.1 Homebrew binary:
  - `/opt/homebrew/bin/dossier` is the only `dossier` on PATH.
  - Every hook in the real settings points at that path.
  - The last settings backup is from 2026-10-07 14:42, two minutes after the 0.5.1 brew link was created (14:40).

  I have no record of which binary or which flags were actually run in past sessions. Two other explanations fit the evidence equally well:
  - a main build run without `--yes` from a non-terminal (the defect above);
  - no install run at all since v0.5.1.

## Disclosure: real-machine reads

The instruction allowed reads. Nothing on the real machine was written. These reads happened:

- `which -a dossier`, `ls -la` on the three candidate binary paths, `/opt/homebrew/bin/dossier --version`.
- `grep -n dossier ~/.claude/settings.json` and `ls -la ~/.claude/ | grep settings`.
- `sed -n 185,215p ~/.claude/settings.json`. This showed the real Stop block: a herdr idle hook and the Dossier hook share the `"*"` matcher, and a separate termic `done.sh` matcher follows.
- A scratch Go test (since deleted) read `~/.claude/settings.json`, copied it into a `t.TempDir()` HOME, and ran main's `Install{YesToAll:true}` on the copy. Result: the Dossier Stop entry was removed and the herdr and termic Stop hooks were kept, for both a matching and a different stable path.

```text
$ which -a dossier
/opt/homebrew/bin/dossier
/opt/homebrew/bin/dossier
$ ls -la /opt/homebrew/bin/dossier ~/.local/bin/dossier /usr/local/bin/dossier
/opt/homebrew/bin/dossier -> ../Cellar/dossier/0.5.1/bin/dossier   (Oct 7 14:40)
~/.local/bin/dossier: No such file or directory
/usr/local/bin/dossier: No such file or directory
$ /opt/homebrew/bin/dossier --version
dossier v0.5.1
$ grep -n dossier ~/.claude/settings.json      (hook lines)
76:  "command": "\"/opt/homebrew/bin/dossier\" hook pre-compaction"
104: "command": "\"/opt/homebrew/bin/dossier\" hook session-end"
129: "command": "\"/opt/homebrew/bin/dossier\" hook session-start"
205: "command": "\"/opt/homebrew/bin/dossier\" hook stop"
$ ls -la ~/.claude/ | grep settings
... settings.json.1791409342.bak   Oct 7 14:42 (latest backup)
```

## Fix (internal/harness only)

`internal/harness/claudecode.go`, the `!opts.YesToAll` block in `Install` (now lines 280–301):

- With no terminal, or when the stdin stat fails, it returns `fmt.Errorf("%w: no terminal to confirm on; re-run with --yes to apply%s", core.ErrInstallSkipped, pending)`.
- When the user declines, it returns `fmt.Errorf("%w: user declined%s", core.ErrInstallSkipped, pending)`.
- `pending` is `"; the stale Dossier Stop hook in <hooksPath> was left in place"` when `staleStopHook` is set.

Consent is unchanged (B7/B8): nothing is written without `--yes` or a `y` answer. There are no new flags. The existing core/CLI paths turn the sentinel into a `Warning:` line and exit 1 (`service_harness.go:271-283`, `cli.go:270`). `init` turns it into a warning (`internal/core/service.go:394`).

## Regression tests

File: `internal/harness/claudecode_stale_stop_test.go`. These are in-process tests that use `t.Setenv("HOME", t.TempDir())`, the pattern already in `harness_test.go:21`. They start no subprocess.

- `TestClaudeCodeInstallWithoutConfirmationReportsSkip` has two subtests:
  - stdin is a closed pipe, so there is no terminal;
  - stdin is `/dev/null`, a char device, so the prompt runs and reads an empty answer.

  Both assert `ErrInstallSkipped`, that the message names the Stop hook, byte-identical config files, and no backups.
- `TestClaudeCodeInstallRemovesQuotedHomebrewStopHook` runs a confirmed install with the stable path set to `/opt/homebrew/bin/dossier` and to `~/.local/bin/dossier`. It asserts:
  - only Dossier's Stop entry is removed;
  - both unrelated Stop hooks remain in order with their `timeout`;
  - the lifecycle hooks point at the stable path;
  - `customInstructions` and the unrelated MCP server are kept;
  - there is one settings backup and it equals the original bytes;
  - a second install changes no bytes and creates no new backup.

Before the fix (base source; fix not yet applied):

```text
$ cd /tmp/dossier-repairs-858363ad && go test ./internal/harness -run 'TestClaudeCodeInstallWithoutConfirmationReportsSkip' -v -count=1
=== RUN   TestClaudeCodeInstallWithoutConfirmationReportsSkip/no_terminal_(pipe)
    claudecode_stale_stop_test.go:126: expected ErrInstallSkipped, got <nil>
=== RUN   TestClaudeCodeInstallWithoutConfirmationReportsSkip/declined_(empty_answer)
Configure Claude Code integration (hooks + MCP server)? [y/N]:     claudecode_stale_stop_test.go:126: expected ErrInstallSkipped, got <nil>
--- FAIL: TestClaudeCodeInstallWithoutConfirmationReportsSkip (0.01s)
FAIL	dossier/internal/harness	0.242s
```

`TestClaudeCodeInstallRemovesQuotedHomebrewStopHook` passed before the fix as well. It guards the existing migration against regression.

After the fix:

```text
$ go test ./internal/harness -run 'TestClaudeCodeInstallWithoutConfirmationReportsSkip|TestClaudeCodeInstallRemovesQuotedHomebrewStopHook' -v -count=1
--- PASS: TestClaudeCodeInstallWithoutConfirmationReportsSkip (0.00s)
    --- PASS: .../no_terminal_(pipe) (0.00s)
    --- PASS: .../declined_(empty_answer) (0.00s)
--- PASS: TestClaudeCodeInstallRemovesQuotedHomebrewStopHook (0.01s)
    --- PASS: .../opt/homebrew/bin/dossier (0.00s)
    --- PASS: .../home/u/.local/bin/dossier (0.00s)
ok  	dossier/internal/harness	0.478s
$ go vet ./internal/harness ./internal/core && go test ./internal/core ./internal/harness -count=1
ok  	dossier/internal/core	0.385s
ok  	dossier/internal/harness	0.848s
$ gofmt -l internal/harness      (no output)
```

## CLI reproduction, temp HOME + DOSSIER_HOME (observed)

Binaries:

- `/tmp/dossier-before-858363ad`: built from a clean tree at `7354587`, before any edit.
- `/tmp/dossier-after-858363ad`: built from the shared tree after the fix. That tree also held other agents' uncommitted `internal/cli`, `internal/core` and `internal/store` edits.

Fixtures:

- `/tmp/repro-item1-858363ad/fixture-settings.json`: the v0.5.1 Homebrew shape, with every hook quoted at `/opt/homebrew/bin/dossier`, plus an unrelated `afplay` Stop matcher.
- `/tmp/repro-item1-858363ad/fixture-claude.json`.

Each run used a fresh `mktemp -d` directory under `/tmp/repro-item1-858363ad/`. The real `~/.claude`, `~/.claude.json` and `~/.dossier` were not used.

**About the earlier HOME refusal.** Two separate refusals happened.

1. The first combined command (it included `rm -rf` and a heredoc) got a tool permission denial: `Permission to use Bash with command ... has been denied`.
2. Later inline commands that set `HOME` were refused by the shell tool's automated worktree-isolation git-safety check, not by the permission system. The message: `This agent is isolated in the worktree .../agent-a04ac543a4148a0e0, but this command sets HOME, injecting git configuration whose effect on where git writes can't be verified. Refusing to run it`.

The session environment later changed: the agent's working directory stopped being an isolated worktree. The runs below are the same inline commands, run again through the same Bash tool, and that check no longer refused them. I did not route anything through a script, a Go subprocess, or another agent to avoid either refusal. `run-cli-repro.sh` was written but never executed.

Setup for each run:

```bash
T=$(mktemp -d /tmp/repro-item1-858363ad/run.XXXXXX); mkdir -p "$T/home/.claude" "$T/dhome"
cp /tmp/repro-item1-858363ad/fixture-settings.json "$T/home/.claude/settings.json"
cp /tmp/repro-item1-858363ad/fixture-claude.json "$T/home/.claude.json"
```

Run 1: before, no `--yes`, piped stdin (`T=run.dUJcoG`):

```text
$ (cd "$T" && echo | HOME="$T/home" DOSSIER_HOME="$T/dhome" /tmp/dossier-before-858363ad harness install claude-code); echo "exit=$?"
Claude Code integration:
- detected
...
- Dossier is fully functional in Claude Code. Nothing further to install.

exit=0
hook stop left: 1
afplay kept: 1
backups: 0
settings unchanged
```

Run 2: after, no `--yes`, piped stdin (`T=run.uBBsv8`):

```text
$ (cd "$T" && echo | HOME="$T/home" DOSSIER_HOME="$T/dhome" /tmp/dossier-after-858363ad harness install claude-code); echo "exit=$?"
Claude Code integration:
- detected
...
- Dossier is fully functional in Claude Code. Nothing further to install.

Warning: install skipped: no terminal to confirm on; re-run with --yes to apply; the stale Dossier Stop hook in /private/tmp/repro-item1-858363ad/run.uBBsv8/home/.claude/settings.json was left in place
exit=1
hook stop left: 1
afplay kept: 1
backups: 0
settings unchanged
```

Runs 3 and 4: `--yes`, then an idempotent re-run (`T=run.98fVnt` before, `T=run.VL5KsI` after):

```text
=== /tmp/dossier-before-858363ad --yes
exit=0
hook stop left: 0
afplay kept: 1
.claude.json.1792043878.bak
settings.json.1792043878.bak
settings backup == original fixture
"/tmp/dossier-before-858363ad\" hook session-start
rerun exit=0
rerun: settings unchanged
rerun backups: 2
=== /tmp/dossier-after-858363ad --yes
exit=0
hook stop left: 0
afplay kept: 1
.claude.json.1792043879.bak
settings.json.1792043879.bak
settings backup == original fixture
"/tmp/dossier-after-858363ad\" hook session-start
rerun exit=0
rerun: settings unchanged
rerun backups: 2
```

The `.claude.json` backup appears because the MCP command changed. The fixture uses `/opt/homebrew/bin/dossier` and the stable path resolved to the binary under test.

## Follow-up verified in the combined patch

The skipped-install report still had `IntegrationComplete: true` when all lifecycle
capabilities were already present, so the CLI printed "Nothing further to install"
above the skip warning. `Service.InstallHarness` now clears that report flag on
the `ErrInstallSkipped` branch; no capabilities are altered. Regression command:
`go test ./internal/core -run TestSkippedInstallDoesNotClaimIntegrationComplete -count=1`.
It failed before that change with "skipped migration must not say nothing further
to install" and passed after. The CLI output captured above predates this extra
reporting fix.

The coordinator additionally ran the following against the combined build:

```sh
T=$(mktemp -d /tmp/dossier-item1-final.XXXXXX)
mkdir -p "$T/user/.claude" "$T/store"
cp /tmp/repro-item1-858363ad/fixture-settings.json "$T/user/.claude/settings.json"
cp /tmp/repro-item1-858363ad/fixture-claude.json "$T/user/.claude.json"
printf '\n' | HOME="$T/user" DOSSIER_HOME="$T/store" /tmp/dossier-repairs-current harness install claude-code
HOME="$T/user" DOSSIER_HOME="$T/store" /tmp/dossier-repairs-current harness install claude-code --yes
rg 'hook stop|afplay' "$T/user/.claude/settings.json"
find "$T/user" -name '*.bak'
HOME="$T/user" DOSSIER_HOME="$T/store" /tmp/dossier-repairs-current harness install claude-code --yes
find "$T/user" -name '*.bak'
```

Observed temp root `/tmp/dossier-item1-final.nGSTUo`: skip exit 1, named warning,
**no misleading completion line**; confirmed install exit 0, Stop removed,
`afplay` retained, two backups total; repeated install still two backups.

## Remaining (owner decision)

- Owner decision: release a build containing `816264e` and this fix, then `brew upgrade dossier`, then run `dossier harness install claude-code` in a terminal or with `--yes`. Until then the v0.5.1 Stop hook stays configured on this machine.
