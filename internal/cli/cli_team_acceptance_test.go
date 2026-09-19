package cli

import (
	"bytes"
	"dossier/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func buildDossierBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), stableBinaryName())
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/dossier")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dossier: %v\n%s", err, output)
	}
	return binary
}

func runDossier(t *testing.T, binary, home, fakeHome, stdin string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = filteredEnv("DOSSIER_HOME", "HOME", "USERPROFILE")
	cmd.Env = append(cmd.Env, "DOSSIER_HOME="+home, "HOME="+fakeHome, "USERPROFILE="+fakeHome)
	cmd.Stdin = bytes.NewBufferString(stdin)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err == nil {
		return 0, output.String()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), output.String()
	}
	t.Fatalf("run dossier %v: %v\n%s", args, err, output.String())
	return -1, output.String()
}

func filteredEnv(keys ...string) []string {
	var env []string
	for _, entry := range os.Environ() {
		keep := true
		for _, key := range keys {
			if strings.HasPrefix(entry, key+"=") {
				keep = false
				break
			}
		}
		if keep {
			env = append(env, entry)
		}
	}
	return env
}

func makeEmptyBare(t *testing.T, path string) {
	t.Helper()
	repo, err := git.PlainInit(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
}

func remoteMainHash(t *testing.T, path string) plumbing.Hash {
	t.Helper()
	repo, err := git.PlainOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	return ref.Hash()
}

func assertNoTeamRemote(t *testing.T, home string) {
	t.Helper()
	cfg, err := config.Load(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team.Remote != "" {
		t.Fatalf("team.remote unexpectedly set to %q", cfg.Team.Remote)
	}
}

func TestTeamCLIAcceptance(t *testing.T) {
	binary := buildDossierBinary(t)
	root := t.TempDir()
	fakeHome := filepath.Join(root, "fakehome")
	if err := os.Mkdir(fakeHome, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := filepath.Join(root, "manager")
	remote := filepath.Join(root, "remote.git")
	makeEmptyBare(t, remote)

	if code, output := runDossier(t, binary, manager, fakeHome, "", "init", "--yes"); code != 0 {
		t.Fatalf("init failed (%d): %s", code, output)
	}
	for _, name := range []string{"Team pricing review", "Private topic"} {
		if code, output := runDossier(t, binary, manager, fakeHome, "", "promote", name); code != 0 {
			t.Fatalf("promote %q failed (%d): %s", name, code, output)
		}
	}
	if code, output := runDossier(t, binary, manager, fakeHome, "", "archive", "private-topic"); code != 0 {
		t.Fatalf("archive failed (%d): %s", code, output)
	}

	// EOF is a non-interactive refusal, and the preview must include both dossiers.
	code, output := runDossier(t, binary, manager, fakeHome, "", "team", "create", remote)
	if code == 0 || !strings.Contains(output, "confirmation required") {
		t.Fatalf("EOF should refuse confirmation (%d): %s", code, output)
	}
	for _, want := range []string{"Team pricing review", "team-pricing-review", "Private topic", "private-topic"} {
		if !strings.Contains(output, want) {
			t.Fatalf("preview missing %q: %s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Join(manager, ".git")); !os.IsNotExist(err) {
		t.Fatalf("EOF refusal changed .git: %v", err)
	}
	assertNoTeamRemote(t, manager)

	// Explicit decline has the same transactional guarantees.
	code, output = runDossier(t, binary, manager, fakeHome, "n\n", "team", "create", remote)
	if code == 0 || !strings.Contains(output, "Team create refused") {
		t.Fatalf("decline should refuse (%d): %s", code, output)
	}
	if _, err := os.Stat(filepath.Join(manager, ".git")); !os.IsNotExist(err) {
		t.Fatalf("decline changed .git: %v", err)
	}
	assertNoTeamRemote(t, manager)

	// --yes succeeds and persists team.remote only after the push succeeds.
	code, output = runDossier(t, binary, manager, fakeHome, "", "team", "create", remote, "--yes")
	if code != 0 {
		t.Fatalf("--yes create failed (%d): %s", code, output)
	}
	cfg, err := config.Load(filepath.Join(manager, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team.Remote != remote {
		t.Fatalf("team.remote = %q, want %q", cfg.Team.Remote, remote)
	}

	// A non-empty remote is rejected without touching a fresh store or refs.
	beforeNonEmpty := remoteMainHash(t, remote)
	other := filepath.Join(root, "other")
	if code, output := runDossier(t, binary, other, fakeHome, "", "team", "create", remote, "--yes"); code == 0 || !strings.Contains(output, "not empty") {
		t.Fatalf("non-empty remote should be rejected (%d): %s", code, output)
	}
	if _, err := os.Stat(filepath.Join(other, ".git")); !os.IsNotExist(err) {
		t.Fatalf("non-empty rejection changed .git: %v", err)
	}
	assertNoTeamRemote(t, other)
	if got := remoteMainHash(t, remote); got != beforeNonEmpty {
		t.Fatalf("non-empty rejection changed remote ref: before=%s after=%s", beforeNonEmpty, got)
	}

	// A failed join leaves no local team config or .git, then a retry works.
	joined := filepath.Join(root, "joined")
	code, output = runDossier(t, binary, joined, fakeHome, "", "team", "join", filepath.Join(root, "missing.git"))
	if code == 0 || !strings.Contains(output, "Team join failed") {
		t.Fatalf("bad join should fail (%d): %s", code, output)
	}
	if _, err := os.Stat(filepath.Join(joined, ".git")); !os.IsNotExist(err) {
		t.Fatalf("bad join left .git: %v", err)
	}
	assertNoTeamRemote(t, joined)
	if matches, err := filepath.Glob(joined + ".failed-join-*"); err != nil || len(matches) == 0 {
		t.Fatalf("bad join archive missing: %v (%v)", matches, err)
	}
	code, output = runDossier(t, binary, joined, fakeHome, "", "team", "join", remote)
	if code != 0 || !strings.Contains(output, "Successfully joined team store") {
		t.Fatalf("join retry failed (%d): %s", code, output)
	}
	if code, output = runDossier(t, binary, joined, fakeHome, "", "ls"); code != 0 || !strings.Contains(output, "Team pricing review") {
		t.Fatalf("joined dossier missing from ls (%d): %s", code, output)
	}

}
