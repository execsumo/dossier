package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type unsavedHarness struct {
	t     *testing.T
	store *localFakeStore
	svc   *Service
	clock *mockClock
	ctx   context.Context
}

func newUnsavedHarness(t *testing.T) *unsavedHarness {
	t.Helper()
	store := newLocalFakeStore()
	clock := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, clock, Config{Author: "Alice"}, nil)
	h := &unsavedHarness{t: t, store: store, svc: svc, clock: clock, ctx: context.Background()}
	if _, err := svc.Save(h.ctx, SaveReq{
		DistilledStateMarkdown: "# Recovery\n\n## Situation\nInitial.",
		FrontmatterUpdates:     map[string]any{"name": "Recovery", "status": "active", "priority": "medium"},
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	return h
}

func (h *unsavedHarness) tick() { h.clock.now = h.clock.now.Add(time.Hour) }

func (h *unsavedHarness) rev() Revision {
	h.t.Helper()
	_, rev, err := h.store.Read("dos_fake_id")
	if err != nil {
		h.t.Fatalf("read: %v", err)
	}
	return rev
}

// bind opens a session bound at the current revision.
func (h *unsavedHarness) bind(session string) {
	h.t.Helper()
	h.tick()
	if err := h.store.SaveSessionBinding(&SessionBinding{
		SessionBindingID: session, Harness: "claude-code", DossierID: "dos_fake_id", LastSeenRevision: string(h.rev()),
	}); err != nil {
		h.t.Fatalf("bind: %v", err)
	}
}

// boundary runs a session-end / pre-compaction boundary with no distilled_state.
func (h *unsavedHarness) boundary(session string) {
	h.t.Helper()
	h.tick()
	if _, err := h.svc.SessionEnd(h.ctx, session, "", "transcript for "+session+" "+h.clock.now.String()); err != nil {
		h.t.Fatalf("SessionEnd(%s): %v", session, err)
	}
}

// saveState lands a real Distilled State save.
func (h *unsavedHarness) saveState(body string) {
	h.t.Helper()
	h.tick()
	if _, err := h.svc.Save(h.ctx, SaveReq{ID: "dos_fake_id", BaseRevision: h.rev(), DistilledStateMarkdown: body}); err != nil {
		h.t.Fatalf("save: %v", err)
	}
}

// recoverSession saves a Distilled State that mentions every transcript still
// listed for session: as span citations, or as Evidence index lines.
func (h *unsavedHarness) recoverSession(session string, asEvidence bool) {
	h.t.Helper()
	got, err := h.svc.UnsavedSessions("dos_fake_id")
	if err != nil {
		h.t.Fatalf("UnsavedSessions: %v", err)
	}
	var lines []string
	for _, u := range got {
		if u.SessionID != session {
			continue
		}
		for _, id := range u.TranscriptArtifactIDs {
			if asEvidence {
				lines = append(lines, "- `"+id+"` (transcript): background only.")
			} else {
				lines = append(lines, "- [observed] Recovered from "+session+". [src:"+id+"#L1-L1]")
			}
		}
	}
	if len(lines) == 0 {
		h.t.Fatalf("recoverSession(%s): nothing listed to recover", session)
	}
	heading := "## Findings"
	if asEvidence {
		heading = "## Evidence"
	}
	h.saveState("# Recovery\n\n" + heading + "\n" + strings.Join(lines, "\n"))
}

// boundaryNoTranscript runs a boundary that captured no transcript.
func (h *unsavedHarness) boundaryNoTranscript(session string) {
	h.t.Helper()
	h.tick()
	if _, err := h.svc.SessionEnd(h.ctx, session, "", ""); err != nil {
		h.t.Fatalf("SessionEnd(%s): %v", session, err)
	}
}

// saveArtifactOnly archives an artifact without touching the Distilled State.
func (h *unsavedHarness) saveArtifactOnly() {
	h.t.Helper()
	h.tick()
	if _, err := h.svc.Save(h.ctx, SaveReq{
		ID: "dos_fake_id", BaseRevision: h.rev(),
		Artifacts: []Artifact{{Type: ArtifactTypeQuery, Title: "evidence", Content: "x", ContentFormat: ContentFormatText}},
	}); err != nil {
		h.t.Fatalf("artifact save: %v", err)
	}
}

func TestUnsavedSessions(t *testing.T) {
	tests := []struct {
		name string
		run  func(h *unsavedHarness)
		// want: session id -> number of transcript artifacts
		want map[string]int
	}{
		{
			name: "unsaved session end is listed with its transcript",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
			},
			want: map[string]int{"sess_a": 1},
		},
		{
			name: "save citing the transcript clears it",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.recoverSession("sess_a", false)
			},
			want: map[string]int{},
		},
		{
			name: "Evidence index line for the transcript clears it",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.recoverSession("sess_a", true)
			},
			want: map[string]int{},
		},
		{
			name: "unrelated Distilled State save does not clear it",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.saveState("# Recovery\n\n## Situation\nAnother session's own work.")
			},
			want: map[string]int{"sess_a": 1},
		},
		{
			name: "no transcript captured: listed, then cleared by any body change",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundaryNoTranscript("sess_a")
				h.saveState("# Recovery\n\n## Situation\nChanged.")
			},
			want: map[string]int{},
		},
		{
			name: "no transcript captured and no later save stays listed",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundaryNoTranscript("sess_a")
			},
			want: map[string]int{"sess_a": 0},
		},
		{
			name: "later artifact-only save does not clear it",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.saveArtifactOnly()
			},
			want: map[string]int{"sess_a": 1},
		},
		{
			name: "two unsaved sessions are both listed",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.bind("sess_b")
				h.boundary("sess_b")
			},
			want: map[string]int{"sess_a": 1, "sess_b": 1},
		},
		{
			name: "save after the first of two sessions clears only the first",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.recoverSession("sess_a", false)
				h.bind("sess_b")
				h.boundary("sess_b")
			},
			want: map[string]int{"sess_b": 1},
		},
		{
			name: "session that saved during the session has no notice",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.saveState("# Recovery\n\n## Situation\nSaved mid-session.")
				h.boundary("sess_a")
			},
			want: map[string]int{},
		},
		{
			name: "pre-compaction then end, both unsaved, is one entry with both transcripts",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.boundary("sess_a")
			},
			want: map[string]int{"sess_a": 2},
		},
		{
			name: "pre-compaction unsaved, recovered, then end unsaved lists only the later transcript",
			run: func(h *unsavedHarness) {
				h.bind("sess_a")
				h.boundary("sess_a")
				h.recoverSession("sess_a", false)
				h.bind("sess_a") // resumed at the saved revision
				h.boundary("sess_a")
			},
			want: map[string]int{"sess_a": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newUnsavedHarness(t)
			tt.run(h)
			got, err := h.svc.UnsavedSessions("dos_fake_id")
			if err != nil {
				t.Fatalf("UnsavedSessions: %v", err)
			}
			gotMap := map[string]int{}
			for _, u := range got {
				if _, dup := gotMap[u.SessionID]; dup {
					t.Errorf("duplicate entry for session %s", u.SessionID)
				}
				gotMap[u.SessionID] = len(u.TranscriptArtifactIDs)
				if u.Author != "Alice" || u.TS.IsZero() {
					t.Errorf("entry missing author/timestamp: %+v", u)
				}
				seen := map[string]bool{}
				for _, id := range u.TranscriptArtifactIDs {
					if id == "" || seen[id] {
						t.Errorf("garbled artifact ids: %v", u.TranscriptArtifactIDs)
					}
					seen[id] = true
				}
			}
			if fmt.Sprint(gotMap) != fmt.Sprint(tt.want) {
				t.Fatalf("unsaved = %v, want %v", gotMap, tt.want)
			}
			notice := h.svc.UnsavedSessionsNotice("dos_fake_id", "Recovery")
			if (len(tt.want) == 0) != (notice == "") {
				t.Fatalf("notice presence mismatch: %q", notice)
			}
		})
	}
}

