# resumeeval: resumption-fidelity eval

A standalone harness (not part of the `dossier` binary) that answers one question: **does a Distillation Guide produce Distilled States that let a fresh agent resume without losing facts?** It runs two guide versions over the same cases and reports which facts each one kept.

## What it measures

Per case, guide variant and run:

1. **Distill.** One model call receives the guide, the operating instructions (`assets/instructions.md`) and the transcript, and returns a Distilled State Markdown body (saved under `<out>/states/`).
2. **Resume.** A fresh model call receives ONLY that Distilled State plus the probe questions, and returns JSON answers keyed by probe id.
3. **Score.** Each probe is scored:
   - `verbatim`: `expect` must appear exactly (case-sensitive substring) in the distilled state AND in the resumed agent's answer. A fact in the state that the answer misses fails; so does a fact the answer knows that the state lost.
   - `judge`: an LLM judge decides whether the answer satisfies `expect`, returning pass/fail and a one-line reason. Malformed judge output counts as a failure and is flagged as an errored probe (a pipeline problem, not a lost fact).

Transcripts and probes are never shown to the resumed agent. Only the Distilled State is.

## Case format

One directory per case under the `--cases` dir:

```
cases/
  my-case/
    transcript.md      # or transcript.jsonl (raw Claude Code JSONL; compiled with core.CompileTranscript)
    probes.yaml
```

Exactly one of `transcript.md` / `transcript.jsonl` must exist. `probes.yaml` is a list; unknown fields are rejected.

| field | values |
| --- | --- |
| `id` | unique within the case |
| `kind` | `value`, `correction`, `rejected`, `decision`, `assumption`, `state` |
| `question` | what the resumed agent is asked |
| `expect` | verbatim: the exact text. judge: the requirement the answer must satisfy |
| `match` | `verbatim` or `judge` |

Full example (this is a trimmed version of `testdata/cases/sample-billing`):

```yaml
- id: lock-timeout
  kind: value
  question: What lock timeout is configured for the batch claim?
  expect: 500ms
  match: verbatim

- id: merge-scope
  kind: decision
  question: May you merge to main without asking the user?
  expect: Only for diffs under 50 lines with green CI; otherwise (larger diffs, migrations) it needs the user's review.
  match: judge

- id: redis-lock
  kind: rejected
  question: Should we try a Redis lock for the batch claim?
  expect: No. Redis was rejected because it adds about 40ms p99 latency per claim.
  match: judge
```

## Running

Always give explicit git refs so both guides come from known commits. `git:<ref>:<path>` is resolved with `git show` (run in `--repo`, default `.`). With a `git:` guide, instructions default to `assets/instructions.md` at the same ref; override with `--instructions-a/-b`. File paths work too.

```bash
# Plan only: prints variants, probe counts and the estimated number of model calls
go run ./tools/resumeeval --cases tools/resumeeval/testdata/cases \
  --guide-a 'git:b2a4f73^:assets/guide.md' --guide-b 'git:b2a4f73:assets/guide.md' --dry-run

# Cheap smoke run
go run ./tools/resumeeval --cases tools/resumeeval/testdata/cases \
  --guide-a 'git:b2a4f73^:assets/guide.md' --guide-b 'git:b2a4f73:assets/guide.md' \
  --runs 1 --model haiku --out /tmp/resumeeval-out

# Real comparison (default 3 runs for variance), stronger model, cheaper judge
go run ./tools/resumeeval --cases ~/dossier-eval-cases \
  --guide-a 'git:b2a4f73^:assets/guide.md' --guide-b 'git:b2a4f73:assets/guide.md' \
  --model sonnet --judge-model haiku --out ./eval-out
```

Flags: `--cases`, `--guide-a`, `--guide-b`, `--instructions-a`, `--instructions-b`, `--runs` (default 3), `--model`, `--judge-model` (default: `--model`), `--out`, `--bin` (claude binary), `--repo`, `--bare`, `--dry-run`.

Outputs in `--out`:

- `results.json`: every probe result per case/variant/run (answer, pass, reason, in-state/in-answer for verbatim), plus cost and the variant specs.
- `report.md`: per-variant pass rates overall and by probe kind, per-case breakdown, run-to-run spread (min/max/mean/population std dev of per-run pass rates), and the probes whose pass rate differs between variants.
- `states/<case>__<variant>__run<N>.md`: each distilled state, for reading by hand.

## Isolation

The user's Claude Code carries Dossier SessionStart hooks (which inject a library and the guide), a Dossier MCP server and global CLAUDE.md files. Any of those would contaminate the measurement, so every model call runs as:

