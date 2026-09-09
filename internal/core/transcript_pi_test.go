package core

import (
	"strings"
	"testing"
)

// piTrace is a Pi session JSONL trace in the shape Pi 0.85 actually writes,
// built from its documented session format (docs/session-format.md in the
// @earendil-works/pi-coding-agent package, version 3):
//
//   - a `session` header line that is metadata, not conversation;
//   - `message` entries whose `message` field is an AgentMessage;
//   - Pi's camelCase roles and block types (`toolResult`, `toolCall`), which
//     differ from Claude Code's (`tool_result`, `tool_use`);
//   - roles that carry content outside `content` entirely (`bashExecution`,
//     `compactionSummary`);
//   - a `custom` message, which is what the bundled Pi extension injects at
//     session start.
//
// Until Dossier bridged Pi's lifecycle nothing archived a Pi transcript, so the
// compiler had only ever been exercised against Claude Code's shape.
const piTrace = `{"type":"session","version":3,"id":"11111111-2222-3333-4444-555555555555","timestamp":"2026-09-09T10:00:00.000Z","cwd":"/home/dev/projects/dossier"}
{"type":"message","id":"a1b2c3d4","parentId":null,"timestamp":"2026-09-09T10:00:01.000Z","message":{"role":"user","content":"Bridge Pi's lifecycle events.","timestamp":1757412001000}}
{"type":"message","id":"b2c3d4e5","parentId":"a1b2c3d4","timestamp":"2026-09-09T10:00:02.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Private reasoning that must not appear."},{"type":"text","text":"Reading the extension first."},{"type":"toolCall","id":"call_9","name":"read","arguments":{"path":"assets/pi-extension.ts"}}],"api":"anthropic","provider":"anthropic","model":"claude-sonnet-4-5","stopReason":"toolUse","timestamp":1757412002000}}
{"type":"message","id":"c3d4e5f6","parentId":"b2c3d4e5","timestamp":"2026-09-09T10:00:03.000Z","message":{"role":"toolResult","toolCallId":"call_9","toolName":"read","content":[{"type":"text","text":"export default function (pi) {}"}],"isError":false,"timestamp":1757412003000}}
{"type":"message","id":"d4e5f6a7","parentId":"c3d4e5f6","timestamp":"2026-09-09T10:00:04.000Z","message":{"role":"bashExecution","command":"go test ./...","output":"ok  dossier/internal/core","exitCode":0,"cancelled":false,"truncated":false,"timestamp":1757412004000}}
{"type":"message","id":"e5f6a7b8","parentId":"d4e5f6a7","timestamp":"2026-09-09T10:00:05.000Z","message":{"role":"compactionSummary","summary":"Earlier: wired the session pointer.","tokensBefore":42000,"timestamp":1757412005000}}
{"type":"message","id":"f6a7b8c9","parentId":"e5f6a7b8","timestamp":"2026-09-09T10:00:06.000Z","message":{"role":"custom","customType":"dossier-context","content":"1 open dossier(s): Pi parity.","display":false,"timestamp":1757412006000}}`

func TestCompilePiTranscriptKeepsEveryConversationalRole(t *testing.T) {
	out, format, warnings := CompileTranscript(piTrace)
	if format != ContentFormatMarkdown {
		t.Errorf("format = %q, want markdown — a Pi trace is JSONL, not a plain-text passthrough", format)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings compiling a well-formed Pi trace: %v", warnings)
	}

	for _, want := range []string{
		"Bridge Pi's lifecycle events.",   // user, bare-string content
		"Reading the extension first.",    // assistant text block
		"path: assets/pi-extension.ts",    // toolCall arguments (Pi spells it `arguments`)
		"export default function (pi) {}", // toolResult content
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compiled Pi transcript dropped %q:\n%s", want, out)
		}
	}

	for _, want := range []string{"## [1] user", "## [2] assistant", "## [3] tool_call read (call_9)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing role header %q:\n%s", want, out)
		}
	}
	// Pi names the role `toolResult`; the compiled view must use Dossier's
	// canonical role so citations read the same across harnesses.
	if !strings.Contains(out, "## [4] tool_result read (call_9)") {
		t.Errorf("Pi's toolResult role was not normalized:\n%s", out)
	}
}

