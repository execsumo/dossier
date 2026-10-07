package mcp

import (
	"dossier/internal/core"
	"encoding/json"
	"testing"
	"time"
)

// TestDossierSaveAttributesCallingSession proves a save made through MCP
// records the harness session on its audit event, which per-session save
// counts and eval scheduling depend on (ADR 0016).
func TestDossierSaveAttributesCallingSession(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-save-attr")
	svc, fake := newSessionTestStoreAndService(t)
	_, rev, err := fake.Read("dos_1")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"id": "dos_1", "base_revision": rev, "distilled_state_markdown": "# Test\n\n## Situation\nSaved via MCP."})
	env := callTool(t, svc, "dossier_save", string(args))
	if !env.OK {
		t.Fatalf("dossier_save failed: %+v", env.Error)
	}
	var found bool
	for _, e := range fake.Audits["dos_1"] {
		if e.Event == core.AuditEventSave && e.SessionID == "sess-save-attr" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no save event attributed to the calling session: %+v", fake.Audits["dos_1"])
	}
}

func TestDossierStatsReturnsRowsByVersion(t *testing.T) {
	svc, fake := newSessionTestStoreAndService(t)
	ts := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	fake.Audits["dos_1"] = []core.AuditEvent{
		{TS: ts, Event: core.AuditEventSessionEnded, DossierID: "dos_1", SessionID: "s1", Version: "v0.4.0", GuideHash: "abc123def456"},
		{TS: ts.Add(time.Minute), Event: core.AuditEventSessionEval, DossierID: "dos_1", SessionID: "s1", Version: "v0.4.0",
			Eval: &core.EvalSummary{Probes: 4, Passed: 3, ByKind: map[string]core.EvalKindScore{"value": {Probes: 2, Passed: 2}}}},
	}
	env := callTool(t, svc, "dossier_stats", `{}`)
	if !env.OK {
		t.Fatalf("dossier_stats failed: %+v", env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var rep core.StatsReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Rows[0].Version != "v0.4.0" || rep.Rows[0].Sessions != 1 || rep.Rows[0].EvalPassed != 3 {
		t.Fatalf("rows = %+v", rep.Rows)
	}
}

// TestDossierChangesReportsOK guards a fixed bug: the handler never set
// res.OK, so a successful dossier_changes call reported ok:false.
func TestDossierChangesReportsOK(t *testing.T) {
	svc, _ := newSessionTestStoreAndService(t)
	if env := callTool(t, svc, "dossier_changes", `{"since":"2026-01-01T00:00:00Z"}`); !env.OK || env.Error != nil {
		t.Fatalf("successful dossier_changes must report ok: %+v", env)
	}
}
