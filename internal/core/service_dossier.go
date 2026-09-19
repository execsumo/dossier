package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type SaveReq struct {
	ID                     string
	BaseRevision           Revision
	DistilledStateMarkdown string
	FrontmatterUpdates     map[string]any
	Artifacts              []Artifact
	SessionID              string
}

// GenerateUnifiedDiff produces a line-by-line diff of two strings using LCS.
func GenerateUnifiedDiff(a, b string) string {
	aLines := strings.Split(strings.ReplaceAll(a, "\r\n", "\n"), "\n")
	bLines := strings.Split(strings.ReplaceAll(b, "\r\n", "\n"), "\n")

	n := len(aLines)
	m := len(bLines)

	if n*m > 10000000 {
		return fmt.Sprintf("--- Diff too large to compute for files of %d and %d lines ---\n\n(See Rejected Proposal for the attempted body)", n, m)
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if aLines[i-1] == bLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	var diff []string
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && aLines[i-1] == bLines[j-1] {
			diff = append(diff, "  "+aLines[i-1])
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			diff = append(diff, "+ "+bLines[j-1])
			j--
		} else if i > 0 && (j == 0 || dp[i-1][j] >= dp[i][j-1]) {
			diff = append(diff, "- "+aLines[i-1])
			i--
		}
	}

	for l, r := 0, len(diff)-1; l < r; l, r = l+1, r-1 {
		diff[l], diff[r] = diff[r], diff[l]
	}

	return strings.Join(diff, "\n")
}

func getFMField(fm Frontmatter, field string) any {
	switch field {
	case "name":
		return fm.Name
	case "description":
		return fm.Description
	case "status":
		return string(fm.Status)
	case "lead":
		return fm.Lead
	case "interfaces":
		return strings.Join(fm.Interfaces, "|||")
	case "next_action":
		return fm.NextAction
	case "priority":
		return string(fm.Priority)
	case "due_date":
		return fm.DueDate
	default:
		return nil
	}
}

func applyFrontmatterUpdates(d *Dossier, updates map[string]any) {
	if val, ok := updates["name"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Name = strVal
		}
	}
	if val, ok := updates["description"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Description = strVal
		}
	}
	if val, ok := updates["status"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Status = NormalizeStatus(Status(strVal))
		}
	}
	if val, ok := updates["lead"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Lead = strVal
		}
	}
	if val, ok := updates["interfaces"]; ok {
		d.Frontmatter.Interfaces = interfaceNamesFromValue(val)
	}
	if val, ok := updates["next_action"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.NextAction = strVal
		}
	}
	if val, ok := updates["priority"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Priority = Priority(strVal)
		}
	}
	if val, ok := updates["due_date"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.DueDate = strVal
		}
	}
}

func interfaceNamesFromValue(value any) []string {
	var names []string
	switch values := value.(type) {
	case []string:
		names = append(names, values...)
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func configuredValueAllowed(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func strictStringSlice(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return append([]string{}, values...), true
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, name)
		}
		return result, true
	default:
		return nil, false
	}
}

func (s *Service) validateConfiguredFrontmatterUpdates(updates map[string]any) error {
	if updates == nil {
		return nil
	}
	roster, hasRoster := s.currentRoster()
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if value, ok := updates["interfaces"]; ok {
		names, valid := strictStringSlice(value)
		if !valid {
			return fmt.Errorf("interfaces must be a list of strings")
		}
		for _, name := range names {
			if !configuredValueAllowed(name, s.cfg.Interfaces) {
				return fmt.Errorf("invalid interface: %q (configure available values in config.yaml)", name)
			}
		}
	}
	if value, ok := updates["lead"]; ok {
		lead, valid := value.(string)
		if !valid {
			return fmt.Errorf("lead must be a string")
		}
		if lead != "" {
			if hasRoster {
				if !roster.Has(lead) {
					return fmt.Errorf("invalid lead: %q (choose a current team member)", lead)
				}
			} else if len(s.cfg.Leads) > 0 && !configuredValueAllowed(lead, s.cfg.Leads) {
				return fmt.Errorf("invalid lead: %q (configure available values in config.yaml)", lead)
			}
		}
	}
	return nil
}