func TestUnsavedSessionsNoticeNamesSessionAndTranscript(t *testing.T) {
	h := newUnsavedHarness(t)
	h.bind("sess_a")
	h.boundary("sess_a")
	got, _ := h.svc.UnsavedSessions("dos_fake_id")
	notice := h.svc.UnsavedSessionsNotice("dos_fake_id", "Recovery")
	for _, want := range []string{"Dossier Recovery", "1 session(s)", got[0].TranscriptArtifactIDs[0], "session sess_a", "2026-06-14", "by Alice", "predates this work"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice %q missing %q", notice, want)
		}
	}
}

func TestFormatUnsavedSessionsNoticeCapsWithMoreTail(t *testing.T) {
	var sessions []UnsavedSession
	for i := 0; i < 8; i++ {
		sessions = append(sessions, UnsavedSession{SessionID: fmt.Sprintf("s%d", i), TS: time.Now(), TranscriptArtifactIDs: []string{fmt.Sprintf("art_%d", i)}})
	}
	notice := FormatUnsavedSessionsNotice("X", sessions)
	if !strings.Contains(notice, "8 session(s)") || !strings.Contains(notice, "(+3 more)") {
		t.Fatalf("expected total count and +3 more tail: %s", notice)
	}
	if strings.Contains(notice, "s5") || !strings.Contains(notice, "s4") {
		t.Fatalf("expected exactly the first five listed: %s", notice)
	}
	if FormatUnsavedSessionsNotice("X", nil) != "" {
		t.Fatal("empty list must render nothing")
	}
}

