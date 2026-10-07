package core

import "time"

// AuditEvent represents a single event entry in a Dossier's audit log.
type AuditEvent struct {
	TS             time.Time `json:"ts"`
	Event          string    `json:"event"`
	DossierID      string    `json:"dossier_id"`
	Actor          string    `json:"actor,omitempty"`
	Author         string    `json:"author,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	BeforeRevision string    `json:"before_revision,omitempty"`
	AfterRevision  string    `json:"after_revision,omitempty"`
	ArtifactsAdded []string  `json:"artifacts_added,omitempty"`
	TokenEstimate  int       `json:"token_estimate,omitempty"`
	Message        string    `json:"message,omitempty"`
	// Version and GuideHash identify the binary and the Distillation Guide +
	// Operating Instructions in force. Set on session_ended and session_eval.
	Version   string       `json:"version,omitempty"`
	GuideHash string       `json:"guide_hash,omitempty"`
	Eval      *EvalSummary `json:"eval,omitempty"`
	// SessionModel and SessionEffort are the session's dominant model and
	// reasoning effort (most main-thread turns), read from its transcript;
	// ModelMix lists every model/effort pair with turn counts. Set on
	// session_ended. Outside Dossier's control, recorded to separate model
	// effects from Guide effects.
	SessionModel  string `json:"session_model,omitempty"`
	SessionEffort string `json:"session_effort,omitempty"`
	ModelMix      string `json:"model_mix,omitempty"`
}

// EvalSummary is the synced outcome of one automatic session eval. Probe
// text, answers and judge reasons stay machine-local; only counts sync.
type EvalSummary struct {
	Model string `json:"model,omitempty"`
	// Effort is the reasoning effort requested for the eval calls; empty
	// means the model's default. Models without effort support ignore it.
	Effort   string                   `json:"effort,omitempty"`
	Revision string                   `json:"revision,omitempty"`
	Probes   int                      `json:"probes"`
	Passed   int                      `json:"passed"`
	ByKind   map[string]EvalKindScore `json:"by_kind,omitempty"`
	CostUSD  float64                  `json:"cost_usd,omitempty"`
	// Skipped names why no score was produced (too large, no transcript,
	// evaluator failure). A skipped eval is recorded, never dropped.
	Skipped string `json:"skipped,omitempty"`
}

// EvalKindScore counts probes of one kind.
type EvalKindScore struct {
	Probes int `json:"probes"`
	Passed int `json:"passed"`
}

// Allowed audit event type constants
const (
	AuditEventCreate                       = "create"
	AuditEventSave                         = "save"
	AuditEventPromote                      = "promote"
	AuditEventLink                         = "link"
	AuditEventMergeStarted                 = "merge_started"
	AuditEventMergeCompleted               = "merge_completed"
	AuditEventMergeConflict                = "merge_conflict"
	AuditEventStatusChanged                = "status_changed"
	AuditEventSlugRenamed                  = "slug_renamed" // legacy slug-only rename event
	AuditEventRenamed                      = "renamed"
	AuditEventArchived                     = "archived"
	AuditEventSnapshotRefreshed            = "snapshot_refreshed"
	AuditEventSnapshotFrozen               = "snapshot_frozen"
	AuditEventAmbiguityConfirmed           = "ambiguity_confirmed"
	AuditEventConflictCreated              = "conflict_created"
	AuditEventConflictResolved             = "conflict_resolved"
	AuditEventTranscriptCaptureUnavailable = "transcript_capture_unavailable"
	AuditEventInstallWarning               = "install_warning"
	AuditEventDistilledStateNotCaptured    = "distilled_state_not_captured"
	AuditEventSessionEnded                 = "session_ended"
	AuditEventSessionEval                  = "session_eval"
)
