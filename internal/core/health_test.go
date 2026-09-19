package core_test

import (
	"testing"
	"time"

	"dossier/internal/core"
)

func TestHealthSummaryLine(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	threeMinutesAgo := now.Add(-3 * time.Minute)
	eighteenMinutesAgo := now.Add(-18 * time.Minute)
	tests := []struct {
		name string
		h    core.HealthSummary
		want string
	}{
		{"local healthy", core.HealthSummary{}, "Store · healthy"},
		{"local issues", core.HealthSummary{Issues: 2}, "Store · 2 issues"},
		{"never synced", core.HealthSummary{TeamSyncConfigured: true}, "Team sync · never synced"},
		{"synced", core.HealthSummary{TeamSyncConfigured: true, LastSuccess: threeMinutesAgo, LocalChanges: 1, Conflicts: 1, Issues: 2}, "Team sync · synced 3m ago · 1 local change · 1 conflict · 2 issues"},
		{"failed", core.HealthSummary{TeamSyncConfigured: true, LastAttemptFailed: true, LastAttempt: eighteenMinutesAgo, Conflicts: 1}, "Team sync · last sync failed 18m ago · work is safe locally · 1 conflict"},
		{"missing credentials", core.HealthSummary{TeamSyncConfigured: true, AuthState: "missing"}, "Team sync · no credentials found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.Line(now); got != tt.want {
				t.Fatalf("Line() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHealthSummaryFromDoctor(t *testing.T) {
	pull := time.Date(2026, 9, 18, 11, 0, 0, 0, time.UTC)
	report := core.DoctorReport{
		ConflictsFound: 1,
		Issues:         []string{"bad dossier"},
		SyncConfigured: true,
		SyncStatus: &core.SyncStatusData{
			Ahead:           2,
			Dirty:           1,
			LastSuccessPull: pull,
			AuthState:       "file",
		},
	}
	h := core.HealthSummaryFromDoctor(report)
	if h.LastSuccess != pull || h.LocalChanges != 3 || h.Conflicts != 1 || h.Issues != 1 {
		t.Fatalf("unexpected summary: %+v", h)
	}
}
