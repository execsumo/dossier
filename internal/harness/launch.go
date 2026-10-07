package harness

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dossier/internal/core"
)

// ClaudeBinEnv overrides the Claude Code executable used for a dossier handoff.
// It exists so a non-PATH install (or a test) can point at an exact binary.
const ClaudeBinEnv = "DOSSIER_CLAUDE_BIN"

// CursorBinEnv overrides the Cursor Agent executable used for a dossier handoff.
const CursorBinEnv = "DOSSIER_CURSOR_BIN"

const DefaultOpenWith = "claude-code"

// LaunchRequest contains the dossier context needed to construct an agent
// handoff. The profile supplies the agent-specific command and arguments.
type LaunchRequest struct {
	SessionID  string
	DossierDir string
	Name       string
	Slug       string
	Headless   bool
	// WorkDir is where the agent starts: the Dossier's resolved primary repo,
	// or empty for the Dossier directory (ADR 0015).
	WorkDir string
	// Repos are the Dossier's repos with their local paths where resolved.
	Repos []core.ResolvedRepo
}

// ClaudeBin resolves the claude executable: $DOSSIER_CLAUDE_BIN when set,
// otherwise whatever "claude" resolves to on PATH. The error is written for a
// human to act on, because a missing binary must degrade visibly rather than
// leave a caller with a silent no-op.
func ClaudeBin() (string, error) {
	if override := os.Getenv(ClaudeBinEnv); override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return "", fmt.Errorf("%s=%q (%s) is not an executable file: %w; set %s to the Claude Code executable or unset it to use PATH", ClaudeBinEnv, override, override, err, ClaudeBinEnv)
		}
		return path, nil
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude was not found on PATH — install Claude Code, or set %s to its full path", ClaudeBinEnv)
	}
	return path, nil
}

// CursorBin resolves the Cursor Agent CLI. Cursor's current terminal agent
// executable is named cursor-agent; the profile itself remains "cursor" so the
// config names the product rather than its implementation detail.
func CursorBin() (string, error) {
	if override := os.Getenv(CursorBinEnv); override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return "", fmt.Errorf("%s=%q (%s) is not an executable file: %w; set %s to the Cursor Agent executable or unset it to use PATH", CursorBinEnv, override, override, err, CursorBinEnv)
		}
		return path, nil
	}
	path, err := exec.LookPath("cursor-agent")
	if err != nil {
		return "", fmt.Errorf("cursor-agent was not found on PATH — install Cursor Agent, or set %s to its full path", CursorBinEnv)
	}
	return path, nil
}

func pathBin(name, display string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s was not found on PATH — install %s first", name, display)
	}
	return path, nil
}

// NewSessionID mints a fresh RFC 4122 version 4 UUID for a newly launched agent
// session. Claude Code accepts it via --session-id; other profiles carry it
// through their supported environment/session mechanism.
func NewSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("minting agent session id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122

	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

// NewClaudeSessionID is retained for callers that used the Claude-specific
// name before launch profiles were configurable.
func NewClaudeSessionID() (string, error) {
	return NewSessionID()
}

// NormalizeOpenWith canonicalizes user-facing launcher names.
func NormalizeOpenWith(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "claude", "claude-code":
		return "claude-code", nil
	case "cursor":
		return "cursor", nil
	case "codex":
		return "codex", nil
	case "pi":
		return "pi", nil
	case "agy", "antigravity":
		return "antigravity", nil
	default:
		return "", fmt.Errorf("unknown open_with %q (choose claude-code, cursor, codex, pi, or antigravity)", name)
	}
}

// HandoffPlan is a fully resolved launch of an agent session bound to a Dossier.
// It is a plain value so the decision of what to run is testable without
// running anything.
type HandoffPlan struct {
	SessionID string
	Bin       string
	Args      []string
	Dir       string
	Env       []string
	// Slug names the Dossier; herdr launches use it to label the workspace and tab.
	Slug string
}

