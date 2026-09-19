package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestMCPListLeadByDisplayName(t *testing.T) {
	fake := store.NewFakeStore()
	fake.Roster = &core.Roster{Members: map[string]string{"btwo": "Bob Two"}}
	fake.Dossiers["dos_bob"] = &core.Dossier{Frontmatter: core.Frontmatter{
		ID: "dos_bob", Name: "Bob's work", Slug: "bobs-work", Status: core.StatusExecute,
		Priority: core.PriorityHigh, Lead: "btwo",
	}}
	fake.Revisions["dos_bob"] = "rev_bob"
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{Author: "alice"}, nil)

	listed := callTool(t, svc, "dossier_list", `{"lead":"Bob Two"}`)
	if !listed.OK {
		t.Fatalf("list by display name failed: %+v", listed.Error)
	}
	data := listed.Data.(map[string]any)
	items := data["items"].([]any)
	if len(items) != 1 || !strings.Contains(string(mustJSON(items[0])), "Bob Two") {
		t.Fatalf("list by display name items = %#v", items)
	}
	if _, ok := data["current_user"]; !ok {
		t.Fatalf("list omitted current_user: %#v", data)
	}
}

func TestMCPListLeadAmbiguousEnvelope(t *testing.T) {
	fake := store.NewFakeStore()
	fake.Roster = &core.Roster{Members: map[string]string{"btwo": "Bob Two", "bthree": "Bob Three"}}
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, nil)

	listed := callTool(t, svc, "dossier_list", `{"lead":"Bob"}`)
	if listed.OK || listed.Error == nil || listed.Error.Code != ErrCodeAmbiguousTarget {
		t.Fatalf("ambiguous list envelope = %+v", listed)
	}
	if len(listed.NextActions) == 0 || listed.Data == nil {
		t.Fatalf("ambiguous list omitted guidance or candidates: %+v", listed)
	}
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func TestMCPTeamLeadResolutionListMineAndSessionUser(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-team-mvp")
	fake := store.NewFakeStore()
	fake.Roster = &core.Roster{
		Manager: "hgill",
		Members: map[string]string{"hgill": "Herwin Gill", "psmith": "Priya Shah"},
	}
	fake.Dossiers["dos_1"] = &core.Dossier{Frontmatter: core.Frontmatter{
		ID: "dos_1", Name: "Assigned", Slug: "assigned", Status: core.StatusExecute,
		Priority: core.PriorityHigh, Lead: "hgill",
	}, DistilledState: core.DistilledState{Body: "# Assigned"}}
	fake.Revisions["dos_1"] = "rev_1"
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{Author: "psmith"}, nil)

	updated := callTool(t, svc, "dossier_update", `{"id":"dos_1","lead":"Priya"}`)
	if !updated.OK {
		t.Fatalf("first-name lead update failed: %+v", updated.Error)
	}
	if fake.Dossiers["dos_1"].Frontmatter.Lead != "psmith" {
		t.Fatalf("stored lead = %q, want psmith", fake.Dossiers["dos_1"].Frontmatter.Lead)
	}

	listed := callTool(t, svc, "dossier_list", `{"lead":"me"}`)
	if !listed.OK {
		t.Fatalf("list mine failed: %+v", listed.Error)
	}
	data, ok := listed.Data.(map[string]any)
	if !ok {
		t.Fatalf("list data = %T, want object", listed.Data)
	}
	items, ok := data["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list mine items = %#v", data["items"])
	}
	raw, _ := json.Marshal(items[0])
	if !strings.Contains(string(raw), "Priya Shah") {
		t.Fatalf("list item omitted display name: %s", raw)
	}
	user, ok := data["current_user"].(map[string]any)
	if !ok || user["username"] != "psmith" || user["display_name"] != "Priya Shah" {
		t.Fatalf("current user = %#v", data["current_user"])
	}

	session := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !session.OK {
		t.Fatalf("session bind failed: %+v", session.Error)
	}
	sessionRaw, _ := json.Marshal(session.Data)
	if !strings.Contains(string(sessionRaw), "Priya Shah") || !strings.Contains(string(sessionRaw), "psmith") {
		t.Fatalf("session omitted lead/user display: %s", sessionRaw)
	}
}
