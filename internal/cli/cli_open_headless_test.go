package cli

import (
	"dossier/internal/core"
	"dossier/internal/harness"
	"dossier/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenHeadlessBindsAndLaunchesLeanPrintSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DOSSIER_HOME", home)
	dir := filepath.Join(home, "topic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	serialized, err := store.FormatDossierFile(core.Frontmatter{ID: "dos_headless", Name: "Topic", Slug: "topic", CreatedAt: now, UpdatedAt: now, Status: core.StatusExecute, Priority: core.PriorityMedium}, "# Topic\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dossier.md"), []byte(serialized), 0o644); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	stub := writeClaudeStub(t, t.TempDir(), argsFile, true)
	t.Setenv(harness.ClaudeBinEnv, stub)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"open", "topic", "--headless", "--home", home})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 || lines[1] != "--print" || lines[2] != "--session-id" {
		t.Fatalf("headless Claude argv = %#v", lines)
	}
	binding, err := store.NewFSStore(home).GetSessionBinding(lines[3])
	if err != nil || binding == nil || binding.DossierID != "dos_headless" {
		t.Fatalf("headless binding = %#v, err=%v", binding, err)
	}
}
