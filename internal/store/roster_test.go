package store

import (
	"os"
	"path/filepath"
	"testing"

	"dossier/internal/core"
)

func TestRosterYAMLRoundTrip(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	want := &core.Roster{
		Manager: "hgill",
		Members: map[string]string{"hgill": "Herwin Gill", "psmith": "Priya Shah"},
		Former:  map[string]string{"old": "Former Person"},
	}
	if err := fs.WriteRoster(want); err != nil {
		t.Fatalf("WriteRoster: %v", err)
	}
	got, err := fs.ReadRoster()
	if err != nil {
		t.Fatalf("ReadRoster: %v", err)
	}
	if got.Manager != want.Manager || len(got.Members) != 2 || got.Members["psmith"] != "Priya Shah" || got.Former["old"] != "Former Person" {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestReadRosterMissingIsEmpty(t *testing.T) {
	got, err := NewFSStore(t.TempDir()).ReadRoster()
	if err != nil {
		t.Fatal(err)
	}
	if got.Manager != "" || len(got.Members) != 0 || len(got.Former) != 0 {
		t.Fatalf("missing roster = %+v, want empty", got)
	}
}

func TestRosterSnapshotRestoreMovesGeneratedRosterAside(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	snap, err := fs.SnapshotRoster()
	if err != nil {
		t.Fatalf("SnapshotRoster: %v", err)
	}
	if snap.Found {
		t.Fatal("empty store snapshot should not be found")
	}
	if err := fs.WriteRoster(&core.Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}}); err != nil {
		t.Fatalf("WriteRoster: %v", err)
	}
	if err := fs.RestoreRoster(snap); err != nil {
		t.Fatalf("RestoreRoster: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "team.yaml")); !os.IsNotExist(err) {
		t.Fatalf("team.yaml still exists, err=%v", err)
	}
	matches, err := filepath.Glob(home + ".failed-create-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("failed-create sidecars = %v, want one", matches)
	}
	if _, err := os.Stat(filepath.Join(matches[0], "team.yaml")); err != nil {
		t.Fatalf("moved roster missing: %v", err)
	}
}

func TestRosterRestoreReinstatesPreviousRoster(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	rosterA := &core.Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}}
	if err := fs.WriteRoster(rosterA); err != nil {
		t.Fatalf("WriteRoster A: %v", err)
	}
	snap, err := fs.SnapshotRoster()
	if err != nil {
		t.Fatalf("SnapshotRoster: %v", err)
	}
	if err := fs.WriteRoster(&core.Roster{Manager: "bob", Members: map[string]string{"bob": "Bob"}}); err != nil {
		t.Fatalf("WriteRoster B: %v", err)
	}
	if err := fs.RestoreRoster(snap); err != nil {
		t.Fatalf("RestoreRoster: %v", err)
	}
	got, err := fs.ReadRoster()
	if err != nil {
		t.Fatalf("ReadRoster: %v", err)
	}
	if got.Manager != rosterA.Manager || got.Members["alice"] != "Alice" || len(got.Members) != 1 {
		t.Fatalf("restored roster = %+v, want %+v", got, rosterA)
	}
	matches, err := filepath.Glob(home + ".failed-create-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected failed-create sidecars = %v", matches)
	}
}
