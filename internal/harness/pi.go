package harness

import (
	"bytes"
	"dossier/assets"
	"dossier/internal/core"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// piExtensionAsset is the embedded Pi extension source Dossier installs.
const piExtensionAsset = "pi-extension.ts"

// PiHarness integrates Dossier with the Pi coding agent.
//
// Pi exposes session identity (PI_SESSION_ID/PI_SESSION_FILE) only to processes
// spawned by its bash tool, so Dossier ships a Pi extension that publishes the
// live session id for every Dossier process Pi owns. Install writes that
// extension into Pi's extension directory; assets/pi-extension.ts documents the
// mechanism. Pi's lifecycle (session-start/session-end/pre-compaction) is not
// bridged yet — see docs/harness-capabilities.md.
type PiHarness struct {
	dossierHome string
}

func NewPiHarness(dossierHome string) *PiHarness {
	return &PiHarness{dossierHome: dossierHome}
}

func (p *PiHarness) Name() string { return "pi" }

// PiExtensionPath returns where the Dossier Pi extension is installed. Pi
// auto-discovers `<agent dir>/extensions/*/index.ts`.
func PiExtensionPath() string {
	agentDir := PiAgentDir()
	if agentDir == "" {
		return ""
	}
	return filepath.Join(agentDir, "extensions", "dossier", "index.ts")
}

// PiInstalled reports whether Pi is present on this device: its agent directory
// exists, the process is running inside Pi, or `pi` is on PATH.
func PiInstalled() bool {
	if os.Getenv("PI_CODING_AGENT") != "" || os.Getenv("PI_SESSION_ID") != "" {
		return true
	}
	if agentDir := PiAgentDir(); agentDir != "" {
		if info, err := os.Stat(agentDir); err == nil && info.IsDir() {
			return true
		}
	}
	if _, err := exec.LookPath("pi"); err == nil {
		return true
	}
	return false
}

// PiExtensionInstalled reports whether the installed extension is byte-identical
// to the bundled one. A drifted or absent file is "not installed": Dossier
// rewrites it (after backing it up) on the next install.
func PiExtensionInstalled() bool {
	content, err := assets.FS.ReadFile(piExtensionAsset)
	if err != nil {
		return false
	}
	path := PiExtensionPath()
	if path == "" {
		return false
	}
	existing, err := os.ReadFile(path)
	return err == nil && bytes.Equal(existing, content)
}

// Detect reports what Pi actually offers Dossier. Nothing here is assumed from
// Pi's presence alone: hook capabilities stay false because Dossier does not
// bridge Pi's lifecycle yet, and MCP stays false because Pi has no built-in MCP
// client (an MCP adapter extension is the user's own choice).
func (p *PiHarness) Detect() (core.Capabilities, error) {
	// A resolvable pointer means a live Pi process owns this one, which is proof
	// of Pi regardless of where its agent directory lives.
	pointer, hasPointer := LookupPiSessionPointer()
	if !hasPointer && !PiInstalled() {
		return core.Capabilities{}, nil
	}

	caps := core.Capabilities{Installed: true}

	caps.SessionIdentity = os.Getenv("PI_SESSION_ID") != "" || hasPointer || PiExtensionInstalled()
	caps.TranscriptCapture = os.Getenv("PI_SESSION_FILE") != "" || (hasPointer && pointer.SessionFile != "")

	return caps, nil
}

// Install writes the bundled Pi extension into Pi's extension directory.
// Non-clobbering and idempotent (B7/B8): an identical file is left alone, a
// modified one is backed up before being replaced, and without confirmation
// (or a terminal to ask on) nothing is written.
func (p *PiHarness) Install(opts core.InstallOpts) error {
	if !PiInstalled() {
		return nil
	}

	content, err := assets.FS.ReadFile(piExtensionAsset)
	if err != nil {
		return fmt.Errorf("failed to read embedded Pi extension asset: %w", err)
	}

	dest := PiExtensionPath()
	if dest == "" {
		return fmt.Errorf("could not determine Pi extension path")
	}
	if resolved, err := filepath.EvalSymlinks(dest); err == nil {
		dest = resolved
	}

	existing, readErr := os.ReadFile(dest)
	if readErr == nil && bytes.Equal(existing, content) {
		return nil
	}

	if !opts.YesToAll {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Not a terminal: never write config the user could not consent to.
			return nil
		}

		fmt.Printf("Install the Dossier Pi extension (session identity) to %s? [y/N]: ", dest)
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create Pi extension directory: %w", err)
	}

	if readErr == nil && len(existing) > 0 {
		backupPath := fmt.Sprintf("%s.%d.bak", dest, time.Now().Unix())
		if err := os.WriteFile(backupPath, existing, 0644); err != nil {
			return fmt.Errorf("failed to back up existing Pi extension: %w", err)
		}
	}

	if err := os.WriteFile(dest, content, 0644); err != nil {
		return fmt.Errorf("failed to write Pi extension: %w", err)
	}

	return nil
}
