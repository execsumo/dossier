package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"dossier/internal/core"
	"dossier/internal/harness"
)

// runStopHook handles Claude Code's Stop event: when enough work has gone
// unsaved, it hands the agent one more turn with an instruction to save first.
// It answers with hookSpecificOutput.additionalContext, the Stop output
// verified to make the model read the text and continue
// (docs/harness-capabilities.md §3); the follow-up Stop arrives with
// stop_hook_active set and ends the turn. Empty output lets the turn end.
//
// A failure here must never wedge a session, so errors let the turn end and are
// reported on stderr, which Claude Code shows to the user rather than the agent.
func runStopHook(out io.Writer, svc *core.Service, sessionID, transcriptPath string, stopHookActive bool) {
	// Claude Code sends transcript_path on Stop. Without it there is nothing to
	// count, and searching ~/.claude/projects after every turn is too costly.
	if stopHookActive || sessionID == "" || transcriptPath == "" {
		return
	}
	// Most sessions are not bound to a Dossier; don't parse their transcripts.
	if svc.SessionDossiers([]string{sessionID})[sessionID] == "" {
		return
	}
	activity, err := harness.ClaudeSessionActivity(transcriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dossier: save checkpoint skipped: %v\n", err)
		return
	}
	reason, err := svc.SaveNudge(context.Background(), core.SaveNudgeReq{
		SessionID:      sessionID,
		StopHookActive: stopHookActive,
		Activity:       activity,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dossier: save checkpoint skipped: %v\n", err)
		return
	}
	if reason == "" {
		return
	}
	_ = json.NewEncoder(out).Encode(map[string]any{
		"hookSpecificOutput": map[string]string{
			"hookEventName":     "Stop",
			"additionalContext": reason,
		},
	})
}
