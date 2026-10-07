package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBackend routes by prompt content: distill, resume and judge calls.
type fakeBackend struct {
	state   string
	answers string
	judge   string
	calls   int
}

func (f *fakeBackend) Complete(_ context.Context, req Request) (Response, error) {
	f.calls++
	switch {
	case strings.Contains(req.Prompt, "<distillation_guide>"):
		return Response{Text: f.state, CostUSD: 0.01}, nil
	case strings.Contains(req.Prompt, "<distilled_state>"):
		return Response{Text: f.answers, CostUSD: 0.01}, nil
	default:
		return Response{Text: f.judge, CostUSD: 0.01}, nil
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseProbesValidation(t *testing.T) {
	good := "- {id: a, kind: value, question: q, expect: e, match: verbatim}\n"
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"ok", good, ""},
		{"bad kind", "- {id: a, kind: nonsense, question: q, expect: e, match: judge}\n", `invalid kind "nonsense"`},
		{"bad match", "- {id: a, kind: value, question: q, expect: e, match: fuzzy}\n", `invalid match "fuzzy"`},
		{"missing id", "- {kind: value, question: q, expect: e, match: judge}\n", "id is required"},
		{"missing expect", "- {id: a, kind: value, question: q, match: judge}\n", "expect is required"},
		{"duplicate id", good + good, "duplicate id"},
		{"unknown field", "- {id: a, kind: value, question: q, expect: e, match: judge, extra: 1}\n", "extra"},
		{"empty", "[]\n", "no probes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseProbes([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadCases(t *testing.T) {
	dir := t.TempDir()
	probes := "- {id: a, kind: value, question: q, expect: e, match: verbatim}\n"
	writeFile(t, filepath.Join(dir, "md", "transcript.md"), "# hello\n")
	writeFile(t, filepath.Join(dir, "md", "probes.yaml"), probes)
	jsonl := `{"type":"user","message":{"role":"user","content":"hi there"},"uuid":"1","timestamp":"2026-01-01T00:00:00Z"}` + "\n"
	writeFile(t, filepath.Join(dir, "jl", "transcript.jsonl"), jsonl)
	writeFile(t, filepath.Join(dir, "jl", "probes.yaml"), probes)

	cases, err := loadCases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "jl" || cases[1].Name != "md" {
		t.Fatalf("unexpected cases: %+v", cases)
	}
	if !strings.Contains(cases[0].Transcript, "hi there") {
		t.Errorf("jsonl not compiled: %q", cases[0].Transcript)
	}

	// Both transcripts present is ambiguous.
	writeFile(t, filepath.Join(dir, "md", "transcript.jsonl"), jsonl)
	if _, err := loadCases(dir); err == nil || !strings.Contains(err.Error(), "keep one") {
		t.Errorf("want ambiguity error, got %v", err)
	}
	// Bad probes name the case.
	os.Remove(filepath.Join(dir, "md", "transcript.jsonl"))
	writeFile(t, filepath.Join(dir, "md", "probes.yaml"), "- {id: a, kind: x, question: q, expect: e, match: judge}\n")
	if _, err := loadCases(dir); err == nil || !strings.Contains(err.Error(), "case md") || !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("want case-scoped kind error, got %v", err)
	}
}

func TestSampleCaseLoads(t *testing.T) {
	cases, err := loadCases("testdata/cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Name != "sample-billing" {
		t.Fatalf("cases = %+v", cases)
	}
	kinds := map[string]bool{}
	for _, p := range cases[0].Probes {
		kinds[p.Kind] = true
	}
	for _, k := range validKinds {
		if !kinds[k] {
			t.Errorf("sample case lacks a %q probe", k)
		}
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestResolveSpecGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, "assets", "guide.md"), "guide v1\n")
	writeFile(t, filepath.Join(repo, "assets", "instructions.md"), "instr v1\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "v1")
	writeFile(t, filepath.Join(repo, "assets", "guide.md"), "guide v2\n")
	writeFile(t, filepath.Join(repo, "assets", "instructions.md"), "instr v2\n")
	gitRun(t, repo, "commit", "-q", "-am", "v2")

	got, err := resolveSpec("git:HEAD^:assets/guide.md", repo)
	if err != nil || got != "guide v1\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	// Plain file path.
	got, err = resolveSpec(filepath.Join(repo, "assets", "guide.md"), repo)
	if err != nil || got != "guide v2\n" {
		t.Fatalf("file: got %q, %v", got, err)
	}
	for _, bad := range []string{"git:nopath", "git::x", "git:HEAD:"} {
		if _, err := resolveSpec(bad, repo); err == nil || !strings.Contains(err.Error(), "bad git spec") {
			t.Errorf("%q: want bad git spec error, got %v", bad, err)
		}
	}
	if _, err := resolveSpec("git:HEAD:assets/missing.md", repo); err == nil {
		t.Error("want error for missing path")
	}

	// Instructions default to the same ref as a git: guide.
	v, err := loadVariant("A", "git:HEAD^:assets/guide.md", "", repo)
	if err != nil {
		t.Fatal(err)
	}
	if v.Guide != "guide v1\n" || v.Instructions != "instr v1\n" || v.InstructionsSpec != "git:HEAD^:assets/instructions.md" {
		t.Errorf("variant = %+v", v)
	}
	// Explicit instructions override the default.
	v, err = loadVariant("B", "git:HEAD^:assets/guide.md", "git:HEAD:assets/instructions.md", repo)
	if err != nil || v.Instructions != "instr v2\n" {
		t.Errorf("override: %+v, %v", v, err)
	}
}

func TestScoreVerbatim(t *testing.T) {
	p := Probe{ID: "x", Kind: "value", Expect: "500ms", Match: "verbatim"}
	tests := []struct {
		name, state, answer string
		pass                bool
		reason              string
	}{
		{"both", "timeout 500ms", "It is 500ms.", true, "present in state and answer"},
		{"in state, missing from answer", "timeout 500ms", "half a second", false, "in state but missing from answer"},
		{"missing from state, answer has it", "timeout set", "500ms", false, "missing from state"},
		{"neither", "timeout set", "unknown", false, "missing from state and answer"},
		{"case sensitive", "timeout 500MS", "500ms", false, "missing from state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := scoreVerbatim(p, tt.state, tt.answer)
			if r.Pass != tt.pass || !strings.Contains(r.Reason, tt.reason) {
				t.Errorf("got pass=%v reason=%q", r.Pass, r.Reason)
			}
		})
	}
}

