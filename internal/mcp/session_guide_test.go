package mcp

import (
	"dossier/internal/core"
	"dossier/internal/store"
	"encoding/json"
	"strings"
	"testing"
)

// guideUnavailableStore models a store whose guide.md is missing from both disk
// and the binary — the only legitimate way ReadContextAsset comes back empty —
// by wrapping FakeStore (which otherwise always resolves context assets from the
// real embedded originals) and failing just that one lookup.
type guideUnavailableStore struct {
	*store.FakeStore
}

func (g *guideUnavailableStore) ReadContextAsset(name string) (string, error) {
	if name == "guide.md" {
		return "", core.NewError(core.ErrNotFound, "guide.md not found on disk or embedded")
	}
	return g.FakeStore.ReadContextAsset(name)
}

// sessionGuideDataFor pulls distillation_guide/distillation_guide_ref out of a
// dossier_session envelope's Data payload.
func sessionGuideDataFor(t *testing.T, env mcpEnvelope) (guide string, guideRef string) {
	t.Helper()
	raw, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal envelope data: %v", err)
	}
	var resp struct {
		Guide    string `json:"distillation_guide"`
		GuideRef string `json:"distillation_guide_ref"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal session response: %v (raw=%s)", err, raw)
	}
	return resp.Guide, resp.GuideRef
}

// TestDossierSessionDeliversGuideOnFirstBind proves the first dossier_session
// call in a session carries the full Guide and no ref — there is nothing to
// point back at yet, since nothing has been delivered.
func TestDossierSessionDeliversGuideOnFirstBind(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-guide-first")

	svc := newSessionTestService(t)
	env := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !env.OK {
		t.Fatalf("expected switch/bind ok, got error: %+v", env.Error)
	}

	guide, guideRef := sessionGuideDataFor(t, env)
	if guide == "" {
		t.Error("expected the first bind in a session to deliver a non-empty Guide")
	}
	if guideRef != "" {
		t.Errorf("expected no distillation_guide_ref on first delivery, got %q", guideRef)
	}
}

// TestDossierSessionSuppressesGuideOnSecondCallSameSession proves a second
// dossier_session call in the same session does not resend the ~3.5k-token
// Guide, and that the ref explicitly says it was already delivered rather than
// leaving the agent to guess why the field went empty.
func TestDossierSessionSuppressesGuideOnSecondCallSameSession(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-guide-repeat")

	svc := newSessionTestService(t)
	first := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !first.OK {
		t.Fatalf("expected first bind ok, got error: %+v", first.Error)
	}
	if guide, _ := sessionGuideDataFor(t, first); guide == "" {
		t.Fatal("test setup: first call must deliver the Guide for the suppression case to mean anything")
	}

	second := callTool(t, svc, "dossier_session", `{}`)
	if !second.OK {
		t.Fatalf("expected second call ok, got error: %+v", second.Error)
	}
	guide, guideRef := sessionGuideDataFor(t, second)
	if guide != "" {
		t.Errorf("expected empty distillation_guide on the second call in the same session, got %d bytes", len(guide))
	}
	if !strings.Contains(guideRef, "already delivered") {
		t.Errorf("expected distillation_guide_ref to say the Guide was already delivered, got %q", guideRef)
	}
}

// TestDossierSessionUnavailableGuideWarnsRatherThanClaimingDelivery is the bug
// this contract guards: when the Guide cannot be read at all (deleted from disk
// and not embedded — modeled here by guideUnavailableStore), telling the agent
// "already delivered" would be worse than saying nothing, since no rules are
// actually in force. The ref must instead warn plainly and point at `dossier init`.
func TestDossierSessionUnavailableGuideWarnsRatherThanClaimingDelivery(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-guide-unavailable")

	fakeStore := store.NewFakeStore()
	fakeStore.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_1", Name: "Test", Slug: "test-dossier", Status: core.StatusActive, Priority: core.PriorityHigh,
			CreatedAt: (&mockClock{}).Now(), UpdatedAt: (&mockClock{}).Now(),
		},
		DistilledState: core.DistilledState{Body: "# Test"},
	}
	fakeStore.Revisions["dos_1"] = "rev_1"
	wrapped := &guideUnavailableStore{FakeStore: fakeStore}
	svc := core.NewService(wrapped, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, nil)

	env := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !env.OK {
		t.Fatalf("expected switch/bind ok, got error: %+v", env.Error)
	}
	guide, guideRef := sessionGuideDataFor(t, env)
	if guide != "" {
		t.Errorf("expected empty distillation_guide when the Guide is unavailable, got %d bytes", len(guide))
	}
	if !strings.HasPrefix(guideRef, "WARNING:") {
		t.Errorf("expected distillation_guide_ref to start with WARNING:, got %q", guideRef)
	}
	if !strings.Contains(guideRef, "dossier init") {
		t.Errorf("expected distillation_guide_ref to point at `dossier init`, got %q", guideRef)
	}
	if strings.Contains(guideRef, "already delivered") {
		t.Errorf("an unavailable Guide must not be reported as already delivered, got %q", guideRef)
	}
}

// TestDossierSessionIncludeGuideForcesGuideDespiteOncePerSessionMarker proves
// include_guide is a real recovery path: even with the once-per-session marker
// already set, asking explicitly still returns the full Guide.
func TestDossierSessionIncludeGuideForcesGuideDespiteOncePerSessionMarker(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-guide-include")

	svc := newSessionTestService(t)
	first := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !first.OK {
		t.Fatalf("expected first bind ok, got error: %+v", first.Error)
	}
	if guide, _ := sessionGuideDataFor(t, first); guide == "" {
		t.Fatal("test setup: first call must deliver the Guide so the marker is actually set")
	}

	// Without include_guide this would be suppressed (see the sibling test); with
	// it, the once-per-session marker must be bypassed.
	forced := callTool(t, svc, "dossier_session", `{"include_guide":true}`)
	if !forced.OK {
		t.Fatalf("expected forced call ok, got error: %+v", forced.Error)
	}
	guide, guideRef := sessionGuideDataFor(t, forced)
	if guide == "" {
		t.Error("expected include_guide:true to return the full Guide despite the once-per-session marker")
	}
	if guideRef != "" {
		t.Errorf("expected no distillation_guide_ref when the Guide is actually included, got %q", guideRef)
	}
}