// describeFrontmatterChanges returns a human-readable, audit-friendly summary of
// which frontmatter fields changed between before and after.
func describeFrontmatterChanges(before, after Frontmatter) string {
	var parts []string
	add := func(field, oldVal, newVal string) {
		if oldVal != newVal {
			parts = append(parts, fmt.Sprintf("%s %q→%q", field, oldVal, newVal))
		}
	}
	add("name", before.Name, after.Name)
	add("description", before.Description, after.Description)
	add("status", string(before.Status), string(after.Status))
	add("lead", before.Lead, after.Lead)
	if strings.Join(before.Interfaces, "|||") != strings.Join(after.Interfaces, "|||") {
		parts = append(parts, fmt.Sprintf("interfaces %q→%q", strings.Join(before.Interfaces, ", "), strings.Join(after.Interfaces, ", ")))
	}
	add("next_action", before.NextAction, after.NextAction)
	add("priority", string(before.Priority), string(after.Priority))
	add("due_date", before.DueDate, after.DueDate)
	return strings.Join(parts, "; ")
}

func (s *Service) Save(ctx context.Context, req SaveReq) (Result, error) {
	result, _, err := s.save(ctx, req)
	return result, err
}

// save is the single write path. It additionally returns the immutable dossier
// ID so internal creation workflows do not need a second full-store scan.
func (s *Service) save(ctx context.Context, req SaveReq) (Result, string, error) {
	if updates, err := s.normalizeLeadUpdate(req.FrontmatterUpdates); err != nil {
		return Result{}, "", err
	} else {
		req.FrontmatterUpdates = updates
	}
	if _, ok := req.FrontmatterUpdates["slug"]; ok {
		return Result{}, "", NewError(ErrInvalidFrontmatter, "slug cannot be changed through Save; use Rename")
	}
	if _, ok := req.FrontmatterUpdates["aliases"]; ok {
		return Result{}, "", NewError(ErrInvalidFrontmatter, "slug aliases are not supported")
	}
	if err := s.validateConfiguredFrontmatterUpdates(req.FrontmatterUpdates); err != nil {
		return Result{}, "", WrapError(ErrInvalidFrontmatter, "invalid frontmatter details", err)
	}

	// An explicit priority is user input and must be one of the canonical values.
	if value, ok := req.FrontmatterUpdates["priority"]; ok {
		priority, validType := value.(string)
		if !validType || !Priority(priority).IsValid() {
			return Result{}, "", NewError(ErrInvalidFrontmatter, fmt.Sprintf("invalid priority: %q", priority))
		}
	}

	var d *Dossier
	var baseRev Revision
	var beforeFM Frontmatter
	var err error

	isNew := req.ID == ""
	sessID := req.SessionID
	if sessID == "" {
		sessID = "sess_default"
	}

	if isNew {
		d = &Dossier{
			Frontmatter: Frontmatter{
				Status:   StatusSpark,
				Priority: PriorityMedium,
			},
		}
	} else {
		d, baseRev, err = s.store.Read(req.ID)
		if err != nil {
			return Result{}, "", err
		}
		beforeFM = d.Frontmatter

		if req.BaseRevision != "" && baseRev != req.BaseRevision {
			// Concurrency mismatch! Attempt to read the dossier at the user's base revision.
			dBase, readRevErr := s.store.ReadRevision(req.ID, req.BaseRevision)
			hasConflict := false

			if readRevErr != nil {
				// Base revision not found, treat as conflict
				hasConflict = true
			} else {
				// Check for body conflict:
				// Did body change in store?
				bodyChangedInStore := (d.DistilledState.Body != dBase.DistilledState.Body)
				// Did user change body?
				userBodyChanged := (req.DistilledStateMarkdown != "" && req.DistilledStateMarkdown != dBase.DistilledState.Body)
				// Overlap conflict if both changed and proposed is different from store
				if bodyChangedInStore && userBodyChanged && (req.DistilledStateMarkdown != d.DistilledState.Body) {
					hasConflict = true
				}

				// Check for frontmatter conflict:
				if !hasConflict && req.FrontmatterUpdates != nil {
					for f, proposedVal := range req.FrontmatterUpdates {
						storeVal := getFMField(d.Frontmatter, f)
						baseVal := getFMField(dBase.Frontmatter, f)

						if storeVal != baseVal {
							var normProposedVal any = proposedVal
							if f == "status" || f == "priority" || f == "lead" || f == "description" {
								if sVal, ok := proposedVal.(string); ok {
									normProposedVal = sVal
								}
							}

							if normProposedVal != storeVal {
								hasConflict = true
								break
							}
						}
					}
				}
			}

			if hasConflict {
				confID := "conf_" + s.clock.Now().Format("20060102150405")
				proposedBody := req.DistilledStateMarkdown
				if proposedBody == "" {
					proposedBody = d.DistilledState.Body
				}

				diff := GenerateUnifiedDiff(d.DistilledState.Body, proposedBody)

				conflict := &Conflict{
					ID:                 confID,
					DossierID:          d.Frontmatter.ID,
					Kind:               "distilled_state_concurrent_edit",
					BaseRevision:       string(req.BaseRevision),
					AttemptedRevision:  string(baseRev),
					Session:            sessID,
					TS:                 s.clock.Now(),
					RejectedBody:       proposedBody,
					DiffAgainstCurrent: diff,
				}

				writeErr := s.store.WriteConflict(conflict)
				if writeErr == nil {
					_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
						TS:             s.clock.Now(),
						Event:          AuditEventConflictCreated,
						Author:         s.cfg.Author,
						DossierID:      d.Frontmatter.ID,
						SessionID:      sessID,
						BeforeRevision: string(req.BaseRevision),
						AfterRevision:  string(baseRev),
						Message:        fmt.Sprintf("Conflict %s created due to concurrent edit", conflict.ID),
					})
				}

				return Result{
					OK:   false,
					Data: conflict,
				}, d.Frontmatter.ID, NewError(ErrConcurrentEdit, fmt.Sprintf("concurrency mismatch: base is %q, current is %q. Conflict artifact %s created.", req.BaseRevision, baseRev, conflict.ID))
			}

			// Auto-merge non-overlapping changes!
			if req.DistilledStateMarkdown != "" && dBase != nil && req.DistilledStateMarkdown != dBase.DistilledState.Body {
				d.DistilledState.Body = req.DistilledStateMarkdown
			}
			if req.FrontmatterUpdates != nil {
				applyFrontmatterUpdates(d, req.FrontmatterUpdates)
			}
			// Write with the current revision as the base to succeed
		}
	}

	if req.FrontmatterUpdates != nil {
		applyFrontmatterUpdates(d, req.FrontmatterUpdates)
	}

	if req.DistilledStateMarkdown != "" {
		d.DistilledState.Body = req.DistilledStateMarkdown
	}

	if err := validateNextActionLength(d.Frontmatter.NextAction); err != nil {
		return Result{}, "", WrapError(ErrInvalidFrontmatter, "invalid frontmatter details", err)
	}

	var warnings []Warning
	newRev, err := s.store.Write(d, baseRev)
	if err != nil {
		return Result{}, "", err
	}

	var addedArtifactIDs []string
	for _, art := range req.Artifacts {
		art.DossierID = d.Frontmatter.ID
		if err := s.store.WriteArtifact(d.Frontmatter.ID, &art); err != nil {
			return Result{}, d.Frontmatter.ID, err
		}
		addedArtifactIDs = append(addedArtifactIDs, art.ID)
	}
	if len(addedArtifactIDs) > 0 {
		_, refreshedRev, err := s.store.Read(d.Frontmatter.ID)
		if err != nil {
			return Result{}, d.Frontmatter.ID, err
		}
		newRev = refreshedRev
	}

	event := AuditEvent{
		TS:             s.clock.Now(),
		DossierID:      d.Frontmatter.ID,
		Author:         s.cfg.Author,
		BeforeRevision: string(baseRev),
		AfterRevision:  string(newRev),
		ArtifactsAdded: addedArtifactIDs,
		TokenEstimate:  s.tok.Estimate(d.DistilledState.Body),
	}
	if isNew {
		event.Event = AuditEventCreate
	} else {
		event.Event = AuditEventSave
		if req.FrontmatterUpdates != nil {
			if msg := describeFrontmatterChanges(beforeFM, d.Frontmatter); msg != "" {
				event.Message = msg
			}
			// SPEC §11 (status §300): a lifecycle status change must be auditable as
			// status_changed, even when it arrives via the unified Save path.
			if beforeFM.Status != d.Frontmatter.Status {
				event.Event = AuditEventStatusChanged
			}
		}
	}
	_ = s.store.AppendAudit(d.Frontmatter.ID, event)

	// A save is the moment the curated view and the Archive can drift apart.
	// Surface evidence the Distilled State does not point at rather than let
	// it accumulate unreachably.
	if artifacts, listErr := s.store.ListArtifacts(d.Frontmatter.ID); listErr == nil {
		if msg := uncitedArtifactWarning(d.DistilledState.Body, artifacts); msg != "" {
			warnings = append(warnings, Warning(msg))
		}
	}

	return Result{
		OK:       true,
		Data:     newRev,
		Warnings: warnings,
	}, d.Frontmatter.ID, nil
}

