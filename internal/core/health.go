package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// HealthSummary is the small, surface-neutral health view shared by CLI and TUI.
// It contains only values already collected by Doctor; formatting is centralized
// in Line so adapters cannot disagree.
type HealthSummary struct {
	TeamSyncConfigured bool
	LastSuccess        time.Time
	LastAttemptFailed  bool
	LastAttempt        time.Time
	LastError          string
	AuthState          string
	LocalChanges       int
	Conflicts          int
	Issues             int
}

// HealthReport combines the canonical one-line summary with the full doctor
// report used by the TUI health overlay.
type HealthReport struct {
	Summary HealthSummary `json:"summary"`
	Doctor  DoctorReport  `json:"doctor"`
}

// HealthSummaryFromSyncStatus projects local sync state without performing I/O.
func HealthSummaryFromSyncStatus(status SyncStatus, conflicts int) HealthSummary {
	h := HealthSummary{
		TeamSyncConfigured: true,
		LastSuccess:        latestTime(status.LastSuccessPull, status.LastSuccessPush),
		LastAttemptFailed:  status.LastError != "",
		LastAttempt:        status.LastAttempt,
		LastError:          status.LastError,
		AuthState:          status.AuthState,
		LocalChanges:       status.Dirty + status.Ahead,
		Conflicts:          conflicts,
	}
	return h
}

// HealthSummaryFromDoctor projects the Doctor result without performing I/O.
// Doctor also records each unresolved conflict as an issue, so that duplicate
// signal is excluded from the general issue count.
func HealthSummaryFromDoctor(report DoctorReport) HealthSummary {
	issues := len(report.Issues) - report.ConflictsFound
	if issues < 0 {
		issues = 0
	}
	h := HealthSummary{
		TeamSyncConfigured: report.SyncConfigured,
		Conflicts:          report.ConflictsFound,
		Issues:             issues,
	}
	if report.SyncStatus == nil {
		return h
	}
	st := report.SyncStatus
	h.LastSuccess = latestTime(st.LastSuccessPull, st.LastSuccessPush)
	h.LastAttempt = st.LastAttempt
	h.LastError = st.LastError
	h.LastAttemptFailed = st.LastError != ""
	h.AuthState = st.AuthState
	h.LocalChanges = st.Dirty + st.Ahead
	return h
}

// Line returns the canonical health footer used by every adapter.
func (h HealthSummary) Line(now time.Time) string {
	if !h.TeamSyncConfigured {
		if h.Issues == 0 {
			return "Store · healthy"
		}
		return fmt.Sprintf("Store · %d %s", h.Issues, plural(h.Issues, "issue", "issues"))
	}

	parts := []string{"Team sync"}
	switch {
	case h.AuthState == "missing":
		parts = append(parts, "no credentials found")
	case h.LastAttemptFailed:
		failed := "last sync failed"
		if !h.LastAttempt.IsZero() {
			failed += " " + age(h.LastAttempt, now)
		}
		parts = append(parts, failed, "work is safe locally")
	case h.LastSuccess.IsZero():
		parts = append(parts, "never synced")
	default:
		parts = append(parts, "synced "+age(h.LastSuccess, now))
	}
	if h.LocalChanges > 0 {
		parts = append(parts, fmt.Sprintf("%d local %s", h.LocalChanges, plural(h.LocalChanges, "change", "changes")))
	}
	if h.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", h.Conflicts, plural(h.Conflicts, "conflict", "conflicts")))
	}
	if h.Issues > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", h.Issues, plural(h.Issues, "issue", "issues")))
	}
	return strings.Join(parts, " · ")
}

// Health runs Doctor and returns the same data plus its canonical summary.
func (s *Service) Health(ctx context.Context) (Result, error) {
	res, err := s.Doctor(ctx)
	if err != nil {
		return res, err
	}
	report, ok := res.Data.(DoctorReport)
	if !ok {
		return Result{OK: false}, fmt.Errorf("doctor returned unexpected data type")
	}
	summary := HealthSummaryFromDoctor(report)
	return Result{OK: res.OK, Data: HealthReport{Summary: summary, Doctor: report}, Warnings: res.Warnings}, nil
}

func latestTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func age(t, now time.Time) string {
	if now.Before(t) {
		return "just now"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// LocalHealthSummary returns sync health from persisted state and local refs
// without contacting the remote.
func (s *Service) LocalHealthSummary(ctx context.Context) (HealthSummary, error) {
	summary, _, err := s.localHealth(ctx)
	return summary, err
}

func (s *Service) localHealth(ctx context.Context) (HealthSummary, []Conflict, error) {
	statuser, ok := s.syncer.(LocalSyncStatuser)
	if !ok {
		return HealthSummary{}, nil, fmt.Errorf("local sync status is unavailable")
	}
	status, err := statuser.LocalStatus(ctx)
	if err != nil {
		return HealthSummary{}, nil, err
	}
	conflicts, err := s.ListConflicts(ctx)
	if err != nil {
		return HealthSummary{}, nil, err
	}
	return HealthSummaryFromSyncStatus(status, len(conflicts)), conflicts, nil
}

// SyncAttention checks health and lists conflicts, returning a warning line and
// the bound Dossier's conflict IDs if attention is needed. It returns ok=false
// if no attention is needed (healthy state) or if team sync is unconfigured.
func (s *Service) SyncAttention(ctx context.Context, dossierID string) (string, []string, bool) {
	return s.syncAttention(ctx, dossierID, nil, nil)
}

func (s *Service) syncAttention(ctx context.Context, dossierID string, syncResult *Result, syncErr error) (string, []string, bool) {
	summary, conflicts, err := s.localHealth(ctx)
	if err != nil {
		return "", nil, false
	}

	var boundConflicts []string
	for _, conflict := range conflicts {
		if dossierID != "" && conflict.DossierID == dossierID {
			boundConflicts = append(boundConflicts, conflict.ID)
		}
	}

	syncHadConflicts := false
	if syncErr != nil {
		summary.LastAttemptFailed = true
		summary.LastError = syncErr.Error()
		if summary.LastAttempt.IsZero() {
			summary.LastAttempt = s.clock.Now()
		}
	} else if syncResult != nil {
		if syncReport, ok := syncResult.Data.(SyncReport); ok {
			syncHadConflicts = len(syncReport.Conflicts) > 0
			if syncReport.Error != "" {
				summary.LastAttemptFailed = true
				summary.LastError = syncReport.Error
				if summary.LastAttempt.IsZero() {
					summary.LastAttempt = s.clock.Now()
				}
			}
		}
		if !syncResult.OK {
			summary.LastAttemptFailed = true
			if summary.LastAttempt.IsZero() {
				summary.LastAttempt = s.clock.Now()
			}
		}
	}

	needsAttention := summary.LastAttemptFailed ||
		summary.AuthState == "missing" ||
		summary.AuthState == "rejected" ||
		syncHadConflicts
	if dossierID != "" {
		needsAttention = needsAttention || len(boundConflicts) > 0
	} else {
		needsAttention = needsAttention || len(conflicts) > 0
	}
	if !needsAttention {
		return "", nil, false
	}

	// Keep the canonical HealthSummary sentence intact; adapters only add the
	// surface-specific placement and conflict hint.
	return summary.Line(s.clock.Now()) + " — tell the user; run dossier sync to see why.", boundConflicts, true
}
