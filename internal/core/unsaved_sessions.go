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
// Distilled State and that no save has followed since.
//
// Nothing is persisted for this: it is computed from the audit log every time,
// so it clears itself, and it is identical on every machine that has the
// (Team Sync'd) audit shards.
//
// A boundary clears once a later audit event's history snapshots
// (ReadRevision) at BeforeRevision and AfterRevision hold different Distilled
// State bodies, or once the current Distilled State mentions the transcript
// archived at it. Saving is the user's job (B29), so the notice answers only
// "has the Dossier been saved since?", not "was this session's transcript
// distilled?". Bodies are compared rather than trusting the event type or
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

		// Any later save that changes the body clears it: saving is the user's
		// job (B29), so the question is only whether the Dossier has been saved
		// since, not whether this session's transcript was distilled. Citing
		// the transcript also clears it.
		cleared := len(artifactIDs) > 0 && haveCurrent && mentionsAnyArtifact(currentBody, artifactIDs)
		if !cleared {
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

// FormatUnsavedSessionsNotice renders the notice. Every surface (SessionStart,
// dossier_session, dossier_recall) goes through it so they cannot diverge.
//
// It is a fact, not a work order. Saving is the user's job, before they exit
// (/save-dossier, prompted by the Stop-hook checkpoint); a missed save is
// surfaced so the gap is visible, not reconstructed. The notice therefore names
// sessions but not their transcript artifacts: naming them invited agents to
// load whole transcripts at the start of a session, which is the context cost
// the Distilled State exists to avoid. Other authors' sessions are counted and
// attributed only.
func FormatUnsavedSessionsNotice(dossierName, currentAuthor string, sessions []UnsavedSession) string {
	var own []UnsavedSession
	othersByAuthor := map[string]int{}
	var otherAuthors []string
	for _, u := range sessions {
		if u.Author == "" || u.Author == currentAuthor {
			own = append(own, u)
			continue
		}
		if othersByAuthor[u.Author] == 0 {
			otherAuthors = append(otherAuthors, u.Author)
		}
		othersByAuthor[u.Author]++
	}
	if len(own) == 0 && len(otherAuthors) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Dossier %s:", dossierName)
	if len(own) > 0 {
		shown := own
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
			parts = append(parts, detail)
		}
		tail := ""
		if more := len(own) - len(shown); more > 0 {
			tail = fmt.Sprintf(" (+%d more)", more)
		}
		fmt.Fprintf(&sb, " %d session(s) ended without saving the Distilled State since its last save (%s%s). The Distilled State predates this work.",
			len(own), strings.Join(parts, "; "), tail)
	}
	if len(otherAuthors) > 0 {
		total := 0
		named := make([]string, 0, len(otherAuthors))
		for _, a := range otherAuthors {
			total += othersByAuthor[a]
			named = append(named, fmt.Sprintf("%s (%d)", a, othersByAuthor[a]))
		}
		also := ""
		if len(own) > 0 {
			also = " also"
		}
		fmt.Fprintf(&sb, " %d session(s) by other authors%s ended without saving: %s.",
			total, also, strings.Join(named, ", "))
	}
	return sb.String()
}

// UnsavedSessionsNotice is the derived notice for one Dossier, or "" when there
// is nothing to recover or it cannot be derived. The notice is advisory, so a
// failure to compute it must not fail the call that carries it.
func (s *Service) UnsavedSessionsNotice(dossierID, dossierName string) string {
	sessions, err := s.UnsavedSessions(dossierID)
	if err != nil {
		return ""
	}
	return FormatUnsavedSessionsNotice(dossierName, s.cfg.Author, sessions)
}
