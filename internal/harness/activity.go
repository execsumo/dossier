package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"dossier/internal/core"
)

// dossierToolPrefix marks Dossier's own MCP tools in a Claude Code transcript.
// Calling them is not work on the Dossier's outcome, so they do not count
// toward the save nudge; dossier_save and dossier_promote are the saves.
const dossierToolPrefix = "mcp__dossier__"

// ClaudeSessionActivity reduces a Claude Code transcript (JSONL) to the
// timestamps core.Service.SaveNudge decides on. Sidechain (subagent) records,
// harness meta records, slash-command echoes and compaction summaries are not
// user turns. Lines that do not parse are skipped: the transcript is
// append-only and a partially written last line is normal while a session runs.
func ClaudeSessionActivity(path string) (core.SessionActivity, error) {
	f, err := os.Open(path)
	if err != nil {
		return core.SessionActivity{}, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var act core.SessionActivity
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var rec struct {
			Type             string    `json:"type"`
			Timestamp        time.Time `json:"timestamp"`
			IsSidechain      bool      `json:"isSidechain"`
			IsMeta           bool      `json:"isMeta"`
			IsCompactSummary bool      `json:"isCompactSummary"`
			Message          struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.IsSidechain || rec.Timestamp.IsZero() {
			continue
		}
		switch rec.Type {
		case "user":
			if rec.IsMeta || rec.IsCompactSummary {
				continue
			}
			if text, ok := userPromptText(rec.Message.Content); ok && !harnessEcho(text) {
				act.Prompts = append(act.Prompts, rec.Timestamp)
			}
		case "assistant":
			var blocks []struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			if json.Unmarshal(rec.Message.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type != "tool_use" {
					continue
				}
				switch {
				case b.Name == dossierToolPrefix+"dossier_save" || b.Name == dossierToolPrefix+"dossier_promote":
					act.Saves = append(act.Saves, rec.Timestamp)
				case strings.HasPrefix(b.Name, dossierToolPrefix):
				default:
					act.ToolUses = append(act.ToolUses, rec.Timestamp)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return act, fmt.Errorf("read transcript: %w", err)
	}
	return act, nil
}

// userPromptText returns the text of a user record that the person typed: a
// plain string, or text blocks. Records that only carry tool results are not
// prompts.
func userPromptText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n"), len(parts) > 0
}

// harnessEcho reports user records Claude Code writes on the user's behalf:
// local command output and interruption markers. A slash command the user
// typed (<command-name>) is a real turn and is kept.
func harnessEcho(text string) bool {
	t := strings.TrimSpace(text)
	return t == "" ||
		strings.HasPrefix(t, "<local-command-") ||
		strings.HasPrefix(t, "[Request interrupted")
}
