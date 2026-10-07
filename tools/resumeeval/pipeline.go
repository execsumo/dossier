package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProbeResult is the scored outcome of one probe in one (case, variant, run).
type ProbeResult struct {
	Case     string `json:"case"`
	Variant  string `json:"variant"`
	Run      int    `json:"run"`
	ProbeID  string `json:"probe_id"`
	Kind     string `json:"kind"`
	Match    string `json:"match"`
	Expect   string `json:"expect"`
	Answer   string `json:"answer"`
	Pass     bool   `json:"pass"`
	Reason   string `json:"reason"`
	InState  *bool  `json:"in_state,omitempty"`  // verbatim probes only
	InAnswer *bool  `json:"in_answer,omitempty"` // verbatim probes only
	// Error marks a probe that failed because the pipeline broke (model error,
	// unparseable output), not because the fact was lost.
	Error bool `json:"error,omitempty"`
}

// RunRecord is one full pipeline pass: distill, resume, score.
type RunRecord struct {
	Case      string        `json:"case"`
	Variant   string        `json:"variant"`
	Run       int           `json:"run"`
	StateFile string        `json:"state_file,omitempty"`
	Error     string        `json:"error,omitempty"`
	CostUSD   float64       `json:"cost_usd"`
	Calls     int           `json:"calls"`
	Results   []ProbeResult `json:"results"`
}

// Options configures a pipeline run.
type Options struct {
	Model      string
	JudgeModel string
	OutDir     string
	Runs       int
}

func judgeCount(c Case) int {
	n := 0
	for _, p := range c.Probes {
		if p.Match == "judge" {
			n++
		}
	}
	return n
}

// plannedCalls is the number of model calls a full run would make: per
// case x variant x run, one distill, one resume, and one judge per judge probe.
func plannedCalls(cases []Case, variants, runs int) int {
	total := 0
	for _, c := range cases {
		total += (2 + judgeCount(c)) * variants * runs
	}
	return total
}

func distillPrompt(v Variant, transcript string) string {
	return "You are producing the Distilled State for a Dossier: the curated Markdown brief a fresh agent will read to resume this work with no other context.\n\n" +
		"Follow the Distillation Guide and the Operating Instructions below.\n\n" +
		"<distillation_guide>\n" + v.Guide + "\n</distillation_guide>\n\n" +
		"<operating_instructions>\n" + v.Instructions + "\n</operating_instructions>\n\n" +
		"<transcript>\n" + transcript + "\n</transcript>\n\n" +
		"Output ONLY the Distilled State Markdown body. No preamble, no code fence around the whole document, no tool calls."
}

func resumePrompt(state string, probes []Probe) string {
	var b strings.Builder
	b.WriteString("You are an agent resuming work. The ONLY context you have is the Distilled State below. Answer each question using only that document. " +
		"If the document does not say, answer \"UNKNOWN\" rather than guessing. When a question asks for an exact value, quote it exactly.\n\n")
	b.WriteString("<distilled_state>\n" + state + "\n</distilled_state>\n\nQuestions:\n")
	for _, p := range probes {
		fmt.Fprintf(&b, "- %s: %s\n", p.ID, p.Question)
	}
	b.WriteString("\nReply with ONLY a JSON object mapping each question id to your answer string, e.g. {\"id1\": \"answer\"}. No other text.")
	return b.String()
}

func judgePrompt(p Probe, answer string) string {
	return "You are grading an answer from an agent that resumed work from a briefing.\n\n" +
		"Question: " + p.Question + "\n" +
		"Required: " + p.Expect + "\n" +
		"Answer: " + answer + "\n\n" +
		"Does the answer satisfy the requirement? Be strict: the answer must actually state what is required, not merely be compatible with it. \"UNKNOWN\" or a refusal to commit fails.\n" +
		"Reply with ONLY a JSON object: {\"pass\": true|false, \"reason\": \"one line\"}."
}

// extractJSON returns the outermost {...} span of s, tolerating code fences
// and surrounding prose.
func extractJSON(s string) (string, bool) {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return "", false
	}
	return s[i : j+1], true
}

