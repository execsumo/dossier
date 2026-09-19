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

func TestServiceListLeadByName(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Members: map[string]string{"hgill": "Herwin Gill", "psmith": "Priya Shah"},
	}}
	seed := func(id, name, lead string) {
		store.dossiers[id] = &Dossier{Frontmatter: Frontmatter{ID: id, Name: name, Slug: id, Status: StatusExecute, Priority: PriorityHigh, Lead: lead}}
		store.revisions[id] = Revision("rev_" + id)
	}
	seed("dos_herwin", "Herwin's work", "hgill")
	seed("dos_priya", "Priya's work", "psmith")
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	for _, query := range []string{"Herwin Gill", "hgill", "Herwin"} {
		res, err := svc.List(context.Background(), ListReq{Lead: query})
		if err != nil {
			t.Fatalf("List(%q): %v", query, err)
		}
		items := res.Data.([]ListItem)
		if len(items) != 1 || items[0].Name != "Herwin's work" || items[0].Lead != "Herwin Gill" {
			t.Fatalf("List(%q) = %+v", query, items)
		}
	}
}

func TestServiceListLeadFormerMember(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Former: map[string]string{"old": "Former Person"},
	}}
	store.dossiers["dos_old"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_old", Name: "Old work", Slug: "old-work", Status: StatusExecute, Priority: PriorityHigh, Lead: "old"}}
	store.revisions["dos_old"] = "rev_old"
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	res, err := svc.List(context.Background(), ListReq{Lead: "Former Person"})
	if err != nil {
		t.Fatal(err)
	}
	items := res.Data.([]ListItem)
	if len(items) != 1 || !items[0].LeadFormer {
		t.Fatalf("former lead list = %+v", items)
	}
}

func TestServiceListLeadAmbiguous(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Members: map[string]string{"btwo": "Bob Two", "bthree": "Bob Three"},
	}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	res, err := svc.List(context.Background(), ListReq{Lead: "Bob"})
	domainErr, ok := err.(*DomainError)
	if !ok || domainErr.Code != ErrAmbiguousTarget || res.OK {
		t.Fatalf("ambiguous list = result=%+v err=%v", res, err)
	}
	members := res.Data.([]RosterMember)
	if len(members) != 2 || members[0].Username != "bthree" || members[1].Username != "btwo" || len(res.NextActions) != 2 {
		t.Fatalf("ambiguous candidates = %+v, next=%v", members, res.NextActions)
	}
}

// TestServiceListLeadDegenerateQueryHidesUnassigned guards the case where a lead
// query normalizes to the empty string: it must return nothing and warn, not
// quietly turn into an "unassigned" filter.
func TestServiceListLeadDegenerateQueryHidesUnassigned(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Members: map[string]string{"hgill": "Herwin Gill"},
	}}
	store.dossiers["dos_unassigned"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_unassigned", Name: "Unassigned", Slug: "unassigned", Status: StatusExecute, Priority: PriorityHigh}}
	store.revisions["dos_unassigned"] = Revision("rev_unassigned")
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	res, err := svc.List(context.Background(), ListReq{Lead: "@corp.com"})
	if err != nil {
		t.Fatal(err)
	}
	if items, _ := res.Data.([]ListItem); len(items) != 0 {
		t.Fatalf("degenerate lead query returned %+v", items)
	}
	if len(res.Warnings) != 1 || !strings.Contains(string(res.Warnings[0]), "@corp.com") {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
}

func TestServiceListLeadUnknownWarns(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{
		Members: map[string]string{"hgill": "Herwin Gill"},
	}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	res, err := svc.List(context.Background(), ListReq{Lead: "Nobody"})
	if err != nil || !res.OK || len(res.Warnings) != 1 || !strings.Contains(string(res.Warnings[0]), "Nobody") {
		t.Fatalf("unknown lead = result=%+v err=%v", res, err)
	}
}

func TestServiceListLeadFreeFormWithoutRoster(t *testing.T) {
	store := newLocalFakeStore()
	store.dossiers["dos_alice"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_alice", Name: "Alice work", Slug: "alice-work", Status: StatusExecute, Priority: PriorityHigh, Lead: "Alice"}}
	store.revisions["dos_alice"] = "rev_alice"
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	res, err := svc.List(context.Background(), ListReq{Lead: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	items := res.Data.([]ListItem)
	if len(items) != 1 || items[0].Name != "Alice work" || len(res.Warnings) != 0 {
		t.Fatalf("free-form lead list = %+v warnings=%v", items, res.Warnings)
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
