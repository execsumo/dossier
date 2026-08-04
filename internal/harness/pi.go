package harness

import (
	"dossier/internal/core"
	"os"
)

// PiHarness describes the integration contract provided by Pi and its
// Claude-like hooks extension. The extension owns lifecycle invocation and
// supplies PI_SESSION_ID/PI_SESSION_FILE; Dossier owns persistence.
type PiHarness struct {
	dossierHome string
}

func NewPiHarness(dossierHome string) *PiHarness {
	return &PiHarness{dossierHome: dossierHome}
}

func (p *PiHarness) Name() string { return "pi" }

func (p *PiHarness) Detect() (core.Capabilities, error) {
	if os.Getenv("PI_SESSION_ID") == "" && os.Getenv("PI_SESSION_FILE") == "" {
		return core.Capabilities{}, nil
	}
	return core.Capabilities{
		MCP:               true,
		SessionStartHook:  true,
		SessionEndHook:    true,
		PreCompactionHook: true,
		TranscriptCapture: os.Getenv("PI_SESSION_FILE") != "",
	}, nil
}

// Install is intentionally a no-op. Pi lifecycle wiring is owned by the
// user's existing hooks extension; Dossier only detects and consumes it.
func (p *PiHarness) Install(core.InstallOpts) error { return nil }