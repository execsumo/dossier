package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// capturingLibraryStore records what ContextRefresh renders into library.md, so
// a test can assert which harness the session was attributed to.
type capturingLibraryStore struct {
	Store
	last LibraryData
	got  bool
}

func (c *capturingLibraryStore) WriteLibraryContext(data LibraryData) error {
	c.last = data
	c.got = true
	return nil
}

// resolvingRegistry is a registry that can name the harness owning the current
// process, modelling internal/harness.Registry.
type resolvingRegistry struct {
	stubRegistry
	active   string
	resolved bool
}

func (r *resolvingRegistry) ActiveHarness() (Harness, Capabilities, bool) {
	if !r.resolved {
		return nil, Capabilities{}, false
	}
	h, err := r.Get(r.active)
	if err != nil || h == nil {
		return nil, Capabilities{}, false
	}
	caps, err := h.Detect()
	if err != nil {
		return nil, Capabilities{}, false
	}
	return h, caps, true
}

// claudeCodeDeviceCaps mirrors ClaudeCodeHarness.Detect: it reports the full
// capability set from the presence of config files on disk, with no knowledge
// of whether this process is actually inside a Claude Code session.
func claudeCodeDeviceCaps() Capabilities {
	return Capabilities{
		MCP:               true,
		SessionStartHook:  true,
		SessionEndHook:    true,
		PreCompactionHook: true,
		TranscriptCapture: true,
		Installed:         true,
		SessionIdentity:   true,
	}
}

// piLiveCaps mirrors PiHarness.Detect inside a live session with the bundled
// extension installed: identity and a transcript, but no MCP and no hooks.
func piLiveCaps() Capabilities {
	return Capabilities{
		Installed:         true,
		SessionIdentity:   true,
		TranscriptCapture: true,
	}
}

func serviceForRegistry(reg HarnessRegistry) (*Service, *capturingLibraryStore) {
	store := &capturingLibraryStore{Store: newLocalFakeStore()}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, reg,
		&mockClock{now: time.Now()}, Config{}, nil)
	return svc, store
}

// The regression this whole port exists for. Claude Code's Detect answers from
// ~/.claude.json on disk, so on a machine where it has ever run it claims every
// capability regardless of what is running now. Scanning the registry for the
// first harness with LiveSession() therefore attributed a live Pi session to
// Claude Code — and handed the agent Claude Code's capability map, advertising
// MCP and hooks that Pi does not have.
func TestContextRefreshAttributesLiveSessionToOwningHarness(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	pi := &stubHarness{name: "pi", caps: piLiveCaps()}
	reg := &resolvingRegistry{
		stubRegistry: stubRegistry{harnesses: []Harness{claude, pi}},
		active:       "pi",
		resolved:     true,
	}
	svc, store := serviceForRegistry(reg)

	if _, err := svc.ContextRefresh(context.Background()); err != nil {
		t.Fatalf("ContextRefresh() error = %v", err)
	}
	if !store.got {
		t.Fatal("ContextRefresh() wrote no library context")
	}
	if store.last.Harness != "Pi" {
		t.Errorf("harness = %q, want %q — a Pi session was attributed to another harness", store.last.Harness, "Pi")
	}
	if store.last.Capabilities["MCP"] {
		t.Error("reported MCP as available under Pi; Pi ships no MCP client")
	}
	if store.last.Capabilities["SessionStartHook"] || store.last.Capabilities["SessionEndHook"] ||
		store.last.Capabilities["PreCompactionHook"] {
		t.Error("reported lifecycle hooks as available under Pi")
	}
	if !store.last.Capabilities["TranscriptCapture"] {
		t.Error("dropped a transcript capability Pi actually has")
	}
}

