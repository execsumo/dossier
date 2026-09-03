package core

import (
	"strings"
	"unicode"
)

// MergeLegacyOpenQuestions moves questions from the former frontmatter list
// into the canonical Markdown section. Existing body text is preserved and a
// question already present in that section is not appended again.
func MergeLegacyOpenQuestions(body string, questions []string) string {
	if len(questions) == 0 {
		return body
	}

	lines := strings.Split(body, "\n")
	heading := -1
	sectionEnd := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if heading < 0 {
			if trimmed == "## Open Questions" {
				heading = i
			}
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			sectionEnd = i
			break
		}
	}

	seen := make(map[string]bool)
	if heading >= 0 {
		for _, line := range lines[heading+1 : sectionEnd] {
			if key := openQuestionKey(line); key != "" {
				seen[key] = true
			}
		}
	}

	var additions []string
	for _, question := range questions {
		question = openQuestionText(question)
		key := openQuestionKey(question)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		additions = append(additions, "- "+question)
	}
	if len(additions) == 0 {
		return body
	}

	if heading >= 0 {
		insertAt := sectionEnd
		for insertAt > heading+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		merged := make([]string, 0, len(lines)+len(additions))
		merged = append(merged, lines[:insertAt]...)
		merged = append(merged, additions...)
		merged = append(merged, lines[insertAt:]...)
		return strings.Join(merged, "\n")
	}

	insertAt := len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Current State" {
			insertAt = i
			break
		}
	}
	section := []string{"## Open Questions", ""}
	section = append(section, additions...)

	prefix := append([]string{}, lines[:insertAt]...)
	for len(prefix) > 0 && strings.TrimSpace(prefix[len(prefix)-1]) == "" {
		prefix = prefix[:len(prefix)-1]
	}
	suffix := append([]string{}, lines[insertAt:]...)
	result := prefix
	if len(result) > 0 {
		result = append(result, "")
	}
	result = append(result, section...)
	if len(suffix) > 0 {
		result = append(result, "")
		result = append(result, suffix...)
	}
	return strings.Join(result, "\n")
}

func openQuestionKey(question string) string {
	return strings.ToLower(strings.Join(strings.Fields(openQuestionText(question)), " "))
}

func openQuestionText(question string) string {
	question = strings.TrimSpace(question)
	for _, prefix := range []string{"- [ ] ", "* [ ] ", "+ [ ] ", "- ", "* ", "+ "} {
		if strings.HasPrefix(question, prefix) {
			question = strings.TrimSpace(strings.TrimPrefix(question, prefix))
			break
		}
	}
	if dot := strings.Index(question, ". "); dot > 0 {
		allDigits := true
		for _, r := range question[:dot] {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			question = strings.TrimSpace(question[dot+2:])
		}
	}
	return question
}
