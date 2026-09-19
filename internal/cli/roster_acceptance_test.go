package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestTeamRosterCommands(t *testing.T) {
	binary := buildDossierBinary(t)
	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	store := filepath.Join(root, "store")
	remote := filepath.Join(root, "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "init", "--yes"); code != 0 {
		t.Fatalf("init failed: %d %s", code, output)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "team", "create", remote, "--yes", "--name", "Herwin Gill"); code != 0 {
		t.Fatalf("team create failed: %d %s", code, output)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "team", "add", `ACME\PSmith`, "Priya Shah"); code != 0 {
		t.Fatalf("team add failed: %d %s", code, output)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "team", "add", "zuser", "Aaron Able"); code != 0 {
		t.Fatalf("second team add failed: %d %s", code, output)
	}
	code, output := runDossier(t, binary, store, fakeHome, "", "team", "members", "--json")
	if code != 0 {
		t.Fatalf("team members failed: %d %s", code, output)
	}
	var roster struct {
		Members []struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(output), &roster); err != nil {
		t.Fatal(err)
	}
	if len(roster.Members) != 3 || roster.Members[1].Username != "zuser" || roster.Members[1].DisplayName != "Aaron Able" || roster.Members[2].Username != "psmith" {
		t.Fatalf("members = %+v", roster.Members)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "team", "remove", "psmith"); code != 0 {
		t.Fatalf("team remove failed: %d %s", code, output)
	}
	data, err := os.ReadFile(filepath.Join(store, "team.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "former:") || !strings.Contains(string(data), "psmith: Priya Shah") || !strings.Contains(string(data), "Herwin Gill") {
		t.Fatalf("former member or manager display name missing from team.yaml:\n%s", data)
	}
}
