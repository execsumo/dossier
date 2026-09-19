package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	ConflictChoiceKeepShared  = "keep_shared"
	ConflictChoiceRestoreMine = "restore_mine"
	ConflictChoiceKeepBoth    = "keep_both"
)

type ResolveConflictReq struct {
	ConflictID string
	Choice     string
	Author     string
}

// ListConflicts returns unresolved conflicts only; the store owns the archive
// boundary and therefore never exposes conflicts moved under resolved/.
func (s *Service) ListConflicts(ctx context.Context) ([]Conflict, error) {
	return s.store.ListConflicts()
}

// ConflictDetail reads the current shared body and computes a fresh diff
// against the preserved proposal. Resolved or unknown conflicts, and conflicts
// whose dossier no longer exists, all return the typed not_found error.
func (s *Service) ConflictDetail(ctx context.Context, conflictID string) (ConflictDetail, error) {
	conflict, err := s.store.ReadConflict(conflictID)
	if err != nil {
		return ConflictDetail{}, err
	}
	if conflict.Kind == "sync_concurrent_roster_edit" || conflict.DossierID == RosterConflictDossierID {
		rosterStore, ok := s.store.(RosterConflictStore)
		if !ok {
			return ConflictDetail{}, NewError(ErrNotFound, "team roster conflict storage is not available")
		}
		shared, err := rosterStore.ReadRosterYAML()
		if err != nil {
			return ConflictDetail{}, err
		}
		diff := GenerateUnifiedDiff(shared, conflict.RejectedBody)
		currentConflict := *conflict
		currentConflict.DiffAgainstCurrent = diff
		return ConflictDetail{
			Conflict:    currentConflict,
			DossierName: "Team roster",
			DossierSlug: "team",
			Shared:      shared,
			Mine:        conflict.RejectedBody,
			Diff:        diff,
		}, nil
	}
	dossier, _, err := s.store.Read(conflict.DossierID)
	if err != nil {
		var domainErr *DomainError
		if errors.As(err, &domainErr) && domainErr.Code == ErrNotFound {
			return ConflictDetail{}, domainErr
		}
		return ConflictDetail{}, WrapError(ErrNotFound, "conflict dossier not found", err)
	}
	shared := dossier.DistilledState.Body
	diff := GenerateUnifiedDiff(shared, conflict.RejectedBody)
	currentConflict := *conflict
	currentConflict.DiffAgainstCurrent = diff
	return ConflictDetail{
		Conflict:    currentConflict,
		DossierName: dossier.Frontmatter.Name,
		DossierSlug: dossier.Frontmatter.Slug,
		Shared:      shared,
		Mine:        conflict.RejectedBody,
		Diff:        diff,
	}, nil
}

// ResolveConflict applies one explicit choice, then archives the conflict.
// Choice validation deliberately happens before reading anything so malformed
// requests cannot produce partial reads or writes.
func (s *Service) ResolveConflict(ctx context.Context, req ResolveConflictReq) (Result, error) {
	if req.Choice != ConflictChoiceKeepShared && req.Choice != ConflictChoiceRestoreMine && req.Choice != ConflictChoiceKeepBoth {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter,
			"invalid conflict choice: must be keep_shared, restore_mine, or keep_both")
	}

	conflict, err := s.store.ReadConflict(req.ConflictID)
	if err != nil {
		return Result{OK: false}, err
	}
	if conflict.Kind == "sync_concurrent_roster_edit" || conflict.DossierID == RosterConflictDossierID {
		return s.resolveRosterConflict(conflict, req)
	}

	dossier, currentRevision, err := s.store.Read(conflict.DossierID)
	if err != nil {
		var domainErr *DomainError
		if errors.As(err, &domainErr) && domainErr.Code == ErrNotFound {
			return Result{OK: false}, domainErr
		}
		return Result{OK: false}, WrapError(ErrNotFound, "conflict dossier not found", err)
	}

	var result Result
	switch req.Choice {
	case ConflictChoiceKeepShared:
		result = Result{OK: true, Data: currentRevision}
	case ConflictChoiceRestoreMine:
		restoredBody := conflict.RejectedBody
		if !strings.HasSuffix(restoredBody, "\n") {
			restoredBody += "\n"
		}
		result, err = s.Save(ctx, SaveReq{
			ID:                     dossier.Frontmatter.ID,
			BaseRevision:           currentRevision,
			DistilledStateMarkdown: restoredBody,
		})
	case ConflictChoiceKeepBoth:
		body := fmt.Sprintf("%s\n\n## Unresolved disagreement (conflict %s)\n\nThe version below was preserved from a concurrent edit on %s; reconcile and remove this section.\n\n%s\n",
			dossier.DistilledState.Body,
			conflict.ID,
			conflict.TS.Format("2006-01-02 15:04:05"),
			conflict.RejectedBody,
		)
		result, err = s.Save(ctx, SaveReq{
			ID:                     dossier.Frontmatter.ID,
			BaseRevision:           currentRevision,
			DistilledStateMarkdown: body,
		})
	}
	if err != nil {
		return Result{OK: false}, err
	}

	author := req.Author
	if author == "" {
		author = s.cfg.Author
	}
	now := s.clock.Now()
	conflict.ResolvedAt = &now
	conflict.ResolvedBy = author
	conflict.Choice = req.Choice
	if err := s.store.ResolveConflict(conflict.ID, conflict); err != nil {
		return Result{OK: false}, err
	}
	if err := s.store.AppendAudit(conflict.DossierID, AuditEvent{
		TS:             now,
		Event:          AuditEventConflictResolved,
		Author:         author,
		DossierID:      conflict.DossierID,
		BeforeRevision: string(currentRevision),
		AfterRevision:  revisionFromResult(result, currentRevision),
		Message:        fmt.Sprintf("Resolved conflict %s with %s", conflict.ID, req.Choice),
	}); err != nil {
		return Result{OK: false}, err
	}
	return result, nil
}

