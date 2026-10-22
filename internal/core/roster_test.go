package core

import (
	"strings"
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`ACME\PSmith`, "psmith"},
		{"AzureAD\\psmith", "psmith"},
		{"psmith", "psmith"},
		{"hgill", "hgill"},
		{"Priya.Shah", "priya.shah"},
		{"psmith@acme.com", "psmith"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeUsername(tt.input); got != tt.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRosterViewOrdersManagerAndDisplayNames(t *testing.T) {
	view := (Roster{Manager: "bob", Members: map[string]string{
		"alice": "Zoe", "bob": "Bob", "carol": "alice",
	}}).View()
	if got := []string{view.Members[0].Username, view.Members[1].Username, view.Members[2].Username}; got[0] != "bob" || got[1] != "carol" || got[2] != "alice" {
		t.Fatalf("member order = %v", got)
	}
}

func TestRosterResolvePerson(t *testing.T) {
	roster := Roster{
		Manager: "hgill",
		Members: map[string]string{
			"psmith":  "Priya Shah",
			"p singh": "Priya Singh",
			"jdoe":    "Jordan Doe",
		},
		Former: map[string]string{"old": "Former Person"},
	}
	tests := []struct {
		name       string
		query      string
		want       string
		ok         bool
		candidates []string
	}{
		{"username", "PSMITH", "psmith", true, nil},
		{"full name", "priya shah", "psmith", true, nil},
		{"unique first name", "Jordan", "jdoe", true, nil},
		{"ambiguous first name", "Priya", "", false, []string{"p singh", "psmith"}},
		{"former member", "Former Person", "old", true, nil},
		{"unknown", "Nobody", "", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, candidates := roster.ResolvePerson(tt.query)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ResolvePerson(%q) = (%q, %t, %v), want (%q, %t, %v)", tt.query, got, ok, candidates, tt.want, tt.ok, tt.candidates)
			}
			if len(candidates) != len(tt.candidates) {
				t.Fatalf("candidates = %v, want %v", candidates, tt.candidates)
			}
			for i := range candidates {
				if candidates[i] != tt.candidates[i] {
					t.Errorf("candidate %d = %q, want %q", i, candidates[i], tt.candidates[i])
				}
			}
		})
	}
}

// TestJoinMismatchAdvisories is the pure form of the join check. A recorded
// login that is not a current human member is flagged; so is a current human
// member with no record. Agents are not expected to join from a Mac login.
func TestJoinMismatchAdvisories(t *testing.T) {
	roster := Roster{
		Manager: "hgill",
		Members: map[string]string{"hgill": "Herwin Gill", "geoffclawson": "Geoff Clawson", "sitroom": "Sit Room"},
		Former:  map[string]string{"olduser": "Old User"},
		Kinds:   map[string]string{"sitroom": "agent"},
	}
	cases := []struct {
		name   string
		joined []string
		want   []string // substrings, one per expected advisory
	}{
		{name: "matching login is quiet", joined: []string{"hgill", "geoffclawson", "sitroom"}},
		{name: "login not on roster", joined: []string{"hgill", "gclawson", "geoffclawson", "sitroom"}, want: []string{"gclawson joined the team store"}},
		{name: "roster member never joined", joined: []string{"hgill", "sitroom"}, want: []string{"geoffclawson is on the roster but has not joined"}},
		{name: "domain-prefixed login normalizes", joined: []string{`ACME\Hgill`, "geoffclawson", "sitroom"}},
		{name: "former member joining is flagged", joined: []string{"hgill", "geoffclawson", "sitroom", "olduser"}, want: []string{"olduser joined the team store"}},
		{name: "agent without record is not flagged", joined: []string{"hgill", "geoffclawson"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := roster.JoinMismatchAdvisories(tc.joined)
			if len(got) != len(tc.want) {
				t.Fatalf("advisories = %v, want %d matching %v", got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Fatalf("advisory %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}
