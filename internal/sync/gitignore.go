package sync

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultGitignoreEntries is the machine-local exclusion set from the Team Sync
// plan (BUILD-DECISIONS B12): these live at the store root and must never be
// committed/synced. Per-author content under <slug>/sessions/<author>/ and
// <slug>/audit/<author>.log is intentionally NOT matched here — it syncs.
//
// The dir entries are anchored at the repo root with a leading "/" so that
// per-slug subdirectories of the same name continue to sync.
var defaultGitignoreEntries = []string{
	"# Dossier sync — machine-local, never committed (managed by internal/sync)",
	"/config.yaml",
	"/credentials",
	"/sessions/", // root session bindings only; <slug>/sessions/ syncs
	"/context/",
	".lock",
	".sync.lock",
	".syncstate.json",
	"*.swp",
}

// EnsureGitignore writes the default machine-local exclusion set to
// <store>/.gitignore if absent. If a .gitignore already exists it is read and
// any missing default entries are appended (merged) — existing user entries are
// never removed or reordered (BUILD-DECISIONS B7 spirit: read/merge, idempotent).
func EnsureGitignore(storeDir string) error {
	path := filepath.Join(storeDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read .gitignore: %w", err)
		}
		// absent: write the default set verbatim
		body := []byte(strings.Join(defaultGitignoreEntries, "\n") + "\n")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
		return nil
	}
	// present: merge any missing defaults (never remove user lines)
	existing := splitGitignoreLines(data)
	var add []string
	for _, e := range defaultGitignoreEntries {
		if !containsLine(existing, e) {
			add = append(add, e)
		}
	}
	if len(add) == 0 {
		return nil // already complete; do not touch (idempotent)
	}
	merged := append([]string{}, existing...)
	merged = append(merged, add...)
	out := []byte(strings.Join(merged, "\n") + "\n")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("merge .gitignore: %w", err)
	}
	return nil
}

func splitGitignoreLines(data []byte) []string {
	var lines []string
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines
}

func containsLine(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.TrimSpace(h) == strings.TrimSpace(needle) {
			return true
		}
	}
	return false
}
