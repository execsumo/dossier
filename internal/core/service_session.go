package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) ContextRefresh(ctx context.Context) (Result, error) {
	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to list dossiers for context refresh", err)
	}

	// Filter and sort open dossiers (non-archived) by canonical priority.
	var openDossierFrontmatter []ListedFrontmatter
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			openDossierFrontmatter = append(openDossierFrontmatter, fm)
		}
	}
	sortListedFrontmatters(openDossierFrontmatter)

	var openDossiers []LibraryDossier
	for _, fm := range openDossierFrontmatter {
		openDossiers = append(openDossiers, LibraryDossier{
			Name:        fm.Name,
			Description: fm.Description,
			Status:      string(fm.Status),
			Slug:        fm.Slug,
			NextAction:  fm.NextAction,
			Priority:    string(fm.Priority),
		})
	}

	// Report the harness this process is running under, not the first one the
	// registry happens to list as installed.
	activeHarness, activeCaps := s.activeHarness()

	harnessName := "CLI"
	harnessCaps := map[string]bool{
		"MCP":               false,
		"SessionStartHook":  false,
		"SessionEndHook":    false,
		"PreCompactionHook": false,
		"TranscriptCapture": false,
	}
	var warnings []string

	if activeHarness != nil {
		harnessName = displayHarnessName(activeHarness.Name())

		harnessCaps["MCP"] = activeCaps.MCP
		harnessCaps["SessionStartHook"] = activeCaps.SessionStartHook
		harnessCaps["SessionEndHook"] = activeCaps.SessionEndHook
		harnessCaps["PreCompactionHook"] = activeCaps.PreCompactionHook
		harnessCaps["TranscriptCapture"] = activeCaps.TranscriptCapture

		if !activeCaps.TranscriptCapture {
			warnings = append(warnings, "Transcript archive is unavailable in this session.")
		}
	} else {
		warnings = append(warnings, "No harness session active. Run from within a supported client harness for full integration.")
	}

	libData := LibraryData{
		Harness:      harnessName,
		Capabilities: harnessCaps,
		Warnings:     warnings,
		OpenDossiers: openDossiers,
	}

	if err := s.store.WriteLibraryContext(libData); err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to write library context", err)
	}

	return Result{
		OK: true,
	}, nil
}

type SwitchReq struct {
	ID        string
	SessionID string
	// HarnessName is the harness the adapter resolved this session id from, or
	// the configured launch profile for a newly spawned session. Empty means
	// unknown (explicit override, manual CLI), in which case the binding falls
	// back to whichever harness is detected.
	HarnessName string
}

func (s *Service) Switch(ctx context.Context, req SwitchReq) (Result, error) {
	if req.SessionID == "" {
		return Result{}, NewError(ErrInternal, "session_id is required for switch")
	}

	oldBinding, err := s.store.GetSessionBinding(req.SessionID)
	// The Guide is dossier-independent, so switching topics inside one session
	// earns no re-send; carry the delivery marker across the rebind.
	var guideDeliveredAt time.Time
	if err == nil && oldBinding != nil {
		guideDeliveredAt = oldBinding.GuideDeliveredAt
		if oldBinding.DossierID != "" {
			_ = s.store.ClearSessionBinding(req.SessionID)
		}
	}

	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	// Record the harness this session actually ran under. Detection order alone
	// would credit the first configured harness on the machine, so a Pi session
	// would be filed as Claude Code — along with Claude Code's capabilities.
	activeHarness, activeCaps := s.sessionHarness(req.HarnessName)

	harnessName := "CLI"
	if req.HarnessName != "" {
		harnessName = req.HarnessName
	}
	if activeHarness != nil {
		harnessName = activeHarness.Name()
	}

	binding := &SessionBinding{
		SessionBindingID: req.SessionID,
		Harness:          harnessName,
		DossierID:        d.Frontmatter.ID,
		BoundAt:          s.clock.Now(),
		LastSeenRevision: string(rev),
		Capabilities:     activeCaps,
		GuideDeliveredAt: guideDeliveredAt,
	}
	if err := s.store.SaveSessionBinding(binding); err != nil {
		return Result{}, WrapError(ErrInternal, "failed to save session binding", err)
	}

	return s.Recall(ctx, RecallReq{ID: d.Frontmatter.ID})
}

