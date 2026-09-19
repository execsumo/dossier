package core

import (
	"reflect"
	"testing"
)

func TestNewLeadScope(t *testing.T) {
	roster := &Roster{
		Members: map[string]string{
			"psmith": "Priya Shah",
			"hgill":  "Herwin Gill",
			"me":     "Someone Else",
			"btwo":   "Bob Two",
			"bthree": "Bob Three",
		},
		Former: map[string]string{"old": "Former Person"},
	}

	tests := []struct {
		name       string
		query      string
		current    string
		hasRoster  bool
		want       leadScope
		candidates []string
		stored     string
		matches    bool
	}{
		{name: "empty", want: leadScope{}, stored: "any", matches: true},
		{name: "me", query: "me", current: "psmith", hasRoster: true, want: leadScope{active: true, username: "psmith"}, stored: "psmith", matches: true},
		{name: "reserved me", query: "me", current: "psmith", hasRoster: true, want: leadScope{active: true, username: "psmith"}, stored: "me", matches: false},
		{name: "username", query: "PSMITH", hasRoster: true, want: leadScope{active: true, username: "psmith"}, stored: "psmith", matches: true},
		{name: "display name", query: "priya shah", hasRoster: true, want: leadScope{active: true, username: "psmith"}, stored: "psmith", matches: true},
		{name: "first name", query: "Priya", hasRoster: true, want: leadScope{active: true, username: "psmith"}, stored: "psmith", matches: true},
		{name: "former member", query: "Former Person", hasRoster: true, want: leadScope{active: true, username: "old"}, stored: "old", matches: true},
		{name: "ambiguous", query: "Bob", hasRoster: true, want: leadScope{active: true}, candidates: []string{"bthree", "btwo"}},
		{name: "unknown roster", query: "Nobody", hasRoster: true, want: leadScope{active: true, unresolved: true, literal: "nobody"}, stored: "nobody", matches: true},
		{name: "free form", query: "Alice", want: leadScope{active: true, unresolved: true, literal: "alice"}, stored: "Alice@corp.com", matches: true},
		// A query that normalizes away entirely must not become an unassigned filter.
		{name: "normalizes to nothing", query: "@corp.com", hasRoster: true, want: leadScope{active: true, unresolved: true}, stored: "", matches: false},
		{name: "normalizes to nothing matches no lead", query: "/", hasRoster: true, want: leadScope{active: true, unresolved: true}, stored: "psmith", matches: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, candidates := newLeadScope(roster, tt.hasRoster, tt.query, tt.current)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("scope = %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(candidates, tt.candidates) {
				t.Fatalf("candidates = %v, want %v", candidates, tt.candidates)
			}
			if len(candidates) == 0 && got.matches(tt.stored) != tt.matches {
				t.Fatalf("scope.matches(%q) = %v, want %v", tt.stored, got.matches(tt.stored), tt.matches)
			}
		})
	}
}
