package harness

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
)

// ClaudeBinEnv overrides the Claude Code executable used for a dossier handoff.
// It exists so a non-PATH install (or a test) can point at an exact binary.
const ClaudeBinEnv = "DOSSIER_CLAUDE_BIN"

// ClaudeBin resolves the claude executable: $DOSSIER_CLAUDE_BIN when set,
// otherwise whatever "claude" resolves to on PATH. The error is written for a
// human to act on, because a missing binary must degrade visibly rather than
// leave a caller with a silent no-op.
func ClaudeBin() (string, error) {
	if override := os.Getenv(ClaudeBinEnv); override != "" {
		return override, nil
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude was not found on PATH — install Claude Code, or set %s to its full path", ClaudeBinEnv)
	}
	return path, nil
}

// NewClaudeSessionID mints a fresh RFC 4122 version 4 UUID for a Claude Code
// session that does not exist yet. Claude Code accepts the id via --session-id,
// which is what lets the TUI bind a Dossier to a session before launching it.
func NewClaudeSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("minting claude session id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122

	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

// HandoffPlan is a fully resolved launch of a Claude Code session bound to a
// Dossier. It is a plain value so the decision of *what* to run is testable
// without running anything.
type HandoffPlan struct {
	SessionID string
	Bin       string
	Args      []string
	Dir       string
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
	prompt := fmt.Sprintf(
		"Resume the Dossier %q (slug: %s). Call dossier_session with id %q to bind it and load its distilled state; if the dossier MCP tools are unavailable, read ./dossier.md in this directory instead.",
		name, slug, slug,
	)
	return HandoffPlan{
		SessionID: sessionID,
		Bin:       bin,
		Args:      []string{"--session-id", sessionID, prompt},
		Dir:       dossierDir,
	}
}

// Command materializes the plan as an *exec.Cmd rooted in the Dossier directory,
// inheriting the caller's environment. Stdio is left to the caller: the TUI hands
// the command to tea.ExecProcess, the CLI attaches its own.
func (p HandoffPlan) Command() *exec.Cmd {
	cmd := exec.Command(p.Bin, p.Args...)
	cmd.Dir = p.Dir
	return cmd
}
