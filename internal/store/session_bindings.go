package store

import (
	"dossier/internal/core"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListSessionBindings reads the machine-local session bindings in filename order.
// A corrupt binding is surfaced rather than silently omitted.
func (s *FSStore) ListSessionBindings() ([]core.SessionBinding, error) {
	entries, err := os.ReadDir(filepath.Join(s.dossierHome, "sessions"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bindings []core.SessionBinding
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dossierHome, "sessions", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read sessions/%s: %w", entry.Name(), err)
		}
		var binding core.SessionBinding
		if err := json.Unmarshal(data, &binding); err != nil {
			return nil, fmt.Errorf("parse sessions/%s: %w", entry.Name(), err)
		}
		// The filename is the lookup identity. Never use untrusted JSON to
		// select a different binding file during archive cleanup.
		binding.SessionBindingID = strings.TrimSuffix(entry.Name(), ".json")
		bindings = append(bindings, binding)
	}
	return bindings, nil
}
