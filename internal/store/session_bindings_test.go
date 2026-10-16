package store

import (
	"dossier/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListSessionBindings(t *testing.T) {
	fs := NewFSStore(t.TempDir())
	if bindings, err := fs.ListSessionBindings(); err != nil || len(bindings) != 0 {
		t.Fatalf("absent directory: %v, %v", bindings, err)
	}
	if err := fs.SaveSessionBinding(&core.SessionBinding{SessionBindingID: "real", DossierID: "dos_1"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fs.dossierHome, "sessions", "real.json")
	if err := os.WriteFile(path, []byte(`{"session_binding_id":"../other","dossier_id":"dos_1"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fs.dossierHome, "sessions", "note.txt"), []byte("not a binding"), 0644); err != nil {
		t.Fatal(err)
	}
	bindings, err := fs.ListSessionBindings()
	if err != nil || len(bindings) != 1 || bindings[0].SessionBindingID != "real" {
		t.Fatalf("filename identity: %v, %v", bindings, err)
	}
	if err := os.WriteFile(path, []byte("broken JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ListSessionBindings(); err == nil || !strings.Contains(err.Error(), "sessions/real.json") {
		t.Fatalf("corruption not surfaced: %v", err)
	}
}
