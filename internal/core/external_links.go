package core

import (
	"regexp"
	"strings"
)

// ExternalLink is a navigational pointer recorded in a Dossier body. A
// non-empty LastPolled value is meaningful only for links returned in the
// ActiveMonitors collection from ParseExternalLinks.
type ExternalLink struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	LastPolled  string `json:"last_polled,omitempty"`
}

// ExternalLinkSet is the parsed, section-aware view of a Dossier's external
// pointers. References and monitors share the same Markdown line grammar; the
// section determines whether the URL is navigational or must be polled.
type ExternalLinkSet struct {
	References     []ExternalLink
	ActiveMonitors []ExternalLink
}

var externalLinkLinePattern = regexp.MustCompile(`^\s*-\s+\[([^:\]]+):\s+([^\]]+)\]\(([^)\s]+)\)\s*(?:—|-)\s*(.*?)\s*$`)
var lastPolledPattern = regexp.MustCompile(`\s*\(Last polled: (\d{4}-\d{2}-\d{2})\)\s*$`)

// ParseExternalLinks extracts the canonical References and Active Monitors
// list items from a Distilled State body. It deliberately parses only list
// items beneath the exact section headings, so examples or ordinary Markdown
// links elsewhere in the body cannot become navigable dossier metadata.
func ParseExternalLinks(body string) ExternalLinkSet {
	var result ExternalLinkSet
	section := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "## References":
			section = "references"
			continue
		case "## Active Monitors":
			section = "monitors"
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			section = ""
			continue
		}
		if section == "" {
			continue
		}

		match := externalLinkLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		link := ExternalLink{
			Kind:        strings.ToLower(strings.TrimSpace(match[1])),
			Label:       strings.TrimSpace(match[2]),
			URL:         strings.TrimSpace(match[3]),
			Description: strings.TrimSpace(match[4]),
		}
		if section == "monitors" {
			if polled := lastPolledPattern.FindStringSubmatch(link.Description); polled != nil {
				link.LastPolled = polled[1]
				link.Description = strings.TrimSpace(strings.TrimSuffix(link.Description, polled[0]))
			}
			result.ActiveMonitors = append(result.ActiveMonitors, link)
		} else {
			result.References = append(result.References, link)
		}
	}
	return result
}
