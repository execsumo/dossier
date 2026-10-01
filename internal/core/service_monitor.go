package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MonitorPolledReq updates the polling date for one Active Monitor through Save.
type MonitorPolledReq struct {
	ID   string
	URL  string
	Date string // optional YYYY-MM-DD; defaults to the service clock's current date
}

// MonitorPolled replaces the selected monitor's Last polled marker using the
// normal optimistic-concurrency Save path. URL must identify exactly one monitor.
func (s *Service) MonitorPolled(ctx context.Context, req MonitorPolledReq) (Result, error) {
	dossier, revision, err := s.store.Read(req.ID)
	if err != nil {
		return Result{OK: false}, err
	}
	date := strings.TrimSpace(req.Date)
	if date == "" {
		date = s.clock.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "monitor poll date must be YYYY-MM-DD")
	}
	if strings.TrimSpace(req.URL) == "" {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "monitor URL is required")
	}

	lines := strings.Split(strings.ReplaceAll(dossier.DistilledState.Body, "\r\n", "\n"), "\n")
	section := ""
	matches := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "## Active Monitors":
			section = "monitors"
		case "## References":
			section = "references"
		default:
			if strings.HasPrefix(trimmed, "## ") {
				section = ""
			}
		}
		if section != "monitors" {
			continue
		}
		parsed := ParseExternalLinks("## Active Monitors\n" + line)
		for _, link := range parsed.ActiveMonitors {
			if link.URL != req.URL {
				continue
			}
			matches++
			lines[i] = fmt.Sprintf("- [%s: %s](%s) — %s (Last polled: %s)", link.Kind, link.Label, link.URL, link.Description, date)
		}
	}
	if matches == 0 {
		return Result{OK: false}, NewError(ErrNotFound, fmt.Sprintf("active monitor %q not found", req.URL))
	}
	if matches > 1 {
		return Result{OK: false}, NewError(ErrAmbiguousTarget, fmt.Sprintf("active monitor URL %q occurs more than once", req.URL))
	}

	body := strings.Join(lines, "\n")
	return s.Save(ctx, SaveReq{ID: dossier.Frontmatter.ID, BaseRevision: revision, DistilledStateMarkdown: body})
}