func TestUnsavedNoticeSurfaces(t *testing.T) {
	h := newUnsavedHarness(t)
	h.bind("sess_a")
	h.boundary("sess_a")
	notice := h.svc.UnsavedSessionsNotice("dos_fake_id", "Recovery")
	if notice == "" {
		t.Fatal("expected a notice")
	}

	// A new session bound to the Dossier receives it at session start.
	h.bind("sess_next")
	out, err := h.svc.SessionStartMode(h.ctx, "sess_next", false)
	if err != nil {
		t.Fatalf("SessionStartMode: %v", err)
	}
	if !strings.Contains(out, notice) {
		t.Fatalf("session start output lacks the notice:\n%s", out)
	}

	agent, err := h.svc.Recall(h.ctx, RecallReq{ID: "dos_fake_id"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(agent.Warnings, notice) {
		t.Fatalf("agent recall warnings lack the notice: %v", agent.Warnings)
	}
	human, err := h.svc.Recall(h.ctx, RecallReq{ID: "dos_fake_id", HumanView: true})
	if err != nil {
		t.Fatal(err)
	}
	if containsWarning(human.Warnings, notice) {
		t.Fatalf("HumanView recall must not carry the notice: %v", human.Warnings)
	}

	// Once the state cites the transcript, no surface carries it.
	h.recoverSession("sess_a", false)
	out, err = h.svc.SessionStartMode(h.ctx, "sess_next", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ended without saving") {
		t.Fatalf("notice should have cleared:\n%s", out)
	}
}

func containsWarning(ws []Warning, sub string) bool {
	for _, w := range ws {
		if strings.Contains(string(w), sub) {
			return true
		}
	}
	return false
}

func TestMentionsAnyArtifactMatchesWholeIDs(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"[src:art_01ab#L1-L4]", true},
		{"- `art_01ab` (transcript): background only.", true},
		{"ends with art_01ab", true},
		{"[src:art_01abc#L1-L4]", false},
		{"art_01abc and art_01abd", false},
		{"art_01abc then art_01ab.", true},
		{"nothing here", false},
	}
	for _, tt := range tests {
		if got := mentionsAnyArtifact(tt.body, []string{"art_01ab"}); got != tt.want {
			t.Errorf("mentionsAnyArtifact(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}
