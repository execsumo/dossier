package exportout

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestInsideDirCaseInsensitiveFS(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".dossier")
	other := filepath.Join(filepath.Dir(store), ".Dossier", "brief.md")

	prev := caseInsensitiveFS
	t.Cleanup(func() { caseInsensitiveFS = prev })

	caseInsensitiveFS = true
	if !insideDir(other, store) {
		t.Fatalf("on a case-insensitive filesystem %s is inside %s", other, store)
	}
	if runtime.GOOS == "windows" {
		return // filepath.Rel itself ignores case on Windows
	}
	caseInsensitiveFS = false
	if insideDir(other, store) {
		t.Fatalf("on a case-sensitive filesystem %s is a different directory", other)
	}
}
