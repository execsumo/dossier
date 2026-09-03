package core

import (
	"strings"
	"testing"
)

const sampleTrace = `{"type":"mode","mode":"normal","sessionId":"s1"}
{"type":"user","message":{"role":"user","content":"Fix the billing lock timeout."}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Check the lock first."},{"type":"tool_use","name":"Bash","id":"toolu_01","input":{"command":"go test ./internal/billing/","description":"Run billing tests"}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"FAIL TestConcurrentBilling\nlock timeout 200ms"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Timeout is too low."}]}}`

func TestCompileTranscriptTagsRolesAndKeepsValues(t *testing.T) {
	out, _, warnings := CompileTranscript(sampleTrace)
	if len(warnings) != 0 {
		t.Fatalf("CompileTranscript() warnings = %v, want none", warnings)
	}

	for _, want := range []string{
		"## [1] user",
		"## [2] thinking",
		"## [3] tool_call Bash (toolu_01)",
		"## [4] tool_result (toolu_01)",
		"## [5] assistant",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compiled transcript missing role header %q\n%s", want, out)
		}
	}

	// Tool arguments are the exact inputs needed to verify a decision.
	if !strings.Contains(out, "command: go test ./internal/billing/") {
		t.Errorf("compiled transcript dropped the tool command:\n%s", out)
	}
	if !strings.Contains(out, "FAIL TestConcurrentBilling") {
		t.Errorf("compiled transcript dropped the tool result:\n%s", out)
	}
}

func TestCompileTranscriptCountsBookkeepingRatherThanDroppingSilently(t *testing.T) {
	out, _, _ := CompileTranscript(sampleTrace)
	if !strings.Contains(out, "mode=1") {
		t.Errorf("compiled transcript did not account for the elided bookkeeping record:\n%s", out)
	}
	if !strings.Contains(out, "Harness bookkeeping records not rendered") {
		t.Errorf("compiled transcript did not surface the elision:\n%s", out)
	}
}

func TestCompileTranscriptPreservesUnparsableLines(t *testing.T) {
	out, _, warnings := CompileTranscript(`{"type":"user","message":{"role":"user","content":"hi"}}` + "\nnot json at all")
	if !strings.Contains(out, "not json at all") {
		t.Errorf("unparsable line was dropped:\n%s", out)
	}
	if len(warnings) != 1 || !strings.Contains(string(warnings[0]), "not valid JSONL") {
		t.Errorf("warnings = %v, want one unparsed-line warning", warnings)
	}
}

func TestCompileTranscriptPreservesAndTalliesUnrecognizedBlocks(t *testing.T) {
	trace := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"See the attached screenshot."},{"type":"image","source":{"type":"base64","data":"..."}}]}}`
	out, _, warnings := CompileTranscript(trace)

	if !strings.Contains(out, "image=1") {
		t.Errorf("compiled transcript did not tally the preserved image block:\n%s", out)
	}
	if !strings.Contains(out, "Unrecognized content block(s) preserved as raw JSON") {
		t.Errorf("compiled transcript did not surface the raw representation:\n%s", out)
	}
	if !strings.Contains(out, `{"type":"image","source":{"type":"base64","data":"..."}}`) {
		t.Errorf("compiled transcript dropped unknown block bytes:\n%s", out)
	}

	var found bool
	for _, w := range warnings {
		if strings.Contains(string(w), "image") && strings.Contains(string(w), "preserved as raw JSON") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want one naming the preserved image block", warnings)
	}
}

func TestCompileTranscriptPreservesNestedNonTextToolResultBlocks(t *testing.T) {
	trace := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_image","content":[{"type":"text","text":"OCR summary"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAECAw=="}},{"type":"future_result","payload":{"exact":[1,2,3]}}]}]}}`
	out, _, warnings := CompileTranscript(trace)

	for _, want := range []string{
		"## [1] tool_result (toolu_image)",
		"OCR summary",
		"[raw tool_result content block: image]",
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAECAw=="}}`,
		"[raw tool_result content block: future_result]",
		`{"type":"future_result","payload":{"exact":[1,2,3]}}`,
		"tool_result/image=1",
		"tool_result/future_result=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compiled transcript missing %q:\n%s", want, out)
		}
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one aggregate warning", warnings)
	}
	warning := string(warnings[0])
	if !strings.Contains(warning, "2 content block(s)") ||
		!strings.Contains(warning, "tool_result/image=1") ||
		!strings.Contains(warning, "tool_result/future_result=1") {
		t.Errorf("warning did not count both nested blocks: %q", warning)
	}
}

func TestCompileTranscriptPassesThroughPlainText(t *testing.T) {
	raw := "A plain text transcript.\nSecond line."
	out, format, warnings := CompileTranscript(raw)
	if out != raw {
		t.Errorf("plain text transcript = %q, want unchanged %q", out, raw)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if format != ContentFormatText {
		t.Errorf("format = %v, want text for plain-text pass-through", format)
	}
}

func TestCompileTranscriptLineNumbersAreCitable(t *testing.T) {
	out, _, _ := CompileTranscript(sampleTrace)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	var target int
	for i, l := range lines {
		if strings.Contains(l, "FAIL TestConcurrentBilling") {
			target = i + 1
			break
		}
	}
	if target == 0 {
		t.Fatalf("could not locate the tool result line:\n%s", out)
	}

	// A citation of that physical line must resolve to that content, since the
	// artifact file's own line numbers are the coordinate system.
	if got := lines[target-1]; !strings.Contains(got, "FAIL TestConcurrentBilling") {
		t.Errorf("line %d = %q, want the tool result", target, got)
	}
}