// PlanOpenWith resolves the configured launcher and builds its handoff.
// Prompts remain profile-owned code, not user configuration.
func PlanOpenWith(name string, req LaunchRequest) (HandoffPlan, error) {
	canonical, err := NormalizeOpenWith(name)
	if err != nil {
		return HandoffPlan{}, err
	}
	if req.SessionID == "" {
		return HandoffPlan{}, fmt.Errorf("launch session id must not be empty")
	}
	if req.DossierDir == "" {
		return HandoffPlan{}, fmt.Errorf("launch dossier directory must not be empty")
	}

	switch canonical {
	case "claude-code":
		bin, err := ClaudeBin()
		if err != nil {
			return HandoffPlan{}, err
		}
		plan := applyRepos(PlanClaudeHandoff(bin, req.SessionID, req.DossierDir, req.Name, req.Slug), req, true)
		if req.Headless {
			plan.Args = append([]string{"--print"}, plan.Args...)
			plan.Env = []string{"DOSSIER_LEAN_SESSION_START=1"}
		}
		return plan, nil
	case "cursor":
		bin, err := CursorBin()
		if err != nil {
			return HandoffPlan{}, err
		}
		return applyRepos(PlanCursorHandoff(bin, req.SessionID, req.DossierDir, req.Name, req.Slug), req, false), nil
	case "codex":
		bin, err := pathBin("codex", "Codex")
		if err != nil {
			return HandoffPlan{}, err
		}
		return applyRepos(PlanPromptHandoff(bin, req.SessionID, req.DossierDir, req.Name, req.Slug, genericLaunchEnv(req.SessionID)), req, false), nil
	case "pi":
		bin, err := pathBin("pi", "Pi")
		if err != nil {
			return HandoffPlan{}, err
		}
		return applyRepos(PlanPromptHandoff(bin, req.SessionID, req.DossierDir, req.Name, req.Slug, []string{
			"PI_SESSION_ID=",
			"PI_SESSION_FILE=",
			"CLAUDE_CODE_SESSION_ID=",
		}), req, false), nil
	case "antigravity":
		bin, err := pathBin("agy", "Antigravity (agy)")
		if err != nil {
			return HandoffPlan{}, err
		}
		return applyRepos(PlanPromptHandoffWithPrefix(bin, []string{"--prompt-interactive"}, req.SessionID, req.DossierDir, req.Name, req.Slug, genericLaunchEnv(req.SessionID)), req, false), nil
	default:
		return HandoffPlan{}, fmt.Errorf("open_with profile %q is not implemented", canonical)
	}
}

// PlanClaudeHandoff builds the launch for a Claude Code session that should come
// up already working on the given Dossier. Pure: no filesystem, no exec.
//
// The session id is passed through --session-id so the session-start hook fires
// with the id the caller already bound, which injects the Distilled State. The
// prompt is a belt-and-braces path for installs that have the MCP server but no
// hooks (or neither) — there, the binding alone would inject nothing.
func PlanClaudeHandoff(bin, sessionID, dossierDir, name, slug string) HandoffPlan {
	// The MCP tool takes the slug in its "id" parameter (it accepts a slug or an
	// id). Naming the parameter matters: dossier_session ignores unknown fields,
	// so a call with a "slug" key would silently return the *active* Dossier
	// instead of binding this one.
	prompt := ResumePrompt(LaunchRequest{DossierDir: dossierDir, Name: name, Slug: slug})
	return HandoffPlan{
		SessionID: sessionID,
		Bin:       bin,
		Args:      []string{"--session-id", sessionID, prompt},
		Dir:       dossierDir,
	}
}

// PlanCursorHandoff builds a Cursor Agent CLI handoff. Cursor does not expose
// Claude's --session-id flag, so Dossier passes the binding id through the
// explicit DOSSIER_SESSION override inherited by Cursor's MCP child process.
func PlanCursorHandoff(bin, sessionID, dossierDir, name, slug string) HandoffPlan {
	return PlanPromptHandoff(bin, sessionID, dossierDir, name, slug, genericLaunchEnv(sessionID))
}

func genericLaunchEnv(sessionID string) []string {
	return []string{
		"DOSSIER_SESSION=" + sessionID,
		// Do not let a parent agent's identity win over the handoff id.
		"CLAUDE_CODE_SESSION_ID=",
		"PI_SESSION_ID=",
		"PI_SESSION_FILE=",
	}
}

// PlanPromptHandoff builds a prompt-positional CLI handoff. This is the common
// shape used by Cursor Agent, Codex, and Pi; profile-specific environment
// entries are supplied by the caller where needed.
func PlanPromptHandoff(bin, sessionID, dossierDir, name, slug string, env []string) HandoffPlan {
	return PlanPromptHandoffWithPrefix(bin, nil, sessionID, dossierDir, name, slug, env)
}

// PlanPromptHandoffWithPrefix builds a prompt handoff for CLIs that require
// flags before the initial prompt, such as Antigravity's -i mode.
func PlanPromptHandoffWithPrefix(bin string, prefix []string, sessionID, dossierDir, name, slug string, env []string) HandoffPlan {
	prompt := ResumePrompt(LaunchRequest{DossierDir: dossierDir, Name: name, Slug: slug})
	args := append([]string{}, prefix...)
	args = append(args, prompt)
	return HandoffPlan{
		SessionID: sessionID,
		Bin:       bin,
		Args:      args,
		Dir:       dossierDir,
		Env:       env,
	}
}