// A bashExecution keeps its command and output in dedicated fields, not in
// `content`. Before this was handled the whole record decoded as contentless:
// every command the agent ran vanished from the archived transcript while being
// counted as "harness bookkeeping ... (no conversational content)".
func TestCompilePiTranscriptKeepsBashExecutions(t *testing.T) {
	out, _, _ := CompileTranscript(piTrace)

	if !strings.Contains(out, "go test ./...") {
		t.Errorf("compiled Pi transcript dropped the bash command:\n%s", out)
	}
	if !strings.Contains(out, "ok  dossier/internal/core") {
		t.Errorf("compiled Pi transcript dropped the bash output:\n%s", out)
	}
	// Split into a call/result pair so a citation can land on the command or on
	// its output independently.
	if !strings.Contains(out, "## [5] tool_call bash") {
		t.Errorf("bash command was not rendered as its own citable node:\n%s", out)
	}
	if !strings.Contains(out, "## [6] tool_result bash (exit 0)") {
		t.Errorf("bash output was not rendered as its own node with its exit status:\n%s", out)
	}
}

// A compaction summary stands in for everything compaction discarded — the most
// information-dense record in a long session. Dropping it silently loses that
// history outright.
func TestCompilePiTranscriptKeepsCompactionSummary(t *testing.T) {
	out, _, _ := CompileTranscript(piTrace)

	if !strings.Contains(out, "Earlier: wired the session pointer.") {
		t.Errorf("compiled Pi transcript dropped the compaction summary:\n%s", out)
	}
	if !strings.Contains(out, "## [7] system compactionSummary") {
		t.Errorf("compaction summary was not labelled as such:\n%s", out)
	}
}

// Thinking is excluded for Pi by exactly the same rule as for Claude Code — the
// filter keys on the normalized node role, not on a harness-specific block
// type — and the count is surfaced rather than dropped silently.
func TestCompilePiTranscriptExcludesThinkingButTalliesIt(t *testing.T) {
	out, _, _ := CompileTranscript(piTrace)

	if strings.Contains(out, "Private reasoning that must not appear.") {
		t.Errorf("Pi thinking content leaked into the compiled view:\n%s", out)
	}
	if !strings.Contains(out, "Assistant thinking turns excluded from this view: 1") {
		t.Errorf("elided Pi thinking was not tallied:\n%s", out)
	}
}

// The session header carries no conversation, so it belongs in the bookkeeping
// tally — visible and counted, never silently discarded.
func TestCompilePiTranscriptTalliesSessionHeader(t *testing.T) {
	out, _, _ := CompileTranscript(piTrace)

	if !strings.Contains(out, "session=1") {
		t.Errorf("Pi session header was not tallied as bookkeeping:\n%s", out)
	}
	if strings.Contains(out, "message=") {
		t.Errorf("message records were counted as contentless bookkeeping:\n%s", out)
	}
}

// Pi's `!!` prefix runs a command deliberately withheld from the model. An
// artifact can sync to a team remote (B13), so archiving it would disclose
// something the user chose not to share even with their own agent.
func TestCompilePiTranscriptElidesPrivateBashButTalliesIt(t *testing.T) {
	const privateBash = `{"type":"session","version":3,"id":"s","timestamp":"2026-09-09T10:00:00.000Z","cwd":"/tmp"}
{"type":"message","id":"a1","parentId":null,"timestamp":"2026-09-09T10:00:01.000Z","message":{"role":"bashExecution","command":"cat .env","output":"OPENAI_API_KEY=sk-do-not-archive","exitCode":0,"cancelled":false,"truncated":false,"excludeFromContext":true,"timestamp":1757412001000}}
{"type":"message","id":"a2","parentId":"a1","timestamp":"2026-09-09T10:00:02.000Z","message":{"role":"user","content":"Carry on.","timestamp":1757412002000}}`

	out, _, _ := CompileTranscript(privateBash)

	if strings.Contains(out, "sk-do-not-archive") || strings.Contains(out, "cat .env") {
		t.Errorf("a bash execution the user withheld from the model was archived:\n%s", out)
	}
	if !strings.Contains(out, "Bash executions excluded from the model's context by the user excluded from this view: 1") {
		t.Errorf("the elision was not surfaced; it must degrade visibly, not silently:\n%s", out)
	}
	if !strings.Contains(out, "Carry on.") {
		t.Errorf("eliding the private bash dropped the rest of the trace:\n%s", out)
	}
}

// Regression guard for the harness-agnostic promise: the Claude Code shape must
// keep compiling identically now that Pi-specific roles are handled.
func TestCompileClaudeCodeTranscriptStillCompiles(t *testing.T) {
	out, format, _ := CompileTranscript(sampleTrace)
	if format != ContentFormatMarkdown {
		t.Fatalf("format = %q, want markdown", format)
	}
	if !strings.Contains(out, "## [1] user") {
		t.Errorf("Claude Code trace no longer compiles as before:\n%s", out)
	}
}
