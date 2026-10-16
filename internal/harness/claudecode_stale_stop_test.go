package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dossier/assets"
	"dossier/internal/core"
)

// staleStopSettings is the shape v0.5.1 left in ~/.claude/settings.json on a
// Homebrew install: every Dossier hook quoted at /opt/homebrew/bin/dossier,
// and the Stop hook sharing its "*" matcher with an unrelated user hook, plus
// a second unrelated Stop matcher.
const staleStopSettings = `{
  "hooks": {
    "SessionStart": [{"matcher": "*", "hooks": [{"type": "command", "command": "\"/opt/homebrew/bin/dossier\" hook session-start"}]}],
    "SessionEnd": [{"matcher": "*", "hooks": [{"type": "command", "command": "\"/opt/homebrew/bin/dossier\" hook session-end"}]}],
    "PreCompact": [{"matcher": "*", "hooks": [{"type": "command", "command": "\"/opt/homebrew/bin/dossier\" hook pre-compaction"}]}],
    "Stop": [
      {"matcher": "*", "hooks": [
        {"type": "command", "command": "bash '/home/u/.claude/hooks/idle-on-stop.sh'", "timeout": 10},
        {"type": "command", "command": "\"/opt/homebrew/bin/dossier\" hook stop"}
      ]},
      {"hooks": [{"type": "command", "command": "/home/u/.claude/hooks/done.sh", "timeout": 5}]}
    ]
  },
  "customInstructions": ["Dossier: use dossier_session."]
}
`

const staleStopClaudeJSON = `{"mcpServers":{"dossier":{"type":"stdio","command":"/opt/homebrew/bin/dossier","args":["mcp","serve"]},"other":{"type":"stdio","command":"other"}}}
`

// writeStaleStopFixture lays down the v0.5.1 Homebrew config under a temp
// HOME, with the managed skills already current so the only pending changes
// are the hook path refresh and the stale Stop removal.
func writeStaleStopFixture(t *testing.T) (home, settingsPath, claudeJSONPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	settingsPath = filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath = filepath.Join(home, ".claude.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(staleStopSettings), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeJSONPath, []byte(staleStopClaudeJSON), 0644); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ rel, asset string }{
		{"skills/dossier-delegate/SKILL.md", "dossier-delegate-skill.md"},
		{"skills/spark/SKILL.md", "spark-skill.md"},
		{"skills/save-dossier/SKILL.md", "save-dossier-skill.md"},
	} {
		content, err := assets.FS.ReadFile(item.asset)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(home, ".claude", item.rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return home, settingsPath, claudeJSONPath
}

// withStdin swaps os.Stdin for the duration of the test.
func withStdin(t *testing.T, f *os.File) {
	t.Helper()
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })
}

// Without --yes and without a confirming terminal, Install writes nothing —
// and must say so. It used to return nil, so `dossier harness install
// claude-code` run from an agent's shell (or answered "n") reported success
// while the stale "/opt/homebrew/bin/dossier" hook stop entry stayed put.
func TestClaudeCodeInstallWithoutConfirmationReportsSkip(t *testing.T) {
	cases := []struct {
		name  string
		stdin func(t *testing.T) *os.File
	}{
		{"no terminal (pipe)", func(t *testing.T) *os.File {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			_ = w.Close()
			t.Cleanup(func() { _ = r.Close() })
			return r
		}},
		{"declined (empty answer)", func(t *testing.T) *os.File {
			// /dev/null is a character device, so this takes the prompt path
			// and reads an empty answer: the user did not say yes.
			f, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.Close() })
			return f
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, settingsPath, claudeJSONPath := writeStaleStopFixture(t)
			withStdin(t, tc.stdin(t))

			err := NewClaudeCodeHarness("/tmp/dossier").Install(core.InstallOpts{
				Interactive:      true,
				StableBinaryPath: "/opt/homebrew/bin/dossier",
			})
			if !errors.Is(err, core.ErrInstallSkipped) {
				t.Fatalf("expected ErrInstallSkipped, got %v", err)
			}
			if !strings.Contains(err.Error(), "Stop hook") {
				t.Errorf("skip message should name the pending stale Stop hook, got %q", err)
			}
			if got, _ := os.ReadFile(settingsPath); string(got) != staleStopSettings {
				t.Errorf("settings.json changed without confirmation:\n%s", got)
			}
			if got, _ := os.ReadFile(claudeJSONPath); string(got) != staleStopClaudeJSON {
				t.Errorf(".claude.json changed without confirmation:\n%s", got)
			}
			for _, p := range []string{settingsPath, claudeJSONPath} {
				if baks, _ := filepath.Glob(p + ".*.bak"); len(baks) != 0 {
					t.Errorf("unexpected backup without confirmation: %v", baks)
				}
			}
		})
	}
}

