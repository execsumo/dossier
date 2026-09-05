package harness

import (
	"dossier/assets"
	"dossier/internal/core"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// piTestEnv points Pi's agent dir and the pointer dir at throwaway locations and
// clears the session environment, so tests describe Pi rather than the machine.
func piTestEnv(t *testing.T) string {
	t.Helper()
	agentDir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	t.Setenv("DOSSIER_PI_SESSION_DIR", filepath.Join(agentDir, "dossier", "sessions"))
	t.Setenv("PI_CODING_AGENT", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("PI_SESSION_FILE", "")
	return agentDir
}

// Pi's own session environment reaches only bash-tool children, and Dossier does
// not bridge Pi's lifecycle yet — so identity and transcript are the only things
// Detect may claim.
func TestPiHarnessDetectsSessionIdentityWithoutClaimingHooks(t *testing.T) {
	piTestEnv(t)
	t.Setenv("PI_SESSION_ID", "pi-session")
	t.Setenv("PI_SESSION_FILE", "/tmp/pi-session.jsonl")

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.Installed || !caps.SessionIdentity || !caps.TranscriptCapture {
		t.Fatalf("expected Pi presence, identity and transcript, got %+v", caps)
	}
	if caps.MCP || caps.SessionStartHook || caps.SessionEndHook || caps.PreCompactionHook {
		t.Fatalf("Pi must not claim MCP or lifecycle hooks Dossier does not provide, got %+v", caps)
	}
}

func TestPiHarnessDoesNotClaimTranscriptWithoutSessionFile(t *testing.T) {
	piTestEnv(t)
	t.Setenv("PI_SESSION_ID", "pi-session")

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caps.TranscriptCapture {
		t.Fatal("Pi must not claim transcript capture without a session file")
	}
}

// Pi installed on the device but never started is still installable: that is
// what makes `dossier init` able to wire it up ahead of first use.
func TestPiHarnessDetectsInstalledPiOutsideASession(t *testing.T) {
	piTestEnv(t)

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.Installed {
		t.Fatal("expected Pi to be detected from its agent directory")
	}
	if caps.SessionIdentity {
		t.Fatal("session identity must stay unavailable until the extension is installed")
	}
	if !caps.Present() || caps.LiveSession() {
		t.Fatalf("installed-but-idle Pi should be present, not live: %+v", caps)
	}
}

func TestPiHarnessNotDetectedWithoutPi(t *testing.T) {
	if _, err := exec.LookPath("pi"); err == nil {
		t.Skip("pi is installed on this machine")
	}
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("PI_CODING_AGENT_DIR", missing)
	t.Setenv("DOSSIER_PI_SESSION_DIR", missing)
	t.Setenv("PI_CODING_AGENT", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("PI_SESSION_FILE", "")

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caps.Present() {
		t.Fatalf("expected no Pi capabilities, got %+v", caps)
	}
}

// The pointer is the identity source for a process Pi did not spawn via bash.
func TestPiHarnessDetectsIdentityFromPointer(t *testing.T) {
	piTestEnv(t)
	dir := os.Getenv("DOSSIER_PI_SESSION_DIR")
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:      PiPointerSchema,
		PID:         os.Getpid(),
		SessionID:   "pi-pointer-session",
		SessionFile: "/tmp/pi-session.jsonl",
	})

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.SessionIdentity || !caps.TranscriptCapture {
		t.Fatalf("expected pointer to supply identity and transcript, got %+v", caps)
	}
}

// A live pointer proves Pi owns this process even when its agent directory is
// somewhere Dossier would not look — the binding must still be filed under Pi.
func TestPiHarnessDetectedFromPointerAloneWithoutAgentDir(t *testing.T) {
	if _, err := exec.LookPath("pi"); err == nil {
		t.Skip("pi is installed on this machine")
	}
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	t.Setenv("PI_CODING_AGENT", "")
	t.Setenv("PI_SESSION_ID", "")
	t.Setenv("PI_SESSION_FILE", "")
	writePointer(t, dir, os.Getpid(), PiSessionPointer{
		Schema:    PiPointerSchema,
		PID:       os.Getpid(),
		SessionID: "pi-pointer-session",
	})

	caps, err := NewPiHarness("/tmp/dossier").Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.Installed || !caps.SessionIdentity {
		t.Fatalf("expected a live pointer to identify Pi, got %+v", caps)
	}
}

func TestPiHarnessInstallsExtensionIdempotently(t *testing.T) {
	agentDir := piTestEnv(t)
	h := NewPiHarness("/tmp/dossier")

	if err := h.Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	dest := filepath.Join(agentDir, "extensions", "dossier", "index.ts")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("expected extension at %s: %v", dest, err)
	}
	want, err := assets.FS.ReadFile("pi-extension.ts")
	if err != nil {
		t.Fatalf("read embedded asset: %v", err)
	}
	if string(got) != string(want) {
		t.Error("installed extension does not match the bundled asset")
	}

	sparkPath := PiSparkSkillPath()
	sparkGot, err := os.ReadFile(sparkPath)
	if err != nil {
		t.Fatalf("expected spark skill at %s: %v", sparkPath, err)
	}
	sparkWant, err := assets.FS.ReadFile("spark-skill.md")
	if err != nil {
		t.Fatalf("read embedded spark skill: %v", err)
	}
	if string(sparkGot) != string(sparkWant) {
		t.Error("installed spark skill does not match the bundled asset")
	}

	// Installing the extension is what turns session identity on.
	caps, err := h.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.SessionIdentity {
		t.Error("expected session identity to be available after install")
	}

	if err := h.Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	baks, _ := filepath.Glob(dest + ".*.bak")
	if len(baks) != 0 {
		t.Errorf("idempotent install should not back up an identical file, got %v", baks)
	}
}

