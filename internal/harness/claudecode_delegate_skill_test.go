package harness

import (
	"dossier/assets"
	"dossier/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMinimalClaudeConfig creates a bare-bones ~/.claude.json so Install
// doesn't bail out early (Install requires settings.json or .claude.json to
// already exist before it will do anything).
func writeMinimalClaudeConfig(t *testing.T, tempHome string) string {
	t.Helper()
	claudeJSONPath := filepath.Join(tempHome, ".claude.json")
	if err := os.WriteFile(claudeJSONPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write minimal .claude.json: %v", err)
	}
	return claudeJSONPath
}

func delegateSkillAssetContent(t *testing.T) []byte {
	t.Helper()
	content, err := assets.FS.ReadFile("dossier-delegate-skill.md")
	if err != nil {
		t.Fatalf("failed to read embedded dossier-delegate skill asset: %v", err)
	}
	return content
}

// TestClaudeCodeHarnessInstallsDelegateSkill covers a fresh Install writing
// the dossier-delegate skill into Claude Code's own skills directory
// (~/.claude/skills/dossier-delegate/SKILL.md), not Dossier's ~/.dossier
// store, with content byte-identical to the embedded asset.
func TestClaudeCodeHarnessInstallsDelegateSkill(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	writeMinimalClaudeConfig(t, tempHome)

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	skillPath := filepath.Join(tempHome, ".claude", "skills", "dossier-delegate", "SKILL.md")
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected dossier-delegate SKILL.md to be written, got error: %v", err)
	}

	want := delegateSkillAssetContent(t)
	if string(got) != string(want) {
		t.Errorf("installed SKILL.md content does not match embedded asset")
	}
}

// TestClaudeCodeHarnessDelegateSkillIdempotent covers a second Install, once
// hooks/MCP/customInstructions/skill are all already correct, being a full
// no-op: no error, and the skill file is left untouched (same content, same
// mtime, no new backup).
func TestClaudeCodeHarnessDelegateSkillIdempotent(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	writeMinimalClaudeConfig(t, tempHome)

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	skillPath := filepath.Join(tempHome, ".claude", "skills", "dossier-delegate", "SKILL.md")
	firstContent, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected SKILL.md after first install, got error: %v", err)
	}
	firstStat, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("failed to stat SKILL.md after first install: %v", err)
	}

	baksBefore, _ := filepath.Glob(skillPath + ".*.bak")

	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	secondContent, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected SKILL.md after second install, got error: %v", err)
	}
	if string(secondContent) != string(firstContent) {
		t.Errorf("SKILL.md content changed on idempotent re-install")
	}

	secondStat, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("failed to stat SKILL.md after second install: %v", err)
	}
	if !secondStat.ModTime().Equal(firstStat.ModTime()) {
		t.Errorf("expected SKILL.md mtime unchanged on idempotent re-install, got %v (was %v)", secondStat.ModTime(), firstStat.ModTime())
	}

	baksAfter, _ := filepath.Glob(skillPath + ".*.bak")
	if len(baksAfter) != len(baksBefore) {
		t.Errorf("expected no new SKILL.md backup on idempotent run, got %d backups (was %d)", len(baksAfter), len(baksBefore))
	}
}

// TestClaudeCodeHarnessDelegateSkillOverwritesStaleContent covers the case
// where ~/.claude/skills/dossier-delegate/SKILL.md already exists with
// different content than the embedded asset (a stale bundled version, or a
// user hand-edit): Install must back up the original content to a
// timestamped .bak file, then overwrite with the current embedded asset.
func TestClaudeCodeHarnessDelegateSkillOverwritesStaleContent(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	writeMinimalClaudeConfig(t, tempHome)

	skillDir := filepath.Join(tempHome, ".claude", "skills", "dossier-delegate")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create pre-existing skill dir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	staleContent := []byte("---\nname: dossier-delegate\ndescription: stale old version\n---\nstale body\n")
	if err := os.WriteFile(skillPath, staleContent, 0644); err != nil {
		t.Fatalf("failed to write stale SKILL.md: %v", err)
	}

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected SKILL.md after install, got error: %v", err)
	}
	want := delegateSkillAssetContent(t)
	if string(got) != string(want) {
		t.Errorf("SKILL.md was not overwritten with the current embedded asset")
	}

	baks, err := filepath.Glob(skillPath + ".*.bak")
	if err != nil {
		t.Fatalf("failed to glob for backups: %v", err)
	}
	if len(baks) != 1 {
		t.Fatalf("expected exactly 1 backup of stale SKILL.md, got %d", len(baks))
	}
	backupContent, err := os.ReadFile(baks[0])
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}
	if string(backupContent) != string(staleContent) {
		t.Errorf("backup content does not match original stale content")
	}
}

// TestClaudeCodeHarnessDelegateSkillNotInCustomInstructions guards the
// pull-only design constraint: installing the skill file must never touch
// customInstructions or otherwise wire the skill into automatic
// session-start injection.
func TestClaudeCodeHarnessDelegateSkillNotInCustomInstructions(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	writeMinimalClaudeConfig(t, tempHome)

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	claudeJSONPath := filepath.Join(tempHome, ".claude.json")
	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		t.Fatalf("failed to read .claude.json: %v", err)
	}
	if strings.Contains(string(data), "dossier-delegate") {
		t.Errorf("dossier-delegate must not appear in .claude.json / customInstructions, got: %s", data)
	}
}
