package core

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type teamJoinRequestTestStore struct {
	*rosterTestStore
	requests  []TeamJoinRequest
	appendErr error
}

func (s *teamJoinRequestTestStore) AppendTeamJoinRequest(request TeamJoinRequest) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	request.Username = NormalizeUsername(request.Username)
	if request.ID == "" {
		request.ID = fmt.Sprintf("test-request-%d", len(s.requests)+1)
	}
	s.requests = append(s.requests, request)
	return nil
}

func (s *teamJoinRequestTestStore) AppendTeamJoinResolution(request TeamJoinRequest) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	request.Username = NormalizeUsername(request.Username)
	s.requests = append(s.requests, request)
	return nil
}

func (s *teamJoinRequestTestStore) TeamJoinRequests() ([]TeamJoinRequest, error) {
	return append([]TeamJoinRequest(nil), s.requests...), nil
}

func (s *teamJoinRequestTestStore) TeamJoinRequestSyncPath(username string) string {
	shard := base64.RawURLEncoding.EncodeToString([]byte(NormalizeUsername(username)))
	return "team-requests/" + shard + ".log"
}

type teamJoinRequestSyncer struct {
	teamTestSyncer
	report SyncReport
	err    error
}

func (s *teamJoinRequestSyncer) Sync(context.Context) (SyncReport, error) {
	return s.report, s.err
}

func newTeamJoinRequestTestService(t *testing.T, author string, store *teamJoinRequestTestStore, syncer *teamJoinRequestSyncer, now time.Time) *Service {
	t.Helper()
	return NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{
		Author: author, DossierHome: t.TempDir(),
	}, syncer)
}

func TestTeamJoinSavesAndSyncsUnlistedLoginRequest(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{rosterTestStore: &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
	}}
	syncer := &teamJoinRequestSyncer{report: SyncReport{Pushed: true}}
	svc := newTeamJoinRequestTestService(t, "gclawson", store, syncer, now)

	result, err := svc.TeamJoin(context.Background(), TeamJoinReq{Actor: "human:gclawson", RemoteURL: "team-remote"})
	if err != nil {
		t.Fatal(err)
	}
	status, ok := result.Data.(TeamJoinStatus)
	if !ok || !status.RosterChecked || !status.RosterExists || status.OnRoster || !status.JoinRequestSaved || !status.JoinRequestSynced {
		t.Fatalf("join status = %#v", result.Data)
	}
	if len(store.requests) != 1 || store.requests[0].Username != "gclawson" || store.requests[0].Actor != "human:gclawson" {
		t.Fatalf("stored requests = %+v", store.requests)
	}
	requests, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending := requests.Data.([]PendingTeamJoinRequest)
	if len(pending) != 1 || pending[0].Username != "gclawson" {
		t.Fatalf("pending requests = %+v", pending)
	}
}

func TestTeamJoinReportsRequestSaveAndSyncFailuresHonestly(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	newStore := func() *teamJoinRequestTestStore {
		return &teamJoinRequestTestStore{rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
		}}
	}
	t.Run("request write fails", func(t *testing.T) {
		store := newStore()
		store.appendErr = errors.New("disk full")
		svc := newTeamJoinRequestTestService(t, "bob", store, &teamJoinRequestSyncer{report: SyncReport{Pushed: true}}, now)
		result, err := svc.TeamJoin(context.Background(), TeamJoinReq{Actor: "human:bob", RemoteURL: "team-remote"})
		if err != nil {
			t.Fatal(err)
		}
		status := result.Data.(TeamJoinStatus)
		if status.JoinRequestSaved || status.JoinRequestSynced {
			t.Fatalf("failed request reported as saved/synced: %+v", status)
		}
		if !teamRequestWarningsContain(result.Warnings, "pending join request could not be saved") {
			t.Fatalf("missing save failure warning: %v", result.Warnings)
		}
	})

	t.Run("request sync fails", func(t *testing.T) {
		store := newStore()
		svc := newTeamJoinRequestTestService(t, "bob", store, &teamJoinRequestSyncer{report: SyncReport{Error: "offline"}}, now)
		result, err := svc.TeamJoin(context.Background(), TeamJoinReq{Actor: "human:bob", RemoteURL: "team-remote"})
		if err != nil {
			t.Fatal(err)
		}
		status := result.Data.(TeamJoinStatus)
		if !status.JoinRequestSaved || status.JoinRequestSynced {
			t.Fatalf("sync failure status = %+v", status)
		}
		if !teamRequestWarningsContain(result.Warnings, "saved locally but could not be synced") {
			t.Fatalf("missing sync failure warning: %v", result.Warnings)
		}
	})

	t.Run("request exclusion is not reported as synced when another file was pushed", func(t *testing.T) {
		store := newStore()
		syncer := &teamJoinRequestSyncer{report: SyncReport{
			Pushed:   true,
			Excluded: []SyncExcluded{{Path: store.TeamJoinRequestSyncPath("bob"), Warning: "request shard exceeded size limit"}},
		}}
		svc := newTeamJoinRequestTestService(t, "bob", store, syncer, now)
		result, err := svc.TeamJoin(context.Background(), TeamJoinReq{Actor: "human:bob", RemoteURL: "team-remote"})
		if err != nil {
			t.Fatal(err)
		}
		status := result.Data.(TeamJoinStatus)
		if !status.JoinRequestSaved || status.JoinRequestSynced {
			t.Fatalf("excluded request sync status = %+v", status)
		}
		if !teamRequestWarningsContain(result.Warnings, "sync excluded it") {
			t.Fatalf("missing request exclusion warning: %v", result.Warnings)
		}
	})
}