type ActiveReq struct {
	SessionID string
}

func (s *Service) Active(ctx context.Context, req ActiveReq) (Result, error) {
	if req.SessionID == "" {
		return Result{}, NewError(ErrInternal, "session_id is required")
	}

	binding, err := s.store.GetSessionBinding(req.SessionID)
	if err != nil {
		return Result{}, err
	}

	return Result{
		OK:   true,
		Data: binding,
	}, nil
}

type ArchiveReq struct {
	ID string
}

func (s *Service) Archive(ctx context.Context, req ArchiveReq) (Result, error) {
	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	d.Frontmatter.Status = StatusDone

	newRev, err := s.store.Write(d, rev)
	if err != nil {
		return Result{}, err
	}

	_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventArchived,
		Author:         s.cfg.Author,
		DossierID:      d.Frontmatter.ID,
		BeforeRevision: string(rev),
		AfterRevision:  string(newRev),
	})

	return Result{
		OK:   true,
		Data: newRev,
	}, nil
}

type PathReq struct {
	ID string
}

func (s *Service) Path(ctx context.Context, req PathReq) (Result, error) {
	d, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	dossierPath := filepath.Join(s.cfg.DossierHome, d.Frontmatter.Slug)
	return Result{
		OK:   true,
		Data: dossierPath,
	}, nil
}

