package cli

import (
	"dossier/internal/core"
	"dossier/internal/harness"
	"dossier/internal/store"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestOpenCommandBindsThenLaunches exercises the full `dossier open` handoff
// against a stub "claude" that records how it was invoked: the dossier must be
// bound to the same session id the binary is handed (ADR 0006).
func TestOpenCommandBindsThenLaunches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary is a shell script")
	}

	tempHome := t.TempDir()
	// The service resolves dossier paths from its config, which defaults to
	// $DOSSIER_HOME — the --home flag alone only redirects the store.
	t.Setenv("DOSSIER_HOME", tempHome)
	dossierDir := filepath.Join(tempHome, "pricing-model-refresh")
	if err := os.MkdirAll(dossierDir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	fm := core.Frontmatter{
		ID:        "dos_open123",
		Name:      "Pricing model refresh",
		Slug:      "pricing-model-refresh",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    core.StatusActive,
		Priority:  core.PriorityHigh,
	}
	serialized, err := store.FormatDossierFile(fm, "# Pricing model refresh\n\n## Situation\nDraft.")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(serialized), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, ".lock"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Stub claude: record argv and the working directory, then exit cleanly.
	binDir := t.TempDir()
	argsFile := filepath.Join(binDir, "argv.txt")
	stub := filepath.Join(binDir, "claude-stub")
	script := "#!/bin/sh\n{ pwd; for a in \"$@\"; do echo \"$a\"; done; } > " + argsFile + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(harness.ClaudeBinEnv, stub)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"open", "pricing-model-refresh", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("open failed: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("stub claude was never invoked: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected cwd + 3 args, got %v", lines)
	}
	cwd, args := lines[0], lines[1:]

	// The stub runs in the dossier directory (macOS may hand back /private/...).
	if !strings.HasSuffix(cwd, "pricing-model-refresh") {
		t.Errorf("claude ran in %q, want the dossier directory", cwd)
	}
	if args[0] != "--session-id" {
		t.Fatalf("expected --session-id first, got %v", args)
	}
	sessionID := args[1]
	if !strings.Contains(args[2], "pricing-model-refresh") {
		t.Errorf("prompt should name the slug, got %q", args[2])
	}

	// The binding must already exist for that exact id, so the session-start
	// hook finds it the moment Claude comes up.
	binding, err := store.NewFSStore(tempHome).GetSessionBinding(sessionID)
	if err != nil {
		t.Fatalf("no binding for session %q: %v", sessionID, err)
	}
	if binding == nil || binding.DossierID != "dos_open123" {
		t.Errorf("binding = %+v, want DossierID dos_open123", binding)
	}
}

// An invalid explicit override must fail loudly before a binding is written.
func TestOpenCommandInvalidOverrideLeavesNoBinding(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("DOSSIER_HOME", tempHome)
	dossierDir := filepath.Join(tempHome, "pricing-model-refresh")
	if err := os.MkdirAll(dossierDir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	serialized, err := store.FormatDossierFile(core.Frontmatter{
		ID:        "dos_open123",
		Name:      "Pricing model refresh",
		Slug:      "pricing-model-refresh",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    core.StatusActive,
		Priority:  core.PriorityHigh,
	}, "# Pricing model refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(serialized), 0644); err != nil {
		t.Fatal(err)
	}

	override := filepath.Join(t.TempDir(), "missing-claude")
	t.Setenv(harness.ClaudeBinEnv, override)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"open", "pricing-model-refresh", "--home", tempHome})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an invalid DOSSIER_CLAUDE_BIN")
	}
	if msg := err.Error(); !strings.Contains(msg, harness.ClaudeBinEnv) || !strings.Contains(msg, override) || !strings.Contains(msg, "executable") {
		t.Errorf("error should explain how to fix the invalid override, got: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(tempHome, "sessions")); len(entries) != 0 {
		t.Errorf("expected no session bindings written, got %v", entries)
	}
}

// A missing claude binary must fail loudly and leave no binding behind.
func TestOpenCommandMissingBinary(t *testing.T) {
	tempHome := t.TempDir()
	// The service resolves dossier paths from its config, which defaults to
	// $DOSSIER_HOME — the --home flag alone only redirects the store.
	t.Setenv("DOSSIER_HOME", tempHome)
	dossierDir := filepath.Join(tempHome, "pricing-model-refresh")
	if err := os.MkdirAll(dossierDir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	serialized, err := store.FormatDossierFile(core.Frontmatter{
		ID:        "dos_open123",
		Name:      "Pricing model refresh",
		Slug:      "pricing-model-refresh",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    core.StatusActive,
		Priority:  core.PriorityHigh,
	}, "# Pricing model refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(serialized), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(harness.ClaudeBinEnv, "")
	t.Setenv("PATH", t.TempDir())

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"open", "pricing-model-refresh", "--home", tempHome})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when claude is not installed")
	}
	if !strings.Contains(err.Error(), harness.ClaudeBinEnv) {
		t.Errorf("error should point at the override env var, got: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(tempHome, "sessions")); len(entries) != 0 {
		t.Errorf("expected no session bindings written, got %v", entries)
	}
}
