package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	transcript := filepath.Join(t.TempDir(), "sess-stop.jsonl")
	lines := []string{
		`{"type":"user","timestamp":"2026-10-07T10:00:00Z","message":{"content":"one"}}`,
		`{"type":"assistant","timestamp":"2026-10-07T10:00:01Z","message":{"content":[{"type":"tool_use","name":"Edit"}]}}`,
		`{"type":"user","timestamp":"2026-10-07T10:01:00Z","message":{"content":"two"}}`,
		`{"type":"user","timestamp":"2026-10-07T10:02:00Z","message":{"content":"three"}}`,
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
