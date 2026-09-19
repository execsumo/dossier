package sync

import "testing"

func TestSync_BothModifiedTeamYAMLConflicts(t *testing.T) {
	bare, storeA, storeB := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")
	syncB := newSyncer(storeB, bare, "bob")
	writeFile(t, storeA, "team.yaml", "manager: alice\nmembers:\n  alice: Alice\n")
	mustSync(t, syncA)
	mustSync(t, syncB)
	writeFile(t, storeA, "team.yaml", "manager: alice\nmembers:\n  alice: Alice\n  bob: Bob\n")
	writeFile(t, storeB, "team.yaml", "manager: alice\nmembers:\n  alice: Alice\n  carol: Carol\n")
	mustSync(t, syncA)
	report := mustSync(t, syncB)
	if len(report.Conflicts) != 1 || report.Conflicts[0].Path != "team.yaml" {
		t.Fatalf("team roster conflicts = %+v", report.Conflicts)
	}
	assertFile(t, storeB, "team.yaml", "manager: alice\nmembers:\n  alice: Alice\n  bob: Bob\n")
}
