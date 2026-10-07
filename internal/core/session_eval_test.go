package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeEvaluator returns canned outputs in call order and records prompts.
type fakeEvaluator struct {
	outputs []string
	errs    []error
	cost    float64
	prompts []string
}

func (f *fakeEvaluator) Complete(_ context.Context, _ string, prompt string) (string, float64, error) {
	i := len(f.prompts)
	f.prompts = append(f.prompts, prompt)
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.cost, f.errs[i]
	}
	if i >= len(f.outputs) {
		return "", f.cost, errors.New("unexpected call")
	}
	return f.outputs[i], f.cost, nil
}

const (
	evalProbesOut = "Here you go:\n```json\n[" +
		`{"kind":"value","question":"What lock timeout is set?","expect":"500ms"},` +
		`{"kind":"correction","question":"May tests use mocks?","expect":"No, real Postgres"},` +
		`{"kind":"vibe","question":"Where did work stop?","expect":"PR not pushed"},` +
		`{"kind":"value","question":"","expect":"dropped: empty question"}` +
		"]\n```"
	evalAnswersOut = `{"answers":{"1":"500ms","2":"No, use the real Postgres container","3":"NOT IN STATE"}}`
	evalJudgeOut   = `{"results":{"1":{"pass":true,"reason":"exact"},"2":{"pass":true,"reason":"same"},"3":{"pass":false,"reason":"missing"}}}`
)

// newEvalHarness builds the unsaved-session harness with a versioned,
// eval-enabled service over the same store and clock.
func newEvalHarness(t *testing.T, enabled bool) (*unsavedHarness, *Service) {
	t.Helper()
	h := newUnsavedHarness(t)
	svc := NewService(h.store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, h.clock,
		Config{Author: "Alice", Version: "v1.2.3", Eval: EvalConfig{Enabled: enabled, Model: "haiku"}}, nil)
	return h, svc
}

// agentSave lands a save attributed to a session, as dossier_save does.
func (h *unsavedHarness) agentSave(session, body string) {
	h.t.Helper()
	h.tick()
	if _, err := h.svc.Save(h.ctx, SaveReq{ID: "dos_fake_id", BaseRevision: h.rev(), DistilledStateMarkdown: body, SessionID: session, Actor: "agent:claude"}); err != nil {
		h.t.Fatalf("agent save: %v", err)
	}
}

func lastAuditEvent(t *testing.T, store Store, event string) *AuditEvent {
	t.Helper()
	events, err := store.ReadAuditLog("dos_fake_id")
	if err != nil {
		t.Fatal(err)
	}
	var found *AuditEvent
	for i := range events {
		if events[i].Event == event {
			found = &events[i]
		}
	}
	return found
}

func TestSaveRecordsCallerSessionOnAudit(t *testing.T) {
	h := newUnsavedHarness(t)
	h.agentSave("sess_a", "# Recovery\n\n## Situation\nSaved.")
	e := lastAuditEvent(t, h.store, AuditEventSave)
	if e == nil || e.SessionID != "sess_a" {
		t.Fatalf("save event should carry the caller's session id, got %+v", e)
	}
	if !isAgentSave(*e) {
		t.Fatalf("a Service.Save event by an agent must count as an agent save: %+v", e)
	}
}

func TestSessionEvalDue(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		run     func(h *unsavedHarness)
		want    bool
	}{
		{"saved session with evals on", true, func(h *unsavedHarness) {
			h.bind("sess_a")
			h.agentSave("sess_a", "# Recovery\n\n## Situation\nSaved.")
			h.boundary("sess_a")
		}, true},
		{"evals off", false, func(h *unsavedHarness) {
			h.bind("sess_a")
			h.agentSave("sess_a", "# Recovery\n\n## Situation\nSaved.")
			h.boundary("sess_a")
		}, false},
		{"unsaved session", true, func(h *unsavedHarness) {
			h.bind("sess_a")
			h.boundary("sess_a")
		}, false},
		{"another session's save does not count", true, func(h *unsavedHarness) {
			h.bind("sess_a")
			h.agentSave("sess_b", "# Recovery\n\n## Situation\nOther.")
			h.boundary("sess_a")
		}, false},
		{"hook bookkeeping under an agent actor is not a save", true, func(h *unsavedHarness) {
			h.bind("sess_a")
			h.tick()
			if _, err := h.svc.SessionEndAs(h.ctx, "sess_a", "agent:rolodex", "", "transcript"); err != nil {
				h.t.Fatal(err)
			}
		}, false},
		{"unbound session", true, func(h *unsavedHarness) {}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, svc := newEvalHarness(t, tt.enabled)
			tt.run(h)
			id, due := svc.SessionEvalDue("sess_a")
			if due != tt.want {
				t.Fatalf("due = %v, want %v", due, tt.want)
			}
			if due && id != "dos_fake_id" {
				t.Fatalf("dossier id = %q", id)
			}
		})
	}
}

