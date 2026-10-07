// Package evaluator is the driven adapter behind core.Evaluator: it runs the
// model calls of automatic session evals (ADR 0016) through the Claude Code
// CLI, isolated from the user's harness configuration.
package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCLI runs `claude -p` in a sterile environment: no hooks (so an eval
// can never fire Dossier's own SessionStart/SessionEnd and recurse), no MCP
// servers, no settings sources, CLAUDE.md files or memory, no tools, no session
// persistence, and an empty working directory and DOSSIER_HOME. The flag set
// was verified empirically for tools/resumeeval (see its README, "Isolation").
type ClaudeCLI struct {
	// Bin is the claude executable; empty means "claude" on PATH.
	Bin string
	// Timeout bounds one call; zero means 10 minutes.
	Timeout time.Duration
}

// Args returns the isolated argument list for one call.
func (c ClaudeCLI) Args(model string) []string {
	a := []string{
		"-p",
		"--output-format", "json",
		"--no-session-persistence",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--settings", `{"disableAllHooks":true}`,
		"--setting-sources", "",
		"--tools", "",
		"--disable-slash-commands",
		"--system-prompt", "You are a careful assistant. Follow the user's instructions exactly.",
	}
	if model != "" {
		a = append(a, "--model", model)
	}
	return a
}

// Complete implements core.Evaluator.
func (c ClaudeCLI) Complete(ctx context.Context, model, prompt string) (string, float64, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", 0, fmt.Errorf("claude CLI not found (%s): %w", bin, err)
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cwd, err := os.MkdirTemp("", "dossier-eval-cwd-*")
	if err != nil {
		return "", 0, fmt.Errorf("temp cwd: %w", err)
	}
	defer os.RemoveAll(cwd)
	home, err := os.MkdirTemp("", "dossier-eval-home-*")
	if err != nil {
		return "", 0, fmt.Errorf("temp DOSSIER_HOME: %w", err)
	}
	defer os.RemoveAll(home)

	cmd := exec.CommandContext(ctx, path, c.Args(model)...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"DOSSIER_HOME="+home,
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",
	)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return "", 0, fmt.Errorf("claude: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseOutput(stdout.Bytes())
}

// ParseOutput extracts the result text and cost from `--output-format json`.
func ParseOutput(b []byte) (string, float64, error) {
	var out struct {
		Result  string  `json:"result"`
		IsError bool    `json:"is_error"`
		Cost    float64 `json:"total_cost_usd"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", 0, fmt.Errorf("parse claude output: %w", err)
	}
	if out.IsError {
		return "", out.Cost, fmt.Errorf("claude reported an error: %s", out.Result)
	}
	return out.Result, out.Cost, nil
}
