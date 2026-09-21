package core

import (
	"context"
	"errors"
	"fmt"
)

// CapabilityState is how a capability should be *reported*, which is not the
// same question as whether the boolean is true.
//
// "unavailable" means Dossier wanted this and did not get it: something is
// missing and the user can act on it. A capability Dossier deliberately does
// not use for a harness is a different statement entirely, and printing it as
// "unavailable" reads as a broken install — the user asks what is wrong and
// what they have lost, when the honest answer is "nothing, this is by design".
type CapabilityState string

const (
	CapabilityAvailable   CapabilityState = "available"
	CapabilityUnavailable CapabilityState = "unavailable"
	// CapabilityNotApplicable: not part of this harness's integration by
	// design. Always carries a note saying what covers the same ground.
	CapabilityNotApplicable CapabilityState = "not applicable"
)

// CapabilityStatus is one capability as it should be presented to a user.
type CapabilityStatus struct {
	State CapabilityState `json:"state"`
	Note  string          `json:"note,omitempty"`
}

// HarnessReport is the per-harness detection result surfaced by init, doctor and
// `dossier harness`.
type HarnessReport struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Detected    bool   `json:"detected"`
	// Capabilities is the raw boolean map. Retained as the machine-readable
	// answer; CapabilityStatuses is the one to render to a person.
	Capabilities       map[string]bool             `json:"capabilities"`
	CapabilityStatuses map[string]CapabilityStatus `json:"capability_statuses,omitempty"`
	// IntegrationComplete reports that Dossier has everything it needs from this
	// harness — every capability is either available or not applicable by
	// design. It is what lets a surface say so plainly instead of leaving the
	// user to infer it from a list containing the word "unavailable".
	IntegrationComplete bool     `json:"integration_complete"`
	Notes               []string `json:"notes,omitempty"`
}

// capabilityStatuses renders the capability booleans as reportable statuses,
// applying the per-harness knowledge of what Dossier actually uses.
func capabilityStatuses(name string, caps Capabilities) map[string]CapabilityStatus {
	state := func(ok bool) CapabilityStatus {
		if ok {
			return CapabilityStatus{State: CapabilityAvailable}
		}
		return CapabilityStatus{State: CapabilityUnavailable}
	}
	statuses := map[string]CapabilityStatus{
		"SessionIdentity":   state(caps.SessionIdentity),
		"MCP":               state(caps.MCP),
		"SessionStartHook":  state(caps.SessionStartHook),
		"SessionEndHook":    state(caps.SessionEndHook),
		"PreCompactionHook": state(caps.PreCompactionHook),
		"TranscriptCapture": state(caps.TranscriptCapture),
	}

	// Pi ships no MCP client, so Dossier drives Pi through its CLI instead —
	// the same operations, a different transport. Nothing is missing and there
	// is nothing for the user to install (B2, ADR 0009).
	if name == "pi" && !caps.MCP {
		statuses["MCP"] = CapabilityStatus{
			State: CapabilityNotApplicable,
			Note:  "Pi has no MCP client; Dossier drives Pi through its CLI instead, which covers the same operations",
		}
	}
	return statuses
}

// integrationComplete reports whether every capability is either available or
// not applicable by design — i.e. nothing is missing that the user could fix.
func integrationComplete(statuses map[string]CapabilityStatus) bool {
	for _, st := range statuses {
		if st.State == CapabilityUnavailable {
			return false
		}
	}
	return true
}

func newHarnessReport(name string, caps Capabilities) HarnessReport {
	statuses := capabilityStatuses(name, caps)
	return HarnessReport{
		Name:                name,
		DisplayName:         displayHarnessName(name),
		Detected:            caps.Present(),
		Capabilities:        capabilityMap(caps),
		CapabilityStatuses:  statuses,
		IntegrationComplete: caps.Present() && integrationComplete(statuses),
	}
}

func capabilityMap(caps Capabilities) map[string]bool {
	return map[string]bool{
		"MCP":               caps.MCP,
		"SessionStartHook":  caps.SessionStartHook,
		"SessionEndHook":    caps.SessionEndHook,
		"PreCompactionHook": caps.PreCompactionHook,
		"TranscriptCapture": caps.TranscriptCapture,
		"Installed":         caps.Installed,
		"SessionIdentity":   caps.SessionIdentity,
	}
}