// RenameReq changes a dossier's title and/or canonical slug. At least one of
// NewName and NewSlug must be supplied. BaseRevision is optional for interactive
// callers; when provided it protects against a stale rename.
type RenameReq struct {
	ID           string
	NewSlug      string
	NewName      string
	BaseRevision Revision
}

// RenameSlugReq remains an alias for callers of the original slug-only API.
type RenameSlugReq = RenameReq

// RenameResult describes the committed rename. The immutable ID remains the
// durable identity; callers must use the new canonical slug after a rename.
type RenameResult struct {
	ID       string   `json:"id"`
	OldName  string   `json:"old_name,omitempty"`
	Name     string   `json:"name"`
	OldSlug  string   `json:"old_slug,omitempty"`
	Slug     string   `json:"slug"`
	Revision Revision `json:"revision"`
	Path     string   `json:"path"`
}

// RenameSlugResult remains an alias for callers of the original slug-only API.
type RenameSlugResult = RenameResult

// Rename changes the title and/or slug through one concurrency-checked store
// operation. Keeping this separate from Save ensures a slug change cannot update
// frontmatter without moving the backing directory.
func (s *Service) Rename(ctx context.Context, req RenameReq) (Result, error) {
	if req.ID == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "dossier id or slug is required")
	}
	if req.NewSlug == "" && req.NewName == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "new slug or title is required")
	}
	if req.NewSlug != "" {
		if err := ValidateCanonicalSlug(req.NewSlug); err != nil {
			return Result{}, WrapError(ErrInvalidFrontmatter, "invalid slug", err)
		}
	}
	if req.NewName != "" && strings.TrimSpace(req.NewName) == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "title cannot be blank")
	}

	current, currentRev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}
	old := current.Frontmatter
	newSlug := req.NewSlug
	if newSlug == "" {
		newSlug = old.Slug
	}
	newName := req.NewName
	if newName == "" {
		newName = old.Name
	}
	if newSlug == old.Slug && newName == old.Name {
		return Result{OK: true, Data: RenameResult{
			ID: old.ID, Name: old.Name, Slug: old.Slug,
			Revision: currentRev, Path: filepath.Join(s.cfg.DossierHome, old.Slug),
		}}, nil
	}

	base := req.BaseRevision
	if base == "" {
		base = currentRev
	}
	var updated *Dossier
	var newRev Revision
	if renamer, ok := s.store.(Renamer); ok {
		updated, newRev, err = renamer.Rename(old.ID, newSlug, newName, base)
	} else if req.NewName == "" {
		// Preserve compatibility with stores that only implement the original port.
		updated, newRev, err = s.store.RenameSlug(old.ID, newSlug, base)
	} else if newSlug == old.Slug {
		// A title-only rename is safely representable by the older Save port.
		saved, saveErr := s.Save(ctx, SaveReq{ID: old.ID, BaseRevision: base, FrontmatterUpdates: map[string]any{"name": newName}})
		err = saveErr
		if err == nil {
			newRev = saved.Data.(Revision)
			updated, _, err = s.store.Read(old.ID)
		}
	} else {
		err = NewError(ErrInvalidFrontmatter, "store does not support combined title and slug renames")
	}
	if err != nil {
		return Result{}, err
	}

	result := RenameResult{
		ID: updated.Frontmatter.ID, OldName: old.Name, Name: updated.Frontmatter.Name,
		OldSlug: old.Slug, Slug: updated.Frontmatter.Slug,
		Revision: newRev, Path: filepath.Join(s.cfg.DossierHome, updated.Frontmatter.Slug),
	}
	var warnings []Warning
	event := AuditEventSlugRenamed
	if req.NewName != "" {
		event = AuditEventRenamed
	}
	if err := s.store.AppendAudit(updated.Frontmatter.ID, AuditEvent{
		TS: s.clock.Now(), Event: event, Author: s.cfg.Author,
		DossierID: updated.Frontmatter.ID, BeforeRevision: string(base),
		AfterRevision: string(newRev), Message: describeFrontmatterChanges(old, updated.Frontmatter),
	}); err != nil {
		warnings = append(warnings, Warning(fmt.Sprintf("Dossier was renamed, but the audit event could not be written: %v", err)))
	}
	return Result{OK: true, Data: result, Warnings: warnings}, nil
}

