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
	LastAttempt     time.Time        `json:"last_attempt,omitempty"`
	LastSuccessPull time.Time        `json:"last_success_pull,omitempty"`
	LastSuccessPush time.Time        `json:"last_success_push,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	AuthState       string           `json:"auth_state,omitempty"`
	Conflicts       []ConflictRecord `json:"conflicts,omitempty"`
	// LastSync is the pre-P0-3 field, which also advanced on failed runs. It is
	// read once for migration and never written.
	LastSync *time.Time `json:"last_sync,omitempty"`
}

func statePath(storeDir string) string {
	return filepath.Join(storeDir, ".syncstate.json")
}

func loadState(storeDir string) syncState {
	var st syncState
	if data, err := os.ReadFile(statePath(storeDir)); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.LastSync != nil && st.LastSuccessPull.IsZero() && st.LastSuccessPush.IsZero() {
		st.LastSuccessPull = *st.LastSync
		st.LastSuccessPush = *st.LastSync
	}
	st.LastSync = nil
	return st
}

func saveState(storeDir string, st syncState) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err == nil {
		_ = os.WriteFile(statePath(storeDir), data, 0o600)
	}
}