// primaryHarnessCapabilities picks the capability map that best answers "what
// does this machine have": the first live session, else the first detected
// harness, else an all-false map.
func primaryHarnessCapabilities(reports []HarnessReport) map[string]bool {
	for _, r := range reports {
		if r.Capabilities["MCP"] || r.Capabilities["SessionStartHook"] || r.Capabilities["SessionEndHook"] ||
			r.Capabilities["PreCompactionHook"] || r.Capabilities["TranscriptCapture"] {
			return r.Capabilities
		}
	}
	for _, r := range reports {
		if r.Detected {
			return r.Capabilities
		}
	}
	return capabilityMap(Capabilities{})
}

// harnessAdvisories names what a detected harness is still missing, so a partial
// integration degrades visibly instead of looking like Dossier losing state.
func harnessAdvisories(name string, caps Capabilities) []string {
	var notes []string
	if caps.Installed && !caps.SessionIdentity {
		notes = append(notes, fmt.Sprintf(
			"%s is installed but cannot give Dossier a session id yet; run `dossier harness install %s` (and restart %s) to install the session bridge.",
			displayHarnessName(name), name, displayHarnessName(name)))
	} else if caps.Installed && !caps.SessionStartHook && !caps.SessionEndHook && !caps.PreCompactionHook {
		// Identity resolves but nothing calls Dossier at a session boundary. The
		// failure mode is quiet and easy to misread as Dossier losing state, so
		// it has to be named separately: the session works, but nothing is
		// captured when it ends or compacts.
		notes = append(notes, fmt.Sprintf(
			"%s can identify sessions but its lifecycle is not bridged; nothing saves state at session end or before compaction. Run `dossier harness install %s` (and restart %s).",
			displayHarnessName(name), name, displayHarnessName(name)))
	}
	return notes
}

// displayHarnessName maps a harness identifier to its human-readable label.
func displayHarnessName(name string) string {
	switch name {
	case "claude-code":
		return "Claude Code"
	case "pi":
		return "Pi"
	case "cursor":
		return "Cursor"
	case "codex":
		return "Codex"
	case "antigravity":
		return "Antigravity"
	default:
		return name
	}
}

// activeHarness resolves the harness owning the current process, plus its
// capabilities. Prefer this over scanning hreg.All() for LiveSession(): the
// registry order is not evidence of which harness a session is running under,
// and Detect() is device-level (see ActiveHarnessResolver).
//
// The fallback keeps the pre-port behaviour for registries that cannot answer —
// first harness offering a live session surface — because reporting the wrong
// harness is still better than reporting none for a single-harness machine,
// which is the only shape where the fallback is reliable.
func (s *Service) activeHarness() (Harness, Capabilities) {
	if s.hreg == nil {
		return nil, Capabilities{}
	}
	if r, ok := s.hreg.(ActiveHarnessResolver); ok {
		if h, caps, found := r.ActiveHarness(); found {
			return h, caps
		}
		return nil, Capabilities{}
	}
	for _, h := range s.hreg.All() {
		if caps, err := h.Detect(); err == nil && caps.LiveSession() {
			return h, caps
		}
	}
	return nil, Capabilities{}
}

// sessionHarness resolves the harness a session belongs to: the one the adapter
// named, when it is present on this device, else whichever harness owns the
// current process.
func (s *Service) sessionHarness(name string) (Harness, Capabilities) {
	if name != "" {
		if h, err := s.hreg.Get(name); err == nil && h != nil {
			if caps, err := h.Detect(); err == nil && caps.Present() {
				return h, caps
			}
		}
		// An explicit harness name is authoritative. Do not silently attribute
		// a newly launched session to another live harness on the same machine.
		return nil, Capabilities{}
	}
	return s.activeHarness()
}