// parseJudge decodes the judge's verdict. Malformed output is an error so the
// caller can mark the probe as a pipeline failure rather than a lost fact.
func parseJudge(s string) (bool, string, error) {
	raw, ok := extractJSON(s)
	if !ok {
		return false, "", fmt.Errorf("judge output has no JSON object: %q", truncate(s, 120))
	}
	var v struct {
		Pass   *bool  `json:"pass"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return false, "", fmt.Errorf("judge output not valid JSON: %w", err)
	}
	if v.Pass == nil {
		return false, "", fmt.Errorf("judge output missing boolean \"pass\": %q", truncate(raw, 120))
	}
	return *v.Pass, strings.TrimSpace(v.Reason), nil
}

// parseAnswers decodes the resumed agent's id -> answer JSON.
func parseAnswers(s string) (map[string]string, error) {
	raw, ok := extractJSON(s)
	if !ok {
		return nil, fmt.Errorf("resume output has no JSON object: %q", truncate(s, 120))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("resume output not valid JSON: %w", err)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			out[k] = str
			continue
		}
		b, _ := json.Marshal(v)
		out[k] = string(b)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func boolPtr(b bool) *bool { return &b }

// scoreVerbatim passes only when expect appears exactly in both the state and
// the answer: a fact the answer happens to know but the state lost is a fail.
func scoreVerbatim(p Probe, state, answer string) ProbeResult {
	inState := strings.Contains(state, p.Expect)
	inAnswer := strings.Contains(answer, p.Expect)
	r := ProbeResult{InState: boolPtr(inState), InAnswer: boolPtr(inAnswer), Pass: inState && inAnswer}
	switch {
	case r.Pass:
		r.Reason = "expected text present in state and answer"
	case !inState && !inAnswer:
		r.Reason = "expected text missing from state and answer"
	case !inState:
		r.Reason = "expected text missing from state (answer has it)"
	default:
		r.Reason = "expected text in state but missing from answer"
	}
	return r
}

func errResults(c Case, v Variant, run int, reason string) []ProbeResult {
	out := make([]ProbeResult, 0, len(c.Probes))
	for _, p := range c.Probes {
		out = append(out, ProbeResult{Case: c.Name, Variant: v.Name, Run: run, ProbeID: p.ID, Kind: p.Kind,
			Match: p.Match, Expect: p.Expect, Reason: reason, Error: true})
	}
	return out
}

// runOne executes distill -> resume -> score for one case, variant and run.
func runOne(ctx context.Context, be Backend, opt Options, c Case, v Variant, run int) RunRecord {
	rec := RunRecord{Case: c.Name, Variant: v.Name, Run: run}
	fail := func(reason string) RunRecord {
		rec.Error = reason
		rec.Results = errResults(c, v, run, reason)
		return rec
	}

	d, err := be.Complete(ctx, Request{Model: opt.Model, Prompt: distillPrompt(v, c.Transcript)})
	rec.Calls++
	rec.CostUSD += d.CostUSD
	if err != nil {
		return fail(fmt.Sprintf("distill failed: %v", err))
	}
	state := strings.TrimSpace(d.Text)
	if state == "" {
		return fail("distill returned empty state")
	}
	stateFile := filepath.Join("states", fmt.Sprintf("%s__%s__run%d.md", c.Name, v.Name, run))
	if err := os.MkdirAll(filepath.Join(opt.OutDir, "states"), 0o755); err != nil {
		return fail(fmt.Sprintf("create states dir: %v", err))
	}
	if err := os.WriteFile(filepath.Join(opt.OutDir, stateFile), []byte(state+"\n"), 0o644); err != nil {
		return fail(fmt.Sprintf("write state: %v", err))
	}
	rec.StateFile = stateFile

	r, err := be.Complete(ctx, Request{Model: opt.Model, Prompt: resumePrompt(state, c.Probes)})
	rec.Calls++
	rec.CostUSD += r.CostUSD
	if err != nil {
		return fail(fmt.Sprintf("resume failed: %v", err))
	}
	answers, err := parseAnswers(r.Text)
	if err != nil {
		return fail(err.Error())
	}

	for _, p := range c.Probes {
		answer := answers[p.ID]
		var res ProbeResult
		if p.Match == "verbatim" {
			res = scoreVerbatim(p, state, answer)
		} else {
			j, jerr := be.Complete(ctx, Request{Model: opt.JudgeModel, Prompt: judgePrompt(p, answer)})
			rec.Calls++
			rec.CostUSD += j.CostUSD
			if jerr != nil {
				res.Error, res.Reason = true, fmt.Sprintf("judge call failed: %v", jerr)
			} else if pass, reason, perr := parseJudge(j.Text); perr != nil {
				res.Error, res.Reason = true, perr.Error()
			} else {
				res.Pass, res.Reason = pass, reason
			}
		}
		res.Case, res.Variant, res.Run = c.Name, v.Name, run
		res.ProbeID, res.Kind, res.Match, res.Expect, res.Answer = p.ID, p.Kind, p.Match, p.Expect, answer
		rec.Results = append(rec.Results, res)
	}
	return rec
}
