package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Automatic session evals (ADR 0016) measure, for real everyday sessions,
// whether the Distilled State a session left behind lets a fresh agent recover
// what that session established. Three model calls per session: extract probes
// from the session's transcript, answer them from the current Distilled State
// alone, and judge the answers. Counts are recorded as a synced session_eval
// audit event stamped with the binary version and guide hash; probe text stays
// machine-local (written by the adapter from EvalResult).

const (
	// maxEvalProbes bounds the probes taken from one extraction.
	maxEvalProbes = 8
	// maxEvalTranscriptBytes keeps the extraction prompt inside a small
	// model's context. Larger sessions are skipped visibly, never truncated:
	// a truncated transcript would score only the part that survived.
	maxEvalTranscriptBytes = 400_000
	// evalActor attributes eval audit events.
	evalActor = "system:eval"
)

// EvalProbeKinds are the probe categories, mirroring what the Distillation
// Guide says a resuming agent needs.
var EvalProbeKinds = []string{"value", "correction", "approval", "decision", "rejected", "assumption", "state"}

// EvalProbeResult is one probe with its outcome. Machine-local detail.
type EvalProbeResult struct {
	Kind     string `json:"kind"`
	Question string `json:"question"`
	Expect   string `json:"expect"`
	Answer   string `json:"answer"`
	Pass     bool   `json:"pass"`
	Reason   string `json:"reason,omitempty"`
}

// EvalResult is everything one session eval produced.
type EvalResult struct {
	DossierID    string            `json:"dossier_id"`
	SessionID    string            `json:"session_id"`
	TranscriptID string            `json:"transcript_id,omitempty"`
	Version      string            `json:"version"`
	GuideHash    string            `json:"guide_hash"`
	Summary      EvalSummary       `json:"summary"`
	Probes       []EvalProbeResult `json:"probes,omitempty"`
}

// SetEvaluator wires the model-call adapter for session evals. Without one,
// EvaluateSession reports that evals are unavailable.
func (s *Service) SetEvaluator(e Evaluator) {
	s.eval = e
}

// EvalConfig returns the configured eval knob.
func (s *Service) EvalConfig() EvalConfig {
	return s.cfg.Eval
}

// GuideHash identifies the Distillation Guide and Operating Instructions in
// force: the on-disk copies win over the embedded ones (a deliberate local
// edit is honoured), so the version alone does not say which rules an agent
// followed.
func (s *Service) GuideHash() string {
	h := sha256.Sum256([]byte(s.GetGuide() + "\x00" + s.GetInstructions()))
	return hex.EncodeToString(h[:])[:12]
}

// RecordSessionEnded appends the session_ended audit event that anchors
// per-version session metrics. Called at the true end of a session only (not
// at pre-compaction). A session with no binding has no Dossier to record in.
func (s *Service) RecordSessionEnded(sessionID string) error {
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil || binding.DossierID == "" {
		return nil
	}
	return s.store.AppendAudit(binding.DossierID, AuditEvent{
		TS:        s.clock.Now(),
		Event:     AuditEventSessionEnded,
		DossierID: binding.DossierID,
		Actor:     "system:session-end",
		Author:    s.cfg.Author,
		SessionID: sessionID,
		Version:   s.cfg.Version,
		GuideHash: s.GuideHash(),
		Message:   binding.Harness,
	})
}

// SessionEvalDue reports whether an automatic eval should run for a session
// that just ended: evals are enabled, the session is bound, and it saved
// something during the session. An unsaved session has nothing to score; the
// unsaved rate already measures it.
func (s *Service) SessionEvalDue(sessionID string) (dossierID string, due bool) {
	if !s.cfg.Eval.Enabled || sessionID == "" {
		return "", false
	}
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil || binding.DossierID == "" {
		return "", false
	}
	events, err := s.store.ReadAuditLog(binding.DossierID)
	if err != nil {
		return "", false
	}
	for _, e := range events {
		if e.SessionID == sessionID && isAgentSave(e) {
			return binding.DossierID, true
		}
	}
	return "", false
}

// isAgentSave reports whether an audit event is a save made during a session
// by its agent (or person). Every event Service.Save writes carries the body's
// TokenEstimate; the bookkeeping events SessionEndAs appends directly (the
// transcript archive, warnings) never do, and they may carry an agent actor
// when DOSSIER_AGENT is set, so the actor alone cannot tell them apart.
func isAgentSave(e AuditEvent) bool {
	if e.Event != AuditEventSave && e.Event != AuditEventStatusChanged {
		return false
	}
	return e.TokenEstimate > 0 && !strings.HasPrefix(e.Actor, "system:")
}