func TestParseJudge(t *testing.T) {
	tests := []struct {
		name, in string
		pass     bool
		reason   string
		wantErr  bool
	}{
		{"plain", `{"pass": true, "reason": "ok"}`, true, "ok", false},
		{"fenced", "```json\n{\"pass\": false, \"reason\": \"wrong value\"}\n```", false, "wrong value", false},
		{"prose around", `Verdict: {"pass": true, "reason": "good"} done`, true, "good", false},
		{"no json", "looks fine to me", false, "", true},
		{"broken json", `{"pass": tru`, false, "", true},
		{"missing pass", `{"reason": "hm"}`, false, "", true},
		{"pass not bool", `{"pass": "yes"}`, false, "", true},
		{"empty", "", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass, reason, err := parseJudge(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (pass != tt.pass || reason != tt.reason) {
				t.Errorf("got %v %q", pass, reason)
			}
		})
	}
}

func TestParseAnswers(t *testing.T) {
	m, err := parseAnswers("```json\n{\"a\": \"x\", \"b\": 5}\n```")
	if err != nil || m["a"] != "x" || m["b"] != "5" {
		t.Fatalf("got %v, %v", m, err)
	}
	if _, err := parseAnswers("no json"); err == nil {
		t.Error("want error")
	}
}

func testCase() Case {
	return Case{Name: "c1", Transcript: "t", Probes: []Probe{
		{ID: "v", Kind: "value", Question: "q1", Expect: "500ms", Match: "verbatim"},
		{ID: "j", Kind: "rejected", Question: "q2", Expect: "no, latency", Match: "judge"},
	}}
}

