package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dossier/internal/core"

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

// TestLsLeadFlagSurfacesUnknownLeadWarning keeps the CLI honest about a lead
// filter that matched nobody: "No dossiers found." alone reads as "this person
// has no work", which is the failure the core warning exists to prevent.
func TestLsLeadFlagSurfacesUnknownLeadWarning(t *testing.T) {
	tempHome := t.TempDir()
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}
	if _, err := svc.Init(context.Background(), core.InitReq{YesToAll: true}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"ls", "--lead", "Nobody", "--home", tempHome})
	execErr := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if execErr != nil {
		t.Fatalf("ls --lead failed: %v", execErr)
	}
	output := buf.String()
	if !strings.Contains(output, "Warning:") || !strings.Contains(output, "Nobody") {
		t.Fatalf("ls --lead Nobody output = %q, want an unknown-lead warning", output)
	}
}
