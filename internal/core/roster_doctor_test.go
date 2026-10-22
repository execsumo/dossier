package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDoctorAdvisesUnknownRosterLead(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "hgill", Members: map[string]string{"hgill": "Herwin Gill"}},
	}
	now := time.Now().Truncate(time.Second)
	dossier := &Dossier{Frontmatter: Frontmatter{
		ID: "dos_unknown_lead", Name: "Unknown lead", Slug: "unknown-lead",
		CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium, Lead: "missing",
	}, DistilledState: DistilledState{Body: "# State\n"}}
	store.dossiers[dossier.Frontmatter.ID] = dossier
	store.revisions[dossier.Frontmatter.ID] = CalculateRevision(dossier.Frontmatter, dossier.DistilledState.Body, nil)
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range res.Warnings {
		if strings.Contains(string(warning), "not a member of the team roster") {
			return
		}
	}
	t.Fatalf("unknown lead advisory missing: %v", res.Warnings)
}

// TestDoctorLeadAdvisoryMatchesAssignmentRules pins doctor to the same roster
// rules the lead write path applies (normalizeLeadUpdate, B17/B19): a stored
// lead is clean exactly when assigning it again would succeed and store the
// same username. Every other value gets an advisory that says what is wrong
// and the exact `dossier lead` command that fixes it, never a misleading
// "not a member" for someone who is one.
func TestDoctorLeadAdvisoryMatchesAssignmentRules(t *testing.T) {
	roster := Roster{
		Manager: "hgill",
		Members: map[string]string{
			"hgill": "Herwin Gill", "psmith": "Priya Shah",
			"slee": "Sam Lee", "sleeb": "Sam Lee", "sitroom": "Sit Room",
		},
		Former: map[string]string{"olduser": "Old User"},
		Kinds:  map[string]string{"sitroom": "agent"},
	}
	cases := []struct {
		name    string
		lead    string
		want    []string // substrings of the lead advisory; nil means no advisory
		notWant []string
		clean   bool
	}{
		{name: "canonical username", lead: "psmith", clean: true},
		{name: "case and domain variants normalize to the username", lead: `ACME\PSmith`, clean: true},
		{name: "display name of a member", lead: "Priya Shah",
			want:    []string{`lead "Priya Shah" is not stored as a roster username`, `Priya Shah (psmith)`, "dossier lead lead-case psmith"},
			notWant: []string{"not a member"}},
		{name: "ambiguous display name", lead: "Sam Lee",
			want:    []string{`lead "Sam Lee" matches more than one team member: slee, sleeb`, "dossier lead lead-case <username>"},
			notWant: []string{"not a member"}},
		{name: "agent member", lead: "sitroom",
			want: []string{`lead "sitroom" is an agent (Sit Room)`, "Delegation Contract", "dossier lead lead-case <username>"}},
		{name: "former member by username", lead: "olduser",
			want: []string{`lead "olduser" is a former team member (Old User)`, "dossier lead lead-case <username>"}},
		{name: "former member by display name", lead: "Old User",
			want:    []string{`lead "Old User" is a former team member (Old User)`},
			notWant: []string{"not a member"}},
		{name: "unknown", lead: "missing",
			want: []string{`lead "missing" is not a member of the team roster`, "dossier lead lead-case <username>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: roster}
			now := time.Now().Truncate(time.Second)
			d := &Dossier{Frontmatter: Frontmatter{
				ID: "dos_lead_case", Name: "Lead case", Slug: "lead-case",
				CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium, Lead: tc.lead,
			}, DistilledState: DistilledState{Body: "# State\n"}}
			store.dossiers[d.Frontmatter.ID] = d
			store.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, nil)
			svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)

			// The write path's verdict on the same stored value.
			updates, err := svc.normalizeLeadUpdate(map[string]any{"lead": tc.lead})
			writeClean := err == nil && NormalizeUsername(updates["lead"].(string)) == NormalizeUsername(tc.lead)
			if writeClean != tc.clean {
				t.Fatalf("fixture disagrees with the write path: clean=%v, write path clean=%v (err=%v)", tc.clean, writeClean, err)
			}

			res, err := svc.Doctor(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var leadWarnings []string
			for _, w := range res.Warnings {
				if strings.Contains(string(w), "lead ") {
					leadWarnings = append(leadWarnings, string(w))
				}
			}
			if tc.clean {
				if len(leadWarnings) != 0 {
					t.Fatalf("a lead the write path accepts unchanged must be silent: %v", leadWarnings)
				}
				return
			}
			if len(leadWarnings) != 1 {
				t.Fatalf("want exactly one lead advisory, got %v", leadWarnings)
			}
			got := leadWarnings[0]
			if !strings.Contains(got, "dos_lead_case") {
				t.Fatalf("advisory must name the dossier: %q", got)
			}
			for _, sub := range tc.want {
				if !strings.Contains(got, sub) {
					t.Fatalf("advisory %q missing %q", got, sub)
				}
			}
			for _, sub := range tc.notWant {
				if strings.Contains(got, sub) {
					t.Fatalf("advisory %q must not say %q", got, sub)
				}
			}
			if !res.OK {
				t.Fatalf("lead advisories are not store damage; doctor must stay OK: %v", res.Warnings)
			}
		})
	}
}

// joinReaderStore adds the team-audit join records doctor reads to the roster
// test store.
type joinReaderStore struct {
	*rosterTestStore
	joined []string
}

func (s *joinReaderStore) TeamJoinAuthors() ([]string, error) { return s.joined, nil }

// TestDoctorFlagsJoinedLoginMissingFromRoster is the Geoff case: the roster lists
// geoffclawson, but the Mac login that joined is gclawson. Both sides are
// flagged, so the manager can correct the roster.
func TestDoctorFlagsJoinedLoginMissingFromRoster(t *testing.T) {
	store := &joinReaderStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster: Roster{Manager: "hgill", Members: map[string]string{
				"hgill": "Herwin Gill", "geoffclawson": "Geoff Clawson",
			}},
		},
		joined: []string{"hgill", "gclawson"},
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{Author: "hgill"}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, w := range res.Warnings {
		text = append(text, string(w))
	}
	joined := strings.Join(text, "\n")
	for _, want := range []string{"gclawson joined the team store, but is not on the roster", "geoffclawson is on the roster but has not joined"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in warnings:\n%s", want, joined)
		}
	}
}

// TestDoctorJoinCheckQuietWhenRosterMatchesJoins confirms a clean team produces
// no join advisories.
func TestDoctorJoinCheckQuietWhenRosterMatchesJoins(t *testing.T) {
	store := &joinReaderStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster: Roster{Manager: "hgill", Members: map[string]string{
				"hgill": "Herwin Gill", "gclawson": "Geoff Clawson",
			}},
		},
		joined: []string{"hgill", "gclawson"},
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{Author: "hgill"}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range res.Warnings {
		if strings.Contains(string(w), "roster") && (strings.Contains(string(w), "joined the team store") || strings.Contains(string(w), "has not joined")) {
			t.Fatalf("unexpected join advisory: %s", w)
		}
	}
}