func TestRunOne(t *testing.T) {
	out := t.TempDir()
	opt := Options{Model: "m", JudgeModel: "j", OutDir: out, Runs: 1}
	v := Variant{Name: "A", Guide: "g", Instructions: "i"}

	t.Run("mixed results", func(t *testing.T) {
		fb := &fakeBackend{state: "timeout 500ms", answers: `{"v": "wait, unsure", "j": "No, too slow"}`, judge: `{"pass": true, "reason": "says no"}`}
		rec := runOne(context.Background(), fb, opt, testCase(), v, 1)
		if rec.Error != "" || len(rec.Results) != 2 || rec.Calls != 3 || fb.calls != 3 {
			t.Fatalf("rec = %+v calls=%d", rec, fb.calls)
		}
		if rec.Results[0].Pass { // in state, missing from answer
			t.Error("verbatim probe should fail when answer lacks the value")
		}
		if !rec.Results[1].Pass || rec.Results[1].Reason != "says no" {
			t.Errorf("judge result = %+v", rec.Results[1])
		}
		if _, err := os.Stat(filepath.Join(out, rec.StateFile)); err != nil {
			t.Errorf("state not saved: %v", err)
		}
	})
	t.Run("malformed judge output is an errored fail", func(t *testing.T) {
		fb := &fakeBackend{state: "s 500ms", answers: `{"v": "500ms", "j": "x"}`, judge: "I think it passes"}
		rec := runOne(context.Background(), fb, opt, testCase(), v, 1)
		if !rec.Results[0].Pass {
			t.Error("verbatim should pass")
		}
		if r := rec.Results[1]; r.Pass || !r.Error || !strings.Contains(r.Reason, "no JSON") {
			t.Errorf("judge result = %+v", r)
		}
	})
	t.Run("unparseable resume fails all probes visibly", func(t *testing.T) {
		fb := &fakeBackend{state: "s", answers: "sorry", judge: ""}
		rec := runOne(context.Background(), fb, opt, testCase(), v, 1)
		if rec.Error == "" || len(rec.Results) != 2 {
			t.Fatalf("rec = %+v", rec)
		}
		for _, r := range rec.Results {
			if r.Pass || !r.Error {
				t.Errorf("result = %+v", r)
			}
		}
	})
	t.Run("empty distill", func(t *testing.T) {
		fb := &fakeBackend{state: "  \n"}
		rec := runOne(context.Background(), fb, opt, testCase(), v, 1)
		if !strings.Contains(rec.Error, "empty state") {
			t.Errorf("rec.Error = %q", rec.Error)
		}
	})
}

func TestSummarize(t *testing.T) {
	mk := func(variant string, run int, probe, kind string, pass bool) ProbeResult {
		return ProbeResult{Case: "c", Variant: variant, Run: run, ProbeID: probe, Kind: kind, Pass: pass}
	}
	recs := []RunRecord{
		{Case: "c", Variant: "A", Run: 1, Results: []ProbeResult{mk("A", 1, "p1", "value", true), mk("A", 1, "p2", "rejected", false)}},
		{Case: "c", Variant: "A", Run: 2, Results: []ProbeResult{mk("A", 2, "p1", "value", true), mk("A", 2, "p2", "rejected", true)}},
		{Case: "c", Variant: "B", Run: 1, Results: []ProbeResult{mk("B", 1, "p1", "value", true), mk("B", 1, "p2", "rejected", true)}},
		{Case: "c", Variant: "B", Run: 2, Results: []ProbeResult{mk("B", 2, "p1", "value", false), mk("B", 2, "p2", "rejected", true)}},
	}
	recs[3].Results[0].Error = true
	s := summarize([]string{"A", "B"}, recs)

	if got := s.Overall["A"]; got != (Tally{3, 4}) {
		t.Errorf("A overall = %+v", got)
	}
	if got := s.Overall["B"]; got != (Tally{3, 4}) {
		t.Errorf("B overall = %+v", got)
	}
	if got := s.ByKind["rejected"]["A"]; got != (Tally{1, 2}) {
		t.Errorf("rejected/A = %+v", got)
	}
	if got := s.ByKind["value"]["B"]; got != (Tally{1, 2}) {
		t.Errorf("value/B = %+v", got)
	}
	if s.Errors["B"] != 1 || s.Errors["A"] != 0 {
		t.Errorf("errors = %v", s.Errors)
	}
	// A run rates: 0.5, 1.0. mean .75, population stddev .25.
	sp := s.Spread["A"]
	if sp.Min != 0.5 || sp.Max != 1.0 || sp.Mean != 0.75 || sp.StdDev != 0.25 || sp.Runs != 2 {
		t.Errorf("spread A = %+v", sp)
	}
	// p1: A 2/2 vs B 1/2 differ; p2: A 1/2 vs B 2/2 differ.
	if len(s.Diffs) != 2 || s.Diffs[0].ProbeID != "p1" || s.Diffs[1].ProbeID != "p2" {
		t.Errorf("diffs = %+v", s.Diffs)
	}
	if (Tally{}).Rate() != 0 {
		t.Error("empty tally rate should be 0")
	}

	// Identical results produce no diffs.
	same := summarize([]string{"A", "B"}, recs[:1:1])
	if len(same.Diffs) != 0 {
		t.Errorf("single-variant data should have no diffs: %+v", same.Diffs)
	}

	rep := renderReport([]Variant{{Name: "A", GuideSpec: "ga"}, {Name: "B", GuideSpec: "gb"}}, Options{Model: "m", JudgeModel: "j", Runs: 2}, recs, 0.5)
	for _, want := range []string{"75.0% (3/4)", "## Pass rate by probe kind", "## Per-case breakdown", "## Run-to-run spread", "## Probes that differ", "| c | p1 | value |"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report missing %q:\n%s", want, rep)
		}
	}
}