// RenameSlug is the backwards-compatible slug-only spelling.
func (s *Service) RenameSlug(ctx context.Context, req RenameSlugReq) (Result, error) {
	return s.Rename(ctx, req)
}

type LinkReq struct {
	ID           string
	FromFilePath string
	Content      string
	Title        string
}

func (s *Service) Link(ctx context.Context, req LinkReq) (Result, error) {
	now := s.clock.Now()

	if req.ID == "" {
		var suggestions []Suggestion
		scorer := newDossierScorer(req.Content, now)
		scanWarnings, err := s.scanDossiers("all", func(d *Dossier) error {
			suggestions = keepTopSuggestions(suggestions, scorer.Score(d), 3)
			return nil
		})
		if err != nil {
			return Result{}, err
		}

		return Result{
			OK:       false,
			Data:     suggestions,
			Warnings: scanWarnings,
		}, NewError(ErrAmbiguousTarget, "Multiple likely Dossiers match this link request.")
	}

	d, baseRev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	newRev, err := s.store.Write(d, baseRev)
	if err != nil {
		return Result{}, err
	}

	title := req.Title
	if title == "" {
		title = "Linked Session Content"
	}

	art := Artifact{
		DossierID:     d.Frontmatter.ID,
		Type:          ArtifactTypeSourceSnapshot,
		Title:         title,
		Provenance:    Provenance{Origin: "linked session content"},
		ContentFormat: ContentFormatText,
		Content:       req.Content,
		CapturedAt:    now,
		RefreshedAt:   now,
	}

	if err := s.store.WriteArtifact(d.Frontmatter.ID, &art); err != nil {
		return Result{}, err
	}

	_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
		TS:             now,
		Event:          AuditEventSave,
		Author:         s.cfg.Author,
		DossierID:      d.Frontmatter.ID,
		BeforeRevision: string(baseRev),
		AfterRevision:  string(newRev),
		ArtifactsAdded: []string{art.ID},
	})

	return Result{
		OK:   true,
		Data: newRev,
	}, nil
}

