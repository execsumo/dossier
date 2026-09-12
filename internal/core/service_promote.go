package core

import (
	"context"
	"fmt"
)

// Stubs for future milestones

type PromoteReq struct {
	Name                   string
	Description            string
	Priority               Priority
	DistilledStateMarkdown string
	FromFilePath           string
	Content                string
	Lead                   string
	Interfaces             []string
	Force                  bool
}

func (s *Service) Promote(ctx context.Context, req PromoteReq) (Result, error) {
	if req.Name == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "dossier name is required")
	}

	now := s.clock.Now()
	var warnings []Warning

	if !req.Force {
		scorer := newDossierScorer(req.Name, now)
		var candidates []Suggestion
		score := func(d *Dossier) error {
			sug := scorer.Score(d)
			if sug.Confidence == "high" || sug.Confidence == "medium" {
				candidates = keepTopSuggestions(candidates, sug, 3)
			}
			return nil
		}
		scanWarnings, scanErr := s.scanDossiers("all", score)
		warnings = append(warnings, scanWarnings...)
		if scanErr != nil {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Ambiguity check unavailable: %v", scanErr)))
		} else {
			if len(candidates) > 0 {
				return Result{
					OK:       false,
					Data:     candidates,
					Warnings: warnings,
					NextActions: []NextAction{
						`Present the candidates to the user: "I found Dossiers that look related — [for each: Name (status, N days since last update)]. Is one of these the right one to continue, or is this a separate thread?"`,
						`If the user picks one: call dossier_session with its slug to bind it, then dossier_recall to load its state.`,
						`If the user confirms this is a new topic: call dossier_promote again with force=true.`,
					},
				}, NewError(ErrAmbiguousTarget, "Multiple likely Dossiers match this promote request.")
			}
		}
	}

	updates := map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"lead":        req.Lead,
		"interfaces":  req.Interfaces,
	}
	if req.Priority != "" {
		updates["priority"] = string(req.Priority)
	}
	saveRes, newID, err := s.save(ctx, SaveReq{
		DistilledStateMarkdown: req.DistilledStateMarkdown,
		FrontmatterUpdates:     updates,
	})
	if err != nil {
		return Result{}, err
	}

	newRevision := saveRes.Data.(Revision)
	if newID == "" {
		return Result{}, NewError(ErrInternal, "could not locate the newly promoted dossier for artifact capture")
	}

	if req.Content != "" && newID != "" {
		compiled, compiledFormat, compileWarnings := CompileTranscript(req.Content)
		warnings = append(warnings, compileWarnings...)

		// Compilation is a view, never a replacement for source. If it changes
		// the supplied bytes, archive the raw JSONL first so a later compiled or
		// audit write failure still cannot destroy the only lossless copy.
		if compiled != req.Content {
			raw := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Raw Captured Session Transcript (JSONL)",
				Provenance:    Provenance{Origin: "promote session content (raw JSONL, byte-preserved)"},
				ContentFormat: ContentFormatText,
				Content:       req.Content,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			var writeErr error
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &raw,
				"Archived byte-preserved raw promote session transcript before compilation.")
			if writeErr != nil {
				return Result{}, writeErr
			}

			compiledArt := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Compiled Captured Session Transcript",
				Provenance:    Provenance{Origin: fmt.Sprintf("promote session content (compiled citable view of %s)", raw.ID)},
				ContentFormat: compiledFormat,
				Content:       compiled,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &compiledArt,
				fmt.Sprintf("Archived compiled citable transcript view derived from raw artifact %s.", raw.ID))
			if writeErr != nil {
				return Result{}, writeErr
			}
		} else {
			art := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Captured Session Transcript",
				Provenance:    Provenance{Origin: "promote session content (verbatim plain-text passthrough)"},
				ContentFormat: compiledFormat,
				Content:       compiled,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			var writeErr error
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &art,
				"Archived verbatim plain-text promote session transcript.")
			if writeErr != nil {
				return Result{}, writeErr
			}
		}
	}

	activeHarness, activeCaps := s.activeHarness()
	if activeHarness == nil || !activeCaps.TranscriptCapture {
		warnings = append(warnings, Warning("Transcript archive is unavailable in this session."))
	}

	return Result{
		OK:       true,
		Data:     newID,
		Warnings: warnings,
	}, nil
}

// writePromoteTranscriptArtifact persists and audits one promote capture. Each
// source/view write is audited independently so a partial failure remains
// legible, and every store error is returned rather than silently ignored.
func (s *Service) writePromoteTranscriptArtifact(dossierID string, before Revision, art *Artifact, message string) (Revision, error) {
	if err := s.store.WriteArtifact(dossierID, art); err != nil {
		return before, fmt.Errorf("archive promote transcript %q: %w", art.Title, err)
	}
	_, after, err := s.store.Read(dossierID)
	if err != nil {
		return before, fmt.Errorf("read revision after archiving promote transcript %s: %w", art.ID, err)
	}
	if err := s.store.AppendAudit(dossierID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventSave,
		Author:         s.cfg.Author,
		DossierID:      dossierID,
		BeforeRevision: string(before),
		AfterRevision:  string(after),
		ArtifactsAdded: []string{art.ID},
		Message:        message,
	}); err != nil {
		return after, fmt.Errorf("audit promote transcript artifact %s: %w", art.ID, err)
	}
	return after, nil
}