// EvaluateSession runs one session eval and records it. Every outcome,
// including a skip or a failure partway, is recorded as a session_eval event
// so coverage gaps show up in stats instead of disappearing.
func (s *Service) EvaluateSession(ctx context.Context, dossierID, sessionID string) (EvalResult, error) {
	d, rev, err := s.store.Read(dossierID)
	if err != nil {
		return EvalResult{}, err
	}
	dossierID = d.Frontmatter.ID
	res := EvalResult{
		DossierID: dossierID,
		SessionID: sessionID,
		Version:   s.cfg.Version,
		GuideHash: s.GuideHash(),
		Summary:   EvalSummary{Model: s.cfg.Eval.Model, Revision: string(rev)},
	}
	record := func() {
		summary := res.Summary
		_ = s.store.AppendAudit(dossierID, AuditEvent{
			TS:        s.clock.Now(),
			Event:     AuditEventSessionEval,
			DossierID: dossierID,
			Actor:     evalActor,
			Author:    s.cfg.Author,
			SessionID: sessionID,
			Version:   res.Version,
			GuideHash: res.GuideHash,
			Eval:      &summary,
		})
	}
	skip := func(reason string) (EvalResult, error) {
		res.Summary.Skipped = reason
		record()
		return res, nil
	}
	fail := func(stage string, err error) (EvalResult, error) {
		res.Summary.Skipped = fmt.Sprintf("%s failed: %v", stage, err)
		record()
		return res, fmt.Errorf("session eval %s: %w", stage, err)
	}

	if s.eval == nil {
		return skip("no evaluator available")
	}
	transcriptID, err := s.sessionTranscriptID(dossierID, sessionID)
	if err != nil {
		return fail("reading audit log", err)
	}
	if transcriptID == "" {
		return skip("no transcript archived for the session")
	}
	res.TranscriptID = transcriptID
	art, err := s.store.ReadArtifact(dossierID, transcriptID)
	if err != nil {
		return fail("reading transcript", err)
	}
	if len(art.Content) > maxEvalTranscriptBytes {
		return skip(fmt.Sprintf("transcript too large to evaluate (%d bytes > %d)", len(art.Content), maxEvalTranscriptBytes))
	}

	model := s.cfg.Eval.Model
	text, cost, err := s.eval.Complete(ctx, model, evalProbePrompt(art.Content))
	res.Summary.CostUSD += cost
	if err != nil {
		return fail("probe extraction", err)
	}
	probes, err := parseEvalProbes(text)
	if err != nil {
		return fail("probe extraction", err)
	}
	if len(probes) == 0 {
		return skip("no probes extracted from the transcript")
	}

	text, cost, err = s.eval.Complete(ctx, model, evalAnswerPrompt(d.DistilledState.Body, probes))
	res.Summary.CostUSD += cost
	if err != nil {
		return fail("resume", err)
	}
	answers, err := parseEvalKeyed[string](text, "answers")
	if err != nil {
		return fail("resume", err)
	}
	for i := range probes {
		probes[i].Answer = answers[strconv.Itoa(i+1)]
	}

	text, cost, err = s.eval.Complete(ctx, model, evalJudgePrompt(probes))
	res.Summary.CostUSD += cost
	if err != nil {
		return fail("judge", err)
	}
	verdicts, err := parseEvalKeyed[evalVerdict](text, "results")
	if err != nil {
		return fail("judge", err)
	}

	res.Summary.ByKind = map[string]EvalKindScore{}
	for i := range probes {
		v, ok := verdicts[strconv.Itoa(i+1)]
		probes[i].Pass = ok && v.Pass
		probes[i].Reason = v.Reason
		if !ok {
			probes[i].Reason = "judge returned no verdict"
		}
		k := res.Summary.ByKind[probes[i].Kind]
		k.Probes++
		res.Summary.Probes++
		if probes[i].Pass {
			k.Passed++
			res.Summary.Passed++
		}
		res.Summary.ByKind[probes[i].Kind] = k
	}
	res.Probes = probes
	record()
	return res, nil
}

// sessionTranscriptID returns the newest transcript archived for a session.
// The session-end capture holds the whole session, so the newest is complete.
func (s *Service) sessionTranscriptID(dossierID, sessionID string) (string, error) {
	events, err := s.store.ReadAuditLog(dossierID)
	if err != nil {
		return "", err
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })
	latest := ""
	for _, e := range events {
		if e.SessionID != sessionID || e.Event != AuditEventSave || len(e.ArtifactsAdded) == 0 {
			continue
		}
		for _, id := range e.ArtifactsAdded {
			if art, err := s.store.ReadArtifact(dossierID, id); err == nil && art.Type == ArtifactTypeTranscript {
				latest = id
			}
		}
	}
	return latest, nil
}

