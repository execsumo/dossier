#!/usr/bin/env python3
"""contextdiag: explain what filled a Claude Code session's context window.

Reads a Claude Code transcript (~/.claude/projects/<cwd>/<session>.jsonl) and
the Dossier store, and reports:

  * the context size the API actually billed on each assistant turn
    (input + cache_creation + cache_read tokens), so the baseline before any
    tool call and every later jump are measured, not estimated;
  * what entered context between turns (hook output, attachments, user
    prompts, tool results) with its size, so each jump has a named cause;
  * for a session bound to a Dossier, the Dossier's artifacts with their size
    and whether the Distilled State cites them.

Only metadata leaves the transcript: tool names, short input summaries, and
sizes. Message and tool-result content is never printed, so the report is safe
to paste from a work machine. Use --no-inputs to drop tool inputs as well.

Stdlib only; Python 3.8+.

Usage:
  contextdiag.py                    # list recent Dossier-bound sessions, peak context
  contextdiag.py <session-id|path>  # full report for one session
  contextdiag.py --latest           # full report for the newest bound session
"""

import argparse
import glob
import json
import os
import re
import sys
from datetime import datetime

BYTES_PER_TOKEN = 4  # rough estimate for sizes the transcript does not bill


def claude_dir():
    return os.environ.get("CLAUDE_CONFIG_DIR") or os.path.expanduser("~/.claude")


def dossier_home(arg):
    if arg:
        return arg
    if os.environ.get("DOSSIER_HOME"):
        return os.environ["DOSSIER_HOME"]
    cfg = os.path.expanduser("~/.dossier/config.yaml")
    try:
        with open(cfg) as f:
            for line in f:
                m = re.match(r"\s*dossier_home:\s*(.+?)\s*$", line)
                if m:
                    return os.path.expanduser(m.group(1).strip("\"'"))
    except OSError:
        pass
    return os.path.expanduser("~/.dossier")


def find_transcript(session_id):
    hits = glob.glob(os.path.join(claude_dir(), "projects", "*", session_id + ".jsonl"))
    return max(hits, key=os.path.getmtime) if hits else None


def load_binding(home, session_id):
    try:
        with open(os.path.join(home, "sessions", session_id + ".json")) as f:
            return json.load(f)
    except (OSError, ValueError):
        return None


def find_dossier_dir(home, dossier_id):
    for md in glob.glob(os.path.join(home, "*", "dossier.md")) + glob.glob(
        os.path.join(home, "archive", "*", "dossier.md")
    ):
        try:
            with open(md) as f:
                head = f.read(2000)
        except OSError:
            continue
        if re.search(r"^id:\s*%s\s*$" % re.escape(dossier_id), head, re.M):
            return os.path.dirname(md)
    return None


def size(value):
    if value is None:
        return 0
    if isinstance(value, str):
        return len(value.encode("utf-8"))
    return len(json.dumps(value, ensure_ascii=False).encode("utf-8"))


def ktok(n):
    return "%.1fk" % (n / 1000.0)


def summarize_input(name, inp, limit=110):
    if not isinstance(inp, dict):
        return ""
    keys = ("file_path", "path", "command", "query", "pattern", "url", "id",
            "artifact_id", "dossier_id", "fragment", "skill", "description", "prompt")
    parts = []
    for k in keys:
        if k in inp and inp[k] not in ("", None):
            parts.append("%s=%s" % (k, str(inp[k]).replace("\n", " ")))
    s = " ".join(parts) or ",".join(sorted(inp.keys()))
    return s if len(s) <= limit else s[: limit - 1] + "…"


