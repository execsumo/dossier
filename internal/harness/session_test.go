package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestResolveSessionID(t *testing.T) {
	tests := []struct {
		name         string
		explicit     string
		claudeEnv    string
		piEnv        string
		dossierEnv   string
		allowDefault bool
		want         string
		wantErr      bool
	}{
		{"explicit wins over env", "explicit-1", "claude-1", "", "dossier-1", false, "explicit-1", false},
		{"claude env beats pi and dossier env", "", "claude-1", "pi-1", "dossier-1", false, "claude-1", false},
		{"pi env beats dossier env", "", "", "pi-1", "dossier-1", false, "pi-1", false},
		{"dossier env when no harness env", "", "", "", "dossier-1", false, "dossier-1", false},
		{"default when allowed and nothing set", "", "", "", "", true, DefaultSessionID, false},
		{"error when not allowed and nothing set", "", "", "", "", false, "", true},
		{"explicit still wins when default allowed", "explicit-2", "", "", "", true, "explicit-2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Empty value behaves as unset for our != "" checks, and t.Setenv restores after.
			t.Setenv("CLAUDE_CODE_SESSION_ID", tt.claudeEnv)
			t.Setenv("PI_SESSION_ID", tt.piEnv)
			t.Setenv("DOSSIER_SESSION", tt.dossierEnv)
			// Point pointer lookups at an empty directory so a real Pi session on
			// the machine running the tests cannot leak into the table.
			t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir())

			got, err := ResolveSessionID(tt.explicit, tt.allowDefault)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveSessionID(%q, %v) = %q, want %q", tt.explicit, tt.allowDefault, got, tt.want)
			}
		})
	}
}

func TestResolveSessionNamesItsSource(t *testing.T) {
	tests := []struct {
		name        string
		explicit    string
		claudeEnv   string
		piEnv       string
		dossierEnv  string
		wantHarness string
	}{
		{"claude env", "", "claude-1", "", "", "claude-code"},
		{"pi env", "", "", "pi-1", "", "pi"},
		{"explicit override has no source", "explicit-1", "", "", "", ""},
		{"manual override has no source", "", "", "", "dossier-1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_SESSION_ID", tt.claudeEnv)
			t.Setenv("PI_SESSION_ID", tt.piEnv)
			t.Setenv("DOSSIER_SESSION", tt.dossierEnv)
			t.Setenv("DOSSIER_PI_SESSION_DIR", t.TempDir())

			_, harnessName, err := ResolveSession(tt.explicit, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if harnessName != tt.wantHarness {
				t.Errorf("harness = %q, want %q", harnessName, tt.wantHarness)
			}
		})
	}
}

func TestResolveSessionAttributesPointerToPi(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:    PiPointerSchema,
		PID:       os.Getpid(),
		SessionID: "pi-pointer-session",
	})

	_, harnessName, err := ResolveSession("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if harnessName != "pi" {
		t.Errorf("harness = %q, want %q", harnessName, "pi")
	}
}

// writePointer publishes a pointer for pid the way the Pi extension does.
func writePointer(t *testing.T, dir string, pid int, p PiSessionPointer) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir pointer dir: %v", err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)+".json"), data, 0600); err != nil {
		t.Fatalf("write pointer: %v", err)
	}
}

// The Pi extension's pointer is what an MCP server — which Pi never spawned
// through the bash tool, so it has no PI_SESSION_ID — resolves by.
func TestResolveSessionIDUsesPiPointerWhenEnvIsAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:    PiPointerSchema,
		PID:       os.Getpid(),
		SessionID: "pi-pointer-session",
	})

	got, err := ResolveSessionID("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pi-pointer-session" {
		t.Errorf("expected pointer session id, got %q", got)
	}
}

func TestResolveSessionIDUsesPiEnvWhenPointerAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "pi-env-session")
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	// Do not write pointer.

	got, err := ResolveSessionID("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pi-env-session" {
		t.Errorf("expected env session id to win when no pointer, got %q", got)
	}
}

func TestResolveSessionIDPrefersPiPointerOverEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("PI_SESSION_ID", "pi-env-session")
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:    PiPointerSchema,
		PID:       os.Getpid(),
		SessionID: "pi-pointer-session",
	})

	got, err := ResolveSessionID("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pi-pointer-session" {
		t.Errorf("expected pointer session id to win, got %q", got)
	}
}