```
claude -p --output-format json --no-session-persistence \
  --strict-mcp-config --mcp-config '{"mcpServers":{}}' \
  --settings '{"disableAllHooks":true}' --setting-sources "" \
  --tools "" --disable-slash-commands \
  --system-prompt "You are a careful assistant. ..." [--model M]
```

with the prompt on stdin, cwd set to an empty temp dir, `DOSSIER_HOME` set to an empty temp dir, and `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` and `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` in the environment.

| flag | why |
| --- | --- |
| `--strict-mcp-config` + empty `--mcp-config` | no MCP servers (Dossier's or claude.ai connectors) |
| `--settings {"disableAllHooks":true}` | no hooks (SessionStart injection) |
| `--setting-sources ""` | load no user/project/local settings files |
| `--tools ""` | no tools; the model can only answer |
| `--disable-slash-commands` | no skills |
| `--system-prompt` | replaces the default prompt (no memory instructions, no environment-specific sections beyond what the CLI always adds) |
| `--no-session-persistence` | nothing written to or resumed from disk |
| env vars | disable auto-memory and CLAUDE.md discovery; empty `DOSSIER_HOME` and cwd |

`--bare` would be the strictest option, but it only authenticates via `ANTHROPIC_API_KEY`/`apiKeyHelper`, not OAuth login, so it is opt-in (`--bare`) and off by default.

Empirical check, same probe prompt ("list every system reminder, hook output, CLAUDE.md content, memory, MCP server, tool name or mention of Dossier you can see"), haiku, throwaway cwd:

- Plain `claude -p` (no isolation): the model reported the Dossier SessionStart library (named open dossiers), MCP servers and hook output, and the global CLAUDE.md (CodeGraph, subagent routing).
- With the flags above: hooks NONE, CLAUDE.md NONE, memory NONE, MCP NONE, Dossier NONE. Residual, harmless to this measurement: the CLI's own environment block (cwd, platform, date, model name) and the account email line, which cannot be removed without `--bare`.

Re-verify after upgrading Claude Code; flag behaviour can change.

## Collecting a real case

1. Pick a finished or well-advanced session where facts accumulated: values, corrections, a rejected path, an approval with limits.
2. Find the transcript: Claude Code stores sessions as JSONL at `~/.claude/projects/<project-path-with-slashes-as-dashes>/<session-id>.jsonl`. For `/home/me/projects/foo` that is `~/.claude/projects/-home-me-projects-foo/`. Sort by mtime to find the one you want.
3. Copy it to `cases/<name>/transcript.jsonl` (or compile it to Markdown yourself and use `transcript.md`).
4. **Strip secrets** before it leaves your machine: API keys, tokens, `.env` contents, passwords, customer data, private URLs, emails. Search for `sk-`, `ghp_`, `AKIA`, `Bearer`, `password`, `secret`. Replace with obvious placeholders; do not alter the facts you intend to probe. Tool results (file reads, command output) are the usual leak.
5. Write `probes.yaml` from the transcript, not from memory.

## Writing good probes

Aim for 8 to 20 probes per case, mixing kinds:

- **value**: exact numbers, names, versions, paths, ids. Use `verbatim` and put the exact token in `expect`, so the state has to carry it and the agent has to quote it.
- **correction**: where the user overrode an earlier claim. Ask for the current value and (judge) require that it replaces the old one. A distillation that keeps the pre-correction value is the classic failure.
- **decision / approval scope**: what the agent may do without asking, and the limits (size, CI, who must review). Judge probes; put the limit in `expect`.
- **rejected**: alternatives that were ruled out and why. Ask "should we try X?" and require "no, because <reason>".
- **assumption**: unverified beliefs and who must confirm them.
- **state**: in-flight status: what is merged, what is pending, the next action.

Tips: each probe tests one fact; keep `expect` to the core fact, since a judge penalises answers for omitting extra clauses you wrote into a long `expect`; make questions answerable from the transcript alone; ask questions a resuming agent would actually need answered; prefer `verbatim` when an exact token exists, `judge` for meaning.

## Cost

Calls per case/variant/run = 2 (distill + resume) + one per `judge` probe. `--dry-run` prints the total. The distill call carries the whole transcript plus the guide (~7k tokens of guide and instructions), so long transcripts dominate cost. The sample case with `--runs 1 --model haiku` over two variants (16 calls) cost about $0.22 as reported by the CLI. Use a cheap `--judge-model`, and `--runs 1` while iterating.

## Reading results

Variance is real: a single run on one small case flips probes between identical guides. Use `--runs 3` or more and several cases before concluding that one guide is better; look at the run-to-run spread and the "probes that differ" table, and read the saved states for the probes that flipped.
