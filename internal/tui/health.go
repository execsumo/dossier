package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const healthTimeout = 10 * time.Second

func healthTick() tea.Cmd {
	return tea.Tick(time.Minute, func(time.Time) tea.Msg { return healthTickMsg{} })
}

func (m Model) healthCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
		defer cancel()
		res, err := m.svc.Health(ctx)
		if err != nil {
			return healthMsg{err: err}
		}
		report, ok := res.Data.(core.HealthReport)
		if !ok {
			return healthMsg{err: fmt.Errorf("invalid health data type")}
		}
		return healthMsg{summary: report.Summary, report: report.Doctor, warnings: res.Warnings}
	}
}

func (m *Model) openHealth() {
	m.healthViewport.SetContent(renderDoctorReport(m.healthSummary, m.healthReport))
	m.healthViewport.GotoTop()
	m.recalculateHealthViewportLayout()
	m.pushOverlay(ViewHealth)
}

func renderDoctorReport(summary core.HealthSummary, report core.DoctorReport) string {
	var b strings.Builder
	b.WriteString("Health: ")
	b.WriteString(summary.Line(time.Now()))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Checked: %d dossiers, %d artifacts, %d audit logs\n", report.DossiersChecked, report.ArtifactsChecked, report.AuditLogsChecked)
	if report.SyncConfigured {
		b.WriteString("\nTeam Sync Status:\n")
		if report.SyncStatus == nil {
			b.WriteString("  Status unavailable\n")
		} else {
			st := report.SyncStatus
			fmt.Fprintf(&b, "  Last attempt: %s\n", formatHealthTime(st.LastAttempt))
			fmt.Fprintf(&b, "  Last pull: %s\n", formatHealthTime(st.LastSuccessPull))
			fmt.Fprintf(&b, "  Last push: %s\n", formatHealthTime(st.LastSuccessPush))
			if st.LastError != "" {
				fmt.Fprintf(&b, "  Last error: %s\n", st.LastError)
			}
			fmt.Fprintf(&b, "  Auth state: %s\n  Ahead: %d, Behind: %d\n  Dirty: %d\n  Unresolved conflicts: %d\n", st.AuthState, st.Ahead, st.Behind, st.Dirty, st.ConflictsFound)
		}
	}
	if len(report.Issues) > 0 {
		b.WriteString("\nIssues:\n")
		for _, issue := range report.Issues {
			b.WriteString("- ")
			b.WriteString(issue)
			b.WriteByte('\n')
		}
	} else {
		b.WriteString("\nNo issues found.\n")
	}
	return b.String()
}

func formatHealthTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

func (m Model) healthFooterLine() string {
	if !m.healthReady {
		return "Checking health…"
	}
	return m.healthSummary.Line(time.Now())
}

func truncateHealthLine(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	// Keep failure/conflict signal ahead of optional reassurance/counts on narrow
	// terminals; the footer's ordinary truncation may otherwise hide the reason.
	if strings.Contains(line, "last sync failed") && strings.Contains(line, "conflict") {
		line = strings.Replace(line, " · work is safe locally", "", 1)
	}
	return truncateCell(line, width)
}
