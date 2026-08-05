package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// stubHarness models a harness whose capabilities change once installed — the
// shape of Pi, whose session identity only exists after the extension lands.
type stubHarness struct {
	name        string
	caps        Capabilities
	afterCaps   *Capabilities
	installs    int
	installFail error
}

func (h *stubHarness) Name() string { return h.name }

func (h *stubHarness) Detect() (Capabilities, error) {
	if h.installs > 0 && h.afterCaps != nil {
		return *h.afterCaps, nil
	}
	return h.caps, nil
}

func (h *stubHarness) Install(opts InstallOpts) error {
	if h.installFail != nil {
		return h.installFail
	}
	h.installs++
	return nil
}

type stubRegistry struct{ harnesses []Harness }

func (r *stubRegistry) All() []Harness { return r.harnesses }

func (r *stubRegistry) Get(name string) (Harness, error) {
	for _, h := range r.harnesses {
		if h.Name() == name {
			return h, nil
		}
	}
	return nil, NewError(ErrNotFound, "unknown harness "+name)
}

func serviceWithHarnesses(t *testing.T, harnesses ...Harness) *Service {
	t.Helper()
	return NewService(newLocalFakeStore(), &mockSearcher{}, &mockTokenizer{},
		&stubRegistry{harnesses: harnesses}, &mockClock{now: time.Now()}, Config{}, nil)
}

func reportFor(t *testing.T, reports []HarnessReport, name string) HarnessReport {
	t.Helper()
	for _, r := range reports {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no report for harness %q in %+v", name, reports)
	return HarnessReport{}
}

// Every harness reports for itself: a single merged capability map let the last
// harness scanned answer for all of them.
func TestInitReportsEachHarnessSeparately(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: Capabilities{
		MCP: true, SessionStartHook: true, SessionEndHook: true,
		PreCompactionHook: true, TranscriptCapture: true, Installed: true, SessionIdentity: true,
	}}
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}}
	svc := serviceWithHarnesses(t, claude, pi)

	res, err := svc.Init(context.Background(), InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	data := res.Data.(map[string]any)
	reports := data["harnesses"].([]HarnessReport)

	if got := reportFor(t, reports, "claude-code"); !got.Capabilities["MCP"] {
		t.Error("Claude Code's capabilities were overwritten by another harness")
	}
	if got := reportFor(t, reports, "pi"); got.Capabilities["MCP"] {
		t.Error("Pi must not inherit Claude Code's capabilities")
	}
	if caps := data["harness_capabilities"].(map[string]bool); !caps["MCP"] {
		t.Error("expected the live harness to answer for harness_capabilities")
	}
}

// A harness present on the device but not currently running still gets its
// integration installed — that is how Pi is wired up ahead of first use.
func TestInitInstallsPresentButIdleHarness(t *testing.T) {
	identity := Capabilities{Installed: true, SessionIdentity: true}
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}, afterCaps: &identity}
	svc := serviceWithHarnesses(t, pi)

	res, err := svc.Init(context.Background(), InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if pi.installs != 1 {
		t.Fatalf("expected Pi to be installed once, got %d", pi.installs)
	}

	data := res.Data.(map[string]any)
	if !data["harness_detected"].(bool) {
		t.Error("expected Pi to count as detected")
	}
	// Re-detected after install, so the report reflects what the user now has.
	if got := reportFor(t, data["harnesses"].([]HarnessReport), "pi"); !got.Capabilities["SessionIdentity"] {
		t.Error("expected session identity to be reported after install")
	}
}

// An integration that did not take (declined prompt, no terminal) has to be
// visible — a silently identity-less Pi looks like Dossier losing bindings.
func TestInitWarnsWhenSessionIdentityStaysUnavailable(t *testing.T) {
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}}
	svc := serviceWithHarnesses(t, pi)

	res, err := svc.Init(context.Background(), InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !warningsContain(res.Warnings, "dossier harness install pi") {
		t.Errorf("expected an actionable advisory, got %v", res.Warnings)
	}
}

func TestInstallHarnessInstallsNamedHarness(t *testing.T) {
	identity := Capabilities{Installed: true, SessionIdentity: true}
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}, afterCaps: &identity}
	svc := serviceWithHarnesses(t, pi)

	res, err := svc.InstallHarness(context.Background(), InstallHarnessReq{Name: "pi", YesToAll: true})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if pi.installs != 1 {
		t.Fatalf("expected one install, got %d", pi.installs)
	}
	report := res.Data.(HarnessReport)
	if !report.Capabilities["SessionIdentity"] {
		t.Error("expected the post-install report to show session identity")
	}
	if len(res.Warnings) != 0 {
		t.Errorf("expected no advisories after a successful install, got %v", res.Warnings)
	}
}

