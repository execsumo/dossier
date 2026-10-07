package harness

import (
	"dossier/assets"
	"dossier/internal/core"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuoteCommandPath(t *testing.T) {
	if got := quoteCommandPath(`C:\\Program Files\\Dossier\\dossier.exe`); got != `"C:\\Program Files\\Dossier\\dossier.exe"` {
		t.Fatalf("quoteCommandPath() = %q", got)
	}
}

func TestClaudeCodeHarness(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	// Before creating config files, Detect should return empty capabilities
	h := NewClaudeCodeHarness("/tmp/dossier")
	caps, err := h.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caps.SessionStartHook || caps.SessionEndHook {
		t.Errorf("expected empty capabilities when config doesn't exist, got %+v", caps)
	}

	// Create fake .claude.json with a mix of styles to test migration and preservation
	claudeJSONPath := filepath.Join(tempHome, ".claude.json")
	initialConfig := map[string]any{
		"mcpServers": map[string]any{
			"unrelated": map[string]any{
				"type":    "stdio",
				"command": "echo",
			},
		},
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "echo 'unrelated'",
						},
					},
				},
			},
			"PreCompact": "old-style-string-hook",
		},
	}
	configBytes, _ := json.Marshal(initialConfig)
	if err := os.WriteFile(claudeJSONPath, configBytes, 0644); err != nil {
		t.Fatalf("failed to write fake config: %v", err)
	}

	// Detect should now return capabilities
	caps, err = h.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.SessionStartHook || !caps.SessionEndHook || !caps.PreCompactionHook || !caps.TranscriptCapture {
		t.Errorf("expected full capabilities, got %+v", caps)
	}

	// Install hooks with YesToAll = true
	err = h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"})
	if err != nil {
		t.Fatalf("failed to install: %v", err)
	}

	// Check that config is updated
	updatedBytes, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	var updatedConfig map[string]any
	if err := json.Unmarshal(updatedBytes, &updatedConfig); err != nil {
		t.Fatalf("failed to unmarshal updated config: %v", err)
	}

	hooks, ok := updatedConfig["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks not found in config")
	}

	// Assert the exact structure for SessionStart (Claude Code schema: array of matchers)
	startVal, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("expected SessionStart to be a slice, got %T", hooks["SessionStart"])
	}
	if len(startVal) != 1 {
		t.Fatalf("expected SessionStart slice to have 1 matcher, got %d", len(startVal))
	}
	matcherObj, ok := startVal[0].(map[string]any)
	if !ok {
		t.Fatalf("expected matcher object, got %T", startVal[0])
	}
	if matcherObj["matcher"] != "*" {
		t.Errorf("expected matcher to be '*', got %v", matcherObj["matcher"])
	}
	hooksList, ok := matcherObj["hooks"].([]any)
	if !ok {
		t.Fatalf("expected hooks list to be slice, got %T", matcherObj["hooks"])
	}
	if len(hooksList) != 1 {
		t.Fatalf("expected hooks list to have 1 item, got %d", len(hooksList))
	}
	hookEntry, ok := hooksList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected hook entry map, got %T", hooksList[0])
	}
	if hookEntry["type"] != "command" {
		t.Errorf("expected type to be 'command', got %v", hookEntry["type"])
	}
	if !strings.Contains(hookEntry["command"].(string), "hook session-start") {
		t.Errorf("expected command to contain 'hook session-start', got %v", hookEntry["command"])
	}
	if !strings.Contains(hookEntry["command"].(string), "/tmp/dossier") {
		t.Errorf("expected command to contain stable path /tmp/dossier, got %v", hookEntry["command"])
	}

	// Assert PreCompact was converted from string to array
	preCompactVal, ok := hooks["PreCompact"].([]any)
	if !ok {
		t.Fatalf("expected PreCompact to be converted to slice, got %T", hooks["PreCompact"])
	}
	if len(preCompactVal) != 1 {
		t.Fatalf("expected PreCompact to have 1 matcher, got %d", len(preCompactVal))
	}

	// A fresh install must not add a Stop hook: the save checkpoint it was for
	// was dropped as too much friction in ordinary conversations (B29
	// amendment). See TestClaudeCodeHarnessRemovesStaleStopHook for the
	// migration that cleans it off an install that already has it.
	if _, ok := hooks["Stop"]; ok {
		t.Errorf("expected no Stop hook to be installed, got %v", hooks["Stop"])
	}

	// Assert unrelated hook UserPromptSubmit was preserved
	unrelatedVal, ok := hooks["UserPromptSubmit"].([]any)
	if !ok {
		t.Fatalf("expected UserPromptSubmit to be slice, got %T", hooks["UserPromptSubmit"])
	}
	if len(unrelatedVal) != 1 {
		t.Fatalf("expected UserPromptSubmit to have 1 item, got %d", len(unrelatedVal))
	}

	// Assert MCP is registered and unrelated preserved
	mcpServers, ok := updatedConfig["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers map, got %T", updatedConfig["mcpServers"])
	}
	unrelatedMCP, ok := mcpServers["unrelated"].(map[string]any)
	if !ok {
		t.Fatalf("expected unrelated MCP server to be preserved")
	}
	if unrelatedMCP["command"] != "echo" {
		t.Errorf("expected unrelated command to be 'echo', got %v", unrelatedMCP["command"])
	}
	dossierMCP, ok := mcpServers["dossier"].(map[string]any)
	if !ok {
		t.Fatalf("expected dossier MCP server to be registered")
	}
	if dossierMCP["command"] != "/tmp/dossier" {
		t.Errorf("expected dossier command to be '/tmp/dossier', got %v", dossierMCP["command"])
	}

	// Test idempotency: running Install again should not create new backup and should not change config
	files, _ := filepath.Glob(filepath.Join(tempHome, ".claude.json.*.bak"))
	initialBackupCount := len(files)

	err = h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"})
	if err != nil {
		t.Fatalf("failed to install: %v", err)
	}

	files, _ = filepath.Glob(filepath.Join(tempHome, ".claude.json.*.bak"))
	if len(files) != initialBackupCount {
		t.Errorf("expected no new backup on idempotent run, got %d backups (initially %d)", len(files), initialBackupCount)
	}
}