type MergeReq struct {
	SourceID          string
	TargetID          string
	ResolvedConflicts []string
}

func (s *Service) Merge(ctx context.Context, req MergeReq) (Result, error) {
	sourceD, sourceRev, err := s.store.Read(req.SourceID)
	if err != nil {
		return Result{}, WrapError(ErrNotFound, "failed to read source dossier", err)
	}
	targetD, targetRev, err := s.store.Read(req.TargetID)
	if err != nil {
		return Result{}, WrapError(ErrNotFound, "failed to read target dossier", err)
	}

	// Conflict detection
	hasConflict := false
	var conflictReason []string

	if sourceD.Frontmatter.Status != targetD.Frontmatter.Status {
		hasConflict = true
		conflictReason = append(conflictReason, fmt.Sprintf("incompatible statuses: source is %q, target is %q", sourceD.Frontmatter.Status, targetD.Frontmatter.Status))
	}
	if sourceD.Frontmatter.NextAction != "" && targetD.Frontmatter.NextAction != "" && sourceD.Frontmatter.NextAction != targetD.Frontmatter.NextAction {
		hasConflict = true
		conflictReason = append(conflictReason, fmt.Sprintf("divergent next actions: source is %q, target is %q", sourceD.Frontmatter.NextAction, targetD.Frontmatter.NextAction))
	}

	isResolved := false
	if hasConflict {
		confID := "conf_merge_" + s.clock.Now().Format("20060102150405")
		for _, rc := range req.ResolvedConflicts {
			if rc == confID || rc == "all" {
				isResolved = true
				break
			}
		}

		if !isResolved {
			diff := GenerateUnifiedDiff(targetD.DistilledState.Body, sourceD.DistilledState.Body)
			conflict := &Conflict{
				ID:                 confID,
				DossierID:          targetD.Frontmatter.ID,
				Kind:               "merge_conflict",
				BaseRevision:       string(targetRev),
				AttemptedRevision:  string(sourceRev),
				TS:                 s.clock.Now(),
				RejectedBody:       sourceD.DistilledState.Body,
				DiffAgainstCurrent: diff,
			}

			_ = s.store.WriteConflict(conflict)

			_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
				TS:             s.clock.Now(),
				Event:          AuditEventMergeConflict,
				Author:         s.cfg.Author,
				DossierID:      targetD.Frontmatter.ID,
				BeforeRevision: string(targetRev),
				AfterRevision:  string(targetRev),
				Message:        fmt.Sprintf("Merge conflict %s with source %s: %s", confID, req.SourceID, strings.Join(conflictReason, "; ")),
			})

			return Result{
				OK:   false,
				Data: conflict,
			}, NewError(ErrConflictDetected, fmt.Sprintf("Merge conflict: %s. Conflict artifact %s created.", strings.Join(conflictReason, "; "), confID))
		}
	}

	_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
		TS:        s.clock.Now(),
		Event:     AuditEventMergeStarted,
		Author:    s.cfg.Author,
		DossierID: targetD.Frontmatter.ID,
		Message:   fmt.Sprintf("Starting merge of source %s into target %s", req.SourceID, req.TargetID),
	})

	if targetD.Frontmatter.NextAction == "" {
		targetD.Frontmatter.NextAction = sourceD.Frontmatter.NextAction
	}
	if sourceD.DistilledState.Body != targetD.DistilledState.Body {
		targetD.DistilledState.Body += "\n\n## Merged Distilled State (" + sourceD.Frontmatter.Name + ")\n" + sourceD.DistilledState.Body
	}

	srcArts, _ := s.store.ListArtifacts(sourceD.Frontmatter.ID)
	for _, art := range srcArts {
		fullArt, err := s.store.ReadArtifact(sourceD.Frontmatter.ID, art.ID)
		if err == nil {
			fullArt.DossierID = targetD.Frontmatter.ID
			_ = s.store.WriteArtifact(targetD.Frontmatter.ID, fullArt)
		}
	}

	newTargetRev, err := s.store.Write(targetD, targetRev)
	if err != nil {
		return Result{}, err
	}

	sourceD.Frontmatter.Status = StatusArchived
	sourceD.Frontmatter.NextAction = "Merged into " + targetD.Frontmatter.ID
	_, _ = s.store.Write(sourceD, sourceRev)

	_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventMergeCompleted,
		Author:         s.cfg.Author,
		DossierID:      targetD.Frontmatter.ID,
		BeforeRevision: string(targetRev),
		AfterRevision:  string(newTargetRev),
		Message:        fmt.Sprintf("Completed merge of source %s into target %s", req.SourceID, req.TargetID),
	})

	return Result{
		OK:   true,
		Data: newTargetRev,
	}, nil
}

