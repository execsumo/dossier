package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// syncState persists the last-sync time and pending conflict records so that
// [GitSync.Status] can report them without re-deriving. It lives at
// <store>/.syncstate.json (machine-local, gitignored).
type syncState struct {
	LastSync  time.Time        `json:"last_sync"`
	Conflicts []ConflictRecord `json:"conflicts,omitempty"`
}

func statePath(storeDir string) string {
	return filepath.Join(storeDir, ".syncstate.json")
}

func loadState(storeDir string) syncState {
	var st syncState
	if data, err := os.ReadFile(statePath(storeDir)); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func saveState(storeDir string, st syncState) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err == nil {
		_ = os.WriteFile(statePath(storeDir), data, 0o600)
	}
}