// TestClaudeCodeHarnessSplitConfig covers the real-world layout where both
// ~/.claude/settings.json and ~/.claude.json exist: hooks must go to settings.json,
// the MCP server must go to ~/.claude.json, and a stale dossier MCP entry that an
// older buggy version wrote into settings.json must be stripped (migration).
func TestClaudeCodeHarnessSplitConfig(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	claudeDir := filepath.Join(tempHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	claudeJSONPath := filepath.Join(tempHome, ".claude.json")

	// settings.json holds hooks plus a STALE dossier MCP entry (the bug) and an unrelated MCP server.
	settings := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{"matcher": "*", "hooks": []any{
					map[string]any{"type": "command", "command": "echo unrelated"},
				}},
			},
		},
		"mcpServers": map[string]any{
			"dossier":   map[string]any{"type": "stdio", "command": "dossier", "args": []any{"mcp", "serve"}},
			"unrelated": map[string]any{"type": "stdio", "command": "echo"},
		},
	}
	settingsBytes, _ := json.Marshal(settings)
	if err := os.WriteFile(settingsPath, settingsBytes, 0644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	// .claude.json holds an unrelated MCP server only.
	claudeJSON := map[string]any{
		"mcpServers": map[string]any{
			"codegraph": map[string]any{"type": "stdio", "command": "codegraph", "args": []any{"serve"}},
		},
	}
	claudeJSONBytes, _ := json.Marshal(claudeJSON)
	if err := os.WriteFile(claudeJSONPath, claudeJSONBytes, 0644); err != nil {
		t.Fatalf("failed to write .claude.json: %v", err)
	}

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// settings.json: hooks installed, unrelated hook preserved, stale dossier MCP STRIPPED, unrelated MCP preserved.
	var gotSettings map[string]any
	b, _ := os.ReadFile(settingsPath)
	if err := json.Unmarshal(b, &gotSettings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}
	sHooks, _ := gotSettings["hooks"].(map[string]any)
	if _, ok := sHooks["SessionStart"].([]any); !ok {
		t.Errorf("expected SessionStart hook in settings.json, got %T", sHooks["SessionStart"])
	}
	if _, ok := sHooks["UserPromptSubmit"].([]any); !ok {
		t.Errorf("expected unrelated UserPromptSubmit hook preserved in settings.json")
	}
	if sMCP, ok := gotSettings["mcpServers"].(map[string]any); ok {
		if _, has := sMCP["dossier"]; has {
			t.Errorf("stale dossier MCP entry should have been stripped from settings.json")
		}
		if _, has := sMCP["unrelated"]; !has {
			t.Errorf("unrelated MCP server should be preserved in settings.json")
		}
	}

	// .claude.json: dossier MCP registered with stable path, codegraph preserved.
	var gotClaude map[string]any
	b, _ = os.ReadFile(claudeJSONPath)
	if err := json.Unmarshal(b, &gotClaude); err != nil {
		t.Fatalf("failed to parse .claude.json: %v", err)
	}
	cMCP, ok := gotClaude["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers in .claude.json, got %T", gotClaude["mcpServers"])
	}
	dossierMCP, ok := cMCP["dossier"].(map[string]any)
	if !ok {
		t.Fatalf("expected dossier MCP registered in .claude.json")
	}
	if dossierMCP["command"] != "/tmp/dossier" {
		t.Errorf("expected dossier MCP command to be stable path '/tmp/dossier', got %v", dossierMCP["command"])
	}
	if _, has := cMCP["codegraph"]; !has {
		t.Errorf("codegraph MCP server should be preserved in .claude.json")
	}

	// Idempotency: re-running Install creates no new backups.
	settingsBaks, _ := filepath.Glob(settingsPath + ".*.bak")
	claudeBaks, _ := filepath.Glob(claudeJSONPath + ".*.bak")
	before := len(settingsBaks) + len(claudeBaks)
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	settingsBaks, _ = filepath.Glob(settingsPath + ".*.bak")
	claudeBaks, _ = filepath.Glob(claudeJSONPath + ".*.bak")
	if after := len(settingsBaks) + len(claudeBaks); after != before {
		t.Errorf("expected no new backups on idempotent run, got %d (was %d)", after, before)
	}
}

