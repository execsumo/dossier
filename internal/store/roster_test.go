package store

import (
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