// ResumePrompt is the first message of a launched session. It names the Dossier
// by absolute path, because with a resolved repo the agent starts in the repo,
// where "./dossier.md" would be the wrong file (ADR 0015). With a repo, project
// work goes in the repo under its own CLAUDE.md/AGENTS.md and other deliverables
// go in the Dossier's files/; ## Files records repo files as identity:path, never
// as a machine-local absolute path, because the Dossier may be shared.
func ResumePrompt(req LaunchRequest) string {
	var b strings.Builder
	// The MCP tool takes the slug in its "id" parameter (it accepts a slug or an
	// id). Naming the parameter matters: dossier_session ignores unknown fields,
	// so a call with a "slug" key would silently return the *active* Dossier
	// instead of binding this one.
	fmt.Fprintf(&b, "Resume the Dossier %q (slug: %s). Call dossier_session with id %q to bind it and load its distilled state; if the dossier MCP tools are unavailable, read %s instead.",
		req.Name, req.Slug, req.Slug, filepath.Join(req.DossierDir, "dossier.md"))
	files := filepath.Join(req.DossierDir, "files") + string(filepath.Separator)
	inRepo := req.WorkDir != "" && req.WorkDir != req.DossierDir && len(req.Repos) > 0 && req.Repos[0].Path == req.WorkDir
	if !inRepo {
		fmt.Fprintf(&b, " Save any files you produce (decks, HTML, documents) to %s and list them under ## Files in the distilled state.", files)
	} else {
		primary := req.Repos[0]
		fmt.Fprintf(&b, " You are working in the repo %s at %s; follow its own instructions (CLAUDE.md / AGENTS.md).", primary.Identity, primary.Path)
		fmt.Fprintf(&b, " Save project work (code, docs, assets the repo keeps) in the repo; save other deliverables (decks, analyses, briefs) to %s.", files)
		b.WriteString(" List both under ## Files in the distilled state: repo files as <repo>:<path relative to the repo> (e.g. " + core.RepoRef(primary.Identity, "docs/plan.md") + "), never as absolute paths; Dossier files relative to the Dossier.")
	}
	var others []string
	for i, r := range req.Repos {
		if inRepo && i == 0 {
			continue
		}
		if r.Path != "" {
			others = append(others, fmt.Sprintf("%s at %s", r.Identity, r.Path))
		} else {
			others = append(others, fmt.Sprintf("%s (not on this machine)", r.Identity))
		}
	}
	if len(others) > 0 {
		label := "Other repos"
		if !inRepo {
			label = "Repos"
		}
		fmt.Fprintf(&b, " %s for this Dossier: %s.", label, strings.Join(others, "; "))
	}
	return b.String()
}

// applyRepos re-roots a plan for a Dossier with repos: the agent starts in the
// resolved primary repo, the prompt carries the repo guidance, and Claude Code
// is given --add-dir for the Dossier folder so writing files/ needs no extra
// permission prompt. Plans for Dossiers without repos are returned unchanged.
func applyRepos(plan HandoffPlan, req LaunchRequest, claude bool) HandoffPlan {
	if len(req.Repos) == 0 && (req.WorkDir == "" || req.WorkDir == req.DossierDir) {
		return plan
	}
	if req.WorkDir != "" {
		plan.Dir = req.WorkDir
	}
	args := append([]string{}, plan.Args...)
	args[len(args)-1] = ResumePrompt(req)
	if claude && plan.Dir != req.DossierDir {
		args = append(args[:len(args)-1:len(args)-1], "--add-dir", req.DossierDir, args[len(args)-1])
	}
	plan.Args = args
	return plan
}

// Command materializes the plan as an *exec.Cmd rooted in the Dossier directory,
// inheriting the caller's environment. Stdio is left to the caller: the TUI hands
// the command to tea.ExecProcess, the CLI attaches its own.
func (p HandoffPlan) Command() *exec.Cmd {
	cmd := exec.Command(p.Bin, p.Args...)
	cmd.Dir = p.Dir
	if len(p.Env) > 0 {
		env := os.Environ()
		for _, replacement := range p.Env {
			key, _, ok := strings.Cut(replacement, "=")
			if !ok {
				continue
			}
			filtered := env[:0]
			for _, existing := range env {
				existingKey, _, existingOK := strings.Cut(existing, "=")
				if !existingOK || existingKey != key {
					filtered = append(filtered, existing)
				}
			}
			env = append(filtered, replacement)
		}
		cmd.Env = env
	}
	return cmd
}
