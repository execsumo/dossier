package core

import "strings"

// Query is a compiled search query: whitespace-separated terms, all of which
// must match. The zero value matches everything.
type Query struct {
	terms []string
}

// NewQuery compiles raw user input into a Query. Case-insensitive.
func NewQuery(raw string) Query {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	return Query{terms: terms}
}

// IsEmpty reports whether the query imposes no constraint.
func (q Query) IsEmpty() bool {
	return len(q.terms) == 0
}

// Matches reports whether an already-lowercased haystack satisfies every term.
func (q Query) Matches(haystack string) bool {
	for _, term := range q.terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

// Haystack builds the lowercased searchable text for one item.
func Haystack(item ListItem) string {
	fields := []string{item.Name, item.Description, item.Lead}
	fields = append(fields, item.Interfaces...)
	fields = append(fields, item.Slug)
	return strings.ToLower(strings.Join(fields, "\n"))
}

// MatchesQuery is the one-shot convenience form used by Service.List.
func MatchesQuery(item ListItem, raw string) bool {
	return NewQuery(raw).Matches(Haystack(item))
}
