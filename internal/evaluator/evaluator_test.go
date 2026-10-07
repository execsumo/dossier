package evaluator

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
)

func TestArgsIsolateTheCall(t *testing.T) {
	args := strings.Join(ClaudeCLI{}.Args("haiku", "high"), " ")
	for _, want := range []string{
		"-p", "--no-session-persistence", "--strict-mcp-config", `{"mcpServers":{}}`,
		`{"disableAllHooks":true}`, "--setting-sources", "--tools", "--disable-slash-commands", "--model haiku", "--effort high",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}
	if bare := strings.Join(ClaudeCLI{}.Args("", ""), " "); strings.Contains(bare, "--model") || strings.Contains(bare, "--effort") {
		t.Error("an empty model or effort must not pass its flag")
	}
}

func TestParseOutput(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantText string
		wantCost float64
		wantErr  bool
	}{
		{"ok", `{"result":"[1]","is_error":false,"total_cost_usd":0.012}`, "[1]", 0.012, false},
		{"error flag", `{"result":"overloaded","is_error":true,"total_cost_usd":0.001}`, "", 0.001, true},
		{"not json", `oops`, "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, cost, err := ParseOutput([]byte(tt.in))
			if (err != nil) != tt.wantErr || text != tt.wantText || cost != tt.wantCost {
				t.Fatalf("got %q %v %v", text, cost, err)
			}
		})
	}
}

func TestCompleteReportsMissingBinary(t *testing.T) {
	_, _, err := ClaudeCLI{Bin: "definitely-not-a-claude-binary"}.Complete(t.Context(), core.EvalCall{Model: "haiku", Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "claude CLI not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestSpawnDetachedRunsAndLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh; the Windows detach flags are covered by GOOS=windows go vet")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	log := filepath.Join(t.TempDir(), "evals", "eval.log")
	if err := SpawnDetached(sh, []string{"-c", "echo spawned-ok"}, log); err != nil {
		t.Fatalf("SpawnDetached: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(log); strings.Contains(string(b), "spawned-ok") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("detached child did not write to the log")
}
