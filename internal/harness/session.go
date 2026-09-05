package harness

import (
	"errors"
	"os"
)

// DefaultSessionID is the shared fallback bucket used by the CLI for manual,
// non-session-scoped invocations where no real harness session exists.
const DefaultSessionID = "sess_default"

// ErrNoSessionID is returned when no session id can be resolved from a caller
// override or the environment. Adapters that must not silently share a binding
// (the MCP server) surface this rather than falling back to DefaultSessionID.
var ErrNoSessionID = errors.New("no session id available: the harness did not provide one and no session override was given. In Pi, install the session bridge with `dossier harness install pi` and restart Pi; otherwise pass an explicit session id or set DOSSIER_SESSION")

// ResolveSessionID determines the per-session binding key for an adapter call,
// keeping internal/core pure (core always takes an explicit SessionID).
//
// Precedence:
//  1. explicit — a caller-supplied session id (MCP session_id param, CLI --session flag).
//  2. CLAUDE_CODE_SESSION_ID — set by Claude Code in each session's process env;
//     verified identical to the transcript UUID and the hook stdin session_id, so a
//     binding written here lines up with what the session-start/end hooks read.
//  3. Pi session pointer — the file the bundled Dossier Pi extension publishes
//     for the owning Pi process, found by walking this process's ancestry. This
//     is the path an MCP server (or any process Pi did not spawn through the
//     bash tool) resolves by. It takes precedence over the inherited environment
//     because it is refreshed on every session start, whereas Pi's spawn env is
//     frozen and shadows live state after a /new or /resume.
//  4. PI_SESSION_ID — set by Pi, but only in the bash tool's spawn environment.
//     Only consulted if no pointer resolves.
//  5. DOSSIER_SESSION — manual / power-user override.
//  6. DefaultSessionID — only when allowDefault is true (CLI manual use).
//
// When allowDefault is false (the MCP path) and none of 1-5 resolve, it returns
// ErrNoSessionID so the adapter can degrade visibly instead of silently binding the
// shared bucket and cross-contaminating concurrent sessions. This preserves the
// "no global active Dossier — binding is per session" invariant.
func ResolveSessionID(explicit string, allowDefault bool) (string, error) {
	id, _, err := ResolveSession(explicit, allowDefault)
	return id, err
}

// ResolveSession resolves the session id and names the harness it came from
// ("claude-code", "pi", or "" when the id came from an explicit override or the
// default bucket). The source lets a binding record which harness a session
// actually ran under, instead of whichever harness happens to be configured on
// the machine.
func ResolveSession(explicit string, allowDefault bool) (id string, harnessName string, err error) {
	if explicit != "" {
		return explicit, "", nil
	}
	if v := os.Getenv("CLAUDE_CODE_SESSION_ID"); v != "" {
		return v, "claude-code", nil
	}
	if p, ok := LookupPiSessionPointer(); ok {
		return p.SessionID, "pi", nil
	}
	if v := os.Getenv("PI_SESSION_ID"); v != "" {
		return v, "pi", nil
	}
	if v := os.Getenv("DOSSIER_SESSION"); v != "" {
		return v, "", nil
	}
	if allowDefault {
		return DefaultSessionID, "", nil
	}
	return "", "", ErrNoSessionID
}
