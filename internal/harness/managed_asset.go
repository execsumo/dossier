package harness

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// managedAssetInstalled reports whether path contains the exact bundled asset.
func managedAssetInstalled(path string, content []byte) bool {
	existing, err := os.ReadFile(path)
	return err == nil && bytes.Equal(existing, content)
}

// installManagedAsset writes a bundled integration asset without clobbering a
// user's existing file. The caller is responsible for obtaining confirmation;
// this helper only handles the idempotent write and backup.
func installManagedAsset(path string, content []byte, timestamp int64) error {
	existing, readErr := os.ReadFile(path)
	if readErr == nil && bytes.Equal(existing, content) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create managed asset directory: %w", err)
	}
	if readErr == nil && len(existing) > 0 {
		backupPath := fmt.Sprintf("%s.%d.bak", path, timestamp)
		if err := os.WriteFile(backupPath, existing, 0644); err != nil {
			return fmt.Errorf("back up managed asset: %w", err)
		}
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write managed asset: %w", err)
	}
	return nil
}
