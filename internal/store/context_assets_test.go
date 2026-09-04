package store

import (
	"bytes"
	"dossier/assets"
	"os"
	"path/filepath"
	"testing"
)

// TestInitProjectsContextAssetsByteForByte proves the on-disk copies Init leaves
// under context/ are exact projections of the embedded originals, since the
// embedded copy is authoritative and the disk file exists only to be readable.
func TestInitProjectsContextAssetsByteForByte(t *testing.T) {
	tempHome := t.TempDir()
	s := NewFSStore(tempHome)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	for _, name := range []string{"guide.md", "instructions.md"} {
		embedded, err := assets.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("failed to read embedded %s: %v", name, err)
		}
		onDisk, err := os.ReadFile(filepath.Join(tempHome, "context", name))
		if err != nil {
			t.Fatalf("expected %s to exist on disk after Init: %v", name, err)
		}
		if !bytes.Equal(onDisk, embedded) {
			t.Errorf("%s on disk does not match embedded original byte-for-byte", name)
		}
	}
}

// TestEnsureContextAssetsIsIdempotentWhenNothingChanged proves a second call with
// no drift reports nothing rewritten — the whole point of the byte comparison is
// that every command can call this cheaply without touching disk on the common path.
func TestEnsureContextAssetsIsIdempotentWhenNothingChanged(t *testing.T) {
	tempHome := t.TempDir()
	s := NewFSStore(tempHome)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	refreshed, err := s.EnsureContextAssets()
	if err != nil {
		t.Fatalf("EnsureContextAssets() failed: %v", err)
	}
	if len(refreshed) != 0 {
		t.Errorf("expected no assets rewritten on an unmodified store, got %v", refreshed)
	}
}

// TestEnsureContextAssetsRestoresCorruptedAsset is the upgrade-drift case that
// motivates the method: a disk copy left behind by an older binary (or edited by
// hand) must be caught and overwritten with the embedded original, and reported
// by name so callers know what changed.
func TestEnsureContextAssetsRestoresCorruptedAsset(t *testing.T) {
	tempHome := t.TempDir()
	s := NewFSStore(tempHome)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	guidePath := filepath.Join(tempHome, "context", "guide.md")
	if err := os.WriteFile(guidePath, []byte("STALE GUIDE FROM AN OLDER BINARY"), 0644); err != nil {
		t.Fatalf("failed to corrupt guide.md: %v", err)
	}

	refreshed, err := s.EnsureContextAssets()
	if err != nil {
		t.Fatalf("EnsureContextAssets() failed: %v", err)
	}
	if len(refreshed) != 1 || refreshed[0] != "guide.md" {
		t.Fatalf("expected only guide.md reported rewritten, got %v", refreshed)
	}

	embedded, err := assets.FS.ReadFile("guide.md")
	if err != nil {
		t.Fatalf("failed to read embedded guide.md: %v", err)
	}
	onDisk, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("failed to read restored guide.md: %v", err)
	}
	if !bytes.Equal(onDisk, embedded) {
		t.Error("EnsureContextAssets() did not restore guide.md to the embedded original")
	}
}

// TestEnsureContextAssetsNeverCreatesContextDir is the team-join regression guard.
// `dossier team join` clones into a directory that must contain nothing but
// config.yaml/.gitignore, and wire() calls EnsureContextAssets on every command —
// including the one running inside that half-cloned directory. If this method
// created context/, a team join would be unrecoverable.
func TestEnsureContextAssetsNeverCreatesContextDir(t *testing.T) {
	tempHome := t.TempDir() // no Init(): home exists, context/ does not

	s := NewFSStore(tempHome)
	refreshed, err := s.EnsureContextAssets()
	if err != nil {
		t.Fatalf("EnsureContextAssets() on a home with no context/ returned an error: %v", err)
	}
	if refreshed != nil {
		t.Errorf("expected nil, got %v", refreshed)
	}

	if _, err := os.Stat(filepath.Join(tempHome, "context")); !os.IsNotExist(err) {
		t.Fatal("EnsureContextAssets() created context/ on a store that never had one")
	}
}

// TestReadContextAssetPrefersDiskThenFallsBackToEmbedded proves the two halves of
// the contract: a deliberate local edit is honoured over the embedded original,
// and a deleted disk file still resolves rather than leaving the agent with no
// Guide and nothing said about it.
func TestReadContextAssetPrefersDiskThenFallsBackToEmbedded(t *testing.T) {
	tempHome := t.TempDir()
	s := NewFSStore(tempHome)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	guidePath := filepath.Join(tempHome, "context", "guide.md")

	embedded, err := assets.FS.ReadFile("guide.md")
	if err != nil {
		t.Fatalf("failed to read embedded guide.md: %v", err)
	}

	// Deliberate local edit: the disk copy now differs from the embedded original,
	// and the reader must return the disk content, not silently override it.
	if err := os.WriteFile(guidePath, []byte("LOCALLY CUSTOMIZED GUIDE"), 0644); err != nil {
		t.Fatalf("failed to write local edit: %v", err)
	}
	content, err := s.ReadContextAsset("guide.md")
	if err != nil {
		t.Fatalf("ReadContextAsset() failed: %v", err)
	}
	if content != "LOCALLY CUSTOMIZED GUIDE" {
		t.Errorf("ReadContextAsset() = %q, want the on-disk edit honoured", content)
	}
	if content == string(embedded) {
		t.Fatal("test setup did not actually diverge disk from embedded")
	}

	// Deleted disk file: must fall back to the embedded original rather than error
	// or return empty.
	if err := os.Remove(guidePath); err != nil {
		t.Fatalf("failed to delete guide.md: %v", err)
	}
	content, err = s.ReadContextAsset("guide.md")
	if err != nil {
		t.Fatalf("ReadContextAsset() failed after disk deletion: %v", err)
	}
	if content != string(embedded) {
		t.Error("ReadContextAsset() did not fall back to the embedded original after the disk copy was deleted")
	}
}

// TestStaleContextAssetsNamesOnlyWhatDiffersOrIsMissing proves StaleContextAssets
// is quiet right after Init (nothing has drifted yet) and names exactly the asset
// that is corrupted or deleted — no more, no less.
func TestStaleContextAssetsNamesOnlyWhatDiffersOrIsMissing(t *testing.T) {
	tempHome := t.TempDir()
	s := NewFSStore(tempHome)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if stale := s.StaleContextAssets(); len(stale) != 0 {
		t.Fatalf("expected no stale assets right after Init, got %v", stale)
	}

	guidePath := filepath.Join(tempHome, "context", "guide.md")
	if err := os.WriteFile(guidePath, []byte("CORRUPTED"), 0644); err != nil {
		t.Fatalf("failed to corrupt guide.md: %v", err)
	}
	if stale := s.StaleContextAssets(); len(stale) != 1 || stale[0] != "guide.md" {
		t.Fatalf("StaleContextAssets() after corrupting guide.md = %v, want [guide.md]", stale)
	}

	// Restore via the public API, then delete instructions.md instead — missing
	// must be flagged too, not just "differs".
	if _, err := s.EnsureContextAssets(); err != nil {
		t.Fatalf("failed to restore guide.md: %v", err)
	}
	instructionsPath := filepath.Join(tempHome, "context", "instructions.md")
	if err := os.Remove(instructionsPath); err != nil {
		t.Fatalf("failed to delete instructions.md: %v", err)
	}
	if stale := s.StaleContextAssets(); len(stale) != 1 || stale[0] != "instructions.md" {
		t.Fatalf("StaleContextAssets() after deleting instructions.md = %v, want [instructions.md]", stale)
	}
}