func teamRequestWarningsContain(warnings []Warning, text string) bool {
	for _, warning := range warnings {
		if strings.Contains(string(warning), text) {
			return true
		}
	}
	return false
}

func TestTeamRequestsAndApprovalAreManagerOnlyAndRequestBound(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
		},
		requests: []TeamJoinRequest{{ID: "bob-request-1", Username: "bob", Actor: "human:bob", RequestedAt: now}},
	}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	listed, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending := listed.Data.([]PendingTeamJoinRequest)
	if len(pending) != 1 || pending[0].Username != "bob" {
		t.Fatalf("pending requests = %+v", pending)
	}

	if _, err := svc.TeamApproveJoin(context.Background(), "human:someone", "bob", "Bob Builder"); err == nil || !strings.Contains(err.Error(), "only the team manager") {
		t.Fatalf("non-manager approval error = %v", err)
	}
	if _, err := svc.TeamApproveJoin(context.Background(), "human:alice", "mallory", "Mallory"); err == nil || !strings.Contains(err.Error(), "no pending join request") {
		t.Fatalf("unrequested login approval error = %v", err)
	}
	if _, err := svc.TeamApproveJoin(context.Background(), "human:alice", "Bob Builder", "Bob Builder"); err == nil || !strings.Contains(err.Error(), "no pending join request") {
		t.Fatalf("display name was accepted as requested login: %v", err)
	}

	if _, err := svc.TeamApproveJoin(context.Background(), "human:alice", "bob", "Bob Builder"); err != nil {
		t.Fatalf("approve exact request: %v", err)
	}
	if store.roster.Members["bob"] != "Bob Builder" || store.roster.Kind("bob") != "human" {
		t.Fatalf("approved roster = %+v", store.roster)
	}
	if len(store.requests) != 2 || store.requests[1].Resolution != TeamJoinRequestResolutionApproved {
		t.Fatalf("approval was not recorded append-only: %+v", store.requests)
	}
	listed, err = svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.Data.([]PendingTeamJoinRequest); len(got) != 0 {
		t.Fatalf("approved request still pending: %+v", got)
	}
	if _, err := svc.TeamRemoveAs(context.Background(), "human:alice", "bob"); err != nil {
		t.Fatalf("remove previously approved member: %v", err)
	}
	listed, err = svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.Data.([]PendingTeamJoinRequest); len(got) != 0 {
		t.Fatalf("historical request reappeared after removal: %+v", got)
	}
}

func TestTeamRequestsHideLoginAlreadyOnAgentRoster(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster: Roster{
				Manager: "alice",
				Members: map[string]string{"alice": "Alice", "sitroom": "Sit Room"},
				Kinds:   map[string]string{"sitroom": "agent"},
			},
		},
		requests: []TeamJoinRequest{{ID: "sitroom-request-1", Username: "sitroom", Actor: "agent:sitroom", RequestedAt: now}},
	}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	result, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Data.([]PendingTeamJoinRequest); len(got) != 0 {
		t.Fatalf("request for existing agent roster member is still pending: %+v", got)
	}
}