type RecallReq struct {
	ID string
}

func (s *Service) Recall(ctx context.Context, req RecallReq) (Result, error) {
	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	tokens := s.tok.Estimate(d.DistilledState.Body)

	var warnings []Warning
	target := s.TokenLimit()
	if tokens > target {
		warnings = append(warnings, Warning(fmt.Sprintf("Distilled State exceeds token target (%d > %d tokens). Consider condensing.", tokens, target)))
	}

	index, indexWarnings := s.evidenceIndex(d.Frontmatter.ID, d.DistilledState.Body)
	warnings = append(warnings, indexWarnings...)
	externalLinks := ParseExternalLinks(d.DistilledState.Body)

	dossierPath := filepath.Join(s.cfg.DossierHome, d.Frontmatter.Slug)
	return Result{
		OK:       true,
		Data:     RecallResult{DistilledState: d.DistilledState.Body, Frontmatter: d.Frontmatter, Revision: rev, TokenEstimate: tokens, Path: dossierPath, Artifacts: index, References: externalLinks.References, ActiveMonitors: externalLinks.ActiveMonitors},
		Warnings: warnings,
	}, nil
}

// splitContentLines is the canonical split of artifact content into physical
// lines. Every line-addressing path goes through it so a citation, a search
// hit, and a fetch cannot disagree about which line is line N. A single
// trailing newline terminates the last line rather than starting a new empty
// one; any blank line before that is a real line and is preserved.
func splitContentLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// artifactLineCount reports the number of physical lines in artifact content.
// These are the coordinates a [src:art_x#L10-L20] citation addresses.
func artifactLineCount(content string) int {
	return len(splitContentLines(content))
}

// ArtifactLineCount is the exported form of artifactLineCount, for adapters
// (e.g. the filesystem store) that need to persist the same line count the
// core uses to bound citations.
func ArtifactLineCount(content string) int {
	return artifactLineCount(content)
}

// numberLines renders lines with absolute 1-indexed numbers, so the span a
// caller reads is the span they can cite back without counting by hand.
func numberLines(lines []string, startLine int) string {
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%d\t%s\n", startLine+i, line))
	}
	return sb.String()
}

// evidenceIndex summarizes a dossier's archived artifacts and flags the ones
// the Distilled State never cites.
func (s *Service) evidenceIndex(dossierID string, body string) ([]ArtifactSummary, []Warning) {
	artifacts, err := s.store.ListArtifacts(dossierID)
	if err != nil {
		return nil, []Warning{Warning(fmt.Sprintf("Artifacts could not be listed for the evidence index: %v", err))}
	}
	cited := citedArtifactIDs(body)

	var (
		index    []ArtifactSummary
		warnings []Warning
	)
	for _, art := range artifacts {
		// The store persists the line count in frontmatter so listing an
		// evidence index never has to load an artifact's body. When Content
		// is populated (e.g. a store that returns full bodies from list),
		// prefer counting it directly so it can't disagree with Lines.
		lines := art.Lines
		if art.Content != "" {
			lines = artifactLineCount(art.Content)
		}
		index = append(index, ArtifactSummary{
			ID:            art.ID,
			Type:          string(art.Type),
			Title:         art.Title,
			ContentFormat: string(art.ContentFormat),
			Lines:         lines,
			CapturedAt:    art.CapturedAt,
			Origin:        art.Provenance.Origin,
			URL:           art.Provenance.URL,
			Cited:         cited[art.ID],
		})
	}

	if msg := uncitedArtifactWarning(body, artifacts); msg != "" {
		warnings = append(warnings, Warning(msg))
	}
	return index, warnings
}

