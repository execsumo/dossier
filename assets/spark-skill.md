---
name: spark
description: "Quickly capture an unstructured thought as a new Dossier in the spark stage. Use only when the user invokes /spark or explicitly asks to capture a new topic."
---

# /spark — capture a new Dossier quickly

This is an inbox capture workflow, not a planning or distillation workflow. The
user is deliberately allowed to ramble. Do not turn the capture into a
structured plan or ask for priority, lead, due date, or next action.

## Input

The text after `/spark` is the raw capture. Preserve it exactly, including its
wording and paragraph breaks. If the command has no text, ask the user for the
thought they want to capture, then treat their reply as the raw capture.

Derive a short, descriptive Dossier name from the capture (roughly 2–7 words).
Do not invent details in the name. If no useful name can be inferred, use a
plain fallback such as `New spark` and tell the user what name was chosen.

## Create

Use the existing Dossier promote flow with:

- the derived name;
- the raw capture as `distilled_state_markdown`, unchanged;
- no lead, interfaces, due date, or next action;
- no explicit priority (new Dossiers default to `medium`);
- `force: false` on the first attempt.

In Claude Code, use the `dossier_promote` MCP tool. In Pi, where Dossier MCP
is not built in, use the installed `dossier` CLI. Prefer a temporary file and
`dossier promote <name> --distilled-file <path>` so multiline text and shell
characters remain byte-for-byte safe; remove the temporary file afterward.

Never bypass the existing duplicate check. If likely matching Dossiers are
returned, show them and ask whether this should be added to an existing topic
or created as a new one. Only retry with `force: true` after the user clearly
confirms it is a new topic.

Do not switch the current session's active Dossier automatically. After a
successful creation, report the new name, slug/ID, and confirm that it was
created in `spark` with `medium` priority.