// SessionStart returns the injected context payload for a harness session.
func (s *Service) SessionStart(ctx context.Context, sessionID string) (string, error) {
	var syncResult *Result
	var syncErr error
	if s.syncer != nil {
		syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err := s.Sync(syncCtx)
		cancel()
		syncResult = &res
		syncErr = err
	}

	binding, err := s.store.GetSessionBinding(sessionID)
	var activeDossierID string
	if err == nil && binding != nil {
		activeDossierID = binding.DossierID
	}

	attentionLine, boundConflicts, needsAttention := s.syncAttention(ctx, activeDossierID, syncResult, syncErr)

	// Fetch open dossiers
	fms, err := s.store.List("all")
	if err != nil {
		return "", err
	}

	sortListedFrontmatters(fms)
	var names []string
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			names = append(names, fm.Name)
		}
	}
	namesStr := "(none)"
	if len(names) > 0 {
		namesStr = strings.Join(names, ", ")
	}

	// Detect capabilities for the harness owning this process.
	activeHarness, activeCaps := s.activeHarness()

	var sb strings.Builder
	sb.WriteString("# Dossier Library\n\n")

	if needsAttention {
		if len(boundConflicts) > 0 {
			sb.WriteString(attentionLine + " (Conflict IDs: " + strings.Join(boundConflicts, ", ") + ")\n\n")
		} else {
			sb.WriteString(attentionLine + "\n\n")
		}
	}

	if activeHarness != nil && !activeCaps.TranscriptCapture {
		sb.WriteString("Warning: Transcript archive is unavailable in this session.\n\n")
	}

	// Deliberately a single-line nudge, not a full payload: this fires on every
	// session regardless of relevance to Dossier, so it must stay cheap for
	// sessions that don't touch it. The heavy payload (Distillation Guide, full
	// Distilled State) is delivered by the MCP tool calls themselves — see
	// dossier_session's response — the moment the agent actually enters a
	// dossier's context, not passively here.
	sb.WriteString(fmt.Sprintf(
		"%d open dossier(s): %s. Before choosing or creating a topic, use dossier_list to check for a match; use dossier_promote for a confirmed new thread, dossier_session to bind/resume one, or dossier_recall to read its state. Guide: ~/.dossier/context/guide.md\n",
		len(names), namesStr,
	))

	if activeDossierID != "" {
		// Deliver the Guide here, at the earliest point in the session, so it is
		// in context ahead of any tool call the agent makes rather than arriving
		// alongside one. Resetting first is what makes that true after a
		// compaction: the marker survives in the binding, but the context the
		// Guide was written into does not.
		s.resetGuideDelivery(sessionID)
		if guide := s.GuideForSession(sessionID); guide != "" {
			sb.WriteString("\nDistillation Guide:\n")
			sb.WriteString(guide)
			sb.WriteString("\n")
		}

		recallRes, err := s.Recall(ctx, RecallReq{ID: activeDossierID})
		if err == nil {
			recData := recallRes.Data.(RecallResult)
			sb.WriteString("\nActive Dossier:\n")
			sb.WriteString(fmt.Sprintf("ID: %s\n", recData.Frontmatter.ID))
			sb.WriteString(fmt.Sprintf("Name: %s\n", recData.Frontmatter.Name))
			sb.WriteString(fmt.Sprintf("Revision: %s\n\n", recData.Revision))
			sb.WriteString("Distilled State:\n")
			sb.WriteString(recData.DistilledState)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func (s *Service) GetGuide() string {
	guide, err := s.store.ReadContextAsset("guide.md")
	if err != nil {
		return ""
	}
	return guide
}

// GuideForSession returns the Distillation Guide the first time it is requested
// within a session and "" on every request after that, so the session-start hook
// and the dossier_session response stop spending the same ~3.5k tokens twice on
// a resumed or post-compaction session.
//
// Suppression is deliberately biased toward delivering: an unknown session, an
// unreadable binding, or a failed write of the marker all return the Guide. The
// Guide must be in context *before* the agent composes a write, so a duplicate
// copy is a cost and a missing copy is a correctness failure — when the two
// trade off, pay the cost.
func (s *Service) GuideForSession(sessionID string) string {
	guide := s.GetGuide()
	if guide == "" || sessionID == "" {
		return guide
	}

	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil {
		return guide
	}
	if !binding.GuideDeliveredAt.IsZero() {
		return ""
	}

	binding.GuideDeliveredAt = s.clock.Now()
	_ = s.store.SaveSessionBinding(binding)
	return guide
}

// resetGuideDelivery clears the delivery marker so the next GuideForSession call
// re-sends. Called at session start, where the context window is new by
// definition — a startup, a resume, or the rebuild that follows compaction — and
// any Guide delivered earlier in the session is no longer in it.
func (s *Service) resetGuideDelivery(sessionID string) {
	if sessionID == "" {
		return
	}
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil || binding.GuideDeliveredAt.IsZero() {
		return
	}
	binding.GuideDeliveredAt = time.Time{}
	_ = s.store.SaveSessionBinding(binding)
}

func (s *Service) GetInstructions() string {
	instructions, err := s.store.ReadContextAsset("instructions.md")
	if err != nil {
		return ""
	}
	return instructions
}

// EnsureContextAssets refreshes the on-disk projection of the embedded context
// assets. Callers run it at wiring time so an upgraded binary never reads the
// previous release's Guide.
func (s *Service) EnsureContextAssets() ([]string, error) {
	return s.store.EnsureContextAssets()
}

// SessionEnd saves state and appends the transcript artifact on session completion.
func (s *Service) SessionEnd(ctx context.Context, sessionID string, distilledState string, transcript string) ([]Warning, error) {
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil {
		return nil, nil
	}

	now := s.clock.Now()
	finalRevision := Revision(binding.LastSeenRevision)
	var warnings []Warning

	// Read the revision before anything below writes, so this reflects only what
	// the session itself persisted. Sampling it later would fold in this hook's
	// own transcript artifact and report every session as having saved.
	var persistedDuringSession bool
	if _, revAtBoundary, revErr := s.store.Read(binding.DossierID); revErr == nil {
		persistedDuringSession = string(revAtBoundary) != binding.LastSeenRevision
	}

	if distilledState != "" {
		saveRes, err := s.Save(ctx, SaveReq{
			ID:                     binding.DossierID,
			BaseRevision:           Revision(binding.LastSeenRevision),
			DistilledStateMarkdown: distilledState,
			SessionID:              sessionID,
		})
		if err != nil {
			return warnings, err
		}
		finalRevision = saveRes.Data.(Revision)
	}

	if transcript != "" {
		// The stash keeps the raw trace byte-for-byte; the artifact stores the
		// compiled full view, whose physical line numbers are stable enough to
		// cite. A range into raw JSONL lands mid-record and cites nothing.
		compiled, compiledFormat, compileWarnings := CompileTranscript(transcript)
		for _, w := range compileWarnings {
			warnings = append(warnings, w)
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   string(w),
			})
		}

		if stashErr := s.store.WriteSessionStash(binding.DossierID, s.cfg.Author, sessionID, transcript); stashErr != nil {
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   fmt.Sprintf("Warning: failed to write session stash: %v", stashErr),
			})
		}

		art := Artifact{
			DossierID:     binding.DossierID,
			Type:          ArtifactTypeTranscript,
			Title:         binding.Harness + " Session Transcript",
			Provenance:    Provenance{Origin: binding.Harness + " session transcript (compiled)", Harness: binding.Harness},
			ContentFormat: compiledFormat,
			Content:       compiled,
			CapturedAt:    now,
			RefreshedAt:   now,
		}
		if err := s.store.WriteArtifact(binding.DossierID, &art); err != nil {
			return warnings, err
		}
		_, refreshedRev, err := s.store.Read(binding.DossierID)
		if err != nil {
			return warnings, err
		}
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:             now,
			Event:          AuditEventSave,
			Author:         s.cfg.Author,
			DossierID:      binding.DossierID,
			SessionID:      sessionID,
			BeforeRevision: string(finalRevision),
			AfterRevision:  string(refreshedRev),
			ArtifactsAdded: []string{art.ID},
		})
		finalRevision = refreshedRev
	} else {
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:        now,
			Event:     AuditEventTranscriptCaptureUnavailable,
			Author:    s.cfg.Author,
			DossierID: binding.DossierID,
			SessionID: sessionID,
			Message:   "Session boundary reached without transcript payload; no transcript artifact was captured.",
		})
	}

	if distilledState == "" {
		// No harness in the registry supplies a distilled_state payload on its
		// lifecycle hooks — a hook runs a binary, it cannot ask the agent to
		// distill. So this branch is the normal path, and the only question is
		// whether the session persisted anything on its own.
		if persistedDuringSession {
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   "Session boundary reached without distilled_state payload; Distilled State was already saved during the session.",
			})
		} else {
			// Nothing reached the Distilled State this session and the boundary
			// cannot distill on the agent's behalf. Per the degrade-visibly rule
			// this is a surfaced warning, not an audit line nobody reads.
			w := Warning("Distilled State was not updated this session — the session-end boundary cannot distill on the agent's behalf, and nothing was saved while the session ran. The transcript is archived, so nothing is lost, but resuming this Dossier will show the state as of its last explicit save. Save during the session (dossier_save) as decisions land.")
			warnings = append(warnings, w)
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventDistilledStateNotCaptured,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   string(w),
			})
		}
	}

	if finalRevision != "" && string(finalRevision) != binding.LastSeenRevision {
		binding.LastSeenRevision = string(finalRevision)
		_ = s.store.SaveSessionBinding(binding)
	}

	if s.syncer != nil {
		syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		// Outcome is persisted via syncState and reported on the next SessionStart;
		// do not add noise here, as this hook's output is likely invisible.
		_, _ = s.Sync(syncCtx) // Best-effort bounded push
	}

	return warnings, nil
}