func TestEvaluateSessionScoresAndRecords(t *testing.T) {
	h, svc := newEvalHarness(t, true)
	h.bind("sess_a")
	h.agentSave("sess_a", "# Recovery\n\n## Situation\nLock timeout 500ms. Use real Postgres.")
	h.boundary("sess_a")
	fe := &fakeEvaluator{outputs: []string{evalProbesOut, evalAnswersOut, evalJudgeOut}, cost: 0.01}
	svc.SetEvaluator(fe)

	res, err := svc.EvaluateSession(h.ctx, "dos_fake_id", "sess_a")
	if err != nil {
		t.Fatalf("EvaluateSession: %v", err)
	}
	if res.Summary.Skipped != "" || res.Summary.Probes != 3 || res.Summary.Passed != 2 {
		t.Fatalf("summary = %+v", res.Summary)
	}
	if got := res.Summary.ByKind["value"]; got.Probes != 1 || got.Passed != 1 {
		t.Errorf("value kind = %+v", got)
	}
	if got := res.Summary.ByKind["state"]; got.Probes != 1 || got.Passed != 0 {
		t.Errorf("unknown kind should normalise to state: %+v", res.Summary.ByKind)
	}
	if res.Summary.CostUSD < 0.0299 || res.Summary.CostUSD > 0.0301 {
		t.Errorf("cost = %v, want 0.03 (three calls)", res.Summary.CostUSD)
	}
	if len(fe.prompts) != 3 || !strings.Contains(fe.prompts[0], "transcript for sess_a") || !strings.Contains(fe.prompts[1], "Lock timeout 500ms") {
		t.Fatalf("prompts did not carry the transcript then the Distilled State")
	}
	if strings.Contains(fe.prompts[1], "transcript for sess_a") {
		t.Fatalf("the resume call must see only the Distilled State, not the transcript")
	}
	if res.Probes[2].Answer != "NOT IN STATE" || res.Probes[2].Pass {
		t.Errorf("probe 3 = %+v", res.Probes[2])
	}

	e := lastAuditEvent(t, h.store, AuditEventSessionEval)
	if e == nil || e.Eval == nil {
		t.Fatal("session_eval audit event missing")
	}
	if e.Version != "v1.2.3" || e.GuideHash != svc.GuideHash() || e.SessionID != "sess_a" || e.Eval.Passed != 2 || e.Eval.Model != "haiku" {
		t.Fatalf("session_eval event = %+v / %+v", e, e.Eval)
	}
	if strings.Contains(e.Message, "500ms") {
		t.Fatalf("probe text must stay machine-local, not in the synced audit event")
	}
}

func TestEvaluateSessionRecordsSkipsAndFailures(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(h *unsavedHarness, svc *Service)
		wantSkip   string
		wantErr    bool
		transcript string
	}{
		{"no evaluator", func(h *unsavedHarness, svc *Service) {}, "no evaluator available", false, "t"},
		{"no transcript", func(h *unsavedHarness, svc *Service) {
			svc.SetEvaluator(&fakeEvaluator{})
		}, "no transcript archived", false, ""},
		{"transcript too large", func(h *unsavedHarness, svc *Service) {
			svc.SetEvaluator(&fakeEvaluator{})
		}, "transcript too large", false, strings.Repeat("x", maxEvalTranscriptBytes+1)},
		{"malformed probe output", func(h *unsavedHarness, svc *Service) {
			svc.SetEvaluator(&fakeEvaluator{outputs: []string{"I cannot comply."}})
		}, "probe extraction failed", true, "t"},
		{"model error at judge", func(h *unsavedHarness, svc *Service) {
			svc.SetEvaluator(&fakeEvaluator{outputs: []string{evalProbesOut, evalAnswersOut}, errs: []error{nil, nil, errors.New("rate limited")}})
		}, "judge failed: rate limited", true, "t"},
		{"no probes", func(h *unsavedHarness, svc *Service) {
			svc.SetEvaluator(&fakeEvaluator{outputs: []string{"[]"}})
		}, "no probes extracted", false, "t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, svc := newEvalHarness(t, true)
			h.bind("sess_a")
			h.agentSave("sess_a", "# Recovery\n\n## Situation\nSaved.")
			h.tick()
			if _, err := h.svc.SessionEnd(h.ctx, "sess_a", "", tt.transcript); err != nil {
				t.Fatal(err)
			}
			tt.setup(h, svc)
			res, err := svc.EvaluateSession(h.ctx, "dos_fake_id", "sess_a")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(res.Summary.Skipped, tt.wantSkip) {
				t.Fatalf("skipped = %q, want %q", res.Summary.Skipped, tt.wantSkip)
			}
			e := lastAuditEvent(t, h.store, AuditEventSessionEval)
			if e == nil || e.Eval == nil || !strings.Contains(e.Eval.Skipped, tt.wantSkip) {
				t.Fatalf("the skip must be recorded, got %+v", e)
			}
		})
	}
}