def parse(path):
    """Return the ordered list of context events and per-turn usage."""
    events = []  # (kind, label, bytes)
    turns = []  # dicts: idx, context, out, events_before
    seen_msg = set()
    tool_names = {}
    pending = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                r = json.loads(line)
            except ValueError:
                continue
            if r.get("isSidechain"):
                continue
            t = r.get("type")
            if t == "attachment":
                a = r.get("attachment") or {}
                kind = a.get("type", "?")
                # Bookkeeping records that are not rendered into the prompt.
                if kind in ("total_tokens_reminder", "deferred_tools_record", "prompt_snapshot"):
                    continue
                label = a.get("hookName") or kind
                b = size(a.get("content") or a.get("rendered") or a)
                pending.append(("attachment", label, b))
            elif t == "user":
                content = (r.get("message") or {}).get("content")
                if isinstance(content, str):
                    pending.append(("user", "prompt", size(content)))
                    continue
                for c in content or []:
                    if c.get("type") == "tool_result":
                        name, inp = tool_names.get(c.get("tool_use_id"), ("?", ""))
                        pending.append(("tool_result", "%s %s" % (name, summarize_input(name, inp)), size(c.get("content"))))
                    elif c.get("type") == "text":
                        pending.append(("user", "text", size(c.get("text"))))
                    else:
                        pending.append(("user", c.get("type", "?"), size(c)))
            elif t == "assistant":
                msg = r.get("message") or {}
                for c in msg.get("content") or []:
                    if c.get("type") == "tool_use":
                        tool_names[c.get("id")] = (c.get("name"), c.get("input"))
                mid = msg.get("id")
                if mid in seen_msg:
                    continue
                seen_msg.add(mid)
                u = msg.get("usage") or {}
                ctx = (u.get("input_tokens") or 0) + (u.get("cache_creation_input_tokens") or 0) + (
                    u.get("cache_read_input_tokens") or 0
                )
                if ctx == 0:
                    continue
                turns.append({"context": ctx, "out": u.get("output_tokens") or 0,
                              "ts": r.get("timestamp", ""), "before": pending})
                events.extend(pending)
                pending = []
    return turns, events


def artifact_report(ddir):
    rows = []
    try:
        with open(os.path.join(ddir, "dossier.md"), encoding="utf-8") as f:
            body = f.read()
    except OSError:
        body = ""
    for p in sorted(glob.glob(os.path.join(ddir, "artifacts", "*"))):
        name = os.path.basename(p)
        art_id = name.split(".")[0]
        atype, lines = "?", "?"
        try:
            with open(p, encoding="utf-8", errors="replace") as f:
                head = f.read(1500)
            m = re.search(r"^type:\s*(\S+)", head, re.M)
            atype = m.group(1) if m else "?"
            m = re.search(r"^lines:\s*(\d+)", head, re.M)
            lines = m.group(1) if m else "?"
        except OSError:
            pass
        rows.append((os.path.getsize(p), art_id, atype, lines, art_id in body))
    return sorted(rows, reverse=True)


def print_session(path, home, show_inputs, turns_shown):
    session_id = os.path.splitext(os.path.basename(path))[0]
    turns, events = parse(path)
    if not show_inputs:
        for t in turns:
            t["before"] = [(k, l.split(" ", 1)[0] if k == "tool_result" else l, b) for k, l, b in t["before"]]
        events = [(k, l.split(" ", 1)[0] if k == "tool_result" else l, b) for k, l, b in events]

    print("# contextdiag report")
    print("generated:  %s" % datetime.now().isoformat(timespec="seconds"))
    print("session:    %s" % session_id)
    print("transcript: %s (%s bytes)" % (path, os.path.getsize(path)))

    binding = load_binding(home, session_id)
    ddir = None
    if binding:
        ddir = find_dossier_dir(home, binding.get("dossier_id", ""))
        print("dossier:    %s (%s) harness=%s" % (binding.get("dossier_id"), ddir or "dir not found",
                                                   binding.get("harness")))
    else:
        print("dossier:    no binding in %s/sessions" % home)

    if not turns:
        print("\nNo assistant turns with usage recorded.")
        return
    peak = max(t["context"] for t in turns)
    print("\n## Context per turn (billed tokens; Δ vs previous turn)")
    print("baseline (turn 1): %s   peak: %s   turns: %d" % (ktok(turns[0]["context"]), ktok(peak), len(turns)))
    prev = 0
    for i, t in enumerate(turns[:turns_shown], 1):
        delta = t["context"] - prev
        print("\nturn %-3d ctx=%-7s Δ=%+.1fk  %s" % (i, ktok(t["context"]), delta / 1000.0, t["ts"][11:19]))
        for kind, label, b in t["before"]:
            flag = "  <-- large" if b >= 20000 else ""
            print("    %-11s %8s B ~%-6s %s%s" % (kind, b, ktok(b / BYTES_PER_TOKEN), label, flag))
        prev = t["context"]
    if len(turns) > turns_shown:
        print("\n… %d more turns (use --turns N)" % (len(turns) - turns_shown))

    print("\n## Largest single inputs (whole session)")
    for kind, label, b in sorted(events, key=lambda e: -e[2])[:10]:
        print("  %8s B ~%-6s %-11s %s" % (b, ktok(b / BYTES_PER_TOKEN), kind, label))

    jumps = []
    prev = 0
    for i, t in enumerate(turns, 1):
        jumps.append((t["context"] - prev, i))
        prev = t["context"]
    print("\n## Biggest jumps")
    for d, i in sorted(jumps, reverse=True)[:5]:
        if d <= 0:
            break
        print("  turn %-3d +%s" % (i, ktok(d)))

    if ddir:
        print("\n## Dossier artifacts (%s)" % ddir)
        rows = artifact_report(ddir)
        if not rows:
            print("  none")
        for b, art_id, atype, lines, cited in rows:
            print("  %8s B ~%-6s %-16s %6s lines  %s  %s" % (b, ktok(b / BYTES_PER_TOKEN), atype, lines,
                                                         "cited  " if cited else "UNCITED", art_id))
        try:
            print("  dossier.md: %d B" % os.path.getsize(os.path.join(ddir, "dossier.md")))
        except OSError:
            pass

    print("\n## Context assets (%s/context)" % home)
    for p in sorted(glob.glob(os.path.join(home, "context", "*"))):
        print("  %8d B  %s" % (os.path.getsize(p), os.path.basename(p)))


