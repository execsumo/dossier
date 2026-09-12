package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// TeamCreateReq specifies parameters for creating a new team store.
type TeamCreateReq struct {
	RemoteURL string
	Branch    string
}

// TeamCreate initializes the current store as a team store and pushes to the remote.
func (s *Service) TeamCreate(ctx context.Context, req TeamCreateReq) (Result, error) {
	if req.RemoteURL == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "remote URL is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if s.syncer == nil {
		return Result{}, NewError(ErrInternal, "syncer is not configured")
	}

	err := s.syncer.Create(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "already a team store") {
			return Result{}, NewError(ErrConflictDetected, "store is already a team store")
		}
		return Result{}, fmt.Errorf("team create failed: %w", err)
	}

	return Result{OK: true}, nil
}

// TeamJoinReq specifies parameters for joining an existing team store.
type TeamJoinReq struct {
	RemoteURL string
	Branch    string
}

// TeamJoin joins an existing team store by cloning it locally.
func (s *Service) TeamJoin(ctx context.Context, req TeamJoinReq) (Result, error) {
	if req.RemoteURL == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "remote URL is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if s.syncer == nil {
		return Result{}, NewError(ErrInternal, "syncer is not configured")
	}

	err := s.syncer.Clone(ctx, req.RemoteURL, s.cfg.DossierHome, 50)
	if err != nil {
		if strings.Contains(err.Error(), "target directory is not empty") {
			return Result{}, NewError(ErrConflictDetected, "target directory is not empty; cannot join into an existing store")
		}
		return Result{}, fmt.Errorf("team join failed: %w", err)
	}

	initRes, initErr := s.Init(ctx, InitReq{})
	if initErr != nil {
		return Result{}, fmt.Errorf("post-join init failed: %w", initErr)
	}

	return Result{OK: true, Warnings: initRes.Warnings}, nil
}

// Sync orchestrates the dossier team sync.
func (s *Service) Sync(ctx context.Context) (Result, error) {
	if s.syncer == nil {
		return Result{OK: false}, NewError(ErrInternal, "team sync is not configured; set team.remote in config")
	}

	report, err := s.syncer.Sync(ctx)
	if err != nil {
		return Result{OK: false}, fmt.Errorf("sync failed: %w", err)
	}

	var warnings []Warning
	if report.Error != "" {
		warnings = append(warnings, Warning(fmt.Sprintf("Sync network error: %s", report.Error)))
	}

	for _, excl := range report.Excluded {
		warnings = append(warnings, Warning(excl.Warning))
	}

	var createdConflicts []string
	for i, conf := range report.Conflicts {
		slugParts := strings.Split(filepath.ToSlash(conf.Path), "/")
		slug := slugParts[0] // always the dossier slug
		var targetID string
		fms, listErr := s.store.List("all")
		if listErr == nil {
			for _, fm := range fms {
				if fm.Slug == slug {
					targetID = fm.ID
					break
				}
			}
		}
		if targetID == "" {
			// If we couldn't resolve the dossier ID by slug, just use the slug as ID fallback.
			targetID = slug
		}

		confID := fmt.Sprintf("conf_%s_%s_%d", s.clock.Now().Format("20060102150405"), slug, i)
		conflict := &Conflict{
			ID:                 confID,
			DossierID:          targetID,
			Kind:               "sync_concurrent_edit",
			BaseRevision:       conf.LocalRevision,
			AttemptedRevision:  conf.RemoteRevision,
			TS:                 s.clock.Now(),
			RejectedBody:       string(conf.LocalContent),
			DiffAgainstCurrent: GenerateUnifiedDiff(string(conf.RemoteContent), string(conf.LocalContent)),
		}

		writeErr := s.store.WriteConflict(conflict)
		if writeErr == nil {
			createdConflicts = append(createdConflicts, confID)
			_ = s.store.AppendAudit(targetID, AuditEvent{
				TS:             s.clock.Now(),
				Event:          AuditEventConflictCreated,
				Author:         s.cfg.Author,
				DossierID:      targetID,
				BeforeRevision: conf.LocalRevision,
				AfterRevision:  conf.RemoteRevision,
				Message:        fmt.Sprintf("Conflict %s created due to sync concurrent edit on %s", confID, conf.Path),
			})
			warnings = append(warnings, Warning(fmt.Sprintf("wrote conflicts/%s.md — remote won the working tree, your version preserved", confID)))
		} else {
			warnings = append(warnings, Warning(fmt.Sprintf("failed to write conflict for %s: %v", conf.Path, writeErr)))
		}
	}

	return Result{
		OK:       report.Error == "",
		Data:     report,
		Warnings: warnings,
	}, nil
}

func (s *Service) SyncStatus(ctx context.Context) (Result, error) {
	if s.syncer == nil {
		return Result{OK: false}, NewError(ErrInternal, "team sync is not configured; set team.remote in config")
	}

	status, err := s.syncer.Status(ctx)
	if err != nil {
		return Result{OK: false}, fmt.Errorf("status failed: %w", err)
	}

	return Result{
		OK:   true,
		Data: status,
	}, nil
}