// ReadArtifactReq addresses an artifact, optionally narrowing to a line range.
type ReadArtifactReq struct {
	DossierID  string
	ArtifactID string
	// Fragment is the raw citation fragment (e.g. "L10-L20"), accepted so a
	// caller can paste a [src:] pointer straight through.
	Fragment  string
	StartLine int
	EndLine   int
}

// ArtifactContent is a fetched artifact span.
type ArtifactContent struct {
	ArtifactSummary
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Ranged    bool   `json:"ranged"`
	Content   string `json:"content"`
}

// largeArtifactLineWarning is the point past which an unranged fetch is worth
// a nudge toward citing a span instead.
const largeArtifactLineWarning = 500

// ReadArtifact resolves an artifact citation to its content.
//
// This is what makes elision safe. The Distillation Guide asks the author to
// compress aggressively and cite what was compressed away; that trade is only
// honest if the citation can be followed back to the verbatim record. Content
// is returned line-numbered so the span read is the span cited.
func (s *Service) ReadArtifact(ctx context.Context, req ReadArtifactReq) (Result, error) {
	if req.ArtifactID == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "artifact id is required")
	}

	d, _, err := s.store.Read(req.DossierID)
	if err != nil {
		return Result{}, err
	}
	dossierID := d.Frontmatter.ID

	art, err := s.store.ReadArtifact(dossierID, req.ArtifactID)
	if err != nil {
		var domainErr *DomainError
		if errors.As(err, &domainErr) {
			return Result{}, domainErr
		}
		return Result{}, WrapError(ErrInternal, fmt.Sprintf("failed to read artifact %s in dossier %s", req.ArtifactID, dossierID), err)
	}

	start, end := req.StartLine, req.EndLine
	if req.Fragment != "" {
		ref, parseErr := ParseProvenanceRef(req.ArtifactID, strings.TrimPrefix(req.Fragment, "#"))
		if parseErr != nil {
			return Result{}, NewError(ErrInvalidFrontmatter, parseErr.Error())
		}
		if ref.HasRange {
			start, end = ref.StartLine, ref.EndLine
		}
	}

	total := artifactLineCount(art.Content)
	summary := ArtifactSummary{
		ID:            art.ID,
		Type:          string(art.Type),
		Title:         art.Title,
		ContentFormat: string(art.ContentFormat),
		Lines:         total,
		CapturedAt:    art.CapturedAt,
		Origin:        art.Provenance.Origin,
		URL:           art.Provenance.URL,
		Cited:         citedArtifactIDs(d.DistilledState.Body)[art.ID],
	}

	if start > 0 && end > 0 && start > end {
		return Result{}, NewError(ErrInvalidFrontmatter, fmt.Sprintf(
			"requested range start_line=%d ends before it starts (end_line=%d)", start, end))
	}

	var warnings []Warning
	ranged := start > 0 || end > 0

	if !ranged {
		if total > largeArtifactLineWarning {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Artifact %s is %d lines and was returned in full. Cite and fetch a range (#L<start>-L<end>) to keep the working context small.",
				art.ID, total)))
		}
		return Result{
			OK: true,
			Data: ArtifactContent{
				ArtifactSummary: summary,
				StartLine:       1,
				EndLine:         total,
				Content:         numberLines(splitContentLines(art.Content), 1),
			},
			Warnings: warnings,
		}, nil
	}

	if start < 1 {
		start = 1
	}
	if end < 1 || end > total {
		if end > total {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Requested range ends at line %d but artifact %s has %d line(s); returning through the last line.", end, art.ID, total)))
		}
		end = total
	}
	if start > total {
		return Result{}, NewError(ErrNotFound, fmt.Sprintf(
			"artifact %s has %d line(s); requested range starts at line %d", art.ID, total, start))
	}

	span := splitContentLines(art.Content)[start-1 : end]

	return Result{
		OK: true,
		Data: ArtifactContent{
			ArtifactSummary: summary,
			StartLine:       start,
			EndLine:         end,
			Ranged:          true,
			Content:         numberLines(span, start),
		},
		Warnings: warnings,
	}, nil
}

// ListArtifactsReq addresses a dossier's evidence index.
type ListArtifactsReq struct {
	DossierID string
}