func TestRecordSessionEndedStampsVersionAndGuide(t *testing.T) {
	h, svc := newEvalHarness(t, true)
	h.bind("sess_a")
	if err := svc.RecordSessionEnded("sess_a"); err != nil {
		t.Fatal(err)
	}
	e := lastAuditEvent(t, h.store, AuditEventSessionEnded)
	if e == nil || e.Version != "v1.2.3" || e.GuideHash == "" || e.SessionID != "sess_a" || e.Message != "claude-code" {
		t.Fatalf("session_ended = %+v", e)
	}
	if err := svc.RecordSessionEnded("sess_unbound"); err != nil {
		t.Fatalf("an unbound session must be a no-op, got %v", err)
	}
}

func TestGuideHashTracksGuideAndInstructions(t *testing.T) {
	h, svc := newEvalHarness(t, true)
	before := svc.GuideHash()
	if len(before) != 12 {
		t.Fatalf("hash %q should be 12 hex chars", before)
	}
	h.store.contextAssets["guide.md"] = "EDITED GUIDE"
	afterGuide := svc.GuideHash()
	h.store.contextAssets["instructions.md"] = "EDITED INSTRUCTIONS"
	afterBoth := svc.GuideHash()
	if before == afterGuide || afterGuide == afterBoth {
		t.Fatalf("hash must change with either asset: %s %s %s", before, afterGuide, afterBoth)
	}
}

func TestStatsByVersion(t *testing.T) {
	h := newUnsavedHarness(t)
	mk := func(version string, author string) *Service {
		return NewService(h.store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, h.clock,
			Config{Author: author, Version: version, Eval: EvalConfig{Enabled: true, Model: "haiku"}}, nil)
	}
	v1, v2, bob := mk("v0.3.1", "Alice"), mk("v0.3.2", "Alice"), mk("v0.3.2", "Bob")

	// v0.3.1: one saved, scored session and one unsaved session.
	h.bind("s1")
	h.agentSave("s1", "# Recovery\n\n## Situation\nOne.")
	h.agentSave("s1", "# Recovery\n\n## Situation\nOne, again.")
	h.boundary("s1")
	if err := v1.RecordSessionEnded("s1"); err != nil {
		t.Fatal(err)
	}
	v1.SetEvaluator(&fakeEvaluator{outputs: []string{evalProbesOut, evalAnswersOut, evalJudgeOut}, cost: 0.02})
	if _, err := v1.EvaluateSession(h.ctx, "dos_fake_id", "s1"); err != nil {
		t.Fatal(err)
	}
	h.bind("s2")
	h.boundary("s2")
	if err := v1.RecordSessionEnded("s2"); err != nil {
		t.Fatal(err)
	}

	// v0.3.2: one saved session whose eval was skipped.
	h.bind("s3")
	h.agentSave("s3", "# Recovery\n\n## Situation\nThree.")
	h.boundary("s3")
	if err := v2.RecordSessionEnded("s3"); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.EvaluateSession(h.ctx, "dos_fake_id", "s3"); err != nil { // no evaluator: recorded skip
		t.Fatal(err)
	}

	// Bob's session on v0.3.2 is excluded unless all authors are asked for.
	h.bind("s4")
	h.boundary("s4")
	if err := bob.RecordSessionEnded("s4"); err != nil {
		t.Fatal(err)
	}

	rep, err := v2.Stats(StatsReq{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 2 || rep.Rows[0].Version != "v0.3.2" || rep.Rows[1].Version != "v0.3.1" {
		t.Fatalf("rows should be newest version first: %+v", rep.Rows)
	}
	r2, r1 := rep.Rows[0], rep.Rows[1]
	if r1.Sessions != 2 || r1.Unsaved != 1 || r1.Saves != 2 || r1.Evals != 1 || r1.EvalProbes != 3 || r1.EvalPassed != 2 {
		t.Errorf("v0.3.1 row = %+v", r1)
	}
	if r1.EvalCostUSD < 0.0599 || r1.EvalCostUSD > 0.0601 || r1.ByKind["value"].Passed != 1 {
		t.Errorf("v0.3.1 cost/kinds = %v %+v", r1.EvalCostUSD, r1.ByKind)
	}
	if r2.Sessions != 1 || r2.Unsaved != 0 || r2.Evals != 0 || r2.EvalSkipped != 1 || r2.SkipReasons["no evaluator available"] != 1 {
		t.Errorf("v0.3.2 row = %+v", r2)
	}

	all, err := v2.Stats(StatsReq{AllAuthors: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.Rows[0].Sessions != 2 || all.Rows[0].Unsaved != 1 {
		t.Errorf("all-authors v0.3.2 row should include Bob's unsaved session: %+v", all.Rows[0])
	}

	since, err := v2.Stats(StatsReq{Since: h.clock.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(since.Rows) != 0 {
		t.Errorf("Since in the future should drop every session: %+v", since.Rows)
	}
}
