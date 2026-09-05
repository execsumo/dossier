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

func TestPlanCursorHandoff(t *testing.T) {
	plan := PlanCursorHandoff("/usr/local/bin/cursor-agent", "cursor-session", "/home/u/.dossier/pricing-model", "Pricing Model", "pricing-model")

	if plan.Bin != "/usr/local/bin/cursor-agent" {
		t.Errorf("Bin = %q", plan.Bin)
	}
	if plan.Dir != "/home/u/.dossier/pricing-model" {
		t.Errorf("Dir = %q", plan.Dir)
	}
	if len(plan.Args) != 1 || !strings.Contains(plan.Args[0], "pricing-model") {
		t.Errorf("Args = %v, want one dossier prompt", plan.Args)
	}
	if len(plan.Env) != 4 || plan.Env[0] != "DOSSIER_SESSION=cursor-session" {
		t.Errorf("Env = %v, want Cursor session override and parent-identity clears", plan.Env)
	}

	cmd := plan.Command()
	if cmd.Dir != plan.Dir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, plan.Dir)
	}
	for _, want := range plan.Env {
		found := false
		for _, got := range cmd.Env {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("command environment missing %q", want)
		}
	}
}

func TestPlanPiHandoff(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "pi")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend dir to PATH so pathBin finds it
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	plan, err := PlanOpenWith("pi", LaunchRequest{
		SessionID:  "pi-session",
		DossierDir: "/tmp/dossier/project",
		Name:       "Project",
		Slug:       "project",
	})
	if err != nil {
		t.Fatalf("PlanOpenWith(pi): %v", err)
	}

	if len(plan.Env) != 3 {
		t.Fatalf("Env = %v, want exactly 3 clears", plan.Env)
	}

	want := map[string]bool{
		"PI_SESSION_ID=":          false,
		"PI_SESSION_FILE=":        false,
		"CLAUDE_CODE_SESSION_ID=": false,
	}
	for _, env := range plan.Env {
		if _, ok := want[env]; ok {
			want[env] = true
		} else {
			t.Errorf("Unexpected env item %q", env)
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("Missing expected env clear %q", k)
		}
	}
}

func TestPlanOpenWithCursorResolvesConfiguredBinary(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "cursor-agent")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(CursorBinEnv, fake)

	plan, err := PlanOpenWith("cursor", LaunchRequest{
		SessionID:  "cursor-session",
		DossierDir: "/tmp/dossier/project",
		Name:       "Project",
		Slug:       "project",
	})
	if err != nil {
		t.Fatalf("PlanOpenWith(cursor): %v", err)
	}
	if plan.Bin != fake || len(plan.Args) != 1 || !strings.Contains(plan.Args[0], "project") {
		t.Fatalf("cursor plan = %+v", plan)
	}
}

func TestPlanPromptHandoffWithPrefix(t *testing.T) {
	plan := PlanPromptHandoffWithPrefix("/usr/local/bin/agy", []string{"--prompt-interactive"}, "agy-session", "/tmp/dossier/project", "Project", "project", nil)
	if len(plan.Args) != 2 || plan.Args[0] != "--prompt-interactive" || !strings.Contains(plan.Args[1], "project") {
		t.Fatalf("Args = %v, want Antigravity interactive flag plus prompt", plan.Args)
	}
}

func TestNormalizeOpenWith(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "claude", want: "claude-code"},
		{input: "claude-code", want: "claude-code"},
		{input: "cursor", want: "cursor"},
		{input: "codex", want: "codex"},
		{input: "pi", want: "pi"},
		{input: "agy", want: "antigravity"},
		{input: "antigravity", want: "antigravity"},
	} {
		got, err := NormalizeOpenWith(tt.input)
		if err != nil {
			t.Fatalf("NormalizeOpenWith(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("NormalizeOpenWith(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	if _, err := NormalizeOpenWith("unknown"); err == nil {
		t.Fatal("NormalizeOpenWith(unknown) succeeded, want unsupported-profile error")
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
