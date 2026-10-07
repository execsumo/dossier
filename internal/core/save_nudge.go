package core

import (
	"context"
	"fmt"
	"time"
)

// DefaultSaveNudgeTurns is how many user turns may pass without a save before
// the Stop hook asks the agent to save. Zero in config disables the nudge.
const DefaultSaveNudgeTurns = 3

// SessionActivity is what a harness transcript says happened in a session,
// reduced to timestamps. Reading the transcript is the harness adapter's job;
// deciding what it means is this package's.
type SessionActivity struct {
	// Prompts are the user's turns (not tool results or harness meta records).
	Prompts []time.Time
	// ToolUses are the agent's tool calls, excluding Dossier's own tools.
	ToolUses []time.Time
	// Saves are calls that write the Distilled State (dossier_save, dossier_promote).
	Saves []time.Time
}

// SaveNudgeReq is one Stop-hook check.
type SaveNudgeReq struct {
	SessionID string
	// StopHookActive is set by the harness when the agent is already
	// continuing because of a Stop hook; the nudge never fires twice in a row.
	// (Claude Code also caps consecutive continuations, but a hook must check.)
	StopHookActive bool
	Activity       SessionActivity
}

// SaveNudge decides whether the agent should save before it hands the turn
// back. It returns the instruction to give the agent, or "" to let it stop.
//
// The Stop hook is the only boundary where the agent still holds the
// session's context, so it is the one place a save can be asked for and still
// be distilled; SessionEnd and PreCompact run after that context is gone. The
// nudge fires once enough user turns with real work (tool calls other than
// Dossier's own) have passed since the later of the last save and the last
// nudge, and records itself so an agent that decides nothing material changed
// is not asked again until more work accumulates.
func (s *Service) SaveNudge(ctx context.Context, req SaveNudgeReq) (string, error) {
	threshold := s.cfg.SaveNudgeTurns
	if threshold <= 0 || req.StopHookActive || req.SessionID == "" {
		return "", nil
	}
	binding, err := s.store.GetSessionBinding(req.SessionID)
	if err != nil || binding == nil || binding.DossierID == "" {
		return "", nil
	}

	cutoff := binding.SaveNudgedAt
	for _, ts := range req.Activity.Saves {
		if ts.After(cutoff) {
			cutoff = ts
		}
	}
	turns := countAfter(req.Activity.Prompts, cutoff)
	if turns < threshold || countAfter(req.Activity.ToolUses, cutoff) == 0 {
		return "", nil
	}

	name := binding.DossierID
	if d, _, err := s.store.Read(binding.DossierID); err == nil && d != nil && d.Frontmatter.Name != "" {
		name = d.Frontmatter.Name
	}

	binding.SaveNudgedAt = s.clock.Now()
	if err := s.store.SaveSessionBinding(binding); err != nil {
		return "", fmt.Errorf("record save nudge: %w", err)
	}

	plural := "s"
	if turns == 1 {
		plural = ""
	}
	return fmt.Sprintf(
		"Dossier checkpoint: %d turn%s of work on %q since its Distilled State was last saved. "+
			"Before you finish, save what this work established to the Dossier with dossier_save, passing base_revision: "+
			"Current State with today's date, Next Steps, and any decisions, findings, facts the user stated, files changed, "+
			"or corrections and preferences the user gave. Anything a fresh session would need counts as material; "+
			"other notes or memory files do not replace the Dossier. Only if this work established nothing of that kind, "+
			"say so in one line and finish. This checkpoint will not repeat until more work accumulates.",
		turns, plural, name), nil
}

func countAfter(times []time.Time, cutoff time.Time) int {
	n := 0
	for _, ts := range times {
		if ts.After(cutoff) {
			n++
		}
	}
	return n
}