// HarnessStatus reports detection for every supported harness without changing
// anything on disk.
func (s *Service) HarnessStatus(ctx context.Context) (Result, error) {
	var reports []HarnessReport
	var warnings []Warning
	for _, h := range s.hreg.All() {
		caps, err := h.Detect()
		if err != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Failed to detect %s: %v", h.Name(), err)))
			continue
		}
		report := newHarnessReport(h.Name(), caps)
		if caps.Present() {
			report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)
		}
		reports = append(reports, report)
	}
	return Result{OK: true, Data: reports, Warnings: warnings}, nil
}

// InstallHarnessReq installs one harness integration by name — the path for a
// user who adds a harness (typically Pi) after running init.
type InstallHarnessReq struct {
	Name             string
	YesToAll         bool
	StableBinaryPath string
}

// InstallHarness installs the integration for a single harness.
func (s *Service) InstallHarness(ctx context.Context, req InstallHarnessReq) (Result, error) {
	h, err := s.hreg.Get(req.Name)
	if err != nil || h == nil {
		return Result{OK: false}, NewError(ErrNotFound, fmt.Sprintf("unknown harness %q", req.Name))
	}

	caps, err := h.Detect()
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, fmt.Sprintf("failed to detect %s", req.Name), err)
	}
	if !caps.Present() {
		return Result{OK: false}, NewError(ErrHarnessCapabilityUnavailable,
			fmt.Sprintf("%s was not found on this device; install it first, then re-run this command", displayHarnessName(req.Name)))
	}

	stablePath := req.StableBinaryPath
	if stablePath == "" {
		stablePath = "dossier"
	}
	installErr := h.Install(InstallOpts{
		Interactive:      !req.YesToAll,
		YesToAll:         req.YesToAll,
		StableBinaryPath: stablePath,
	})

	if installErr != nil {
		if errors.Is(installErr, ErrInstallSkipped) {
			if postCaps, err := h.Detect(); err == nil {
				caps = postCaps
			}
			report := newHarnessReport(h.Name(), caps)
			report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)
			var warnings []Warning
			for _, note := range report.Notes {
				warnings = append(warnings, Warning(note))
			}
			warnings = append(warnings, Warning(installErr.Error()))
			return Result{OK: false, Data: report, Warnings: warnings}, nil
		}
		return Result{OK: false}, WrapError(ErrInternal, fmt.Sprintf("failed to install %s integration", req.Name), installErr)
	}

	if postCaps, err := h.Detect(); err == nil {
		caps = postCaps
	}
	report := newHarnessReport(h.Name(), caps)
	report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)

	if adv, ok := h.(PostInstallAdvisor); ok {
		report.Notes = append(report.Notes, adv.PostInstallNotes()...)
	}

	var warnings []Warning
	for _, note := range report.Notes {
		warnings = append(warnings, Warning(note))
	}

	return Result{OK: true, Data: report, Warnings: warnings}, nil
}

// UninstallHarnessReq removes one harness integration by name.
type UninstallHarnessReq struct {
	Name     string
	YesToAll bool
}

// UninstallHarness removes the integration for a single harness. It does not
// require the client itself to still be installed: stale Dossier config is a
// valid reason to run this command.
func (s *Service) UninstallHarness(ctx context.Context, req UninstallHarnessReq) (Result, error) {
	h, err := s.hreg.Get(req.Name)
	if err != nil || h == nil {
		return Result{OK: false}, NewError(ErrNotFound, fmt.Sprintf("unknown harness %q", req.Name))
	}

	if err := h.Uninstall(InstallOpts{Interactive: !req.YesToAll, YesToAll: req.YesToAll}); err != nil {
		if errors.Is(err, ErrUninstallSkipped) {
			return Result{OK: false, Warnings: []Warning{Warning(err.Error())}}, nil
		}
		return Result{OK: false}, WrapError(ErrInternal, fmt.Sprintf("failed to uninstall %s integration", req.Name), err)
	}

	caps, detectErr := h.Detect()
	if detectErr != nil {
		return Result{OK: true, Warnings: []Warning{Warning(fmt.Sprintf("uninstalled %s integration, but could not verify it: %v", req.Name, detectErr))}}, nil
	}
	return Result{OK: true, Data: newHarnessReport(h.Name(), caps)}, nil
}
