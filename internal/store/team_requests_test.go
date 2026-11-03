package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dossier/internal/core"
)

func TestTeamJoinRequestShardsRoundTripNormalizedLogins(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	requests := []core.TeamJoinRequest{
		{Username: `ACME\GClawson`, Actor: "human:gclawson", RequestedAt: now},
		{Username: "a.b", Actor: "human:a.b", RequestedAt: now.Add(time.Second)},
		{Username: "a-b", Actor: "human:a-b", RequestedAt: now.Add(2 * time.Second)},
	}
	for _, request := range requests {
		if err := fs.AppendTeamJoinRequest(request); err != nil {
			t.Fatalf("append request for %q: %v", request.Username, err)
		}
	}
	persisted, err := fs.TeamJoinRequests()
	if err != nil {
		t.Fatal(err)
	}
	requestID := ""
	for _, request := range persisted {
		if request.Username == "gclawson" {
			requestID = request.ID
		}
	}
	if requestID == "" {
		t.Fatal("append did not assign a request ID")
	}
	if len(requestID) != 32 {
		t.Fatalf("request ID = %q, want 128-bit hex ID", requestID)
	}
	if err := fs.AppendTeamJoinResolution(core.TeamJoinRequest{
		ID: requestID, Username: "gclawson", Actor: "human:alice", RequestedAt: now,
		Resolution: core.TeamJoinRequestResolutionApproved, ResolvedAt: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("append approval: %v", err)
	}
	if err := fs.AppendTeamJoinResolution(core.TeamJoinRequest{
		Username: "gclawson", Actor: "human:alice", Resolution: core.TeamJoinRequestResolutionRosterAdded,
		ResolvedAt: now.Add(4 * time.Second),
	}); err != nil {
		t.Fatalf("append roster-add watermark: %v", err)
	}

	got, err := fs.TeamJoinRequests()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(requests)+2 {
		t.Fatalf("requests = %+v, want %d", got, len(requests)+2)
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	resolved, rosterAdded := false, false
	for _, request := range got {
		seen[request.Username] = true
		if request.Resolution == "" {
			if len(request.ID) != 32 {
				t.Errorf("request ID = %q, want 128-bit hex ID", request.ID)
			}
			if ids[request.ID] {
				t.Errorf("duplicate request ID %q", request.ID)
			}
			ids[request.ID] = true
		}
		if request.Resolution == core.TeamJoinRequestResolutionApproved && request.ResolvedAt.Equal(now.Add(3*time.Second)) {
			resolved = true
		}
		if request.Resolution == core.TeamJoinRequestResolutionRosterAdded && request.ResolvedAt.Equal(now.Add(4*time.Second)) {
			rosterAdded = true
		}
	}
	if !resolved || !rosterAdded {
		t.Fatalf("approval or roster-add resolution missing from %+v", got)
	}
	for _, username := range []string{"gclawson", "a.b", "a-b"} {
		if !seen[username] {
			t.Errorf("request for %q missing from %+v", username, got)
		}
	}

	shards, err := os.ReadDir(filepath.Join(home, "team-requests"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != len(requests) {
		t.Fatalf("request shards = %d, want distinct shard per normalized login", len(shards))
	}
}

func TestTeamJoinRequestRejectsInvalidRecords(t *testing.T) {
	fs := NewFSStore(t.TempDir())
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

	if err := fs.AppendTeamJoinRequest(core.TeamJoinRequest{
		ID: "short", Username: "bob", RequestedAt: now,
	}); err == nil {
		t.Fatal("request with malformed ID should fail")
	}
	if err := fs.AppendTeamJoinRequest(core.TeamJoinRequest{
		Username: "bob",
	}); err == nil {
		t.Fatal("request without timestamp should fail")
	}
	if err := fs.AppendTeamJoinResolution(core.TeamJoinRequest{
		ID: "short", Username: "bob", RequestedAt: now,
		Resolution: core.TeamJoinRequestResolutionApproved, ResolvedAt: now,
	}); err == nil {
		t.Fatal("approval with malformed request ID should fail")
	}
	if err := fs.AppendTeamJoinResolution(core.TeamJoinRequest{
		ID: "unexpected", Username: "bob", Resolution: core.TeamJoinRequestResolutionRosterAdded,
		ResolvedAt: now,
	}); err == nil {
		t.Fatal("roster-add watermark with request ID should fail")
	}
}

func TestTeamJoinRequestRejectsCorruptShard(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	dir := filepath.Join(home, "team-requests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Z2N...log"), []byte("{broken}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.TeamJoinRequests(); err == nil {
		t.Fatal("corrupt request shard should fail visibly")
	}
}
