package harness

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewClaudeSessionID(t *testing.T) {
	a, err := NewClaudeSessionID()
	if err != nil {
		t.Fatalf("NewClaudeSessionID: %v", err)
	}
	if !uuidV4Re.MatchString(a) {
		t.Errorf("id %q is not an RFC 4122 v4 UUID", a)
	}

	b, err := NewClaudeSessionID()
	if err != nil {
		t.Fatalf("NewClaudeSessionID: %v", err)
	}
	if a == b {
		t.Errorf("two mints produced the same id %q", a)
	}
}

func TestPlanClaudeHandoff(t *testing.T) {
	plan := PlanClaudeHandoff("/usr/bin/claude", "11111111-2222-4333-8444-555555555555", "/home/u/.dossier/pricing-model", "Pricing Model", "pricing-model")

	if plan.Bin != "/usr/bin/claude" {
		t.Errorf("Bin = %q", plan.Bin)
	}
	if plan.Dir != "/home/u/.dossier/pricing-model" {
		t.Errorf("Dir = %q", plan.Dir)
	}
	if plan.SessionID != "11111111-2222-4333-8444-555555555555" {
		t.Errorf("SessionID = %q", plan.SessionID)
	}

	if len(plan.Args) != 3 {
		t.Fatalf("expected 3 args, got %v", plan.Args)
	}
	if plan.Args[0] != "--session-id" || plan.Args[1] != plan.SessionID {
		t.Errorf("expected --session-id <id> first, got %v", plan.Args[:2])
	}

	prompt := plan.Args[2]
	if !strings.Contains(prompt, "Pricing Model") {
		t.Errorf("prompt missing dossier name: %q", prompt)
	}
	if !strings.Contains(prompt, "pricing-model") {
		t.Errorf("prompt missing slug: %q", prompt)
	}

	// The plan must be materializable without touching the filesystem.
	cmd := plan.Command()
	if cmd.Dir != plan.Dir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, plan.Dir)
	}
	if got := cmd.Args[1:]; len(got) != len(plan.Args) {
		t.Errorf("cmd args = %v, want %v", got, plan.Args)
	}
}

func TestClaudeBinHonoursOverride(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "my-claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ClaudeBinEnv, fake)
	t.Setenv("PATH", dir) // deliberately no "claude" here

	got, err := ClaudeBin()
	if err != nil {
		t.Fatalf("ClaudeBin: %v", err)
	}
	if got != fake {
		t.Errorf("ClaudeBin = %q, want %q", got, fake)
	}
}

func TestClaudeBinRejectsInvalidOverride(t *testing.T) {
	tests := []struct {
		name string
		path func(*testing.T) string
	}{
		{
			name: "nonexistent",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing-claude")
			},
		},
		{
			name: "directory",
			path: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "non-executable",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "claude")
				if err := os.WriteFile(path, []byte("not executable"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override := tt.path(t)
			t.Setenv(ClaudeBinEnv, override)

			got, err := ClaudeBin()
			if err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
			msg := err.Error()
			if !strings.Contains(msg, ClaudeBinEnv) || !strings.Contains(msg, override) || !strings.Contains(msg, "executable") || !strings.Contains(msg, "unset") {
				t.Errorf("error is not actionable for invalid override: %s", msg)
			}
		})
	}
}

func TestClaudeBinMissingIsAnActionableError(t *testing.T) {
	t.Setenv(ClaudeBinEnv, "")
	t.Setenv("PATH", t.TempDir())

	got, err := ClaudeBin()
	if err == nil {
		t.Fatalf("expected an error, got %q", got)
	}
	msg := err.Error()
	if !strings.Contains(msg, "claude") || !strings.Contains(msg, ClaudeBinEnv) {
		t.Errorf("error must name the binary and the override env var, got: %s", msg)
	}
}
