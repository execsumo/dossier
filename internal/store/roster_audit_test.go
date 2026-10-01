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
