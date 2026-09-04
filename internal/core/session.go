package core

import "time"

// Capabilities defines the integration extension points supported by a harness.
type Capabilities struct {
	MCP               bool `json:"mcp"`
	SessionStartHook  bool `json:"session_start_hook"`
	SessionEndHook    bool `json:"session_end_hook"`
	PreCompactionHook bool `json:"pre_compaction_hook"`
	TranscriptCapture bool `json:"transcript_capture"`

	// Installed reports that the harness is present on this device, whether or
	// not the current process is running inside one of its sessions. It is what
	// makes an integration installable ahead of first use.
	Installed bool `json:"installed"`

	// SessionIdentity reports that Dossier can resolve a per-session id for this
	// harness: the harness exposes one to Dossier processes right now, or the
	// bridge that will expose it (for Pi, the bundled extension) is installed.
	// Without it Dossier refuses to bind a Dossier rather than share the default
	// bucket across concurrent sessions.
	SessionIdentity bool `json:"session_identity"`
}

// LiveSession reports whether the harness is offering a session surface in this
// process — the predicate for "which harness is this session running under".
func (c Capabilities) LiveSession() bool {
	return c.MCP || c.SessionStartHook || c.SessionEndHook || c.PreCompactionHook || c.TranscriptCapture
}

// Present reports whether the harness is usable or installable at all, which is
// broader than LiveSession: Pi installed on the device but not currently running
// is present, not live.
func (c Capabilities) Present() bool {
	return c.Installed || c.SessionIdentity || c.LiveSession()
}

// SessionBinding records which Dossier is currently active in a specific harness session.
type SessionBinding struct {
	SessionBindingID string       `json:"session_binding_id"`
	Harness          string       `json:"harness"`
	DossierID        string       `json:"dossier_id"`
	BoundAt          time.Time    `json:"bound_at"`
	LastSeenRevision string       `json:"last_seen_revision"`
	Capabilities     Capabilities `json:"capabilities"`

	// GuideDeliveredAt records when the Distillation Guide was last emitted into
	// this session's context, so the two delivery paths — the session-start hook
	// and the dossier_session response — don't both spend ~3.5k tokens on the
	// same text within one session. Zero means undelivered.
	//
	// This must stay a per-session record rather than a per-bind one: it is reset
	// by a fresh session-start (including the one that fires after compaction,
	// where the previous copy was just evicted from context) and carried across a
	// Switch (the Guide is dossier-independent, so re-binding earns no re-send).
	GuideDeliveredAt time.Time `json:"guide_delivered_at,omitzero"`
}
