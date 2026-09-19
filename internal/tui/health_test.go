package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

type blockingHealthSyncer struct{}

func (blockingHealthSyncer) Sync(context.Context) (core.SyncReport, error) {
	return core.SyncReport{}, nil
}
func (blockingHealthSyncer) Status(context.Context) (core.SyncStatus, error) {
	time.Sleep(200 * time.Millisecond)
	return core.SyncStatus{}, nil
}
func (blockingHealthSyncer) CheckRemoteEmpty(context.Context, string) error { return nil }
func (blockingHealthSyncer) Create(context.Context, string, string) error   { return nil }
func (blockingHealthSyncer) Clone(context.Context, string, string, int) error {
	return nil
}

func TestHealthFooterRendersCanonicalSummary(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))
	m.healthReady = true
	m.healthSummary = core.HealthSummary{TeamSyncConfigured: true, Conflicts: 1, Issues: 2}
	m.width, m.height = 100, 30
	got := stripANSI(m.footerContent(ViewDashboard))
	if !strings.Contains(got, "Team sync · never synced · 1 conflict · 2 issues") {
		t.Fatalf("footer = %q", got)
	}
}

func TestHealthStatusDoesNotBlockInitOrFirstView(t *testing.T) {
	svc := core.NewService(newTestStore(), testSearcher{}, testTokenizer{}, testHarnessRegistry{}, testClock{}, core.Config{}, blockingHealthSyncer{})
	m := NewModel(svc)
	m.width, m.height = 100, 30
	start := time.Now()
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init returned nil command")
	}
	_ = m.View()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("Init/first View blocked for %s", elapsed)
	}

	// Running the health command itself is allowed to wait; it is a tea.Cmd and
	// therefore runs away from the event loop that renders the first frame.
	done := make(chan tea.Msg, 1)
	go func() { done <- m.healthCmd()() }()
	select {
	case <-done:
		t.Fatal("blocking status returned before the fake delay")
	case <-time.After(50 * time.Millisecond):
	}
	<-done
}

func TestHealthKeyOpensScrollableDoctorOverlay(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))
	m.healthReady = true
	m.healthSummary = core.HealthSummary{}
	m.healthReport = core.DoctorReport{}
	m.width, m.height = 100, 30
	m, _ = press(t, m, "H")
	if m.currentView != ViewHealth || !m.hasOverlay() {
		t.Fatalf("H opened view %v with overlay=%v", m.currentView, m.hasOverlay())
	}
	if !strings.Contains(stripANSI(m.View()), "Doctor Report") {
		t.Fatalf("health overlay missing title:\n%s", stripANSI(m.View()))
	}
}
