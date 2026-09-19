package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// TeamCreateReq specifies parameters for creating a new team store.
type TeamCreateReq struct {
	RemoteURL          string
	Branch             string
	Confirmed          bool
	ManagerDisplayName string
}

// Members returns the current synced team roster. Stores without roster
// support behave as an empty local roster for backwards-compatible tests.
func (s *Service) Members(ctx context.Context) (Roster, error) {
	store, ok := s.store.(RosterStore)
	if !ok {
		return Roster{Members: map[string]string{}, Former: map[string]string{}}, nil
	}
	roster, err := store.ReadRoster()
	if err != nil {
		return Roster{}, err
	}
	if roster == nil {
		roster = &Roster{}
	}
	roster.normalize()
	return *roster, nil
}

func (s *Service) normalizeLeadUpdate(updates map[string]any) (map[string]any, error) {
	if updates == nil {
		return nil, nil
	}
	value, ok := updates["lead"]
	if !ok {
		return updates, nil
	}
	lead, ok := value.(string)
	if !ok {
		return updates, nil
	}
	roster, hasRoster := s.currentRoster()
	if !hasRoster || strings.TrimSpace(lead) == "" {
		return updates, nil
	}
	username, resolved, candidates := roster.ResolvePerson(lead)
	if !resolved {
		if len(candidates) > 1 {
			return updates, NewError(ErrAmbiguousTarget, fmt.Sprintf("lead %q is ambiguous; candidates: %s", lead, strings.Join(candidates, ", ")))
		}
		return updates, NewError(ErrInvalidFrontmatter, fmt.Sprintf("unknown team member %q", lead))
	}
	copy := make(map[string]any, len(updates))
	for key, item := range updates {
		copy[key] = item
	}
	copy["lead"] = username
	return copy, nil
}

func (s *Service) rosterWarning(roster Roster) []Warning {
	if roster.Manager == "" || NormalizeUsername(s.cfg.Author) == NormalizeUsername(roster.Manager) {
		return nil
	}
	return []Warning{Warning(fmt.Sprintf("You are not the roster's manager (%s); roster changes are conventionally manager-owned.", roster.Manager))}
}

// TeamAdd adds or restores a roster member. Usernames are stored normalized.
func (s *Service) TeamAdd(ctx context.Context, username, displayName string) (Result, error) {
	store, ok := s.store.(RosterStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "team roster storage is not configured")
	}
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "username and display name are required")
	}
	roster, err := s.Members(ctx)
	if err != nil {
		return Result{OK: false}, err
	}
	if roster.Members == nil {
		roster.Members = map[string]string{}
	}
	if roster.Former == nil {
		roster.Former = map[string]string{}
	}
	roster.Members[username] = displayName
	delete(roster.Former, username)
	if err := store.WriteRoster(&roster); err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: roster, Warnings: s.rosterWarning(roster)}, nil
}

