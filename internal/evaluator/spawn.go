package evaluator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SpawnDetached starts `exe args...` in the background, detached from the
// calling hook so the hook returns immediately and is never held open by a
// multi-minute eval. Output is appended to logPath. The child is released,
// not waited on.
func SpawnDetached(exe string, args []string, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("eval log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("eval log: %w", err)
	}
	defer logFile.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = logFile, logFile, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start eval: %w", err)
	}
	return cmd.Process.Release()
}
