package core

import "testing"

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
