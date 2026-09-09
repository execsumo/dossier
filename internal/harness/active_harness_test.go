package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// claudeCodeConfiguredHome makes ClaudeCodeHarness.Detect report a fully capable
// Claude Code, which it does from the mere existence of ~/.claude.json. This is
// the device state that made harness attribution wrong: Claude Code claims every
// capability here whether or not this process is inside one of its sessions.
func claudeCodeConfiguredHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write .claude.json: %v", err)
	}
	return home
}

func TestActiveHarnessPrefersTheSessionOwnerOverInstalledHarnesses(t *testing.T) {
	claudeCodeConfiguredHome(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir()) // no pointer: fall through to the env var
	t.Setenv("PI_SESSION_ID", "pi-session-uuid")

	reg := NewRegistry("/tmp/dossier")
	h, caps, ok := reg.ActiveHarness()
	if !ok {
		t.Fatal("ActiveHarness() found no owner for a live Pi session")
	}
	if h.Name() != "pi" {
		t.Errorf("ActiveHarness() = %q, want %q — Claude Code being installed is not evidence of a Claude Code session", h.Name(), "pi")
	}
	if caps.MCP {
		t.Error("attributed MCP to Pi; the capabilities must come from the resolved harness")
	}
}

func TestActiveHarnessResolvesClaudeCodeSession(t *testing.T) {
	claudeCodeConfiguredHome(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "cc-session-uuid")
	t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir())
	t.Setenv("PI_SESSION_ID", "")

	reg := NewRegistry("/tmp/dossier")
	h, caps, ok := reg.ActiveHarness()
	if !ok {
		t.Fatal("ActiveHarness() found no owner for a live Claude Code session")
	}
	if h.Name() != "claude-code" {
		t.Errorf("ActiveHarness() = %q, want %q", h.Name(), "claude-code")
	}
	if !caps.MCP {
		t.Error("dropped Claude Code's MCP capability")
	}
}

// The Pi session pointer outranks PI_SESSION_ID and is the only source
// available to a Dossier process Pi did not spawn through its bash tool.
func TestActiveHarnessResolvesPiFromSessionPointer(t *testing.T) {
	claudeCodeConfiguredHome(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "")
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:      PiPointerSchema,
		PID:         os.Getpid(),
		SessionID:   "pointer-session",
		SessionFile: filepath.Join(dir, "session.jsonl"),
	})

	reg := NewRegistry("/tmp/dossier")
	h, _, ok := reg.ActiveHarness()
	if !ok {
		t.Fatal("ActiveHarness() ignored a live Pi session pointer")
	}
	if h.Name() != "pi" {
		t.Errorf("ActiveHarness() = %q, want %q", h.Name(), "pi")
	}
}

// A session id that names no harness — an explicit --session, or
// DOSSIER_SESSION — must not be attributed to whichever harness happens to be
// installed. Answering "no owner" is what lets core report a plain CLI run
// honestly instead of claiming a session that does not exist.
func TestActiveHarnessDeclinesWhenSessionNamesNoHarness(t *testing.T) {
	claudeCodeConfiguredHome(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir())
	t.Setenv("DOSSIER_SESSION", "manually-supplied")

	reg := NewRegistry("/tmp/dossier")
	if h, _, ok := reg.ActiveHarness(); ok {
		t.Errorf("ActiveHarness() attributed a harness-less session to %q", h.Name())
	}
}

func TestActiveHarnessDeclinesWithNoSessionAtAll(t *testing.T) {
	claudeCodeConfiguredHome(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir())

	reg := NewRegistry("/tmp/dossier")
	if h, _, ok := reg.ActiveHarness(); ok {
		t.Errorf("ActiveHarness() claimed %q owns a bare CLI process", h.Name())
	}
}
