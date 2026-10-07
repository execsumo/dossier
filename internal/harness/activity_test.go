package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeSessionActivity(t *testing.T) {
	lines := []string{
		`{"type":"user","timestamp":"2026-10-07T10:00:00Z","message":{"content":"fix the bug"}}`,
		`{"type":"assistant","timestamp":"2026-10-07T10:00:05Z","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Edit"},{"type":"tool_use","name":"mcp__dossier__dossier_recall"}]}}`,
		`{"type":"user","timestamp":"2026-10-07T10:00:06Z","message":{"content":[{"type":"tool_result","content":"done"}]}}`,
		`{"type":"assistant","timestamp":"2026-10-07T10:01:00Z","message":{"content":[{"type":"tool_use","name":"mcp__dossier__dossier_save"}]}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:00Z","message":{"content":[{"type":"text","text":"<command-name>/save-dossier</command-name>"}]}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:01Z","isMeta":true,"message":{"content":"meta"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:02Z","message":{"content":"<local-command-stdout>x</local-command-stdout>"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:03Z","message":{"content":"[Request interrupted by user]"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:04Z","isCompactSummary":true,"message":{"content":"summary"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:05Z","isSidechain":true,"message":{"content":"subagent prompt"}}`,
		`{"type":"assistant","timestamp":"2026-10-07T10:03:00Z","message":{"content":[{"type":"tool_use","name":"mcp__dossier__dossier_promote"},{"type":"tool_use","name":"Bash"}]}}`,
		`not json`,
		`{"type":"attachment","timestamp":"2026-10-07T10:04:00Z","attachment":{"type":"hook_success"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:05:00Z","message":{"content":"next`, // partial last line
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}

	act, err := ClaudeSessionActivity(path)
	if err != nil {
		t.Fatalf("ClaudeSessionActivity() error = %v", err)
	}
	ts := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	assertTimes(t, "Prompts", act.Prompts, ts("2026-10-07T10:00:00Z"), ts("2026-10-07T10:02:00Z"))
	assertTimes(t, "ToolUses", act.ToolUses, ts("2026-10-07T10:00:05Z"), ts("2026-10-07T10:03:00Z"))
	assertTimes(t, "Saves", act.Saves, ts("2026-10-07T10:01:00Z"), ts("2026-10-07T10:03:00Z"))
}

func TestClaudeSessionActivityMissingFile(t *testing.T) {
	if _, err := ClaudeSessionActivity(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("expected an error for a missing transcript")
	}
}

func assertTimes(t *testing.T, label string, got []time.Time, want ...time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("%s[%d] = %v, want %v", label, i, got[i], want[i])
		}
	}
}
