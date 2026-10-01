package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InHerdr reports whether the process runs inside a herdr-managed pane. herdr
// sets HERDR_ENV=1 in every pane it creates.
func InHerdr() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

// herdrRun executes the herdr CLI and returns its stdout. It is a variable so
// tests can observe the calls without a herdr install.
var herdrRun = func(args ...string) ([]byte, error) {
	bin, err := exec.LookPath("herdr")
	if err != nil {
		return nil, fmt.Errorf("HERDR_ENV=1 but herdr was not found on PATH: %w", err)
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("herdr %s: %w: %s", args[0], err, strings.TrimSpace(string(ee.Stderr)))
		}
		return out, fmt.Errorf("herdr %s: %w", args[0], err)
	}
	return out, nil
}

// ShellLine renders the plan as one POSIX shell command line (what herdr types
// into the new pane's shell). It mirrors Command(): profile env entries override
// the inherited environment, and the agent replaces the pane's shell via exec.
func (p HandoffPlan) ShellLine() string {
	parts := []string{"exec"}
	if len(p.Env) > 0 {
		parts = append(parts, "env")
		for _, e := range p.Env {
			key, val, ok := strings.Cut(e, "=")
			if !ok {
				continue
			}
			parts = append(parts, key+"="+shellQuote(val))
		}
	}
	parts = append(parts, shellQuote(p.Bin))
	for _, a := range p.Args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// LaunchInHerdr opens the handoff in a new split pane to the right of the
// current one and focuses it. It returns once the command has been typed into
// the pane; the agent's lifetime is herdr's concern, not the caller's.
func LaunchInHerdr(plan HandoffPlan) error {
	out, err := herdrRun("pane", "split", "--current", "--direction", "right", "--cwd", plan.Dir, "--focus")
	if err != nil {
		return err
	}
	var resp struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Result.Pane.PaneID == "" {
		return fmt.Errorf("herdr pane split returned no pane id (output: %q)", strings.TrimSpace(string(out)))
	}
	if _, err := herdrRun("pane", "run", resp.Result.Pane.PaneID, plan.ShellLine()); err != nil {
		return err
	}
	labelHerdr(plan.Slug)
	return nil
}

// labelHerdr renames the caller's herdr workspace and tab to the Dossier slug so
// they stop reflecting whatever folder the pane started in. Purely cosmetic and
// best-effort: the agent is already running, so a failed rename is ignored.
func labelHerdr(slug string) {
	if slug == "" {
		return
	}
	if ws := os.Getenv("HERDR_WORKSPACE_ID"); ws != "" {
		_, _ = herdrRun("workspace", "rename", ws, slug)
	}
	if tab := os.Getenv("HERDR_TAB_ID"); tab != "" {
		_, _ = herdrRun("tab", "rename", tab, slug)
	}
}
