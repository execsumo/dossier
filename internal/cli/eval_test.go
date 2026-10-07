package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
)

// endedSavedSession creates a Dossier, binds a session, saves through it as an
// agent would, and runs the session-end boundary with a transcript.
func endedSavedSession(t *testing.T, home string) *core.Service {
	t.Helper()
	svc, err := wire(home)
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	ctx := context.Background()
	if _, err := svc.Save(ctx, core.SaveReq{
		DistilledStateMarkdown: "# Evaluated\n\n## Situation\nStart.",
		FrontmatterUpdates:     map[string]any{"name": "Evaluated", "status": "active", "priority": "medium"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Switch(ctx, core.SwitchReq{ID: "evaluated", SessionID: "sess-cli-eval", HarnessName: "claude-code"}); err != nil {
		t.Fatalf("switch: %v", err)
	}
	recall, err := svc.Recall(ctx, core.RecallReq{ID: "evaluated"})
	if err != nil {
		t.Fatal(err)
	}
	rev := recall.Data.(core.RecallResult).Revision
	if _, err := svc.Save(ctx, core.SaveReq{ID: "evaluated", BaseRevision: rev, SessionID: "sess-cli-eval", Actor: "agent:claude",
		DistilledStateMarkdown: "# Evaluated\n\n## Situation\nSaved in session."}); err != nil {
		t.Fatalf("agent save: %v", err)
	}
	if _, err := svc.SessionEnd(ctx, "sess-cli-eval", "", "user: hi\nassistant: saved"); err != nil {
		t.Fatalf("session end: %v", err)
	}
	return svc
}

func TestStartSessionEvalSpawnsWhenDue(t *testing.T) {
	home := t.TempDir()
	svc := endedSavedSession(t, home)
	var gotArgs []string
	var gotLog string
	orig := spawnDetached
	spawnDetached = func(exe string, args []string, logPath string) error {
		gotArgs, gotLog = args, logPath
		return nil
	}
	defer func() { spawnDetached = orig }()

	var out bytes.Buffer
	startSessionEval(&out, svc, home, "sess-cli-eval")
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--home "+home) || !strings.Contains(joined, "eval run") || !strings.Contains(joined, "--session sess-cli-eval") {
		t.Fatalf("spawn args = %q", joined)
	}
	if gotLog != filepath.Join(home, "local", "evals", "eval.log") {
		t.Fatalf("log path = %q", gotLog)
	}
	if !strings.Contains(out.String(), "Session eval started") {
		t.Fatalf("output = %q", out.String())
	}

	rep, err := svc.Stats(core.StatsReq{AllAuthors: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Rows[0].Sessions != 1 || rep.Rows[0].Saves != 1 || rep.Rows[0].Version == "" {
		t.Fatalf("session_ended should be recorded with a version: %+v", rep.Rows)
	}
}

func TestStartSessionEvalHonoursKnob(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("author: tester\neval:\n  enabled: false\n"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := endedSavedSession(t, home)
	orig := spawnDetached
	spawnDetached = func(string, []string, string) error {
		t.Fatal("no eval may start when eval.enabled is false")
		return nil
	}
	defer func() { spawnDetached = orig }()
	var out bytes.Buffer
	startSessionEval(&out, svc, home, "sess-cli-eval")
	if strings.Contains(out.String(), "eval started") {
		t.Fatalf("output = %q", out.String())
	}
	// Stats still record the session: the knob gates inference, not tracking.
	rep, err := svc.Stats(core.StatsReq{AllAuthors: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Rows[0].Sessions != 1 {
		t.Fatalf("session should be tracked with evals off: %+v", rep.Rows)
	}
}

func TestRenderStats(t *testing.T) {
	rep := core.StatsReport{Rows: []core.StatsRow{{
		Version: "v0.4.0", GuideHash: "abc123def456", Sessions: 4, Unsaved: 1, Saves: 10,
		Evals: 2, EvalProbes: 10, EvalPassed: 8, EvalSkipped: 1, EvalCostUSD: 0.05,
		Last:        time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC),
		ByKind:      map[string]core.EvalKindScore{"value": {Probes: 4, Passed: 4}, "correction": {Probes: 2, Passed: 1}},
		SkipReasons: map[string]int{"transcript too large to evaluate": 1},
	}}}
	var out bytes.Buffer
	renderStats(&out, rep, core.EvalConfig{Enabled: true, Model: "haiku"})
	for _, want := range []string{"Automatic evals: on, model haiku", "v0.4.0", "25%", "2.5", "80%", "$0.05", "correction  50% (1/2)", "skipped: transcript too large to evaluate (1)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	var empty bytes.Buffer
	renderStats(&empty, core.StatsReport{}, core.EvalConfig{Model: "haiku"})
	if !strings.Contains(empty.String(), "Automatic evals: off") || !strings.Contains(empty.String(), "No sessions recorded yet") {
		t.Errorf("empty output = %q", empty.String())
	}
}

func TestEffectiveVersionPrefersReleaseTag(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()
	Version = "v9.9.9"
	if got := effectiveVersion(); got != "v9.9.9" {
		t.Fatalf("release build must report its tag, got %q", got)
	}
	Version = "dev"
	if got := effectiveVersion(); got != "dev" && !strings.HasPrefix(got, "dev+") {
		t.Fatalf("dev build must report dev or dev+<rev>, got %q", got)
	}
}
