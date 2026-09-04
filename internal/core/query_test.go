package core

import "testing"

func TestQueryMatchesListItemFields(t *testing.T) {
	item := ListItem{
		Name:        "Quarterly Planning",
		Description: "Prepare the billing forecast",
		Lead:        "Ada Lovelace",
		Interfaces:  []string{"Pricing WBR", "Steerco"},
		Slug:        "q1-planning",
	}

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"empty query", "", true},
		{"case insensitive", "QUARTERLY", true},
		{"multi term AND", "quarterly billing", true},
		{"description", "forecast", true},
		{"lead", "lovelace", true},
		{"interface", "steerco", true},
		{"slug", "q1-planning", true},
		{"non matching", "roadmap", false},
		{"does not cross fields", "planningprepare", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesQuery(item, tt.query); got != tt.want {
				t.Fatalf("MatchesQuery(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestQueryWhitespaceIsAND(t *testing.T) {
	q := NewQuery("  BILLING   forecast ")
	if q.IsEmpty() || !q.Matches("billing forecast") || q.Matches("billing only") {
		t.Fatalf("unexpected query behavior for whitespace-separated terms")
	}
}