// Registration order must not decide the answer in either direction: the same
// registry resolving to claude-code has to report Claude Code's map.
func TestContextRefreshAttributesClaudeCodeSessionToClaudeCode(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	pi := &stubHarness{name: "pi", caps: piLiveCaps()}
	// Pi first, to prove the result is not simply "whatever is registered first".
	reg := &resolvingRegistry{
		stubRegistry: stubRegistry{harnesses: []Harness{pi, claude}},
		active:       "claude-code",
		resolved:     true,
	}
	svc, store := serviceForRegistry(reg)

	if _, err := svc.ContextRefresh(context.Background()); err != nil {
		t.Fatalf("ContextRefresh() error = %v", err)
	}
	if store.last.Harness != "Claude Code" {
		t.Errorf("harness = %q, want %q", store.last.Harness, "Claude Code")
	}
	if !store.last.Capabilities["MCP"] {
		t.Error("dropped Claude Code's MCP capability")
	}
}

// No harness owns this process (a bare CLI run, or a session id supplied
// explicitly). Reporting the first installed harness would claim a session that
// does not exist, so the resolver's "no" is final.
func TestContextRefreshReportsCLIWhenNoHarnessOwnsTheProcess(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	reg := &resolvingRegistry{
		stubRegistry: stubRegistry{harnesses: []Harness{claude}},
		resolved:     false,
	}
	svc, store := serviceForRegistry(reg)

	if _, err := svc.ContextRefresh(context.Background()); err != nil {
		t.Fatalf("ContextRefresh() error = %v", err)
	}
	if store.last.Harness != "CLI" {
		t.Errorf("harness = %q, want %q", store.last.Harness, "CLI")
	}
	if store.last.Capabilities["MCP"] {
		t.Error("claimed MCP for a process no harness owns")
	}
	if len(store.last.Warnings) == 0 {
		t.Error("expected a visible warning that no harness session is active")
	}
}

// A registry that cannot resolve an owner at all (the lightweight registries
// predating ActiveHarnessResolver) keeps the old first-live-harness behaviour
// rather than losing harness reporting entirely.
func TestContextRefreshFallsBackForRegistryWithoutResolver(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	svc, store := serviceForRegistry(&stubRegistry{harnesses: []Harness{claude}})

	if _, err := svc.ContextRefresh(context.Background()); err != nil {
		t.Fatalf("ContextRefresh() error = %v", err)
	}
	if store.last.Harness != "Claude Code" {
		t.Errorf("harness = %q, want %q", store.last.Harness, "Claude Code")
	}
}

// The session-start nudge is the other place a misattributed harness reaches
// the agent. A Pi session with no resolvable transcript must say so rather than
// inherit Claude Code's transcript capability.
func TestSessionStartWarnsOnPiSessionWithoutTranscript(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	piNoTranscript := &stubHarness{name: "pi", caps: Capabilities{Installed: true, SessionIdentity: true}}
	reg := &resolvingRegistry{
		stubRegistry: stubRegistry{harnesses: []Harness{claude, piNoTranscript}},
		active:       "pi",
		resolved:     true,
	}
	svc, _ := serviceForRegistry(reg)

	text, err := svc.SessionStart(context.Background(), "sess_pi_1")
	if err != nil {
		t.Fatalf("SessionStart() error = %v", err)
	}
	if !strings.Contains(text, "Transcript archive is unavailable") {
		t.Errorf("expected a transcript warning for a Pi session with no transcript, got:\n%s", text)
	}
}

// Promote's transcript warning reads the same capability. Under a Pi session
// with a live transcript it must not fire.
func TestPromoteDoesNotWarnWhenOwningHarnessHasTranscript(t *testing.T) {
	claude := &stubHarness{name: "claude-code", caps: claudeCodeDeviceCaps()}
	pi := &stubHarness{name: "pi", caps: piLiveCaps()}
	reg := &resolvingRegistry{
		stubRegistry: stubRegistry{harnesses: []Harness{claude, pi}},
		active:       "pi",
		resolved:     true,
	}
	svc, _ := serviceForRegistry(reg)

	res, err := svc.Promote(context.Background(), PromoteReq{Name: "Transcript capable"})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	for _, w := range res.Warnings {
		if strings.Contains(string(w), "Transcript archive is unavailable") {
			t.Errorf("warned about transcripts under a harness that has them: %v", res.Warnings)
		}
	}
}
