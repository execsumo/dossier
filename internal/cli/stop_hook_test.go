package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
)

func TestRunStopHook(t *testing.T) {
	home := t.TempDir()
	svc, err := wire(home)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	ctx := context.Background()
	if _, err := svc.Save(ctx, core.SaveReq{
		DistilledStateMarkdown: "# Nudged\n\n## Situation\nStart.",
		FrontmatterUpdates:     map[string]any{"name": "Nudged", "status": "active", "priority": "medium"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Switch(ctx, core.SwitchReq{ID: "nudged", SessionID: "sess-stop", HarnessName: "claude-code"}); err != nil {
		t.Fatalf("switch: %v", err)
	}

	// Generated against core.DefaultSaveNudgeTurns rather than a fixed count,
	// so this test keeps covering "enough turns to cross the threshold"
	// whatever that default is, instead of silently under-shooting it.
	transcript := filepath.Join(t.TempDir(), "sess-stop.jsonl")
	base := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	var lines []string
	lines = append(lines, fmt.Sprintf(
		`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","name":"Edit"}]}}`,
		base.Format(time.RFC3339)))
	for i := 0; i < core.DefaultSaveNudgeTurns; i++ {
		ts := base.Add(time.Duration(i+1) * time.Minute)
		lines = append(lines, fmt.Sprintf(
			`{"type":"user","timestamp":%q,"message":{"content":"turn %d"}}`, ts.Format(time.RFC3339), i))
	}
	if err := os.WriteFile(transcript, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	runStopHook(&out, svc, "sess-stop", transcript, false)
	var decision struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &decision); err != nil {
		t.Fatalf("stop hook output is not the JSON Claude Code reads: %q (%v)", out.String(), err)
	}
	got := decision.HookSpecificOutput
	if got.HookEventName != "Stop" || !strings.Contains(got.AdditionalContext, "Nudged") {
		t.Fatalf("output = %+v, want Stop additionalContext naming the Dossier", got)
	}

	// The nudge is recorded, so the next stop with no new work lets the turn end,
	// as does a stop the hook itself caused.
	for _, active := range []bool{false, true} {
		out.Reset()
		runStopHook(&out, svc, "sess-stop", transcript, active)
		if out.Len() != 0 {
			t.Fatalf("stop (active=%v) after a nudge with no new work printed %q, want nothing", active, out.String())
		}
	}

	// An unbound session is never nudged.
	out.Reset()
	runStopHook(&out, svc, "sess-unbound", transcript, false)
	if out.Len() != 0 {
		t.Fatalf("unbound session printed %q, want nothing", out.String())
	}
}
