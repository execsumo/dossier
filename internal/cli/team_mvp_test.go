package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestLeadCommandAcceptsFirstNameAndReportsCandidates(t *testing.T) {
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
	for _, name := range []string{"Priya Shah", "Priya Singh"} {
		if code, output := runDossier(t, binary, store, fakeHome, "", "team", "add", strings.ToLower(strings.ReplaceAll(name, " ", "")), name); code != 0 {
			t.Fatalf("team add %s failed: %d %s", name, code, output)
		}
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "promote", "Assigned", "--lead", "Priya Shah", "--force"); code != 0 {
		t.Fatalf("promote failed: %d %s", code, output)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "lead", "assigned", "Priya"); code == 0 || !strings.Contains(output, "ambiguous") {
		t.Fatalf("ambiguous first name = (%d, %s)", code, output)
	}
	if code, output := runDossier(t, binary, store, fakeHome, "", "lead", "assigned", "Nobody"); code == 0 || !strings.Contains(output, "unknown team member") {
		t.Fatalf("unknown lead = (%d, %s)", code, output)
	}
}