type evalVerdict struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

func evalProbePrompt(transcript string) string {
	return fmt.Sprintf(`You are auditing whether a work record preserved what matters. Below is the transcript of one working session between a user and an AI agent.

List up to %d facts established in THIS session that an agent resuming the work tomorrow, with no access to this conversation, must know to continue correctly. Prefer, in order: exact values (identifiers, numbers, paths, commands, versions); corrections or preferences the user gave; approvals and their limits; decisions with their reasons; rejected options with their reasons; unverified assumptions; where the work stood when the session ended. Skip anything trivial or not needed to continue.

For each fact, write a question a resuming agent should be able to answer, and the expected answer in as few words as possible.

Return only JSON, no prose: [{"kind": "value|correction|approval|decision|rejected|assumption|state", "question": "...", "expect": "..."}]

<transcript>
%s
</transcript>`, maxEvalProbes, transcript)
}

func evalAnswerPrompt(state string, probes []EvalProbeResult) string {
	var q strings.Builder
	for i, p := range probes {
		fmt.Fprintf(&q, "%d. %s\n", i+1, p.Question)
	}
	return fmt.Sprintf(`You are resuming a piece of work. The document below is the only context you have about it.

Answer each question using only the document. If the document does not contain the answer, reply exactly "NOT IN STATE". Be brief.

Return only JSON, no prose: {"answers": {"1": "...", "2": "..."}}

<document>
%s
</document>

Questions:
%s`, state, q.String())
}

func evalJudgePrompt(probes []EvalProbeResult) string {
	var items strings.Builder
	for i, p := range probes {
		fmt.Fprintf(&items, "%d.\nQuestion: %s\nExpected: %s\nAnswer: %s\n\n", i+1, p.Question, p.Expect, p.Answer)
	}
	return fmt.Sprintf(`For each item, decide whether the answer conveys the expected fact. Accept paraphrase. Reject answers that are missing, vague, contradicted, or "NOT IN STATE". Exact values (numbers, identifiers, paths, commands) must match exactly.

Return only JSON, no prose: {"results": {"1": {"pass": true, "reason": "one short line"}, "2": {...}}}

%s`, items.String())
}

// parseEvalProbes reads the extraction output, dropping malformed entries and
// normalising unknown kinds to "state" rather than failing the whole eval.
func parseEvalProbes(text string) ([]EvalProbeResult, error) {
	raw, err := extractJSON(text, '[', ']')
	if err != nil {
		return nil, err
	}
	var items []struct {
		Kind     string `json:"kind"`
		Question string `json:"question"`
		Expect   string `json:"expect"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("probe list is not valid JSON: %w", err)
	}
	known := map[string]bool{}
	for _, k := range EvalProbeKinds {
		known[k] = true
	}
	var out []EvalProbeResult
	for _, it := range items {
		q, e := strings.TrimSpace(it.Question), strings.TrimSpace(it.Expect)
		if q == "" || e == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(it.Kind))
		if !known[kind] {
			kind = "state"
		}
		out = append(out, EvalProbeResult{Kind: kind, Question: q, Expect: e})
		if len(out) == maxEvalProbes {
			break
		}
	}
	return out, nil
}

// parseEvalKeyed reads {"<field>": {"1": T, ...}} from model output.
func parseEvalKeyed[T any](text, field string) (map[string]T, error) {
	raw, err := extractJSON(text, '{', '}')
	if err != nil {
		return nil, err
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("output is not valid JSON: %w", err)
	}
	inner, ok := wrapper[field]
	if !ok {
		return nil, fmt.Errorf("output has no %q field", field)
	}
	var out map[string]T
	if err := json.Unmarshal(inner, &out); err != nil {
		return nil, fmt.Errorf("%q field is malformed: %w", field, err)
	}
	return out, nil
}

// extractJSON returns the outermost JSON value delimited by open/close,
// tolerating code fences and prose around it.
func extractJSON(text string, open, close byte) (string, error) {
	start := strings.IndexByte(text, open)
	end := strings.LastIndexByte(text, close)
	if start < 0 || end < start {
		return "", fmt.Errorf("no JSON %c...%c in model output", open, close)
	}
	return text[start : end+1], nil
}