// TeamRemove moves a member to former rather than deleting their identity.
func (s *Service) TeamRemove(ctx context.Context, username string) (Result, error) {
	store, ok := s.store.(RosterStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "team roster storage is not configured")
	}
	username = NormalizeUsername(username)
	if username == "" {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "username is required")
	}
	roster, err := s.Members(ctx)
	if err != nil {
		return Result{OK: false}, err
	}
	displayName, exists := roster.Members[username]
	if !exists {
		return Result{OK: false}, NewError(ErrNotFound, fmt.Sprintf("team member %q not found", username))
	}
	delete(roster.Members, username)
	if roster.Former == nil {
		roster.Former = map[string]string{}
	}
	roster.Former[username] = displayName
	if err := store.WriteRoster(&roster); err != nil {
		return Result{OK: false}, err
	}
	return Result{OK: true, Data: roster, Warnings: s.rosterWarning(roster)}, nil
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
	if err := s.syncer.CheckRemoteEmpty(ctx, req.RemoteURL); err != nil {
		return Result{}, err
	}
	if !req.Confirmed {
		dossiers, err := s.store.List("all")
		if err != nil {
			return Result{}, fmt.Errorf("list dossiers for team create: %w", err)
		}
		return Result{
			OK:   true,
			Data: dossiers,
			Warnings: []Warning{
				"everything in the store directory syncs, including archived Dossiers",
			},
		}, nil
	}

	if rosterStore, ok := s.store.(RosterStore); ok {
		roster := &Roster{Manager: NormalizeUsername(s.cfg.Author), Members: map[string]string{}}
		managerName := strings.TrimSpace(req.ManagerDisplayName)
		if managerName == "" {
			managerName = strings.TrimSpace(s.cfg.DisplayName)
		}
		if managerName == "" {
			managerName = roster.Manager
		}
		roster.Members[roster.Manager] = managerName
		if err := rosterStore.WriteRoster(roster); err != nil {
			return Result{}, fmt.Errorf("write team roster: %w", err)
		}
	}

	err := s.syncer.Create(ctx, req.RemoteURL, req.Branch)
	if err != nil {
		if strings.Contains(err.Error(), "already a team store") {
			return Result{}, NewError(ErrConflictDetected, "store is already a team store")
		}
		if strings.Contains(err.Error(), "authentication required") || strings.Contains(err.Error(), "authorization failed") || strings.Contains(err.Error(), "insecure permissions") {
			return Result{}, NewError(ErrSyncAuthFailed, authFailedMessage(req.RemoteURL))
		}
		return Result{}, err // adapters prefix "Team create failed"
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
		if strings.Contains(err.Error(), "authentication required") || strings.Contains(err.Error(), "authorization failed") || strings.Contains(err.Error(), "insecure permissions") {
			return Result{}, NewError(ErrSyncAuthFailed, authFailedMessage(req.RemoteURL))
		}
		return Result{}, err // adapters prefix "Team join failed"
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
		slug := slugParts[0] // dossier slug for dossier.md; team.yaml is root-level
		var targetID, targetName, conflictKind string
		if conf.Path == "team.yaml" {
			targetID = RosterConflictDossierID
			targetName = "Team roster"
			conflictKind = "sync_concurrent_roster_edit"
		} else {
			fms, listErr := s.store.List("all")
			if listErr == nil {
				for _, fm := range fms {
					if fm.Slug == slug {
						targetID = fm.ID
						targetName = fm.Name
						break
					}
				}
			}
			if targetID == "" {
				targetID = slug
			}
			if targetName == "" {
				targetName = slug
			}
			conflictKind = "sync_concurrent_edit"
		}

		confID := fmt.Sprintf("conf_%s_%s_%d", s.clock.Now().Format("20060102150405"), strings.ReplaceAll(slug, "/", "-"), i)
		localBody := string(conf.LocalContent)
		remoteBody := string(conf.RemoteContent)
		if targetID != RosterConflictDossierID {
			localBody = conflictBody(localBody)
			remoteBody = conflictBody(remoteBody)
		}
		conflict := &Conflict{
			ID:                 confID,
			DossierID:          targetID,
			Kind:               conflictKind,
			BaseRevision:       conf.LocalRevision,
			AttemptedRevision:  conf.RemoteRevision,
			TS:                 s.clock.Now(),
			RejectedBody:       localBody,
			DiffAgainstCurrent: GenerateUnifiedDiff(remoteBody, localBody),
		}

		writeErr := s.store.WriteConflict(conflict)
		if writeErr == nil {
			createdConflicts = append(createdConflicts, confID)
			if targetID != RosterConflictDossierID {
				_ = s.store.AppendAudit(targetID, AuditEvent{
					TS:             s.clock.Now(),
					Event:          AuditEventConflictCreated,
					Author:         s.cfg.Author,
					DossierID:      targetID,
					BeforeRevision: conf.LocalRevision,
					AfterRevision:  conf.RemoteRevision,
					Message:        fmt.Sprintf("Conflict %s created due to sync concurrent edit on %s", confID, conf.Path),
				})
			}
			warnings = append(warnings, Warning(fmt.Sprintf("sync conflict: %s on %s; both versions are kept; resolve it with dossier_conflicts", confID, targetName)))
		} else {
			warnings = append(warnings, Warning(fmt.Sprintf("failed to write conflict for %s: %v", conf.Path, writeErr)))
		}
	}

	if report.AuthFailed {
		return Result{OK: false, Data: report, Warnings: warnings}, NewError(ErrSyncAuthFailed, authFailedMessage(s.cfg.TeamRemote))
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

	conflicts, err := s.store.ListConflicts()
	if err == nil {
		status.UnresolvedConflicts = len(conflicts)
	}

	return Result{
		OK:   true,
		Data: status,
	}, nil
}

// conflictBody removes the dossier file envelope before storing a rejected
// proposal. Roster conflicts bypass this helper and retain the complete
// team.yaml so restore/merge can parse the preserved roster.
func conflictBody(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return content
	}
	return content[end+9:]
}

// authFailedMessage is the next step shown for sync_auth_failed on every surface.
func authFailedMessage(remote string) string {
	return fmt.Sprintf("GitHub rejected the credentials, or none were found. Create a fine-grained token with Contents read/write on %s, write it to ~/.dossier/credentials (chmod 600), or run `gh auth login`, then run the command again.", remote)
}
