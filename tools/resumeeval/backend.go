package main

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

// Request is one stateless model call.
type Request struct {
	Model  string
	Prompt string
}

// Response is the model's text plus the cost the backend reported (0 if unknown).
type Response struct {
	Text    string
	CostUSD float64
}

// Backend abstracts the model so tests can use a fake.
type Backend interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// ClaudeCLI shells out to `claude -p` in a deliberately sterile environment so
// the user's hooks, MCP servers, CLAUDE.md files and memory cannot contaminate
// the measurement. See README "Isolation".
type ClaudeCLI struct {
	Bin     string
	Bare    bool
	Timeout time.Duration
}

func (c ClaudeCLI) args(model string) []string {
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
	if c.Bare {
		a = append(a, "--bare")
	}
	if model != "" {
		a = append(a, "--model", model)
	}
	return a
}

func (c ClaudeCLI) Complete(ctx context.Context, req Request) (Response, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cwd, err := os.MkdirTemp("", "resumeeval-cwd-*")
	if err != nil {
		return Response{}, fmt.Errorf("temp cwd: %w", err)
	}
	defer os.RemoveAll(cwd)
	home, err := os.MkdirTemp("", "resumeeval-home-*")
	if err != nil {
		return Response{}, fmt.Errorf("temp DOSSIER_HOME: %w", err)
	}
	defer os.RemoveAll(home)

	cmd := exec.CommandContext(ctx, bin, c.args(req.Model)...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"DOSSIER_HOME="+home,
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",
	)
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return Response{}, fmt.Errorf("%s: %w: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	return parseCLIOutput(stdout.Bytes())
}

// parseCLIOutput extracts the result from `--output-format json`.
func parseCLIOutput(b []byte) (Response, error) {
	var out struct {
		Result  string  `json:"result"`
		IsError bool    `json:"is_error"`
		Cost    float64 `json:"total_cost_usd"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return Response{}, fmt.Errorf("parse claude output: %w", err)
	}
	if out.IsError {
		return Response{}, fmt.Errorf("claude reported error: %s", out.Result)
	}
	return Response{Text: out.Result, CostUSD: out.Cost}, nil
}
