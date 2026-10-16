# Item 5: SessionEnd warning visibility (investigation only)

No hook behavior was changed and no live Claude session was launched. The capability document and handoff were updated to distinguish the documented output contract from live verification.

## Repository facts

At baseline HEAD `7354587`, `internal/cli/cli.go:1824-1836` handles `SessionEndAs` results. It prints returned warnings using `fmt.Printf("Warning: %s\n", w)` to **stdout**, then `Session hook completed successfully.`, and exits successfully. Error paths print to stdout too, then exit 1. The same branch handles pre-compaction.

`internal/core/service_session.go:578-589` derives the unsaved-session warning and writes the `distilled_state_not_captured` audit event. The audit survives independently of UI visibility. The automatic sync path has a five-second budget (`:597-606`) and explicitly acknowledges hook output may be invisible. The Claude installer registers a command hook without an explicit timeout (`internal/harness/claudecode.go:167-172,321`). Line references in this paragraph are baseline references; archive cleanup shifts the core lines in this patch.

## Official documented behavior

Source fetched 2026-10-14: <https://code.claude.com/docs/en/hooks.md>, general exit-code rules and SessionEnd tables:

> For most events, Claude Code writes stdout to the debug log and doesn't show it in the transcript. The exceptions are `UserPromptSubmit`, `UserPromptExpansion`, `SessionStart`, and `PostModelSwitch`, where Claude Code adds plain-text stdout as context that Claude can see and act on.

SessionEnd is not an exception: Dossier's successful stdout warning is documented as **debug-log-only**, not a user-visible warning or model context. This resolves the documentation question at the contract level, not by a live observation. SessionEnd also has no decision control and discards JSON output fields such as `systemMessage`.

The reference additionally states SessionEnd hooks share a **1.5-second budget**, raised by an explicit per-hook timeout up to 60 seconds. The exit-2 table says SessionEnd cannot block and "Shows stderr to user only". Dossier does not use that output path for this warning.

## Inferences and remaining live verification

- **Inferred:** PreCompact's successful stdout warnings are also debug-log-only, because it is not in the exception list.
- **Inferred:** errors printed to stdout with exit 1 may yield a generic hook-error notice without a useful stderr explanation.
- **Possible risk, unmeasured:** transcript capture, disk writes, and sync can exceed SessionEnd's default budget. Nothing in this investigation establishes an actual timeout occurred.
- Live-test exit, `/clear`, and interactive `/resume` in an isolated home with a bound unsaved session, debug logging enabled, and inference/evals disabled. Inspect both visible UI and debug logs for the warning and audit event. Separately measure slow/team-sync cases for cancellation and whether the ending UI renders nonzero-hook notices. This was not run because the item asks for investigation without behavior changes.
- Pi's successful-hook stdout presentation is a separate live question.

The existing `docs/harness-capabilities.md` wording about an immediate visible warning overstates the documented Claude Code behavior; its stale CLI line reference points to an older file layout. The next-session unsaved notice is separate and is not disproved by this finding.
