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
	AuditEventArchived                     = "archived"
	AuditEventSnapshotRefreshed            = "snapshot_refreshed"
	AuditEventSnapshotFrozen               = "snapshot_frozen"
	AuditEventAmbiguityConfirmed           = "ambiguity_confirmed"
	AuditEventConflictCreated              = "conflict_created"
	AuditEventConflictResolved             = "conflict_resolved"
	AuditEventTranscriptCaptureUnavailable = "transcript_capture_unavailable"
	AuditEventInstallWarning               = "install_warning"
)