func TestInstallHarnessRejectsUnknownAndAbsentHarnesses(t *testing.T) {
	pi := &stubHarness{name: "pi", caps: Capabilities{}}
	svc := serviceWithHarnesses(t, pi)

	if _, err := svc.InstallHarness(context.Background(), InstallHarnessReq{Name: "codex", YesToAll: true}); err == nil {
		t.Error("expected an error for an unknown harness")
	}
	_, err := svc.InstallHarness(context.Background(), InstallHarnessReq{Name: "pi", YesToAll: true})
	if err == nil {
		t.Fatal("expected an error when the harness is not on this device")
	}
	if pi.installs != 0 {
		t.Error("must not install into a harness that is not present")
	}
}

func TestHarnessStatusDoesNotInstall(t *testing.T) {
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}}
	svc := serviceWithHarnesses(t, pi)

	res, err := svc.HarnessStatus(context.Background())
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if pi.installs != 0 {
		t.Error("HarnessStatus must not change anything on disk")
	}
	report := reportFor(t, res.Data.([]HarnessReport), "pi")
	if !report.Detected || report.Capabilities["SessionIdentity"] {
		t.Errorf("unexpected report %+v", report)
	}
}

// A missing integration is an advisory, not store damage: doctor says so and
// still reports the workspace healthy.
func TestDoctorAdvisesOnMissingIntegrationWithoutFailing(t *testing.T) {
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true}}
	svc := serviceWithHarnesses(t, pi)

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if !res.OK {
		t.Error("a missing harness integration must not fail the health check")
	}
	if !warningsContain(res.Warnings, "dossier harness install pi") {
		t.Errorf("expected doctor to surface the advisory, got %v", res.Warnings)
	}
}

// A binding has to name the harness the session ran under. Detection order alone
// credits the first configured harness, so a Pi session on a machine that also
// has Claude Code would be filed — with Claude Code's capabilities — as Claude Code.
func TestSwitchRecordsTheHarnessTheSessionCameFrom(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: Capabilities{
		MCP: true, SessionStartHook: true, SessionEndHook: true,
		PreCompactionHook: true, TranscriptCapture: true, Installed: true, SessionIdentity: true,
	}}
	pi := &stubHarness{name: "pi", caps: Capabilities{Installed: true, SessionIdentity: true}}
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{},
		&stubRegistry{harnesses: []Harness{claude, pi}}, &mockClock{now: time.Now()}, Config{}, nil)

	res, err := svc.Promote(context.Background(), PromoteReq{Name: "Pi bound topic", Force: true})
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	id := res.Data.(string)

	if _, err := svc.Switch(context.Background(), SwitchReq{ID: id, SessionID: "pi-1", HarnessName: "pi"}); err != nil {
		t.Fatalf("switch failed: %v", err)
	}

	binding, err := store.GetSessionBinding("pi-1")
	if err != nil {
		t.Fatalf("read binding: %v", err)
	}
	if binding.Harness != "pi" {
		t.Errorf("binding harness = %q, want %q", binding.Harness, "pi")
	}
	if binding.Capabilities.TranscriptCapture {
		t.Error("binding must not inherit Claude Code's transcript capability")
	}
}

// Without a named source (explicit session id, manual CLI use) the old
// detection-order fallback still applies.
func TestSwitchFallsBackToDetectionWhenSourceUnknown(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: Capabilities{
		MCP: true, TranscriptCapture: true, Installed: true, SessionIdentity: true,
	}}
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{},
		&stubRegistry{harnesses: []Harness{claude}}, &mockClock{now: time.Now()}, Config{}, nil)

	res, err := svc.Promote(context.Background(), PromoteReq{Name: "Fallback topic", Force: true})
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if _, err := svc.Switch(context.Background(), SwitchReq{ID: res.Data.(string), SessionID: "s-1"}); err != nil {
		t.Fatalf("switch failed: %v", err)
	}

	binding, err := store.GetSessionBinding("s-1")
	if err != nil {
		t.Fatalf("read binding: %v", err)
	}
	if binding.Harness != "claude-code" {
		t.Errorf("binding harness = %q, want %q", binding.Harness, "claude-code")
	}
}

func warningsContain(warnings []Warning, substr string) bool {
	return strings.Contains(warningsText(warnings), substr)
}
