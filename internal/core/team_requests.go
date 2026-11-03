package core

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// TeamJoinRequest is an append-only request from a login not currently listed
// as a team member.
type TeamJoinRequest struct {
	ID          string    `json:"id,omitempty"`
	Username    string    `json:"username"`
	Actor       string    `json:"actor,omitempty"`
	RequestedAt time.Time `json:"requested_at,omitempty"`
	Resolution  string    `json:"resolution,omitempty"`
	ResolvedAt  time.Time `json:"resolved_at,omitempty"`
}

const (
	// TeamJoinRequestResolutionApproved resolves one request by its immutable ID.
	TeamJoinRequestResolutionApproved = "approved"
	// TeamJoinRequestResolutionRosterAdded resolves requests created before a roster addition.
	TeamJoinRequestResolutionRosterAdded = "roster_added"
)

// PendingTeamJoinRequest is the manager-facing view of a request that has not
// yet been satisfied by the current human roster.
type PendingTeamJoinRequest struct {
	Username    string    `json:"username"`
	RequestedAt time.Time `json:"requested_at"`
	RequestID   string    `json:"-"`
}

// TeamJoinStatus reports the roster check and request-delivery state to the
// joining user without implying the manager was notified when sync failed.
type TeamJoinStatus struct {
	Username          string `json:"username"`
	DisplayName       string `json:"display_name,omitempty"`
	ManagerName       string `json:"manager_name,omitempty"`
	RosterChecked     bool   `json:"roster_checked"`
	RosterExists      bool   `json:"roster_exists"`
	OnRoster          bool   `json:"on_roster"`
	JoinRequestSaved  bool   `json:"join_request_saved"`
	JoinRequestSynced bool   `json:"join_request_synced"`
}

// TeamRequests returns the latest unresolved request for each login not listed
// on the current roster. Requests remain append-only; approval is reflected by
// the manager-owned roster and resolution log rather than rewriting requests.
func (s *Service) TeamRequests(ctx context.Context) (Result, error) {
	requestStore, ok := s.store.(TeamJoinRequestStore)
	if !ok {
		return Result{OK: false}, NewError(ErrInternal, "team join request storage is not configured")
	}
	requests, err := requestStore.TeamJoinRequests()
	if err != nil {
		return Result{OK: false}, err
	}
	roster, err := s.Members(ctx)
	if err != nil {
		return Result{OK: false}, err
	}

	latestByUsername := make(map[string]TeamJoinRequest, len(requests))
	resolvedRequests := make(map[string]map[string]bool)
	resolvedThrough := make(map[string]time.Time)
	for _, request := range requests {
		request.Username = NormalizeUsername(request.Username)
		invalid := request.Username == ""
		switch request.Resolution {
		case "":
			invalid = invalid || request.ID == "" || request.RequestedAt.IsZero() || !request.ResolvedAt.IsZero()
		case TeamJoinRequestResolutionApproved:
			invalid = invalid || request.ID == "" || request.RequestedAt.IsZero() || request.ResolvedAt.IsZero()
		case TeamJoinRequestResolutionRosterAdded:
			invalid = invalid || request.ID != "" || !request.RequestedAt.IsZero() || request.ResolvedAt.IsZero()
		default:
			invalid = true
		}
		if invalid {
			return Result{OK: false}, fmt.Errorf("invalid team join request for %q", request.Username)
		}
		switch request.Resolution {
		case TeamJoinRequestResolutionApproved:
			if resolvedRequests[request.Username] == nil {
				resolvedRequests[request.Username] = make(map[string]bool)
			}
			resolvedRequests[request.Username][teamJoinRequestKey(request)] = true
		case TeamJoinRequestResolutionRosterAdded:
			if request.ResolvedAt.After(resolvedThrough[request.Username]) {
				resolvedThrough[request.Username] = request.ResolvedAt
			}
		default:
			// Shard order, not wall-clock time, selects the latest request
			// Concurrent suffix order is deterministic but does not imply real time.
			latestByUsername[request.Username] = request
		}
	}

	pending := make([]PendingTeamJoinRequest, 0, len(latestByUsername))
	for username, request := range latestByUsername {
		if resolvedRequests[username][teamJoinRequestKey(request)] || roster.Has(username) ||
			(!resolvedThrough[username].IsZero() && !request.RequestedAt.After(resolvedThrough[username])) {
			continue
		}
		pending = append(pending, PendingTeamJoinRequest{
			Username: username, RequestedAt: request.RequestedAt, RequestID: request.ID,
		})
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].RequestedAt.Equal(pending[j].RequestedAt) {
			return pending[i].Username < pending[j].Username
		}
		return pending[i].RequestedAt.Before(pending[j].RequestedAt)
	})
	return Result{OK: true, Data: pending}, nil
}

func teamJoinRequestKey(request TeamJoinRequest) string {
	return request.Username + "\x00" + request.ID
}

// TeamApproveJoin adds only a login with a pending request, keeping roster
// writes manager-only and bound to the request's normalized username.
func (s *Service) TeamApproveJoin(ctx context.Context, actor, username, displayName string) (Result, error) {
	actor = NormalizeActor(actor, s.cfg.Author)
	if err := ValidateActor(actor); err != nil {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, err.Error())
	}
	if err := Authorize(actor, "team_admin"); err != nil {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, err.Error())
	}
	roster, err := s.Members(ctx)
	if err != nil {
		return Result{OK: false}, err
	}
	if err := requireRosterManager(actor, roster); err != nil {
		return Result{OK: false}, err
	}

	username = NormalizeUsername(username)
	if username == "" {
		return Result{OK: false}, NewError(ErrInvalidFrontmatter, "username is required")
	}
	requestsResult, err := s.TeamRequests(ctx)
	if err != nil {
		return Result{OK: false}, err
	}
	pending := requestsResult.Data.([]PendingTeamJoinRequest)
	for _, request := range pending {
		if request.Username != username {
			continue
		}
		result, err := s.teamAddKindAs(ctx, actor, request.Username, displayName, "human", false)
		if err != nil {
			return result, err
		}
		requestStore, ok := s.store.(TeamJoinRequestStore)
		if !ok {
			result.Warnings = append(result.Warnings, Warning("The roster was updated, but the join request resolution could not be recorded."))
			return result, nil
		}
		if err := requestStore.AppendTeamJoinResolution(TeamJoinRequest{
			ID: request.RequestID, Username: request.Username, Actor: actor, RequestedAt: request.RequestedAt,
			Resolution: TeamJoinRequestResolutionApproved, ResolvedAt: s.clock.Now(),
		}); err != nil {
			result.Warnings = append(result.Warnings, Warning("The roster was updated, but the join request resolution could not be recorded; the request may reappear if this member is later removed: "+err.Error()))
		}
		return result, nil
	}
	return Result{OK: false}, NewError(ErrNotFound, fmt.Sprintf("no pending join request for %q", username))
}
