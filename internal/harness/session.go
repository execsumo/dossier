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
var ErrNoSessionID = errors.New("no session id available: harness did not provide a session ID and no session override was provided")

// ResolveSessionID determines the per-session binding key for an adapter call,
// keeping internal/core pure (core always takes an explicit SessionID).
//
// Precedence:
//  1. explicit — a caller-supplied session id (MCP session_id param, CLI --session flag).
//  2. CLAUDE_CODE_SESSION_ID — set by Claude Code in each session's process env;
//     verified identical to the transcript UUID and the hook stdin session_id, so a
//     binding written here lines up with what the session-start/end hooks read.
//  3. PI_SESSION_ID — set by Pi for commands/extensions in the active session.
//  4. DOSSIER_SESSION — manual / power-user override.
//  5. DefaultSessionID — only when allowDefault is true (CLI manual use).
//
// When allowDefault is false (the MCP path) and none of 1-4 resolve, it returns
// ErrNoSessionID so the adapter can degrade visibly instead of silently binding the
// shared bucket and cross-contaminating concurrent sessions. This preserves the
// "no global active Dossier — binding is per session" invariant.
func ResolveSessionID(explicit string, allowDefault bool) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv("CLAUDE_CODE_SESSION_ID"); v != "" {
		return v, nil
	}
	if v := os.Getenv("PI_SESSION_ID"); v != "" {
		return v, nil
	}
	if v := os.Getenv("DOSSIER_SESSION"); v != "" {
		return v, nil
	}
	if allowDefault {
		return DefaultSessionID, nil
	}
	return "", ErrNoSessionID
}
