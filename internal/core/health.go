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

// HealthSummaryFromDoctor projects the Doctor result without performing I/O.
func HealthSummaryFromDoctor(report DoctorReport) HealthSummary {
	h := HealthSummary{
		TeamSyncConfigured: report.SyncConfigured,
		Conflicts:          report.ConflictsFound,
		Issues:             len(report.Issues),
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
