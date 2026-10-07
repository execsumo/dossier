package core

import (
	"context"
	"fmt"
	"strings"
)

// AddReferenceReq appends one external pointer to a Dossier's ## References.
type AddReferenceReq struct {
	Actor       string
	ID          string
	URL         string
	Label       string // defaults to the URL host
	Kind        string // defaults to "link"
	Description string // optional
}

// AddReference records an external URL as a canonical References list item
// (`- [kind: Label](url) — description`) through the normal Save path, creating
// the section when the body has none. A URL already listed under References is
// rejected rather than duplicated.
func (s *Service) AddReference(ctx context.Context, req AddReferenceReq) (Result, error) {
	dossier, revision, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}

	rawURL := strings.TrimSpace(req.URL)
	host, ok := referenceHost(rawURL)
	if !ok {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "reference URL must be an absolute http(s) URL without spaces or ')'")
	}
	oneLine := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	kind := strings.ToLower(oneLine(req.Kind))
	if kind == "" {
		kind = "link"
	}
	label := oneLine(req.Label)
	if label == "" {
		label = host
	}
	if strings.ContainsAny(kind, ":]") || strings.Contains(label, "]") {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "reference kind and label cannot contain ':' or ']'")
	}
	line := fmt.Sprintf("- [%s: %s](%s) —", kind, label, rawURL)
	if desc := oneLine(req.Description); desc != "" {
		line += " " + desc
	}

	for _, existing := range ParseExternalLinks(dossier.DistilledState.Body).References {
		if existing.URL == rawURL {
			return Result{OK: false}, NewError(ErrInvalidFrontmatter, fmt.Sprintf("reference %q is already recorded", rawURL))
		}
	}

	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(dossier.DistilledState.Body, "\r\n", "\n"), "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "## References" {
			start = i
			break
		}
	}
	var out []string
	if start < 0 {
		out = append(append(lines, "", "## References", ""), line)
	} else {
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
				end = i
				break
			}
		}
		// Insert after the section's last non-blank line so spacing before the
		// next heading is preserved.
		at := end
		for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		if at == start+1 {
			line = "\n" + line // keep a blank line under the heading
			at = start + 1
		}
		out = append(append(append([]string{}, lines[:at]...), line), lines[at:]...)
	}

	return s.Save(ctx, SaveReq{Actor: req.Actor, ID: dossier.Frontmatter.ID, BaseRevision: revision, DistilledStateMarkdown: strings.Join(out, "\n") + "\n"})
}

// referenceHost returns the authority of an http(s) URL. It is a deliberately
// small check (core imports no net packages): a scheme, a non-empty host, and
// none of the characters that would break the Markdown link syntax.
func referenceHost(raw string) (string, bool) {
	if strings.ContainsAny(raw, " \t\r\n)") {
		return "", false
	}
	rest, found := strings.CutPrefix(raw, "https://")
	if !found {
		if rest, found = strings.CutPrefix(raw, "http://"); !found {
			return "", false
		}
	}
	host := rest
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	return host, host != ""
}
