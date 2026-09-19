package tui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dossier/internal/config"
	"dossier/internal/core"
	"dossier/internal/store"
	dossiersync "dossier/internal/sync"
)

func TestHealthFooterMatchesCLIDoctor(t *testing.T) {
	home := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runTestCommand(t, "git", "init", "-q", "--bare", "-b", "main", remote)
	runTestCommand(t, "git", "init", "-q", "-b", "main", home)

	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	_, err := fs.Write(&core.Dossier{Frontmatter: core.Frontmatter{
		ID: "dos_health", Slug: "health", Name: "Health", Status: core.StatusActive, Priority: core.PriorityMedium,
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteConflict(&core.Conflict{ID: "conf_health", DossierID: "dos_health", Kind: "sync_concurrent_edit"}); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]string{"last_error": "offline"})
	if err := os.WriteFile(filepath.Join(home, ".syncstate.json"), state, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dossiersync.EnsureGitignore(home); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DossierHome = home
	cfg.Team = config.TeamConfig{Remote: remote, Branch: "main"}
	if err := cfg.Save(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, "git", "-C", home, "config", "user.name", "Dossier Test")
	runTestCommand(t, "git", "-C", home, "config", "user.email", "dossier-test@example.invalid")
	runTestCommand(t, "git", "-C", home, "add", ".")
	runTestCommand(t, "git", "-C", home, "commit", "-qm", "fixture")

	gs := dossiersync.New(dossiersync.Config{StoreDir: home, RemoteURL: remote, Branch: "main", AuthState: "none"})
	svc := core.NewService(fs, nil, nil, nil, nil, core.Config{DossierHome: home, TeamRemote: remote}, dossiersync.NewAdapter(gs))
	result, err := svc.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	health := result.Data.(core.HealthReport)
	m := NewModel(svc)
	m.width, m.height = 120, 30
	updated, _ := m.Update(healthMsg{summary: health.Summary, report: health.Doctor})
	view := stripANSI(updated.(Model).View())

	_, root, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(root), "../.."))
	cmd := exec.Command("go", "run", "./cmd/dossier", "doctor")
	cmd.Dir = repoRoot
	// Keep Go's module cache outside the fixture store. Doctor is expected to
	// exit non-zero here because the fixture deliberately contains a conflict.
	cmd.Env = append(os.Environ(), "DOSSIER_HOME="+home)
	output, _ := cmd.CombinedOutput()
	var cliLine string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Health: ") {
			cliLine = strings.TrimPrefix(line, "Health: ")
			break
		}
	}
	if cliLine == "" {
		t.Fatalf("doctor output had no Health line:\n%s", output)
	}
	if !strings.Contains(view, cliLine) {
		t.Fatalf("TUI footer does not contain CLI health line %q:\n%s", cliLine, view)
	}
}

func runTestCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
}