def list_sessions(home, limit, quiet=False):
    rows = []
    for p in glob.glob(os.path.join(home, "sessions", "*.json")):
        sid = os.path.splitext(os.path.basename(p))[0]
        tr = find_transcript(sid)
        b = load_binding(home, sid) or {}
        if not tr:
            rows.append((0, sid, b.get("dossier_id", "?"), None, None, None))
            continue
        turns, _ = parse(tr)
        base = turns[0]["context"] if turns else 0
        peak = max((t["context"] for t in turns), default=0)
        rows.append((os.path.getmtime(tr), sid, b.get("dossier_id", "?"), base, peak, len(turns)))
    rows.sort(reverse=True)
    if quiet:
        return rows
    print("%-36s  %-30s %8s %8s %6s" % ("session", "dossier", "turn1", "peak", "turns"))
    for mt, sid, did, base, peak, n in rows[:limit]:
        if base is None:
            print("%-36s  %-30s %s" % (sid, did, "(no Claude Code transcript on this machine)"))
        else:
            print("%-36s  %-30s %8s %8s %6d" % (sid, did, ktok(base), ktok(peak), n))
    return rows


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("session", nargs="?", help="session id or path to a transcript .jsonl")
    ap.add_argument("--latest", action="store_true", help="report on the newest Dossier-bound session")
    ap.add_argument("--home", help="Dossier home (default: $DOSSIER_HOME or ~/.dossier)")
    ap.add_argument("--turns", type=int, default=15, help="turns to detail (default 15)")
    ap.add_argument("--limit", type=int, default=20, help="sessions to list (default 20)")
    ap.add_argument("--no-inputs", action="store_true", help="omit tool inputs (paths, commands) from the report")
    args = ap.parse_args()
    home = dossier_home(args.home)

    if args.session:
        path = args.session if os.path.isfile(args.session) else find_transcript(args.session)
        if not path:
            sys.exit("no transcript for %s under %s/projects" % (args.session, claude_dir()))
        print_session(path, home, not args.no_inputs, args.turns)
    elif args.latest:
        rows = [r for r in list_sessions(home, 0, quiet=True) if r[3] is not None]
        if not rows:
            sys.exit("no Dossier-bound session has a transcript on this machine")
        print_session(find_transcript(rows[0][1]), home, not args.no_inputs, args.turns)
    else:
        list_sessions(home, args.limit)
        print("\nRun with a session id (or --latest) for the per-turn breakdown.")


if __name__ == "__main__":
    main()
