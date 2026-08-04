package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// The walk has to reach the *owning Pi process*: a Dossier MCP server is a child
// of Pi, and a hook invoked through the bash tool is a grandchild.
func TestLookupPiSessionPointerWalksAncestry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, 400, PiSessionPointer{Schema: PiPointerSchema, PID: 400, SessionID: "owning-pi"})

	// 100 (dossier) -> 200 (shell) -> 400 (pi)
	parents := map[int]int{100: 200, 200: 400, 400: 1}
	got, ok := lookupPiSessionPointer(100, func(pid int) (int, bool) {
		parent, found := parents[pid]
		return parent, found
	})
	if !ok {
		t.Fatal("expected to find the owning Pi process's pointer")
	}
	if got.SessionID != "owning-pi" {
		t.Errorf("got session id %q, want %q", got.SessionID, "owning-pi")
	}
}

// Two Pi sessions must not read each other's bindings: only an ancestor's
// pointer counts, never "whatever pointer happens to be in the directory".
func TestLookupPiSessionPointerIgnoresUnrelatedSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, 900, PiSessionPointer{Schema: PiPointerSchema, PID: 900, SessionID: "other-pi"})

	parents := map[int]int{100: 200, 200: 1}
	if _, ok := lookupPiSessionPointer(100, func(pid int) (int, bool) {
		parent, found := parents[pid]
		return parent, found
	}); ok {
		t.Fatal("a non-ancestor session's pointer must not resolve")
	}
}

func TestLookupPiSessionPointerRejectsUnusablePointers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, 10, PiSessionPointer{Schema: PiPointerSchema, PID: 10, SessionID: ""})
	writePointer(t, dir, 11, PiSessionPointer{Schema: PiPointerSchema + 1, PID: 11, SessionID: "from-the-future"})
	if err := os.WriteFile(filepath.Join(dir, "12.json"), []byte("{not json"), 0600); err != nil {
		t.Fatalf("write corrupt pointer: %v", err)
	}

	for _, pid := range []int{10, 11, 12} {
		if _, ok := lookupPiSessionPointer(pid, func(int) (int, bool) { return 1, true }); ok {
			t.Errorf("pointer for pid %d should have been rejected", pid)
		}
	}
}

// A broken ancestry read stops the walk instead of guessing at a parent.
func TestLookupPiSessionPointerStopsWhenAncestryUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, 400, PiSessionPointer{Schema: PiPointerSchema, PID: 400, SessionID: "owning-pi"})

	if _, ok := lookupPiSessionPointer(100, func(int) (int, bool) { return 0, false }); ok {
		t.Fatal("expected no pointer when the parent cannot be resolved")
	}
}

// The bound keeps a cyclic or pathological ancestry from spinning.
func TestLookupPiSessionPointerBoundsDepth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOSSIER_PI_SESSION_DIR", dir)
	writePointer(t, dir, 999, PiSessionPointer{Schema: PiPointerSchema, PID: 999, SessionID: "too-far"})

	visits := 0
	// A chain that never reaches pid 999 within the depth bound.
	if _, ok := lookupPiSessionPointer(100, func(pid int) (int, bool) {
		visits++
		return pid + 1, true
	}); ok {
		t.Fatal("expected no pointer beyond the ancestry bound")
	}
	if visits > piAncestryDepth {
		t.Errorf("walked %d ancestors, bound is %d", visits, piAncestryDepth)
	}
}

func TestParentPIDResolvesThisProcess(t *testing.T) {
	got, ok := parentPID(os.Getpid())
	if !ok {
		t.Skip("process ancestry is not readable in this environment")
	}
	if got != os.Getppid() {
		t.Errorf("parentPID = %d, want %d", got, os.Getppid())
	}
}

func TestPiSessionPointerDirHonoursPiAgentDir(t *testing.T) {
	t.Setenv("DOSSIER_PI_SESSION_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "/custom/pi-agent")

	want := filepath.Join("/custom/pi-agent", "dossier", "sessions")
	if got := PiSessionPointerDir(); got != want {
		t.Errorf("PiSessionPointerDir() = %q, want %q", got, want)
	}
}