func TestPiHarnessInstallBacksUpModifiedExtension(t *testing.T) {
	agentDir := piTestEnv(t)
	dest := filepath.Join(agentDir, "extensions", "dossier", "index.ts")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("// hand-edited\n"), 0644); err != nil {
		t.Fatalf("seed extension: %v", err)
	}

	if err := NewPiHarness("/tmp/dossier").Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	baks, _ := filepath.Glob(dest + ".*.bak")
	if len(baks) != 1 {
		t.Fatalf("expected the replaced file to be backed up once, got %v", baks)
	}
	backup, err := os.ReadFile(baks[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "// hand-edited\n" {
		t.Errorf("backup lost the user's content: %q", string(backup))
	}
}

// Without a terminal to confirm on, Install must write nothing (B7/B8).
func TestPiHarnessInstallSkipsWithoutConfirmation(t *testing.T) {
	agentDir := piTestEnv(t)

	err := NewPiHarness("/tmp/dossier").Install(core.InstallOpts{})
	if err == nil || !errors.Is(err, core.ErrInstallSkipped) {
		t.Fatalf("expected ErrInstallSkipped, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "extensions", "dossier", "index.ts")); !os.IsNotExist(err) {
		t.Error("extension should not be written without confirmation")
	}
}

// Nothing to install into: Pi absent means Install is a no-op, not an error.
func TestPiHarnessInstallNoOpWithoutPi(t *testing.T) {
	if _, err := exec.LookPath("pi"); err == nil {
		t.Skip("pi is installed on this machine")
	}
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("PI_CODING_AGENT_DIR", missing)
	t.Setenv("DOSSIER_PI_SESSION_DIR", missing)
	t.Setenv("PI_CODING_AGENT", "")
	t.Setenv("PI_SESSION_ID", "")

	err := NewPiHarness("/tmp/dossier").Install(core.InstallOpts{YesToAll: true})
	if err == nil || !errors.Is(err, core.ErrInstallSkipped) {
		t.Fatalf("expected ErrInstallSkipped, got %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("Install must not create a Pi agent directory")
	}
}

func TestPiHarnessPostInstallNotesOnFirstInstall(t *testing.T) {
	piTestEnv(t)
	h := NewPiHarness("/tmp/dossier")

	if err := h.Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	notes := h.PostInstallNotes()
	if len(notes) == 0 {
		t.Fatal("expected post-install notes on first install, got none")
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "restart Pi") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected restart advice, got notes: %v", notes)
	}
}

func TestPiHarnessPostInstallNotesEmptyOnIdempotentInstall(t *testing.T) {
	piTestEnv(t)
	h := NewPiHarness("/tmp/dossier")

	if err := h.Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	if err := h.Install(core.InstallOpts{YesToAll: true}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	notes := h.PostInstallNotes()
	if len(notes) != 0 {
		t.Errorf("expected no post-install notes on idempotent install, got: %v", notes)
	}
}