func TestNewTeamJoinRequestRemainsPendingAfterPriorResolution(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	newRequestAt := now.Add(-time.Minute)
	store := &teamJoinRequestTestStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
		},
		requests: []TeamJoinRequest{
			{ID: "bob-request-1", Username: "bob", Actor: "human:bob", RequestedAt: now},
			{ID: "bob-request-1", Username: "bob", Actor: "human:alice", RequestedAt: now, Resolution: TeamJoinRequestResolutionApproved, ResolvedAt: now.Add(30 * time.Second)},
			{ID: "bob-request-2", Username: "bob", Actor: "human:bob", RequestedAt: newRequestAt},
		},
	}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	result, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending := result.Data.([]PendingTeamJoinRequest)
	if len(pending) != 1 || pending[0].Username != "bob" || !pending[0].RequestedAt.Equal(newRequestAt) {
		t.Fatalf("new request after earlier approval = %+v", pending)
	}
}

func TestTeamRequestsUseIDsForEqualRequestTimestamps(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
		},
		requests: []TeamJoinRequest{
			{ID: "bob-request-1", Username: "bob", Actor: "human:bob", RequestedAt: now},
			{ID: "bob-request-1", Username: "bob", Actor: "human:alice", RequestedAt: now, Resolution: TeamJoinRequestResolutionApproved, ResolvedAt: now.Add(time.Second)},
			{ID: "bob-request-2", Username: "bob", Actor: "human:bob", RequestedAt: now},
		},
	}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	result, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending := result.Data.([]PendingTeamJoinRequest)
	if len(pending) != 1 || pending[0].RequestID != "bob-request-2" {
		t.Fatalf("same-timestamp new request = %+v", pending)
	}
}

func TestTeamAddResolutionCoversDelayedAndFutureRequests(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{rosterTestStore: &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
	}}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	if _, err := svc.TeamAddKindAs(context.Background(), "human:alice", "bob", "Bob Builder", "human"); err != nil {
		t.Fatal(err)
	}
	if len(store.requests) != 1 || store.requests[0].Resolution != TeamJoinRequestResolutionRosterAdded {
		t.Fatalf("roster-add resolution = %+v", store.requests)
	}
	if err := store.AppendTeamJoinResolution(TeamJoinRequest{
		Username: "bob", Actor: "human:alice", Resolution: TeamJoinRequestResolutionRosterAdded,
		ResolvedAt: now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	store.requests = append(store.requests, TeamJoinRequest{
		ID: "delayed-request", Username: "bob", Actor: "human:bob", RequestedAt: now.Add(-15 * time.Second),
	})
	if _, err := svc.TeamRemoveAs(context.Background(), "human:alice", "bob"); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.Data.([]PendingTeamJoinRequest); len(got) != 0 {
		t.Fatalf("delayed request resurfaced after direct add and removal: %+v", got)
	}

	if err := store.AppendTeamJoinRequest(TeamJoinRequest{
		Username: "bob", Actor: "human:bob", RequestedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.Data.([]PendingTeamJoinRequest); len(got) != 1 || !got[0].RequestedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("new request after removal = %+v", got)
	}
}

func TestTeamApprovalReportsResolutionPersistenceFailure(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &teamJoinRequestTestStore{
		rosterTestStore: &rosterTestStore{
			localFakeStore: newLocalFakeStore(),
			roster:         Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}},
		},
		requests:  []TeamJoinRequest{{ID: "bob-request-1", Username: "bob", Actor: "human:bob", RequestedAt: now}},
		appendErr: errors.New("read-only request log"),
	}
	svc := newTeamJoinRequestTestService(t, "alice", store, &teamJoinRequestSyncer{}, now)

	result, err := svc.TeamApproveJoin(context.Background(), "human:alice", "bob", "Bob Builder")
	if err != nil {
		t.Fatal(err)
	}
	if store.roster.Members["bob"] != "Bob Builder" || len(store.requests) != 1 {
		t.Fatalf("roster/request state after resolution failure: roster=%+v requests=%+v", store.roster, store.requests)
	}
	if !teamRequestWarningsContain(result.Warnings, "resolution could not be recorded; the request may reappear") {
		t.Fatalf("missing actionable resolution warning: %v", result.Warnings)
	}

	if _, err := svc.TeamRemoveAs(context.Background(), "human:alice", "bob"); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.TeamRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.Data.([]PendingTeamJoinRequest); len(got) != 1 || got[0].Username != "bob" {
		t.Fatalf("request should reappear after removal when resolution persistence failed: %+v", got)
	}
}
