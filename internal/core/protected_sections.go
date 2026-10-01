package core

import (
	"sort"
	"strings"
)

var protectedAgentSections = map[string]bool{
	"Objective":   true,
	"Done When":   true,
	"Validation":  true,
	"Constraints": true,
	"Decisions":   true,
}

func changedProtectedSections(before, after string) []string {
	oldSections, newSections := levelTwoSections(before), levelTwoSections(after)
	var changed []string
	for name := range protectedAgentSections {
		if strings.TrimSpace(oldSections[name]) != strings.TrimSpace(newSections[name]) {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func levelTwoSections(body string) map[string]string {
	sections := make(map[string]string)
	current := ""
	var content []string
	fenced := false
	flush := func() {
		if current != "" {
			sections[current] = strings.Join(content, "\n")
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			content = nil
			continue
		}
		if current != "" {
			content = append(content, line)
		}
	}
	flush()
	return sections
}
