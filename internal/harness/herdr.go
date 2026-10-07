package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InHerdr reports whether the process runs inside a herdr-managed pane. herdr
// sets HERDR_ENV=1 in every pane it creates.
func InHerdr() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

// herdrTimeout bounds every herdr CLI call. The TUI polls `agent list` every
// couple of seconds, so a wedged herdr must not pile up blocked subprocesses.
const herdrTimeout = 5 * time.Second

// herdrRun executes the herdr CLI and returns its stdout. It is a variable so
// tests can observe the calls without a herdr install.
var herdrRun = func(args ...string) ([]byte, error) {
	bin, err := exec.LookPath("herdr")
	if err != nil {
		return nil, fmt.Errorf("HERDR_ENV=1 but herdr was not found on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), herdrTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("herdr %s: timed out after %s", args[0], herdrTimeout)
		}
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("herdr %s: %w: %s", args[0], err, strings.TrimSpace(string(ee.Stderr)))
		}
		return out, fmt.Errorf("herdr %s: %w", args[0], err)
	}
	return out, nil
}

// ShellLine renders the plan as one POSIX shell command line (what herdr types
// into the new pane's shell). It mirrors Command(): profile env entries override
// the inherited environment, and the agent replaces the pane's shell via exec,
// so herdr closes the tab when the agent exits (docs/harness-capabilities.md §4).
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

// LaunchInHerdrTab opens the handoff in a new herdr tab in the caller's
// workspace, labelled with the Dossier slug, and focuses it (ADR 0014). It
// returns the new pane id once the command has been typed into the tab's shell;
// the agent's lifetime is herdr's concern, not the caller's.
func LaunchInHerdrTab(plan HandoffPlan) (string, error) {
	args := []string{"tab", "create"}
	if ws := os.Getenv("HERDR_WORKSPACE_ID"); ws != "" {
		args = append(args, "--workspace", ws)
	}
	args = append(args, "--cwd", plan.Dir)
	if plan.Slug != "" {
		args = append(args, "--label", plan.Slug)
	}
	args = append(args, "--focus")
	out, err := herdrRun(args...)
	if err != nil {
		return "", err
	}
	var resp struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Result.RootPane.PaneID == "" {
		return "", fmt.Errorf("herdr tab create returned no pane id (output: %q)", strings.TrimSpace(string(out)))
	}
	paneID := resp.Result.RootPane.PaneID
	if _, err := herdrRun("pane", "run", paneID, plan.ShellLine()); err != nil {
		return "", err
	}
	return paneID, nil
}

// HerdrAgent is one live agent as reported by `herdr agent list`.
type HerdrAgent struct {
	PaneID         string `json:"pane_id"`
	TabID          string `json:"tab_id"`
	WorkspaceID    string `json:"workspace_id"`
	Status         string `json:"agent_status"`
	StateChangeSeq int64  `json:"state_change_seq"`
	Session        struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"agent_session"`
}

// SessionKey derives the id Dossier's session bindings are keyed by. Claude
// Code reports the --session-id it was launched with (kind "id"); Pi reports
// its session file, whose name ends in "_<pi-session-id>.jsonl" (kind "path"),
// and Pi bindings are keyed by that id (ADR 0009). Anything that is not a
// plain id is rejected, because the key becomes a file name in the store.
func (a HerdrAgent) SessionKey() string {
	var key string
	switch a.Session.Kind {
	case "id":
		key = a.Session.Value
	case "path":
		base := strings.TrimSuffix(filepath.Base(a.Session.Value), ".jsonl")
		if i := strings.LastIndex(base, "_"); i >= 0 {
			key = base[i+1:]
		}
	}
	if key == "" || strings.Trim(key, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		return ""
	}
	return key
}

// ListHerdrAgents returns every live agent on the herdr server, across all
// workspaces.
func ListHerdrAgents() ([]HerdrAgent, error) {
	out, err := herdrRun("agent", "list")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			Agents []HerdrAgent `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("herdr agent list: unexpected output: %w", err)
	}
	return resp.Result.Agents, nil
}

// FocusHerdrAgent switches herdr to the agent's tab and focuses its pane.
func FocusHerdrAgent(paneID string) error {
	_, err := herdrRun("agent", "focus", paneID)
	return err
}

// MatchHerdrAgents groups live agents by the Dossier they work on. bound maps
// session keys to Dossier ids (from the session bindings); launched maps pane
// ids to Dossier ids for agents this process opened itself, which covers
// harnesses whose reported session id differs from the one Dossier minted. A
// binding match wins over a launched-pane match; unmatched agents are dropped.
func MatchHerdrAgents(agents []HerdrAgent, bound, launched map[string]string) map[string][]HerdrAgent {
	out := map[string][]HerdrAgent{}
	for _, a := range agents {
		dossierID := bound[a.SessionKey()]
		if dossierID == "" {
			dossierID = launched[a.PaneID]
		}
		if dossierID != "" {
			out[dossierID] = append(out[dossierID], a)
		}
	}
	return out
}

// MostRecentAgent picks the agent with the latest state change, which is the
// one `c` focuses when a Dossier has several. agents must not be empty.
func MostRecentAgent(agents []HerdrAgent) HerdrAgent {
	best := agents[0]
	for _, a := range agents[1:] {
		if a.StateChangeSeq > best.StateChangeSeq {
			best = a
		}
	}
	return best
}

// agentStatusRank orders statuses by how much they need the user.
var agentStatusRank = map[string]int{"blocked": 4, "done": 3, "working": 2, "idle": 1}

// MostUrgentStatus returns the status a Dossier's badge shows when it has
// several agents: blocked > done > working > idle > unknown.
func MostUrgentStatus(agents []HerdrAgent) string {
	status := ""
	for _, a := range agents {
		if status == "" || agentStatusRank[a.Status] > agentStatusRank[status] {
			status = a.Status
		}
	}
	if status == "" || agentStatusRank[status] == 0 {
		return "unknown"
	}
	return status
}