func (s *Service) resolveRosterConflict(conflict *Conflict, req ResolveConflictReq) (Result, error) {
	rosterStore, ok := s.store.(RosterConflictStore)
	if !ok {
		return Result{OK: false}, NewError(ErrNotFound, "team roster conflict storage is not available")
	}
	shared, err := rosterStore.ReadRoster()
	if err != nil {
		return Result{OK: false}, err
	}
	if shared == nil {
		shared = &Roster{}
	}
	shared.normalize()

	var resolved Roster
	var warnings []Warning
	switch req.Choice {
	case ConflictChoiceKeepShared:
		resolved = *shared
	case ConflictChoiceRestoreMine:
		decoded, decodeErr := rosterStore.DecodeRosterYAML(conflict.RejectedBody)
		if decodeErr != nil {
			return Result{OK: false}, fmt.Errorf("restore team roster: %w", decodeErr)
		}
		resolved = *decoded
		if err := rosterStore.WriteRoster(&resolved); err != nil {
			return Result{OK: false}, err
		}
	case ConflictChoiceKeepBoth:
		mine, decodeErr := rosterStore.DecodeRosterYAML(conflict.RejectedBody)
		if decodeErr != nil {
			return Result{OK: false}, fmt.Errorf("merge team roster: %w", decodeErr)
		}
		resolved, warnings = unionRosters(*shared, *mine)
		if err := rosterStore.WriteRoster(&resolved); err != nil {
			return Result{OK: false}, err
		}
	}

	author := req.Author
	if author == "" {
		author = s.cfg.Author
	}
	now := s.clock.Now()
	conflict.ResolvedAt = &now
	conflict.ResolvedBy = author
	conflict.Choice = req.Choice
	// There is no dossier to receive an audit shard for a root team.yaml
	// conflict; the archived conflict frontmatter is the resolution audit.
	if err := s.store.ResolveConflict(conflict.ID, conflict); err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: resolved, Warnings: warnings}, nil
}

func unionRosters(shared, mine Roster) (Roster, []Warning) {
	shared.normalize()
	mine.normalize()
	merged := Roster{
		Manager: NormalizeUsername(shared.Manager),
		Members: map[string]string{},
		Former:  map[string]string{},
	}
	if merged.Manager == "" {
		merged.Manager = NormalizeUsername(mine.Manager)
	}

	sharedNames := make(map[string]string, len(shared.Members)+len(shared.Former))
	addShared := func(source map[string]string, target map[string]string) {
		for username, displayName := range source {
			normalized := NormalizeUsername(username)
			if normalized == "" {
				continue
			}
			if _, exists := sharedNames[normalized]; exists {
				continue
			}
			target[normalized] = displayName
			sharedNames[normalized] = displayName
		}
	}
	addShared(shared.Members, merged.Members)
	addShared(shared.Former, merged.Former)

	var clashes []string
	clashSeen := map[string]bool{}
	addMine := func(source map[string]string, target map[string]string) {
		for username, displayName := range source {
			normalized := NormalizeUsername(username)
			if normalized == "" {
				continue
			}
			if sharedName, exists := sharedNames[normalized]; exists {
				if !strings.EqualFold(strings.TrimSpace(sharedName), strings.TrimSpace(displayName)) && !clashSeen[normalized] {
					clashSeen[normalized] = true
					clashes = append(clashes, fmt.Sprintf("roster display-name clash for %s: shared %q wins over preserved %q", normalized, sharedName, displayName))
				}
				continue
			}
			if _, exists := merged.Members[normalized]; exists {
				continue
			}
			if _, exists := merged.Former[normalized]; exists {
				continue
			}
			target[normalized] = displayName
		}
	}
	addMine(mine.Members, merged.Members)
	addMine(mine.Former, merged.Former)
	sort.Strings(clashes)
	warnings := make([]Warning, len(clashes))
	for i, clash := range clashes {
		warnings[i] = Warning(clash)
	}
	return merged, warnings
}

func revisionFromResult(result Result, fallback Revision) string {
	if revision, ok := result.Data.(Revision); ok {
		return string(revision)
	}
	return string(fallback)
}
