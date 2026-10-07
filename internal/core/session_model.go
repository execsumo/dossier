package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ModelUsage is how many main-thread assistant turns ran on one model at one
// effort level.
type ModelUsage struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
	Turns  int    `json:"turns"`
}

// TranscriptModelUsage reads, from a raw Claude Code JSONL transcript, the
// model and reasoning effort of every main-thread assistant turn, most-used
// first. The session's model is the user's choice, outside Dossier's control,
// and it can drive eval scores as much as the Guide does, so stats need it as
// a dimension (ADR 0016).
//
// The transcript is the only reliable source: $CLAUDE_EFFORT in a
// SessionEnd hook reports the configured default, not the level the session
// ran (verified 2026-10-07; docs/harness-capabilities.md). Subagent
// (sidechain) turns and synthetic messages are skipped. A transcript in
// another harness's format yields nothing.
func TranscriptModelUsage(raw string) []ModelUsage {
	counts := map[[2]string]int{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"assistant"`) {
			continue
		}
		var rec struct {
			Type          string `json:"type"`
			IsSidechain   bool   `json:"isSidechain"`
			Effort        string `json:"effort"`
			PerTurnEffort string `json:"perTurnEffort"`
			Message       struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Type != "assistant" || rec.IsSidechain {
			continue
		}
		model := strings.TrimSpace(rec.Message.Model)
		if model == "" || strings.HasPrefix(model, "<") {
			continue
		}
		effort := rec.PerTurnEffort
		if effort == "" {
			effort = rec.Effort
		}
		counts[[2]string{model, effort}]++
	}
	out := make([]ModelUsage, 0, len(counts))
	for k, n := range counts {
		out = append(out, ModelUsage{Model: k[0], Effort: k[1], Turns: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Turns != out[j].Turns {
			return out[i].Turns > out[j].Turns
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Effort < out[j].Effort
	})
	return out
}

// formatModelMix renders usage as "model/effort:turns, ..." for the audit
// trail, so a mixed session stays visible behind its dominant model.
func formatModelMix(usage []ModelUsage) string {
	parts := make([]string, 0, len(usage))
	for _, u := range usage {
		label := u.Model
		if u.Effort != "" {
			label += "/" + u.Effort
		}
		parts = append(parts, fmt.Sprintf("%s:%d", label, u.Turns))
	}
	return strings.Join(parts, ", ")
}
