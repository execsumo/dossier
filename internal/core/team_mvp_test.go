package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestServiceDisplayLead(t *testing.T) {
	cases := []struct {
		name       string
		store      Store
		lead       string
		wantName   string
		wantFormer bool
	}{
		{
			name: "member", store: &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
				Members: map[string]string{"psmith": "Priya Shah"}, Former: map[string]string{"old": "Former Person"},
			}}, lead: "psmith", wantName: "Priya Shah",
		},
		{
			name: "former", store: &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
				Members: map[string]string{"psmith": "Priya Shah"}, Former: map[string]string{"old": "Former Person"},
			}}, lead: "old", wantName: "Former Person", wantFormer: true,
		},
		{
			name: "unknown", store: &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
				Members: map[string]string{"psmith": "Priya Shah"},
			}}, lead: "legacy", wantName: "legacy",
		},
		{name: "no roster", store: newLocalFakeStore(), lead: "legacy", wantName: "legacy"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
			got := svc.DisplayLead(tt.lead)
			if got.DisplayName != tt.wantName || got.Former != tt.wantFormer {
				t.Fatalf("DisplayLead(%q) = %+v, want name %q former %t", tt.lead, got, tt.wantName, tt.wantFormer)
			}
		})
	}
}

func TestServiceListLeadMeAndCurrentUser(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Manager: "hgill",
		Members: map[string]string{"hgill": "Herwin Gill", "psmith": "Priya Shah"},
	}}
	now := time.Now()
	for _, d := range []*Dossier{
		{Frontmatter: Frontmatter{ID: "dos_mine", Name: "Mine", Slug: "mine", Status: StatusExecute, Priority: PriorityHigh, Lead: "psmith", CreatedAt: now, UpdatedAt: now}},
		{Frontmatter: Frontmatter{ID: "dos_other", Name: "Other", Slug: "other", Status: StatusExecute, Priority: PriorityHigh, Lead: "hgill", CreatedAt: now, UpdatedAt: now}},
	} {
		store.dossiers[d.Frontmatter.ID] = d
		store.revisions[d.Frontmatter.ID] = Revision("rev_" + d.Frontmatter.ID)
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "psmith"}, nil)
	username, displayName := svc.CurrentUser()
	if username != "psmith" || displayName != "Priya Shah" {
		t.Fatalf("CurrentUser() = %q, %q", username, displayName)
	}
	res, err := svc.List(context.Background(), ListReq{Lead: "me"})
	if err != nil {
		t.Fatal(err)
	}
	items := res.Data.([]ListItem)
	if len(items) != 1 || items[0].Name != "Mine" || items[0].Lead != "Priya Shah" {
		t.Fatalf("lead me list = %+v", items)
	}
}

func TestSessionStartNamesCurrentUserOnlyWithRoster(t *testing.T) {
	makeService := func(store Store) *Service {
		return NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "psmith"}, nil)
	}
	withRoster := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Members: map[string]string{"psmith": "Priya Shah"}}}
	got, err := makeService(withRoster).SessionStart(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "You are working as Priya Shah (psmith).") {
		t.Fatalf("SessionStart omitted current user: %s", got)
	}
	withoutRoster, err := makeService(newLocalFakeStore()).SessionStart(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(withoutRoster, "You are working as") {
		t.Fatalf("SessionStart added current user without roster: %s", withoutRoster)
	}
}
