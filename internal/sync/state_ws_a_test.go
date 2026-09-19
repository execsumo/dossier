package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestState_LegacyLoads(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now()
	legacy := map[string]interface{}{
		"last_sync": now.Format(time.RFC3339Nano),
	}
	data, _ := json.Marshal(legacy)
	os.WriteFile(filepath.Join(tmp, ".syncstate.json"), data, 0600)

	st := loadState(tmp)
	if st.LastSuccessPull.IsZero() || st.LastSuccessPush.IsZero() {
		t.Errorf("expected legacy last_sync to populate success times")
	}
}
