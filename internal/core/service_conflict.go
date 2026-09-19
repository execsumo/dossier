package core

import (
	"context"
	"errors"
	"fmt"
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

func revisionFromResult(result Result, fallback Revision) string {
	if revision, ok := result.Data.(Revision); ok {
		return string(revision)
	}
	return string(fallback)
}
