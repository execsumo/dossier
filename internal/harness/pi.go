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
// spawned by its bash tool, so Dossier ships an extension that publishes the
// live session id for every Dossier process Pi owns and exposes the native
// `/spark` alias. Install writes the extension and shared spark skill into Pi's
// global integration directories. Pi's lifecycle (session-start/session-end/
// pre-compaction) is not bridged yet — see docs/harness-capabilities.md.
type PiHarness struct {
	dossierHome string
	notes       []string
}

func NewPiHarness(dossierHome string) *PiHarness {
	return &PiHarness{dossierHome: dossierHome}
}

func (p *PiHarness) PostInstallNotes() []string {
	return p.notes
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

// PiSparkSkillPath returns where the bundled spark skill is installed. Pi
// discovers skills from its global agent skills directory.
func PiSparkSkillPath() string {
	agentDir := PiAgentDir()
	if agentDir == "" {
		return ""
	}
	return filepath.Join(agentDir, "skills", "spark", "SKILL.md")
}

// PiSparkSkillInstalled reports whether Pi has the current bundled spark skill.
func PiSparkSkillInstalled() bool {
	content, err := assets.FS.ReadFile("spark-skill.md")
	if err != nil {
		return false
	}
	path := PiSparkSkillPath()
	return path != "" && managedAssetInstalled(path, content)
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

// Install writes the bundled Pi extension and spark skill into Pi's global
// integration directories. Non-clobbering and idempotent (B7/B8): identical
// files are left alone, modified files are backed up before being replaced, and
// without confirmation (or a terminal to ask on) nothing is written.
func (p *PiHarness) Install(opts core.InstallOpts) error {
	p.notes = nil
	if !PiInstalled() {
		return fmt.Errorf("%w: Pi is not installed on this device", core.ErrInstallSkipped)
	}

	content, err := assets.FS.ReadFile(piExtensionAsset)
	if err != nil {
		return fmt.Errorf("failed to read embedded Pi extension asset: %w", err)
	}
	sparkContent, err := assets.FS.ReadFile("spark-skill.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded spark skill asset: %w", err)
	}

	dest := PiExtensionPath()
	if dest == "" {
		return fmt.Errorf("could not determine Pi extension path")
	}
	if resolved, err := filepath.EvalSymlinks(dest); err == nil {
		dest = resolved
	}
	sparkDest := PiSparkSkillPath()
	if sparkDest == "" {
		return fmt.Errorf("could not determine Pi spark skill path")
	}

	if managedAssetInstalled(dest, content) && managedAssetInstalled(sparkDest, sparkContent) {
		return nil
	}

	// We rely on YesToAll and the TTY check instead of opts.Interactive;
	// if YesToAll is false, we always require a terminal to prompt on,
	// effectively ignoring opts.Interactive.
	if !opts.YesToAll {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return fmt.Errorf("%w: could not stat stdin: %v", core.ErrInstallSkipped, err)
		}
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Not a terminal: never write config the user could not consent to.
			return fmt.Errorf("%w: no terminal to confirm on", core.ErrInstallSkipped)
		}

		fmt.Printf("Install the Dossier Pi integration (session identity + /spark) to %s? [y/N]: ", dest)
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return fmt.Errorf("%w: user declined", core.ErrInstallSkipped)
		}
	}

	timestamp := time.Now().Unix()
	if err := installManagedAsset(dest, content, timestamp); err != nil {
		return fmt.Errorf("failed to install Pi extension: %w", err)
	}
	if err := installManagedAsset(sparkDest, sparkContent, timestamp); err != nil {
		return fmt.Errorf("failed to install Pi spark skill: %w", err)
	}

	_, hasPointer := LookupPiSessionPointer()
	if os.Getenv("PI_SESSION_ID") == "" && !hasPointer {
		p.notes = append(p.notes, "Please restart Pi for the new integration to take effect.")
	}

	return nil
}