func TestPlannedCalls(t *testing.T) {
	cases := []Case{testCase(), testCase()} // each: 1 judge probe -> 3 calls per variant-run
	if got := plannedCalls(cases, 2, 3); got != 2*3*2*3 {
		t.Errorf("plannedCalls = %d, want 36", got)
	}
}

func TestDryRunMakesNoCalls(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "cases", "c1", "transcript.md"), "hello\n")
	writeFile(t, filepath.Join(dir, "cases", "c1", "probes.yaml"),
		"- {id: a, kind: value, question: q, expect: e, match: verbatim}\n- {id: b, kind: state, question: q, expect: e, match: judge}\n")
	writeFile(t, filepath.Join(dir, "ga.md"), "guide a")
	writeFile(t, filepath.Join(dir, "gb.md"), "guide b")
	writeFile(t, filepath.Join(dir, "i.md"), "instr")

	fb := &fakeBackend{}
	var stdout, stderr bytes.Buffer
	args := []string{"--cases", filepath.Join(dir, "cases"), "--guide-a", filepath.Join(dir, "ga.md"), "--guide-b", filepath.Join(dir, "gb.md"),
		"--instructions-a", filepath.Join(dir, "i.md"), "--instructions-b", filepath.Join(dir, "i.md"),
		"--runs", "2", "--out", filepath.Join(dir, "out"), "--dry-run"}
	if err := run(context.Background(), args, &stdout, &stderr, fb); err != nil {
		t.Fatal(err)
	}
	// (2 + 1 judge) calls x 2 variants x 2 runs = 12
	if !strings.Contains(stdout.String(), "Estimated model calls: 12") {
		t.Errorf("plan output:\n%s", stdout.String())
	}
	if fb.calls != 0 {
		t.Errorf("dry run made %d calls", fb.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("dry run created the output dir")
	}
}

func TestFullRunWritesOutputs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "cases", "c1", "transcript.md"), "hello\n")
	writeFile(t, filepath.Join(dir, "cases", "c1", "probes.yaml"),
		"- {id: a, kind: value, question: q, expect: 500ms, match: verbatim}\n- {id: b, kind: state, question: q, expect: e, match: judge}\n")
	writeFile(t, filepath.Join(dir, "g.md"), "guide")
	writeFile(t, filepath.Join(dir, "i.md"), "instr")
	fb := &fakeBackend{state: "500ms", answers: `{"a": "500ms", "b": "e"}`, judge: `{"pass": true, "reason": "r"}`}
	out := filepath.Join(dir, "out")
	var stdout, stderr bytes.Buffer
	args := []string{"--cases", filepath.Join(dir, "cases"), "--guide-a", filepath.Join(dir, "g.md"), "--instructions-a", filepath.Join(dir, "i.md"),
		"--guide-b", filepath.Join(dir, "g.md"), "--instructions-b", filepath.Join(dir, "i.md"), "--runs", "2", "--out", out}
	if err := run(context.Background(), args, &stdout, &stderr, fb); err != nil {
		t.Fatal(err)
	}
	if fb.calls != 12 {
		t.Errorf("calls = %d, want 12", fb.calls)
	}
	for _, f := range []string{"results.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestParseCLIOutput(t *testing.T) {
	r, err := parseCLIOutput([]byte(`{"result":"hi","is_error":false,"total_cost_usd":0.5}`))
	if err != nil || r.Text != "hi" || r.CostUSD != 0.5 {
		t.Errorf("got %+v, %v", r, err)
	}
	if _, err := parseCLIOutput([]byte(`{"result":"Not logged in","is_error":true}`)); err == nil {
		t.Error("want error for is_error")
	}
	if _, err := parseCLIOutput([]byte(`nope`)); err == nil {
		t.Error("want error for bad JSON")
	}
}

func TestClaudeArgsIsolation(t *testing.T) {
	args := strings.Join(ClaudeCLI{Bare: true}.args("haiku"), " ")
	for _, want := range []string{"--no-session-persistence", "--strict-mcp-config", "disableAllHooks", `--setting-sources `, "--tools", "--disable-slash-commands", "--bare", "--model haiku"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
	if strings.Contains(strings.Join(ClaudeCLI{}.args(""), " "), "--bare") {
		t.Error("--bare must be opt-in")
	}
}
