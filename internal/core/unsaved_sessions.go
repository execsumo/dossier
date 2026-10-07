package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// maxUnsavedSessionsListed caps how many sessions the recovery notice names.
// The remainder is counted in a "+N more" tail, never dropped silently.
const maxUnsavedSessionsListed = 5

// UnsavedSession is a session whose boundary (end or pre-compaction) passed
// without the Distilled State being saved, and whose work has not been folded
// into the Distilled State since.
type UnsavedSession struct {
	SessionID string
	// TS is the latest uncleared boundary for the session.
	TS     time.Time
	Author string
	// TranscriptArtifactIDs are the transcripts archived at the session's
	// uncleared boundaries, oldest first. Empty when no transcript was captured.
	TranscriptArtifactIDs []string
}

// UnsavedSessions derives the sessions that ended without saving the
// Distilled State and have not been recovered since.
//
// Nothing is persisted for this: it is computed from the audit log every time,
// so it clears itself once the work is recovered, and it is identical on every
// machine that has the (Team Sync'd) audit shards.
//
// A boundary is recovered when the current Distilled State mentions the
// transcript archived at it, either as a [src:] citation or as its line in the
// Evidence index. Any later body change is not enough: another session saving
// its own, unrelated work would otherwise silently clear a notice for work
// nobody distilled. Requiring the mention ties clearing to the act the
// Operating Instructions ask for (distill the transcript, cite the spans), and
// an Evidence line ("background only") is the honest way to acknowledge a
// session that established nothing material.
//
// A boundary with no captured transcript has nothing to mention, so it falls
// back to the weaker signal: a later audit event whose history snapshots
// (ReadRevision) at BeforeRevision and AfterRevision hold different Distilled
// State bodies. Bodies are compared rather than trusting the event type or
// revision hash, because artifact-only and frontmatter-only saves also advance
// the revision. A snapshot that cannot be read never counts as a change, so
// the notice errs toward staying visible.
func (s *Service) UnsavedSessions(dossierID string) ([]UnsavedSession, error) {
	events, err := s.store.ReadAuditLog(dossierID)
	if err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	// Stable, so events sharing a timestamp keep the order the store gave them.
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	bodies := map[Revision]*string{}
	bodyAt := func(rev Revision) (string, bool) {
		if p, seen := bodies[rev]; seen {
			if p == nil {
				return "", false
			}
			return *p, true
		}
		d, err := s.store.ReadRevision(dossierID, rev)
		if err != nil || d == nil {
			bodies[rev] = nil
			return "", false
		}
		b := d.DistilledState.Body
		bodies[rev] = &b
		return b, true
	}
	var currentBody string
	var haveCurrent bool
	if d, _, err := s.store.Read(dossierID); err == nil && d != nil {
		currentBody, haveCurrent = d.DistilledState.Body, true
	}

	var order []string
	bySession := map[string]*UnsavedSession{}
	for i, e := range events {
		if e.Event != AuditEventDistilledStateNotCaptured {
			continue
		}
		var artifactIDs []string
		for _, t := range events {
			// The transcript save is written at the same instant, in the same
			// call, as the boundary event it belongs to.
			if t.Event == AuditEventSave && t.SessionID == e.SessionID && t.TS.Equal(e.TS) && len(t.ArtifactsAdded) > 0 {
				artifactIDs = append(artifactIDs, t.ArtifactsAdded...)
			}
		}

		var cleared bool
		if len(artifactIDs) > 0 {
			cleared = haveCurrent && mentionsAnyArtifact(currentBody, artifactIDs)
		} else {
			for _, later := range events[i+1:] {
				if later.BeforeRevision == "" || later.AfterRevision == "" || later.BeforeRevision == later.AfterRevision {
					continue
				}
				before, okB := bodyAt(Revision(later.BeforeRevision))
				after, okA := bodyAt(Revision(later.AfterRevision))
				if okB && okA && before != after {
					cleared = true
					break
				}
			}
		}
		if cleared {
			continue
		}

		u, ok := bySession[e.SessionID]
		if !ok {
			u = &UnsavedSession{SessionID: e.SessionID}
			bySession[e.SessionID] = u
			order = append(order, e.SessionID)
		}
		u.TS, u.Author = e.TS, e.Author
		for _, id := range artifactIDs {
			dup := false
			for _, have := range u.TranscriptArtifactIDs {
				dup = dup || have == id
			}
			if !dup {
				u.TranscriptArtifactIDs = append(u.TranscriptArtifactIDs, id)
			}
		}
	}

	out := make([]UnsavedSession, 0, len(order))
	for _, id := range order {
		out = append(out, *bySession[id])
	}
	return out, nil
}

// mentionsAnyArtifact reports whether body names any of ids as a whole token,
// so art_01ab does not match inside art_01abc.
func mentionsAnyArtifact(body string, ids []string) bool {
	for _, id := range ids {
		for rest := body; ; {
			i := strings.Index(rest, id)
			if i < 0 {
				break
			}
			end := i + len(id)
			if end == len(rest) || !isArtifactIDByte(rest[end]) {
				return true
			}
			rest = rest[end:]
		}
	}
	return false
}

func isArtifactIDByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// FormatUnsavedSessionsNotice renders the recovery notice. Every surface
// (SessionStart, dossier_session, dossier_recall) goes through it so they
// cannot diverge. It states facts and does not issue instructions: the
// instruction to act on it lives in the Operating Instructions.
func FormatUnsavedSessionsNotice(dossierName string, sessions []UnsavedSession) string {
	if len(sessions) == 0 {
		return ""
	}
	shown := sessions
	if len(shown) > maxUnsavedSessionsListed {
		shown = shown[:maxUnsavedSessionsListed]
	}
	parts := make([]string, 0, len(shown))
	for _, u := range shown {
		sid := u.SessionID
		if sid == "" {
			sid = "unknown"
		}
		detail := fmt.Sprintf("session %s, %s", sid, u.TS.UTC().Format("2006-01-02"))
		if u.Author != "" {
			detail += ", by " + u.Author
		}
		if len(u.TranscriptArtifactIDs) == 0 {
			parts = append(parts, fmt.Sprintf("no transcript was captured (%s)", detail))
		} else {
			parts = append(parts, fmt.Sprintf("transcript %s (%s)", strings.Join(u.TranscriptArtifactIDs, ", "), detail))
		}
	}
	tail := ""
	if more := len(sessions) - len(shown); more > 0 {
		tail = fmt.Sprintf(" (+%d more)", more)
	}
	return fmt.Sprintf(
		"Dossier %s: %d session(s) ended without saving the Distilled State since its last save. Unsaved work is archived in %s%s. The Distilled State predates this work.",
		dossierName, len(sessions), strings.Join(parts, "; "), tail)
}

// UnsavedSessionsNotice is the derived notice for one Dossier, or "" when there
// is nothing to recover or it cannot be derived. The notice is advisory, so a
// failure to compute it must not fail the call that carries it.
func (s *Service) UnsavedSessionsNotice(dossierID, dossierName string) string {
	sessions, err := s.UnsavedSessions(dossierID)
	if err != nil {
		return ""
	}
	return FormatUnsavedSessionsNotice(dossierName, sessions)
}
