package core

import (
	"sort"
	"strings"
)

// NormalizeUsername converts an OS or organization login into its stable
// username. Domain prefixes and email suffixes are not part of identity.
func NormalizeUsername(username string) string {
	username = strings.TrimSpace(username)
	if i := strings.LastIndexAny(username, `\\/`); i >= 0 {
		username = username[i+1:]
	}
	if i := strings.IndexByte(username, '@'); i >= 0 {
		username = username[:i]
	}
	return strings.ToLower(strings.TrimSpace(username))
}

// SanitizeAuthorString is the path-safe form used for author shard names.
func SanitizeAuthorString(author string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(author) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		return "unknown"
	}
	return value
}

// Roster maps stable usernames to the names teammates see. Former keeps
// removed people resolvable for historical lead values without making them
// eligible for new assignments.
type Roster struct {
	Manager string            `yaml:"manager" json:"manager"`
	Members map[string]string `yaml:"members" json:"members"`
	Former  map[string]string `yaml:"former,omitempty" json:"former,omitempty"`
}

// RosterMember is the stable, display-oriented representation used by team
// surfaces. It avoids exposing map iteration order to humans or JSON clients.
type RosterMember struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// RosterView is the ordered read-only team representation returned by adapters.
type RosterView struct {
	Manager string         `json:"manager"`
	Members []RosterMember `json:"members"`
	Former  []RosterMember `json:"former,omitempty"`
}

// View returns current members with the manager first, then case-insensitive
// display-name order. Former members are sorted by the same stable rule.
func (r Roster) View() RosterView {
	return RosterView{
		Manager: r.Manager,
		Members: orderedRosterMembers(r.Members, r.Manager),
		Former:  orderedRosterMembers(r.Former, ""),
	}
}

func orderedRosterMembers(members map[string]string, manager string) []RosterMember {
	ordered := make([]RosterMember, 0, len(members))
	manager = NormalizeUsername(manager)
	for username, displayName := range members {
		ordered = append(ordered, RosterMember{Username: NormalizeUsername(username), DisplayName: displayName})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		iManager := NormalizeUsername(ordered[i].Username) == manager && manager != ""
		jManager := NormalizeUsername(ordered[j].Username) == manager && manager != ""
		if iManager != jManager {
			return iManager
		}
		left, right := strings.ToLower(strings.TrimSpace(ordered[i].DisplayName)), strings.ToLower(strings.TrimSpace(ordered[j].DisplayName))
		if left != right {
			return left < right
		}
		return ordered[i].Username < ordered[j].Username
	})
	return ordered
}

func (r *Roster) normalize() {
	if r.Members == nil {
		r.Members = map[string]string{}
	}
	if r.Former == nil {
		r.Former = map[string]string{}
	}
}

// DisplayName returns a roster display name, or the username when unknown.
func (r Roster) DisplayName(username string) string {
	username = NormalizeUsername(username)
	for key, name := range r.Members {
		if NormalizeUsername(key) == username {
			if strings.TrimSpace(name) != "" {
				return name
			}
			return username
		}
	}
	for key, name := range r.Former {
		if NormalizeUsername(key) == username {
			if strings.TrimSpace(name) != "" {
				return name
			}
			return username
		}
	}
	return username
}

// Has reports whether username is a current roster member.
func (r Roster) Has(username string) bool {
	username = NormalizeUsername(username)
	for key := range r.Members {
		if NormalizeUsername(key) == username {
			return true
		}
	}
	return false
}

// leadScope is the resolved form of a requested lead filter. Resolving once
// before the scan keeps lead identity matching consistent across every dossier.
type leadScope struct {
	active   bool
	username string
	literal  string
}

// newLeadScope resolves a lead filter against the roster. A non-empty
// candidates result means the requested name was ambiguous.
func newLeadScope(roster *Roster, hasRoster bool, raw, currentUsername string) (scope leadScope, candidates []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return leadScope{}, nil
	}
	// "me" is reserved for the current user, even if a roster contains that username.
	if strings.EqualFold(raw, "me") {
		return leadScope{active: true, username: currentUsername}, nil
	}
	// Roster resolution includes former members: filtering historical work differs from assignment.
	if hasRoster && roster != nil {
		if username, ok, matches := roster.ResolvePerson(raw); ok {
			return leadScope{active: true, username: username}, nil
		} else if len(matches) > 0 {
			return leadScope{active: true}, matches
		}
	}
	// Preserve free-form and legacy leads when the roster cannot resolve the query.
	return leadScope{active: true, literal: NormalizeUsername(raw)}, nil
}

func (l leadScope) matches(storedLead string) bool {
	if !l.active {
		return true
	}
	if l.literal != "" {
		return NormalizeUsername(storedLead) == l.literal
	}
	return NormalizeUsername(storedLead) == l.username
}

// ResolvePerson resolves a username, display name, or unique display-name
// prefix. Ambiguous matches are returned in candidates and never guessed.
func (r Roster) ResolvePerson(query string) (username string, ok bool, candidates []string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false, nil
	}
	normalized := NormalizeUsername(query)
	for key := range r.Members {
		if NormalizeUsername(key) == normalized {
			return NormalizeUsername(key), true, nil
		}
	}
	for key := range r.Former {
		if NormalizeUsername(key) == normalized {
			return NormalizeUsername(key), true, nil
		}
	}

	type person struct{ username, display string }
	people := make([]person, 0, len(r.Members)+len(r.Former))
	seen := map[string]bool{}
	for key, display := range r.Members {
		username := NormalizeUsername(key)
		if !seen[username] {
			people = append(people, person{username, display})
			seen[username] = true
		}
	}
	for key, display := range r.Former {
		username := NormalizeUsername(key)
		if !seen[username] {
			people = append(people, person{username, display})
		}
	}

	lowerQuery := strings.ToLower(query)
	for _, p := range people {
		if strings.EqualFold(strings.TrimSpace(p.display), query) {
			candidates = append(candidates, p.username)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	if len(candidates) > 1 {
		sort.Strings(candidates)
		return "", false, candidates
	}

	for _, p := range people {
		display := strings.ToLower(strings.TrimSpace(p.display))
		first := display
		if i := strings.IndexAny(first, " \t"); i >= 0 {
			first = first[:i]
		}
		if strings.HasPrefix(display, lowerQuery) || strings.HasPrefix(first, lowerQuery) {
			candidates = append(candidates, p.username)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	return "", false, candidates
}
