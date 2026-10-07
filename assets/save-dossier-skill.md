---
name: save-dossier
description: "Save this session's work into its bound Dossier's Distilled State. Use when the user invokes /save-dossier, or asks to save the Dossier before clearing or exiting the session."
---

# /save-dossier — save before you /clear

The session-end hooks archive the transcript but cannot distill it: only you
can, and only while this conversation is still in context. This is the user's
"save" before they `/clear` or exit. Make it complete enough that a fresh
session holding only the Distilled State can take the next step.

Text after `/save-dossier` is a note from the user. Fold it into the save.

## Steps

1. **Find the Dossier.** Use the Dossier this session is bound to (call
   `dossier_session` with no arguments if you are unsure). If none is bound,
   say so and ask which Dossier to save to (`dossier_list`). Never guess, and
   never create one here; `/spark` does that.
2. **Read the current state.** Call `dossier_recall` for the Dossier to get
   its current Distilled State and `revision`. Save against that revision.
3. **Update in place**, following the Distillation Guide. Change what this
   session changed and keep the rest of the existing curation:
   - `## Current State`, with today's date: what is done, what is in progress,
     and where things stand (branch, commit, test status, open PR) when that
     applies.
   - The user's corrections, preferences and approval scope from this session,
     as working agreements under Current State.
   - `## Next Steps`, with the first item being the next action.
   - Findings, open questions, and every file you created under `## Files`.
   - Citations to artifacts for claims that came from them.
4. **Save** with `dossier_save`: pass `base_revision`, the updated status, and
   a concise `next_action`.
   - A change to Objective, Done When, Validation, Constraints or Decisions
     goes in its own separate save; it is held for the user's review. Tell the
     user it is waiting.
   - If the save reports a conflict, stop and show it to the user. Do not
     retry blindly.
5. **Report** in one or two lines: what you saved and the new revision, then
   tell the user it is safe to `/clear`.

If nothing material changed since the last save, say so and do not save.