// Confirmed install against the v0.5.1 Homebrew shape: only Dossier's quoted
// Stop entry goes; both unrelated Stop hooks (same matcher and separate
// matcher), their extra fields, unrelated MCP servers and instructions stay.
// One backup holds the original bytes; a re-run changes nothing.
func TestClaudeCodeInstallRemovesQuotedHomebrewStopHook(t *testing.T) {
	for _, stable := range []string{"/opt/homebrew/bin/dossier", "/home/u/.local/bin/dossier"} {
		t.Run(stable, func(t *testing.T) {
			_, settingsPath, claudeJSONPath := writeStaleStopFixture(t)
			h := NewClaudeCodeHarness("/tmp/dossier")
			opts := core.InstallOpts{YesToAll: true, StableBinaryPath: stable}
			if err := h.Install(opts); err != nil {
				t.Fatalf("install: %v", err)
			}

			raw, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("hook stop")) {
				t.Fatalf("Dossier Stop hook survived install:\n%s", raw)
			}
			var cfg map[string]any
			if err := json.Unmarshal(raw, &cfg); err != nil {
				t.Fatal(err)
			}
			hooks := cfg["hooks"].(map[string]any)
			var stopCmds []string
			for _, m := range hooks["Stop"].([]any) {
				for _, hv := range m.(map[string]any)["hooks"].([]any) {
					hm := hv.(map[string]any)
					stopCmds = append(stopCmds, hm["command"].(string))
					if hm["timeout"] == nil {
						t.Errorf("unrelated hook lost its timeout: %v", hm)
					}
				}
			}
			want := []string{"bash '/home/u/.claude/hooks/idle-on-stop.sh'", "/home/u/.claude/hooks/done.sh"}
			if strings.Join(stopCmds, "|") != strings.Join(want, "|") {
				t.Errorf("Stop hooks = %v, want %v", stopCmds, want)
			}
			for event, sub := range map[string]string{"SessionStart": "session-start", "SessionEnd": "session-end", "PreCompact": "pre-compaction"} {
				if !isHookConfigured(hooks[event], quoteCommandPath(stable)+" hook "+sub) {
					t.Errorf("%s not pointed at %s: %v", event, stable, hooks[event])
				}
			}
			if ci, _ := cfg["customInstructions"].([]any); len(ci) != 1 {
				t.Errorf("customInstructions changed: %v", cfg["customInstructions"])
			}
			mcpRaw, _ := os.ReadFile(claudeJSONPath)
			if !bytes.Contains(mcpRaw, []byte(`"other"`)) {
				t.Errorf("unrelated MCP server lost:\n%s", mcpRaw)
			}

			baks, _ := filepath.Glob(settingsPath + ".*.bak")
			if len(baks) != 1 {
				t.Fatalf("expected one settings.json backup, got %v", baks)
			}
			if b, _ := os.ReadFile(baks[0]); string(b) != staleStopSettings {
				t.Errorf("backup does not hold the original bytes:\n%s", b)
			}

			// Idempotent: second run writes nothing and makes no backup.
			mcpBaks, _ := filepath.Glob(claudeJSONPath + ".*.bak")
			if err := h.Install(opts); err != nil {
				t.Fatalf("second install: %v", err)
			}
			if again, _ := os.ReadFile(settingsPath); !bytes.Equal(again, raw) {
				t.Errorf("second install changed settings.json")
			}
			if b2, _ := filepath.Glob(settingsPath + ".*.bak"); len(b2) != 1 {
				t.Errorf("second install added a settings backup: %v", b2)
			}
			if m2, _ := filepath.Glob(claudeJSONPath + ".*.bak"); len(m2) != len(mcpBaks) {
				t.Errorf("second install added an MCP backup: %v", m2)
			}
		})
	}
}
