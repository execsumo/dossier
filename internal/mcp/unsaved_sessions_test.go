package mcp

import (
	"dossier/internal/core"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDossierSessionCarriesUnsavedSessionsNotice proves the recovery notice
// reaches an agent through dossier_session on both the bind and the re-read
// path, once, and not as a duplicate warning.
func TestDossierSessionCarriesUnsavedSessionsNotice(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-unsaved-mcp")

	svc, fake := newSessionTestStoreAndService(t)
	ts := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	// The test service has no configured author, so SessionEnd would record an
	// empty Author: these are the current author's own sessions. Another
	// author's are counted, not listed (see core TestUnsavedNoticeSeparatesOtherAuthors).
	fake.Audits["dos_1"] = []core.AuditEvent{
		{TS: ts, Event: core.AuditEventSave, DossierID: "dos_1", SessionID: "sess-old", Author: "", BeforeRevision: "rev_1", AfterRevision: "rev_2", ArtifactsAdded: []string{"art_old"}},
		{TS: ts, Event: core.AuditEventDistilledStateNotCaptured, DossierID: "dos_1", SessionID: "sess-old", Author: ""},
	}

	for _, args := range []string{`{"id":"dos_1"}`, `{}`} {
		env := callTool(t, svc, "dossier_session", args)
		if !env.OK {
			t.Fatalf("dossier_session %s failed: %+v", args, env.Error)
		}
		raw, _ := json.Marshal(env.Data)
		var resp struct {
			Unsaved string `json:"unsaved_sessions"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, want := range []string{"Dossier Test", "1 session(s)", "session sess-old", "predates this work"} {
			if !strings.Contains(resp.Unsaved, want) {
				t.Errorf("%s: unsaved_sessions %q missing %q", args, resp.Unsaved, want)
			}
		}
		for _, w := range env.Warnings {
			if strings.Contains(w, "ended without saving") {
				t.Errorf("%s: notice duplicated in warnings: %q", args, w)
			}
		}
	}

	// Without unsaved sessions the field is absent.
	delete(fake.Audits, "dos_1")
	env := callTool(t, svc, "dossier_session", `{}`)
	raw, _ := json.Marshal(env.Data)
	if strings.Contains(string(raw), "unsaved_sessions") {
		t.Fatalf("unsaved_sessions present with nothing to recover: %s", raw)
	}
}
