# contextdiag

Explains what filled a Claude Code session's context window. Dev-only, not in the binary; a single stdlib Python 3.8+ file so it can be copied to any machine.

```bash
python3 contextdiag.py                 # recent Dossier-bound sessions: turn-1 and peak context
python3 contextdiag.py --latest        # per-turn breakdown of the newest bound session
python3 contextdiag.py <session-id>    # ...or of a given session (id or .jsonl path)
python3 contextdiag.py <id> --no-inputs --turns 30 > report.txt
```

How it measures: every assistant record in `~/.claude/projects/<cwd>/<session>.jsonl` carries the API `usage`, so `input + cache_creation + cache_read` is the context actually billed on that turn. The report lists each turn's context and delta alongside what entered between turns (hook output, attachments, prompts, tool results with byte sizes), the largest single inputs, the biggest jumps, and — for a session bound to a Dossier — that Dossier's artifacts by size with cited/UNCITED status.

Privacy: no message or tool-result content is printed, only sizes, tool names and short input summaries (paths, commands, ids). `--no-inputs` drops the summaries too.

Honours `CLAUDE_CONFIG_DIR`, `DOSSIER_HOME` and `--home`. Byte-to-token figures (`~Nk`) are a /4 estimate; the per-turn `ctx` and `Δ` figures are exact.
