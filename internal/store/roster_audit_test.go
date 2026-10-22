package store

import (
	"path/filepath"
	"testing"
	"time"

	"dossier/internal/core"
)

func TestAppendTeamAuditUsesSyncedAuthorShard(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	event := core.AuditEvent{TS: time.Now().UTC(), Event: "team_member_added", Actor: "human:alice", Author: "machine-a", DossierID: core.RosterConflictDossierID, Message: "Added member."}
	if err := fs.AppendTeamAudit(event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "team-audit", "machine-a.log")
	entries, err := ReadAuditEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Actor != event.Actor || entries[0].Author != event.Author || entries[0].DossierID != core.RosterConflictDossierID {
		t.Fatalf("team audit entries = %+v", entries)
	}
}

// TestTeamJoinAuthorsReadsJoinAndCreateRecords checks that doctor's join list
// comes from the real shards: team_created and team_joined count, other team
// events do not, and each author's shard is read.
func TestTeamJoinAuthorsReadsJoinAndCreateRecords(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	now := time.Now().UTC()
	events := []core.AuditEvent{
		{TS: now, Event: "team_created", Author: "hgill", DossierID: core.RosterConflictDossierID},
		{TS: now, Event: "team_member_added", Author: "hgill", DossierID: core.RosterConflictDossierID},
		{TS: now, Event: "team_joined", Author: "gclawson", DossierID: core.RosterConflictDossierID},
	}
	for _, event := range events {
		if err := fs.AppendTeamAudit(event); err != nil {
			t.Fatal(err)
		}
	}
	got, err := fs.TeamJoinAuthors()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, author := range got {
		seen[author]++
	}
	if len(got) != 2 || seen["hgill"] != 1 || seen["gclawson"] != 1 {
		t.Fatalf("join authors = %v, want hgill once from team_created and gclawson once from team_joined", got)
	}
}