// ListArtifacts returns the evidence index for a dossier.
func (s *Service) ListArtifacts(ctx context.Context, req ListArtifactsReq) (Result, error) {
	d, _, err := s.store.Read(req.DossierID)
	if err != nil {
		return Result{}, err
	}
	index, warnings := s.evidenceIndex(d.Frontmatter.ID, d.DistilledState.Body)
	return Result{OK: true, Data: index, Warnings: warnings}, nil
}

type ListReq struct {
	Status     string
	Interfaces []string
	Query      string
}

func matchesInterfaces(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, requested := range want {
		for _, assigned := range have {
			if requested == assigned {
				return true
			}
		}
	}
	return false
}

func priorityBefore(a, b Priority) bool {
	if a == b {
		return false
	}
	switch a {
	case PriorityMax:
		return true
	case PriorityHigh:
		return b != PriorityMax
	case PriorityMedium:
		return b == PriorityLow
	default:
		return false
	}
}

// frontmatterLess is the shared dashboard/library ordering: priority, then due
// date (undated last), then most-recently-updated last within a tie.
func frontmatterLess(a, b Frontmatter) bool {
	if a.Priority != b.Priority {
		return priorityBefore(a.Priority, b.Priority)
	}
	if a.DueDate != b.DueDate {
		if a.DueDate == "" {
			return false
		}
		if b.DueDate == "" {
			return true
		}
		return a.DueDate < b.DueDate
	}
	return a.UpdatedAt.Before(b.UpdatedAt)
}

func sortFrontmatters(items []Frontmatter) {
	sort.SliceStable(items, func(i, j int) bool { return frontmatterLess(items[i], items[j]) })
}

func sortListedFrontmatters(items []ListedFrontmatter) {
	sort.SliceStable(items, func(i, j int) bool {
		return frontmatterLess(items[i].Frontmatter, items[j].Frontmatter)
	})
}

func (s *Service) List(ctx context.Context, req ListReq) (Result, error) {
	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to list dossiers", err)
	}

	var filtered []ListedFrontmatter
	roster, hasRoster := s.currentRoster()
	query := NewQuery(req.Query)
	for _, fm := range fms {
		if !matchesInterfaces(fm.Interfaces, req.Interfaces) {
			continue
		}
		leadDisplay := fm.Lead
		if hasRoster {
			leadDisplay = roster.DisplayName(fm.Lead)
		}
		if !query.IsEmpty() && !query.Matches(Haystack(ListItem{
			Name:        fm.Name,
			Slug:        fm.Slug,
			Description: fm.Description,
			Lead:        leadDisplay + " " + fm.Lead,
			Interfaces:  fm.Interfaces,
		})) {
			continue
		}
		if req.Status == "" {
			if fm.Status.IsOpen() {
				filtered = append(filtered, fm)
			}
		} else if req.Status == "all" || string(fm.Status) == req.Status || fm.Status == NormalizeStatus(Status(req.Status)) {
			filtered = append(filtered, fm)
		}
	}

	sortListedFrontmatters(filtered)

	var items []ListItem
	for _, listed := range filtered {
		fm := listed.Frontmatter
		dossierPath := filepath.Join(s.cfg.DossierHome, fm.Slug)
		leadDisplay := fm.Lead
		if hasRoster {
			leadDisplay = roster.DisplayName(fm.Lead)
		}
		items = append(items, ListItem{
			ID:                        fm.ID,
			Name:                      fm.Name,
			Slug:                      fm.Slug,
			Status:                    string(fm.Status),
			Description:               fm.Description,
			Lead:                      leadDisplay,
			Interfaces:                append([]string(nil), fm.Interfaces...),
			NextAction:                fm.NextAction,
			Priority:                  string(fm.Priority),
			DueDate:                   fm.DueDate,
			Path:                      dossierPath,
			Revision:                  listed.Revision,
			HasOpenDelegationContract: listed.HasOpenDelegationContract,
		})
	}

	return Result{
		OK:   true,
		Data: items,
	}, nil
}

type SearchReq struct {
	Query string
	Scope SearchScope
}

func (s *Service) Search(ctx context.Context, req SearchReq) (Result, error) {
	if req.Scope.DossierID != "" {
		d, _, err := s.store.Read(req.Scope.DossierID)
		if err != nil {
			return Result{}, err
		}
		req.Scope.DossierID = d.Frontmatter.ID
	}

	hits, err := s.search.Search(ctx, req.Query, req.Scope)
	if err != nil {
		return Result{}, WrapError(ErrInternal, "search failed", err)
	}

	return Result{
		OK:   true,
		Data: hits,
	}, nil
}