// TestClaudeCodeHarnessRemovesStaleStopHook covers upgrading from a version
// that installed the Stop hook (v0.5.1's save checkpoint) to one that does
// not want it configured at all (dropped as too much friction; B29
// amendment). Install must actively strip it — unlike SessionStart/End/
// PreCompact, its absence is now the correct state, so nothing would ever
// put it back, and leaving it would run a hook pointed at dead code.
func TestClaudeCodeHarnessRemovesStaleStopHook(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	claudeJSONPath := filepath.Join(tempHome, ".claude.json")

	// A v0.5.1-style config: all four hooks already configured correctly,
	// including Stop sharing its matcher array with an unrelated hook, so
	// removal must be surgical (drop only Dossier's own entry) not wholesale.
	initial := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"matcher": "*", "hooks": []any{
				map[string]any{"type": "command", "command": `"/tmp/dossier" hook session-start`},
			}}},
			"SessionEnd": []any{map[string]any{"matcher": "*", "hooks": []any{
				map[string]any{"type": "command", "command": `"/tmp/dossier" hook session-end`},
			}}},
			"PreCompact": []any{map[string]any{"matcher": "*", "hooks": []any{
				map[string]any{"type": "command", "command": `"/tmp/dossier" hook pre-compaction`},
			}}},
			"Stop": []any{map[string]any{"matcher": "*", "hooks": []any{
				map[string]any{"type": "command", "command": `"/tmp/dossier" hook stop`},
				map[string]any{"type": "command", "command": "echo unrelated-stop-hook"},
			}}},
		},
		"customInstructions": []any{claudeSkillInstruction},
		"mcpServers": map[string]any{
			"dossier": map[string]any{"type": "stdio", "command": "/tmp/dossier", "args": []any{"mcp", "serve"}},
		},
	}
	initialBytes, _ := json.Marshal(initial)
	if err := os.WriteFile(claudeJSONPath, initialBytes, 0644); err != nil {
		t.Fatalf("failed to write fake config: %v", err)
	}
	// Skills already installed and correct, so only the Stop hook is stale —
	// this exercises the staleStopHook-but-hooksOk-already-true path.
	for _, item := range []struct{ rel, asset string }{
		{"skills/dossier-delegate/SKILL.md", "dossier-delegate-skill.md"},
		{"skills/spark/SKILL.md", "spark-skill.md"},
		{"skills/save-dossier/SKILL.md", "save-dossier-skill.md"},
	} {
		content, err := assets.FS.ReadFile(item.asset)
		if err != nil {
			t.Fatalf("read embedded %s: %v", item.asset, err)
		}
		p := filepath.Join(tempHome, ".claude", item.rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	h := NewClaudeCodeHarness("/tmp/dossier")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	var got map[string]any
	b, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("failed to parse .claude.json: %v", err)
	}
	hooks, ok := got["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("expected hooks map, got %T", got["hooks"])
	}
	stopVal, hasStop := hooks["Stop"].([]any)
	if hasStop {
		// Dossier's own entry must be gone; the unrelated hook in the same
		// matcher array must survive.
		matcher, _ := stopVal[0].(map[string]any)
		stopHooks, _ := matcher["hooks"].([]any)
		for _, hv := range stopHooks {
			if cmd, _ := hv.(map[string]any)["command"].(string); strings.Contains(cmd, "hook stop") {
				t.Errorf("Dossier's own Stop hook command survived: %v", cmd)
			}
		}
		if len(stopHooks) != 1 {
			t.Errorf("expected only the unrelated Stop hook to remain, got %v", stopHooks)
		}
	}
	for _, name := range []string{"SessionStart", "SessionEnd", "PreCompact"} {
		if _, ok := hooks[name].([]any); !ok {
			t.Errorf("expected %s to remain configured, got %T", name, hooks[name])
		}
	}
	if baks, _ := filepath.Glob(claudeJSONPath + ".*.bak"); len(baks) != 1 {
		t.Errorf("expected exactly one backup from the migration, got %d", len(baks))
	}

	// Idempotency: a second install, with Stop now absent (or down to just the
	// unrelated hook), makes no further changes.
	before, _ := filepath.Glob(claudeJSONPath + ".*.bak")
	if err := h.Install(core.InstallOpts{YesToAll: true, StableBinaryPath: "/tmp/dossier"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	after, _ := filepath.Glob(claudeJSONPath + ".*.bak")
	if len(after) != len(before) {
		t.Errorf("expected no new backup on idempotent run, got %d (was %d)", len(after), len(before))
	}
}
