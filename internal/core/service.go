package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DefaultTokenLimit is the default warning ceiling for Distilled State tokens.
const DefaultTokenLimit = 100000

// Config holds the service-level configurations used by the core logic.
type Config struct {
	DossierHome string
	Author      string
	Interfaces  []string
	Leads       []string
	TokenLimit  int
}

// Service orchestrates Dossier domain use-cases over the port interfaces.
// It contains zero business logic leakages to driving adapters (CLI/MCP/TUI).
type Service struct {
	store  Store
	search Searcher
	tok    Tokenizer
	hreg   HarnessRegistry
	clock  Clock
	cfg    Config
	syncer Syncer
}

// RecallResult carries the output fields for dossier recall queries.
type RecallResult struct {
	DistilledState string      `json:"distilled_state"`
	Frontmatter    Frontmatter `json:"frontmatter"`
	Revision       Revision    `json:"revision"`
	TokenEstimate  int         `json:"token_estimate"`
	Path           string      `json:"path"`
	// Artifacts is the evidence index: one entry per archived artifact, with
	// the line count that bounds a citable range and whether the Distilled
	// State currently cites it. Recall previously returned the curated view
	// alone, which left the Archive invisible to the caller that has to decide
	// what to cite.
	Artifacts []ArtifactSummary `json:"artifacts,omitempty"`
}

// ArtifactSummary is one entry in the evidence index.
type ArtifactSummary struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Title         string    `json:"title"`
	ContentFormat string    `json:"content_format"`
	Lines         int       `json:"lines"`
	CapturedAt    time.Time `json:"captured_at"`
	Origin        string    `json:"origin,omitempty"`
	URL           string    `json:"url,omitempty"`
	Cited         bool      `json:"cited"`
}

// ListItem represents a single summary item for dossier listings.
type ListItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Status      string   `json:"status"`
	Lead        string   `json:"lead,omitempty"`
	Interfaces  []string `json:"interfaces,omitempty"`
	NextAction  string   `json:"next_action"`
	Description string   `json:"description,omitempty"`
	Priority    string   `json:"priority"`
	DueDate     string   `json:"due_date,omitempty"`
	Path        string   `json:"path"`
}

type SyncStatusData struct {
	Ahead          int       `json:"ahead"`
	Behind         int       `json:"behind"`
	LastSync       time.Time `json:"last_sync"`
	Dirty          int       `json:"dirty"`
	ConflictsFound int       `json:"conflicts_found"`
}

// DoctorReport summarizes integrity checks run by Doctor.
type DoctorReport struct {
	DossiersChecked  int             `json:"dossiers_checked"`
	ArtifactsChecked int             `json:"artifacts_checked"`
	AuditLogsChecked int             `json:"audit_logs_checked"`
	ConflictsFound   int             `json:"conflicts_found"`
	Issues           []string        `json:"issues,omitempty"`
	SyncConfigured   bool            `json:"sync_configured"`
	SyncStatus       *SyncStatusData `json:"sync_status,omitempty"`
}

// NewService instantiates the core orchestration service.
func NewService(store Store, search Searcher, tok Tokenizer, hreg HarnessRegistry, clock Clock, cfg Config, syncer Syncer) *Service {
	if cfg.Interfaces == nil {
		cfg.Interfaces = DefaultDiscussionInterfaces()
	} else {
		cfg.Interfaces = append([]string{}, cfg.Interfaces...)
	}
	cfg.Leads = append([]string{}, cfg.Leads...)
	if cfg.TokenLimit <= 0 {
		cfg.TokenLimit = DefaultTokenLimit
	}
	return &Service{
		store:  store,
		search: search,
		tok:    tok,
		hreg:   hreg,
		clock:  clock,
		cfg:    cfg,
		syncer: syncer,
	}
}

// Interfaces returns the configured discussion-interface vocabulary in display order.
func (s *Service) Interfaces() []string {
	return append([]string{}, s.cfg.Interfaces...)
}

// Leads returns the configured lead vocabulary in display order. An empty list
// preserves free-form lead assignment for backwards compatibility.
func (s *Service) Leads() []string {
	return append([]string{}, s.cfg.Leads...)
}

// TokenLimit returns the configured token limit for distilled state.
func (s *Service) TokenLimit() int {
	if s.cfg.TokenLimit <= 0 {
		return DefaultTokenLimit
	}
	return s.cfg.TokenLimit
}

// InitReq represents the request parameters for service initialization.
type InitReq struct {
	YesToAll         bool
	StableBinaryPath string
}

// Init initializes the store directories, writes default configs and guide.
func (s *Service) Init(ctx context.Context, req InitReq) (Result, error) {
	// For Milestone 1 baseline, we delegate to the store's Init method.
	if err := s.store.Init(); err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to initialize local store", err)
	}

	warnings := []Warning{}
	data := make(map[string]any)

	stablePath := req.StableBinaryPath
	if stablePath == "" {
		stablePath = "dossier"
	}

	// Detect every supported harness, install into the ones present on this
	// device, and report each one separately — a single merged capability map
	// would let the last harness scanned speak for all of them.
	var reports []HarnessReport
	harnessDetected := false
	for _, h := range s.hreg.All() {
		caps, err := h.Detect()
		if err != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Failed to detect %s: %v", h.Name(), err)))
			continue
		}
		if !caps.Present() {
			reports = append(reports, newHarnessReport(h.Name(), caps))
			continue
		}
		harnessDetected = true

		installErr := h.Install(InstallOpts{
			Interactive:      !req.YesToAll,
			YesToAll:         req.YesToAll,
			StableBinaryPath: stablePath,
		})
		if installErr != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Failed to install for %s: %v", h.Name(), installErr)))
		}

		// Re-detect: installing an integration is what turns a capability on
		// (the Pi extension supplies session identity), so the pre-install
		// snapshot would understate what the user now has.
		if postCaps, err := h.Detect(); err == nil {
			caps = postCaps
		}
		report := newHarnessReport(h.Name(), caps)
		report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)
		reports = append(reports, report)
	}

	data["harness_detected"] = harnessDetected
	data["harnesses"] = reports
	// Retained for callers that only ever asked about the primary harness.
	data["harness_capabilities"] = primaryHarnessCapabilities(reports)

	for _, r := range reports {
		for _, note := range r.Notes {
			warnings = append(warnings, Warning(note))
		}
	}

	return Result{
		OK:       true,
		Data:     data,
		Warnings: warnings,
	}, nil
}

// HarnessReport is the per-harness detection result surfaced by init, doctor and
// `dossier harness`.
type HarnessReport struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Detected     bool            `json:"detected"`
	Capabilities map[string]bool `json:"capabilities"`
	Notes        []string        `json:"notes,omitempty"`
}

func newHarnessReport(name string, caps Capabilities) HarnessReport {
	return HarnessReport{
		Name:         name,
		DisplayName:  displayHarnessName(name),
		Detected:     caps.Present(),
		Capabilities: capabilityMap(caps),
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

// sessionHarness resolves the harness a session belongs to: the one the adapter
// named, when it is present on this device, else the first harness offering a
// live session surface.
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
	for _, h := range s.hreg.All() {
		if caps, err := h.Detect(); err == nil && caps.LiveSession() {
			return h, caps
		}
	}
	return nil, Capabilities{}
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
	if err := h.Install(InstallOpts{
		Interactive:      !req.YesToAll,
		YesToAll:         req.YesToAll,
		StableBinaryPath: stablePath,
	}); err != nil {
		return Result{OK: false}, WrapError(ErrInternal, fmt.Sprintf("failed to install %s integration", req.Name), err)
	}

	if postCaps, err := h.Detect(); err == nil {
		caps = postCaps
	}
	report := newHarnessReport(h.Name(), caps)
	report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)

	var warnings []Warning
	for _, note := range report.Notes {
		warnings = append(warnings, Warning(note))
	}

	return Result{OK: true, Data: report, Warnings: warnings}, nil
}

// Doctor validates store integrity and configuration correctness.
func (s *Service) Doctor(ctx context.Context) (Result, error) {
	if s.store == nil {
		return Result{OK: false}, NewError(ErrInternal, "store not configured")
	}

	report := DoctorReport{}
	var warnings []Warning
	// Advisories are surfaced but do not fail the check: an integration the user
	// has not installed yet is worth saying out loud, and is not store damage.
	addAdvisory := func(msg string) {
		warnings = append(warnings, Warning(msg))
	}
	addIssue := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		report.Issues = append(report.Issues, msg)
		warnings = append(warnings, Warning(msg))
	}

	fms, err := s.store.List("all")
	if err != nil {
		addIssue("Failed to list dossiers: %v", err)
		return Result{OK: false, Data: report, Warnings: warnings}, nil
	}

	for _, fm := range fms {
		report.DossiersChecked++
		if err := fm.Validate(); err != nil {
			addIssue("Dossier %s has invalid frontmatter: %v", fm.ID, err)
		}

		d, _, err := s.store.Read(fm.ID)
		if err != nil {
			addIssue("Dossier %s could not be read: %v", fm.ID, err)
			continue
		}

		artifacts, err := s.store.ListArtifacts(fm.ID)
		if err != nil {
			addIssue("Dossier %s artifacts could not be listed: %v", fm.ID, err)
		}
		artifactLines := make(map[string]int, len(artifacts))
		for _, art := range artifacts {
			report.ArtifactsChecked++
			if err := art.Validate(); err != nil {
				addIssue("Dossier %s artifact %s is invalid: %v", fm.ID, art.ID, err)
			}
			if strings.TrimSpace(art.Provenance.Origin) == "" {
				addIssue("Dossier %s artifact %s is missing provenance.origin", fm.ID, art.ID)
			}
			lineCount := art.Lines
			// In-memory/third-party Store implementations may still return a body.
			// The filesystem store returns metadata only and stream-populates Lines
			// for legacy artifacts, so Doctor never has to load huge bodies.
			if lineCount == 0 && art.Content != "" {
				lineCount = artifactLineCount(art.Content)
			}
			artifactLines[art.ID] = lineCount
		}

		for _, issue := range validateDistilledStateProvenance(d.DistilledState.Body, fm.ID, func(artifactID string) (int, bool) {
			lineCount, ok := artifactLines[artifactID]
			return lineCount, ok
		}) {
			addIssue("%s", issue)
		}

		// Advisory, not damage: uncited evidence is a thin-distillation signal.
		if msg := uncitedArtifactWarning(d.DistilledState.Body, artifacts); msg != "" {
			addAdvisory(fmt.Sprintf("Dossier %s: %s", fm.ID, msg))
		}

		for _, issue := range s.store.ValidateArtifactFiles(fm.ID) {
			addIssue("%s", issue)
		}

		for _, issue := range s.store.ValidateAuditShards(fm.ID) {
			addIssue("%s", issue)
		}

		if _, err := s.store.ReadAuditLog(fm.ID); err != nil {
			addIssue("Dossier %s audit log is not readable: %v", fm.ID, err)
		} else {
			report.AuditLogsChecked++
		}
	}

	conflicts, err := s.store.ListConflicts()
	if err != nil {
		addIssue("Conflicts could not be listed: %v", err)
	} else {
		report.ConflictsFound = len(conflicts)
		for _, c := range conflicts {
			addIssue("Unresolved conflict %s for dossier %s", c.ID, c.DossierID)
		}
	}

	// The context assets carry the rules the agent distills by, so a store copy
	// that has drifted from this binary's is worth saying out loud even though
	// it is not store damage — and even though wiring refreshes it, since a
	// read-only or otherwise unwritable context directory would leave it stale.
	if stale := s.store.StaleContextAssets(); len(stale) > 0 {
		addAdvisory(fmt.Sprintf(
			"Context assets differ from this binary's embedded copies: %s. The embedded version is authoritative and is used regardless; run `dossier init` to refresh the readable copies under context/.",
			strings.Join(stale, ", "),
		))
	}

	if s.hreg != nil {
		for _, h := range s.hreg.All() {
			caps, err := h.Detect()
			if err != nil {
				addIssue("Harness %s could not be detected: %v", h.Name(), err)
				continue
			}
			for _, note := range harnessAdvisories(h.Name(), caps) {
				addAdvisory(note)
			}
		}
	}

	if s.syncer != nil {
		report.SyncConfigured = true
		status, err := s.syncer.Status(ctx)
		if err != nil {
			addIssue("Failed to get sync status: %v", err)
		} else {
			report.SyncStatus = &SyncStatusData{
				Ahead:          status.Ahead,
				Behind:         status.Behind,
				LastSync:       status.LastSync,
				Dirty:          status.Dirty,
				ConflictsFound: len(status.Conflicts),
			}
		}
	}

	return Result{
		OK:       len(report.Issues) == 0,
		Data:     report,
		Warnings: warnings,
	}, nil
}

// TeamCreateReq specifies parameters for creating a new team store.
type TeamCreateReq struct {
	RemoteURL string
	Branch    string
}

// TeamCreate initializes the current store as a team store and pushes to the remote.
func (s *Service) TeamCreate(ctx context.Context, req TeamCreateReq) (Result, error) {
	if req.RemoteURL == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "remote URL is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if s.syncer == nil {
		return Result{}, NewError(ErrInternal, "syncer is not configured")
	}

	err := s.syncer.Create(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "already a team store") {
			return Result{}, NewError(ErrConflictDetected, "store is already a team store")
		}
		return Result{}, fmt.Errorf("team create failed: %w", err)
	}

	return Result{OK: true}, nil
}

// TeamJoinReq specifies parameters for joining an existing team store.
type TeamJoinReq struct {
	RemoteURL string
	Branch    string
}

// TeamJoin joins an existing team store by cloning it locally.
func (s *Service) TeamJoin(ctx context.Context, req TeamJoinReq) (Result, error) {
	if req.RemoteURL == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "remote URL is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if s.syncer == nil {
		return Result{}, NewError(ErrInternal, "syncer is not configured")
	}

	err := s.syncer.Clone(ctx, req.RemoteURL, s.cfg.DossierHome, 50)
	if err != nil {
		if strings.Contains(err.Error(), "target directory is not empty") {
			return Result{}, NewError(ErrConflictDetected, "target directory is not empty; cannot join into an existing store")
		}
		return Result{}, fmt.Errorf("team join failed: %w", err)
	}

	initRes, initErr := s.Init(ctx, InitReq{})
	if initErr != nil {
		return Result{}, fmt.Errorf("post-join init failed: %w", initErr)
	}

	return Result{OK: true, Warnings: initRes.Warnings}, nil
}

// Stubs for future milestones

type PromoteReq struct {
	Name                   string
	Description            string
	Priority               Priority
	DistilledStateMarkdown string
	FromFilePath           string
	Content                string
	Lead                   string
	Interfaces             []string
	Force                  bool
}

func (s *Service) Promote(ctx context.Context, req PromoteReq) (Result, error) {
	if req.Name == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "dossier name is required")
	}

	now := s.clock.Now()

	if !req.Force {
		fms, err := s.store.List("all")
		if err == nil {
			var candidates []Suggestion
			for _, fm := range fms {
				d, _, err := s.store.Read(fm.ID)
				if err != nil {
					continue
				}
				sug := ScoreDossier(req.Name, d, now)
				if sug.Confidence == "high" || sug.Confidence == "medium" {
					candidates = append(candidates, sug)
				}
			}

			if len(candidates) > 0 {
				sort.Slice(candidates, func(i, j int) bool {
					return candidates[i].Score > candidates[j].Score
				})
				if len(candidates) > 3 {
					candidates = candidates[:3]
				}

				// Attempt OS dialog prompt
				var dialogOverrodeForce bool
				if runtime.GOOS == "darwin" && !strings.HasSuffix(os.Args[0], ".test") {
					var names []string
					for _, c := range candidates {
						names = append(names, fmt.Sprintf("%q", c.Name))
					}
					promptMsg := fmt.Sprintf("Found related topics: %s. Create a new thread anyway?", strings.Join(names, ", "))
					cmd := exec.Command("osascript", "-e", fmt.Sprintf(`button returned of (display dialog %q buttons {"Yes", "No"} default button "No" with title "Dossier Verification")`, promptMsg))
					if out, err := cmd.Output(); err == nil {
						if strings.TrimSpace(string(out)) == "Yes" {
							req.Force = true
							dialogOverrodeForce = true
						} else {
							return Result{
								OK:   false,
								Data: candidates,
								NextActions: []NextAction{
									`The user REJECTED the creation of a new thread via system dialog. You MUST link your work to one of the candidates provided.`,
								},
							}, NewError(ErrAmbiguousTarget, "User explicitly rejected creation. MUST link to existing dossier.")
						}
					}
				}

				if !dialogOverrodeForce {
					return Result{
						OK:   false,
						Data: candidates,
						NextActions: []NextAction{
							`Present the candidates to the user: "I found Dossiers that look related — [for each: Name (status, N days since last update)]. Is one of these the right one to continue, or is this a separate thread?"`,
							`If the user picks one: call dossier_session with its slug to bind it, then dossier_recall to load its state.`,
							`If the user confirms this is a new topic: call dossier_promote again with force=true.`,
						},
					}, NewError(ErrAmbiguousTarget, "Multiple likely Dossiers match this promote request.")
				}
			}
		}
	}

	updates := map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"lead":        req.Lead,
		"interfaces":  req.Interfaces,
	}
	if req.Priority != "" {
		updates["priority"] = string(req.Priority)
	}
	saveRes, err := s.Save(ctx, SaveReq{
		DistilledStateMarkdown: req.DistilledStateMarkdown,
		FrontmatterUpdates:     updates,
	})
	if err != nil {
		return Result{}, err
	}

	newRevision := saveRes.Data.(Revision)
	fms, err := s.store.List("all")
	if err != nil {
		return Result{}, fmt.Errorf("locate promoted dossier for artifact capture: %w", err)
	}
	var newID string
	for _, fm := range fms {
		if fm.Name != req.Name {
			continue
		}
		_, rev, readErr := s.store.Read(fm.ID)
		if readErr != nil {
			return Result{}, fmt.Errorf("read promoted dossier candidate %s: %w", fm.ID, readErr)
		}
		// Force-promote permits duplicate display names. The just-created
		// revision, unlike the name, identifies the dossier that must receive
		// this transcript.
		if rev == newRevision {
			newID = fm.ID
			break
		}
	}
	if newID == "" {
		return Result{}, NewError(ErrInternal, "could not locate the newly promoted dossier for artifact capture")
	}

	var warnings []Warning
	if req.Content != "" && newID != "" {
		compiled, compiledFormat, compileWarnings := CompileTranscript(req.Content)
		warnings = append(warnings, compileWarnings...)

		// Compilation is a view, never a replacement for source. If it changes
		// the supplied bytes, archive the raw JSONL first so a later compiled or
		// audit write failure still cannot destroy the only lossless copy.
		if compiled != req.Content {
			raw := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Raw Captured Session Transcript (JSONL)",
				Provenance:    Provenance{Origin: "promote session content (raw JSONL, byte-preserved)"},
				ContentFormat: ContentFormatText,
				Content:       req.Content,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			var writeErr error
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &raw,
				"Archived byte-preserved raw promote session transcript before compilation.")
			if writeErr != nil {
				return Result{}, writeErr
			}

			compiledArt := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Compiled Captured Session Transcript",
				Provenance:    Provenance{Origin: fmt.Sprintf("promote session content (compiled citable view of %s)", raw.ID)},
				ContentFormat: compiledFormat,
				Content:       compiled,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &compiledArt,
				fmt.Sprintf("Archived compiled citable transcript view derived from raw artifact %s.", raw.ID))
			if writeErr != nil {
				return Result{}, writeErr
			}
		} else {
			art := Artifact{
				DossierID:     newID,
				Type:          ArtifactTypeTranscript,
				Title:         "Captured Session Transcript",
				Provenance:    Provenance{Origin: "promote session content (verbatim plain-text passthrough)"},
				ContentFormat: compiledFormat,
				Content:       compiled,
				CapturedAt:    now,
				RefreshedAt:   now,
			}
			var writeErr error
			newRevision, writeErr = s.writePromoteTranscriptArtifact(newID, newRevision, &art,
				"Archived verbatim plain-text promote session transcript.")
			if writeErr != nil {
				return Result{}, writeErr
			}
		}
	}

	harnesses := s.hreg.All()
	var activeHarness Harness
	for _, h := range harnesses {
		caps, err := h.Detect()
		if err == nil && (caps.MCP || caps.SessionStartHook || caps.SessionEndHook || caps.PreCompactionHook || caps.TranscriptCapture) {
			activeHarness = h
			if !caps.TranscriptCapture {
				warnings = append(warnings, Warning("Transcript archive is unavailable in this session."))
			}
			break
		}
	}
	if activeHarness == nil {
		warnings = append(warnings, Warning("Transcript archive is unavailable in this session."))
	}

	return Result{
		OK:       true,
		Data:     newID,
		Warnings: warnings,
	}, nil
}

// writePromoteTranscriptArtifact persists and audits one promote capture. Each
// source/view write is audited independently so a partial failure remains
// legible, and every store error is returned rather than silently ignored.
func (s *Service) writePromoteTranscriptArtifact(dossierID string, before Revision, art *Artifact, message string) (Revision, error) {
	if err := s.store.WriteArtifact(dossierID, art); err != nil {
		return before, fmt.Errorf("archive promote transcript %q: %w", art.Title, err)
	}
	_, after, err := s.store.Read(dossierID)
	if err != nil {
		return before, fmt.Errorf("read revision after archiving promote transcript %s: %w", art.ID, err)
	}
	if err := s.store.AppendAudit(dossierID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventSave,
		Author:         s.cfg.Author,
		DossierID:      dossierID,
		BeforeRevision: string(before),
		AfterRevision:  string(after),
		ArtifactsAdded: []string{art.ID},
		Message:        message,
	}); err != nil {
		return after, fmt.Errorf("audit promote transcript artifact %s: %w", art.ID, err)
	}
	return after, nil
}

type SaveReq struct {
	ID                     string
	BaseRevision           Revision
	DistilledStateMarkdown string
	FrontmatterUpdates     map[string]any
	Artifacts              []Artifact
	SessionID              string
}

// GenerateUnifiedDiff produces a line-by-line diff of two strings using LCS.
func GenerateUnifiedDiff(a, b string) string {
	aLines := strings.Split(strings.ReplaceAll(a, "\r\n", "\n"), "\n")
	bLines := strings.Split(strings.ReplaceAll(b, "\r\n", "\n"), "\n")

	n := len(aLines)
	m := len(bLines)

	if n*m > 10000000 {
		return fmt.Sprintf("--- Diff too large to compute for files of %d and %d lines ---\n\n(See Rejected Proposal for the attempted body)", n, m)
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if aLines[i-1] == bLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	var diff []string
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && aLines[i-1] == bLines[j-1] {
			diff = append(diff, "  "+aLines[i-1])
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			diff = append(diff, "+ "+bLines[j-1])
			j--
		} else if i > 0 && (j == 0 || dp[i-1][j] >= dp[i][j-1]) {
			diff = append(diff, "- "+aLines[i-1])
			i--
		}
	}

	for l, r := 0, len(diff)-1; l < r; l, r = l+1, r-1 {
		diff[l], diff[r] = diff[r], diff[l]
	}

	return strings.Join(diff, "\n")
}

func getFMField(fm Frontmatter, field string) any {
	switch field {
	case "name":
		return fm.Name
	case "description":
		return fm.Description
	case "status":
		return string(fm.Status)
	case "lead":
		return fm.Lead
	case "interfaces":
		return strings.Join(fm.Interfaces, "|||")
	case "next_action":
		return fm.NextAction
	case "priority":
		return string(fm.Priority)
	case "due_date":
		return fm.DueDate
	default:
		return nil
	}
}

func applyFrontmatterUpdates(d *Dossier, updates map[string]any) {
	if val, ok := updates["name"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Name = strVal
		}
	}
	if val, ok := updates["description"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Description = strVal
		}
	}
	if val, ok := updates["status"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Status = NormalizeStatus(Status(strVal))
		}
	}
	if val, ok := updates["lead"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Lead = strVal
		}
	}
	if val, ok := updates["interfaces"]; ok {
		d.Frontmatter.Interfaces = interfaceNamesFromValue(val)
	}
	if val, ok := updates["next_action"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.NextAction = strVal
		}
	}
	if val, ok := updates["priority"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Priority = Priority(strVal)
		}
	}
	if val, ok := updates["due_date"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.DueDate = strVal
		}
	}
}

func interfaceNamesFromValue(value any) []string {
	var names []string
	switch values := value.(type) {
	case []string:
		names = append(names, values...)
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func configuredValueAllowed(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func strictStringSlice(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return append([]string{}, values...), true
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, name)
		}
		return result, true
	default:
		return nil, false
	}
}

func (s *Service) validateConfiguredFrontmatterUpdates(updates map[string]any) error {
	if updates == nil {
		return nil
	}
	if value, ok := updates["interfaces"]; ok {
		names, valid := strictStringSlice(value)
		if !valid {
			return fmt.Errorf("interfaces must be a list of strings")
		}
		for _, name := range names {
			if !configuredValueAllowed(name, s.cfg.Interfaces) {
				return fmt.Errorf("invalid interface: %q (configure available values in config.yaml)", name)
			}
		}
	}
	if value, ok := updates["lead"]; ok {
		lead, valid := value.(string)
		if !valid {
			return fmt.Errorf("lead must be a string")
		}
		if lead != "" && len(s.cfg.Leads) > 0 && !configuredValueAllowed(lead, s.cfg.Leads) {
			return fmt.Errorf("invalid lead: %q (configure available values in config.yaml)", lead)
		}
	}
	return nil
}

// describeFrontmatterChanges returns a human-readable, audit-friendly summary of
// which frontmatter fields changed between before and after.
func describeFrontmatterChanges(before, after Frontmatter) string {
	var parts []string
	add := func(field, oldVal, newVal string) {
		if oldVal != newVal {
			parts = append(parts, fmt.Sprintf("%s %q→%q", field, oldVal, newVal))
		}
	}
	add("name", before.Name, after.Name)
	add("description", before.Description, after.Description)
	add("status", string(before.Status), string(after.Status))
	add("lead", before.Lead, after.Lead)
	if strings.Join(before.Interfaces, "|||") != strings.Join(after.Interfaces, "|||") {
		parts = append(parts, fmt.Sprintf("interfaces %q→%q", strings.Join(before.Interfaces, ", "), strings.Join(after.Interfaces, ", ")))
	}
	add("next_action", before.NextAction, after.NextAction)
	add("priority", string(before.Priority), string(after.Priority))
	add("due_date", before.DueDate, after.DueDate)
	return strings.Join(parts, "; ")
}

func (s *Service) Save(ctx context.Context, req SaveReq) (Result, error) {
	if _, ok := req.FrontmatterUpdates["slug"]; ok {
		return Result{}, NewError(ErrInvalidFrontmatter, "slug cannot be changed through Save; use Rename")
	}
	if _, ok := req.FrontmatterUpdates["aliases"]; ok {
		return Result{}, NewError(ErrInvalidFrontmatter, "slug aliases are not supported")
	}
	if err := s.validateConfiguredFrontmatterUpdates(req.FrontmatterUpdates); err != nil {
		return Result{}, WrapError(ErrInvalidFrontmatter, "invalid frontmatter details", err)
	}

	// An explicit priority is user input and must be one of the canonical values.
	if value, ok := req.FrontmatterUpdates["priority"]; ok {
		priority, validType := value.(string)
		if !validType || !Priority(priority).IsValid() {
			return Result{}, NewError(ErrInvalidFrontmatter, fmt.Sprintf("invalid priority: %q", priority))
		}
	}

	var d *Dossier
	var baseRev Revision
	var beforeFM Frontmatter
	var err error

	isNew := req.ID == ""
	sessID := req.SessionID
	if sessID == "" {
		sessID = "sess_default"
	}

	if isNew {
		d = &Dossier{
			Frontmatter: Frontmatter{
				Status:   StatusSpark,
				Priority: PriorityMedium,
			},
		}
	} else {
		d, baseRev, err = s.store.Read(req.ID)
		if err != nil {
			return Result{}, err
		}
		beforeFM = d.Frontmatter

		if req.BaseRevision != "" && baseRev != req.BaseRevision {
			// Concurrency mismatch! Attempt to read the dossier at the user's base revision.
			dBase, readRevErr := s.store.ReadRevision(req.ID, req.BaseRevision)
			hasConflict := false

			if readRevErr != nil {
				// Base revision not found, treat as conflict
				hasConflict = true
			} else {
				// Check for body conflict:
				// Did body change in store?
				bodyChangedInStore := (d.DistilledState.Body != dBase.DistilledState.Body)
				// Did user change body?
				userBodyChanged := (req.DistilledStateMarkdown != "" && req.DistilledStateMarkdown != dBase.DistilledState.Body)
				// Overlap conflict if both changed and proposed is different from store
				if bodyChangedInStore && userBodyChanged && (req.DistilledStateMarkdown != d.DistilledState.Body) {
					hasConflict = true
				}

				// Check for frontmatter conflict:
				if !hasConflict && req.FrontmatterUpdates != nil {
					for f, proposedVal := range req.FrontmatterUpdates {
						storeVal := getFMField(d.Frontmatter, f)
						baseVal := getFMField(dBase.Frontmatter, f)

						if storeVal != baseVal {
							var normProposedVal any = proposedVal
							if f == "status" || f == "priority" || f == "lead" || f == "description" {
								if sVal, ok := proposedVal.(string); ok {
									normProposedVal = sVal
								}
							}

							if normProposedVal != storeVal {
								hasConflict = true
								break
							}
						}
					}
				}
			}

			if hasConflict {
				confID := "conf_" + s.clock.Now().Format("20060102150405")
				proposedBody := req.DistilledStateMarkdown
				if proposedBody == "" {
					proposedBody = d.DistilledState.Body
				}

				diff := GenerateUnifiedDiff(d.DistilledState.Body, proposedBody)

				conflict := &Conflict{
					ID:                 confID,
					DossierID:          d.Frontmatter.ID,
					Kind:               "distilled_state_concurrent_edit",
					BaseRevision:       string(req.BaseRevision),
					AttemptedRevision:  string(baseRev),
					Session:            sessID,
					TS:                 s.clock.Now(),
					RejectedBody:       proposedBody,
					DiffAgainstCurrent: diff,
				}

				writeErr := s.store.WriteConflict(conflict)
				if writeErr == nil {
					_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
						TS:             s.clock.Now(),
						Event:          AuditEventConflictCreated,
						Author:         s.cfg.Author,
						DossierID:      d.Frontmatter.ID,
						SessionID:      sessID,
						BeforeRevision: string(req.BaseRevision),
						AfterRevision:  string(baseRev),
						Message:        fmt.Sprintf("Conflict %s created due to concurrent edit", conflict.ID),
					})
				}

				return Result{
					OK:   false,
					Data: conflict,
				}, NewError(ErrConcurrentEdit, fmt.Sprintf("concurrency mismatch: base is %q, current is %q. Conflict artifact %s created.", req.BaseRevision, baseRev, conflict.ID))
			}

			// Auto-merge non-overlapping changes!
			if req.DistilledStateMarkdown != "" && dBase != nil && req.DistilledStateMarkdown != dBase.DistilledState.Body {
				d.DistilledState.Body = req.DistilledStateMarkdown
			}
			if req.FrontmatterUpdates != nil {
				applyFrontmatterUpdates(d, req.FrontmatterUpdates)
			}
			// Write with the current revision as the base to succeed
		}
	}

	if req.FrontmatterUpdates != nil {
		applyFrontmatterUpdates(d, req.FrontmatterUpdates)
	}

	if req.DistilledStateMarkdown != "" {
		d.DistilledState.Body = req.DistilledStateMarkdown
	}

	if err := validateNextActionLength(d.Frontmatter.NextAction); err != nil {
		return Result{}, WrapError(ErrInvalidFrontmatter, "invalid frontmatter details", err)
	}

	var warnings []Warning
	newRev, err := s.store.Write(d, baseRev)
	if err != nil {
		return Result{}, err
	}

	var addedArtifactIDs []string
	for _, art := range req.Artifacts {
		art.DossierID = d.Frontmatter.ID
		if err := s.store.WriteArtifact(d.Frontmatter.ID, &art); err != nil {
			return Result{}, err
		}
		addedArtifactIDs = append(addedArtifactIDs, art.ID)
	}
	if len(addedArtifactIDs) > 0 {
		_, refreshedRev, err := s.store.Read(d.Frontmatter.ID)
		if err != nil {
			return Result{}, err
		}
		newRev = refreshedRev
	}

	event := AuditEvent{
		TS:             s.clock.Now(),
		DossierID:      d.Frontmatter.ID,
		Author:         s.cfg.Author,
		BeforeRevision: string(baseRev),
		AfterRevision:  string(newRev),
		ArtifactsAdded: addedArtifactIDs,
		TokenEstimate:  s.tok.Estimate(d.DistilledState.Body),
	}
	if isNew {
		event.Event = AuditEventCreate
	} else {
		event.Event = AuditEventSave
		if req.FrontmatterUpdates != nil {
			if msg := describeFrontmatterChanges(beforeFM, d.Frontmatter); msg != "" {
				event.Message = msg
			}
			// SPEC §11 (status §300): a lifecycle status change must be auditable as
			// status_changed, even when it arrives via the unified Save path.
			if beforeFM.Status != d.Frontmatter.Status {
				event.Event = AuditEventStatusChanged
			}
		}
	}
	_ = s.store.AppendAudit(d.Frontmatter.ID, event)

	// A save is the moment the curated view and the Archive can drift apart.
	// Surface evidence the Distilled State does not point at rather than let
	// it accumulate unreachably.
	if artifacts, listErr := s.store.ListArtifacts(d.Frontmatter.ID); listErr == nil {
		if msg := uncitedArtifactWarning(d.DistilledState.Body, artifacts); msg != "" {
			warnings = append(warnings, Warning(msg))
		}
	}

	return Result{
		OK:       true,
		Data:     newRev,
		Warnings: warnings,
	}, nil
}

// RenameReq changes a dossier's title and/or canonical slug. At least one of
// NewName and NewSlug must be supplied. BaseRevision is optional for interactive
// callers; when provided it protects against a stale rename.
type RenameReq struct {
	ID           string
	NewSlug      string
	NewName      string
	BaseRevision Revision
}

// RenameSlugReq remains an alias for callers of the original slug-only API.
type RenameSlugReq = RenameReq

// RenameResult describes the committed rename. The immutable ID remains the
// durable identity; callers must use the new canonical slug after a rename.
type RenameResult struct {
	ID       string   `json:"id"`
	OldName  string   `json:"old_name,omitempty"`
	Name     string   `json:"name"`
	OldSlug  string   `json:"old_slug,omitempty"`
	Slug     string   `json:"slug"`
	Revision Revision `json:"revision"`
	Path     string   `json:"path"`
}

// RenameSlugResult remains an alias for callers of the original slug-only API.
type RenameSlugResult = RenameResult

// Rename changes the title and/or slug through one concurrency-checked store
// operation. Keeping this separate from Save ensures a slug change cannot update
// frontmatter without moving the backing directory.
func (s *Service) Rename(ctx context.Context, req RenameReq) (Result, error) {
	if req.ID == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "dossier id or slug is required")
	}
	if req.NewSlug == "" && req.NewName == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "new slug or title is required")
	}
	if req.NewSlug != "" {
		if err := ValidateCanonicalSlug(req.NewSlug); err != nil {
			return Result{}, WrapError(ErrInvalidFrontmatter, "invalid slug", err)
		}
	}
	if req.NewName != "" && strings.TrimSpace(req.NewName) == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "title cannot be blank")
	}

	current, currentRev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}
	old := current.Frontmatter
	newSlug := req.NewSlug
	if newSlug == "" {
		newSlug = old.Slug
	}
	newName := req.NewName
	if newName == "" {
		newName = old.Name
	}
	if newSlug == old.Slug && newName == old.Name {
		return Result{OK: true, Data: RenameResult{
			ID: old.ID, Name: old.Name, Slug: old.Slug,
			Revision: currentRev, Path: filepath.Join(s.cfg.DossierHome, old.Slug),
		}}, nil
	}

	base := req.BaseRevision
	if base == "" {
		base = currentRev
	}
	var updated *Dossier
	var newRev Revision
	if renamer, ok := s.store.(Renamer); ok {
		updated, newRev, err = renamer.Rename(old.ID, newSlug, newName, base)
	} else if req.NewName == "" {
		// Preserve compatibility with stores that only implement the original port.
		updated, newRev, err = s.store.RenameSlug(old.ID, newSlug, base)
	} else if newSlug == old.Slug {
		// A title-only rename is safely representable by the older Save port.
		saved, saveErr := s.Save(ctx, SaveReq{ID: old.ID, BaseRevision: base, FrontmatterUpdates: map[string]any{"name": newName}})
		err = saveErr
		if err == nil {
			newRev = saved.Data.(Revision)
			updated, _, err = s.store.Read(old.ID)
		}
	} else {
		err = NewError(ErrInvalidFrontmatter, "store does not support combined title and slug renames")
	}
	if err != nil {
		return Result{}, err
	}

	result := RenameResult{
		ID: updated.Frontmatter.ID, OldName: old.Name, Name: updated.Frontmatter.Name,
		OldSlug: old.Slug, Slug: updated.Frontmatter.Slug,
		Revision: newRev, Path: filepath.Join(s.cfg.DossierHome, updated.Frontmatter.Slug),
	}
	var warnings []Warning
	event := AuditEventSlugRenamed
	if req.NewName != "" {
		event = AuditEventRenamed
	}
	if err := s.store.AppendAudit(updated.Frontmatter.ID, AuditEvent{
		TS: s.clock.Now(), Event: event, Author: s.cfg.Author,
		DossierID: updated.Frontmatter.ID, BeforeRevision: string(base),
		AfterRevision: string(newRev), Message: describeFrontmatterChanges(old, updated.Frontmatter),
	}); err != nil {
		warnings = append(warnings, Warning(fmt.Sprintf("Dossier was renamed, but the audit event could not be written: %v", err)))
	}
	return Result{OK: true, Data: result, Warnings: warnings}, nil
}

// RenameSlug is the backwards-compatible slug-only spelling.
func (s *Service) RenameSlug(ctx context.Context, req RenameSlugReq) (Result, error) {
	return s.Rename(ctx, req)
}

type LinkReq struct {
	ID           string
	FromFilePath string
	Content      string
	Title        string
}

func (s *Service) Link(ctx context.Context, req LinkReq) (Result, error) {
	now := s.clock.Now()

	if req.ID == "" {
		fms, err := s.store.List("all")
		if err != nil {
			return Result{}, err
		}

		var suggestions []Suggestion
		for _, fm := range fms {
			d, _, err := s.store.Read(fm.ID)
			if err != nil {
				continue
			}
			sug := ScoreDossier(req.Content, d, now)
			suggestions = append(suggestions, sug)
		}

		sort.Slice(suggestions, func(i, j int) bool {
			return suggestions[i].Score > suggestions[j].Score
		})

		limit := 3
		if len(suggestions) < limit {
			limit = len(suggestions)
		}
		top := suggestions[:limit]

		return Result{
			OK:   false,
			Data: top,
		}, NewError(ErrAmbiguousTarget, "Multiple likely Dossiers match this link request.")
	}

	d, baseRev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	newRev, err := s.store.Write(d, baseRev)
	if err != nil {
		return Result{}, err
	}

	title := req.Title
	if title == "" {
		title = "Linked Session Content"
	}

	art := Artifact{
		DossierID:     d.Frontmatter.ID,
		Type:          ArtifactTypeSourceSnapshot,
		Title:         title,
		Provenance:    Provenance{Origin: "linked session content"},
		ContentFormat: ContentFormatText,
		Content:       req.Content,
		CapturedAt:    now,
		RefreshedAt:   now,
	}

	if err := s.store.WriteArtifact(d.Frontmatter.ID, &art); err != nil {
		return Result{}, err
	}

	_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
		TS:             now,
		Event:          AuditEventSave,
		Author:         s.cfg.Author,
		DossierID:      d.Frontmatter.ID,
		BeforeRevision: string(baseRev),
		AfterRevision:  string(newRev),
		ArtifactsAdded: []string{art.ID},
	})

	return Result{
		OK:   true,
		Data: newRev,
	}, nil
}

type MergeReq struct {
	SourceID          string
	TargetID          string
	ResolvedConflicts []string
}

func (s *Service) Merge(ctx context.Context, req MergeReq) (Result, error) {
	sourceD, sourceRev, err := s.store.Read(req.SourceID)
	if err != nil {
		return Result{}, WrapError(ErrNotFound, "failed to read source dossier", err)
	}
	targetD, targetRev, err := s.store.Read(req.TargetID)
	if err != nil {
		return Result{}, WrapError(ErrNotFound, "failed to read target dossier", err)
	}

	// Conflict detection
	hasConflict := false
	var conflictReason []string

	if sourceD.Frontmatter.Status != targetD.Frontmatter.Status {
		hasConflict = true
		conflictReason = append(conflictReason, fmt.Sprintf("incompatible statuses: source is %q, target is %q", sourceD.Frontmatter.Status, targetD.Frontmatter.Status))
	}
	if sourceD.Frontmatter.NextAction != "" && targetD.Frontmatter.NextAction != "" && sourceD.Frontmatter.NextAction != targetD.Frontmatter.NextAction {
		hasConflict = true
		conflictReason = append(conflictReason, fmt.Sprintf("divergent next actions: source is %q, target is %q", sourceD.Frontmatter.NextAction, targetD.Frontmatter.NextAction))
	}

	isResolved := false
	if hasConflict {
		confID := "conf_merge_" + s.clock.Now().Format("20060102150405")
		for _, rc := range req.ResolvedConflicts {
			if rc == confID || rc == "all" {
				isResolved = true
				break
			}
		}

		if !isResolved {
			diff := GenerateUnifiedDiff(targetD.DistilledState.Body, sourceD.DistilledState.Body)
			conflict := &Conflict{
				ID:                 confID,
				DossierID:          targetD.Frontmatter.ID,
				Kind:               "merge_conflict",
				BaseRevision:       string(targetRev),
				AttemptedRevision:  string(sourceRev),
				TS:                 s.clock.Now(),
				RejectedBody:       sourceD.DistilledState.Body,
				DiffAgainstCurrent: diff,
			}

			_ = s.store.WriteConflict(conflict)

			_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
				TS:             s.clock.Now(),
				Event:          AuditEventMergeConflict,
				Author:         s.cfg.Author,
				DossierID:      targetD.Frontmatter.ID,
				BeforeRevision: string(targetRev),
				AfterRevision:  string(targetRev),
				Message:        fmt.Sprintf("Merge conflict %s with source %s: %s", confID, req.SourceID, strings.Join(conflictReason, "; ")),
			})

			return Result{
				OK:   false,
				Data: conflict,
			}, NewError(ErrConflictDetected, fmt.Sprintf("Merge conflict: %s. Conflict artifact %s created.", strings.Join(conflictReason, "; "), confID))
		}
	}

	_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
		TS:        s.clock.Now(),
		Event:     AuditEventMergeStarted,
		Author:    s.cfg.Author,
		DossierID: targetD.Frontmatter.ID,
		Message:   fmt.Sprintf("Starting merge of source %s into target %s", req.SourceID, req.TargetID),
	})

	if targetD.Frontmatter.NextAction == "" {
		targetD.Frontmatter.NextAction = sourceD.Frontmatter.NextAction
	}
	if sourceD.DistilledState.Body != targetD.DistilledState.Body {
		targetD.DistilledState.Body += "\n\n## Merged Distilled State (" + sourceD.Frontmatter.Name + ")\n" + sourceD.DistilledState.Body
	}

	srcArts, _ := s.store.ListArtifacts(sourceD.Frontmatter.ID)
	for _, art := range srcArts {
		fullArt, err := s.store.ReadArtifact(sourceD.Frontmatter.ID, art.ID)
		if err == nil {
			fullArt.DossierID = targetD.Frontmatter.ID
			_ = s.store.WriteArtifact(targetD.Frontmatter.ID, fullArt)
		}
	}

	newTargetRev, err := s.store.Write(targetD, targetRev)
	if err != nil {
		return Result{}, err
	}

	sourceD.Frontmatter.Status = StatusArchived
	sourceD.Frontmatter.NextAction = "Merged into " + targetD.Frontmatter.ID
	_, _ = s.store.Write(sourceD, sourceRev)

	_ = s.store.AppendAudit(targetD.Frontmatter.ID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventMergeCompleted,
		Author:         s.cfg.Author,
		DossierID:      targetD.Frontmatter.ID,
		BeforeRevision: string(targetRev),
		AfterRevision:  string(newTargetRev),
		Message:        fmt.Sprintf("Completed merge of source %s into target %s", req.SourceID, req.TargetID),
	})

	return Result{
		OK:   true,
		Data: newTargetRev,
	}, nil
}

type RecallReq struct {
	ID string
}

func (s *Service) Recall(ctx context.Context, req RecallReq) (Result, error) {
	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	tokens := s.tok.Estimate(d.DistilledState.Body)

	var warnings []Warning
	target := s.TokenLimit()
	if tokens > target {
		warnings = append(warnings, Warning(fmt.Sprintf("Distilled State exceeds token target (%d > %d tokens). Consider condensing.", tokens, target)))
	}

	index, indexWarnings := s.evidenceIndex(d.Frontmatter.ID, d.DistilledState.Body)
	warnings = append(warnings, indexWarnings...)

	dossierPath := filepath.Join(s.cfg.DossierHome, d.Frontmatter.Slug)
	return Result{
		OK:       true,
		Data:     RecallResult{DistilledState: d.DistilledState.Body, Frontmatter: d.Frontmatter, Revision: rev, TokenEstimate: tokens, Path: dossierPath, Artifacts: index},
		Warnings: warnings,
	}, nil
}

// splitContentLines is the canonical split of artifact content into physical
// lines. Every line-addressing path goes through it so a citation, a search
// hit, and a fetch cannot disagree about which line is line N. A single
// trailing newline terminates the last line rather than starting a new empty
// one; any blank line before that is a real line and is preserved.
func splitContentLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// artifactLineCount reports the number of physical lines in artifact content.
// These are the coordinates a [src:art_x#L10-L20] citation addresses.
func artifactLineCount(content string) int {
	return len(splitContentLines(content))
}

// ArtifactLineCount is the exported form of artifactLineCount, for adapters
// (e.g. the filesystem store) that need to persist the same line count the
// core uses to bound citations.
func ArtifactLineCount(content string) int {
	return artifactLineCount(content)
}

// numberLines renders lines with absolute 1-indexed numbers, so the span a
// caller reads is the span they can cite back without counting by hand.
func numberLines(lines []string, startLine int) string {
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%d\t%s\n", startLine+i, line))
	}
	return sb.String()
}

// evidenceIndex summarizes a dossier's archived artifacts and flags the ones
// the Distilled State never cites.
func (s *Service) evidenceIndex(dossierID string, body string) ([]ArtifactSummary, []Warning) {
	artifacts, err := s.store.ListArtifacts(dossierID)
	if err != nil {
		return nil, []Warning{Warning(fmt.Sprintf("Artifacts could not be listed for the evidence index: %v", err))}
	}
	cited := citedArtifactIDs(body)

	var (
		index    []ArtifactSummary
		warnings []Warning
	)
	for _, art := range artifacts {
		// The store persists the line count in frontmatter so listing an
		// evidence index never has to load an artifact's body. When Content
		// is populated (e.g. a store that returns full bodies from list),
		// prefer counting it directly so it can't disagree with Lines.
		lines := art.Lines
		if art.Content != "" {
			lines = artifactLineCount(art.Content)
		}
		index = append(index, ArtifactSummary{
			ID:            art.ID,
			Type:          string(art.Type),
			Title:         art.Title,
			ContentFormat: string(art.ContentFormat),
			Lines:         lines,
			CapturedAt:    art.CapturedAt,
			Origin:        art.Provenance.Origin,
			URL:           art.Provenance.URL,
			Cited:         cited[art.ID],
		})
	}

	if msg := uncitedArtifactWarning(body, artifacts); msg != "" {
		warnings = append(warnings, Warning(msg))
	}
	return index, warnings
}

// ReadArtifactReq addresses an artifact, optionally narrowing to a line range.
type ReadArtifactReq struct {
	DossierID  string
	ArtifactID string
	// Fragment is the raw citation fragment (e.g. "L10-L20"), accepted so a
	// caller can paste a [src:] pointer straight through.
	Fragment  string
	StartLine int
	EndLine   int
}

// ArtifactContent is a fetched artifact span.
type ArtifactContent struct {
	ArtifactSummary
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Ranged    bool   `json:"ranged"`
	Content   string `json:"content"`
}

// largeArtifactLineWarning is the point past which an unranged fetch is worth
// a nudge toward citing a span instead.
const largeArtifactLineWarning = 500

// ReadArtifact resolves an artifact citation to its content.
//
// This is what makes elision safe. The Distillation Guide asks the author to
// compress aggressively and cite what was compressed away; that trade is only
// honest if the citation can be followed back to the verbatim record. Content
// is returned line-numbered so the span read is the span cited.
func (s *Service) ReadArtifact(ctx context.Context, req ReadArtifactReq) (Result, error) {
	if req.ArtifactID == "" {
		return Result{}, NewError(ErrInvalidFrontmatter, "artifact id is required")
	}

	d, _, err := s.store.Read(req.DossierID)
	if err != nil {
		return Result{}, err
	}
	dossierID := d.Frontmatter.ID

	art, err := s.store.ReadArtifact(dossierID, req.ArtifactID)
	if err != nil {
		var domainErr *DomainError
		if errors.As(err, &domainErr) {
			return Result{}, domainErr
		}
		return Result{}, WrapError(ErrInternal, fmt.Sprintf("failed to read artifact %s in dossier %s", req.ArtifactID, dossierID), err)
	}

	start, end := req.StartLine, req.EndLine
	if req.Fragment != "" {
		ref, parseErr := ParseProvenanceRef(req.ArtifactID, strings.TrimPrefix(req.Fragment, "#"))
		if parseErr != nil {
			return Result{}, NewError(ErrInvalidFrontmatter, parseErr.Error())
		}
		if ref.HasRange {
			start, end = ref.StartLine, ref.EndLine
		}
	}

	total := artifactLineCount(art.Content)
	summary := ArtifactSummary{
		ID:            art.ID,
		Type:          string(art.Type),
		Title:         art.Title,
		ContentFormat: string(art.ContentFormat),
		Lines:         total,
		CapturedAt:    art.CapturedAt,
		Origin:        art.Provenance.Origin,
		URL:           art.Provenance.URL,
		Cited:         citedArtifactIDs(d.DistilledState.Body)[art.ID],
	}

	if start > 0 && end > 0 && start > end {
		return Result{}, NewError(ErrInvalidFrontmatter, fmt.Sprintf(
			"requested range start_line=%d ends before it starts (end_line=%d)", start, end))
	}

	var warnings []Warning
	ranged := start > 0 || end > 0

	if !ranged {
		if total > largeArtifactLineWarning {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Artifact %s is %d lines and was returned in full. Cite and fetch a range (#L<start>-L<end>) to keep the working context small.",
				art.ID, total)))
		}
		return Result{
			OK: true,
			Data: ArtifactContent{
				ArtifactSummary: summary,
				StartLine:       1,
				EndLine:         total,
				Content:         numberLines(splitContentLines(art.Content), 1),
			},
			Warnings: warnings,
		}, nil
	}

	if start < 1 {
		start = 1
	}
	if end < 1 || end > total {
		if end > total {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Requested range ends at line %d but artifact %s has %d line(s); returning through the last line.", end, art.ID, total)))
		}
		end = total
	}
	if start > total {
		return Result{}, NewError(ErrNotFound, fmt.Sprintf(
			"artifact %s has %d line(s); requested range starts at line %d", art.ID, total, start))
	}

	span := splitContentLines(art.Content)[start-1 : end]

	return Result{
		OK: true,
		Data: ArtifactContent{
			ArtifactSummary: summary,
			StartLine:       start,
			EndLine:         end,
			Ranged:          true,
			Content:         numberLines(span, start),
		},
		Warnings: warnings,
	}, nil
}

// ListArtifactsReq addresses a dossier's evidence index.
type ListArtifactsReq struct {
	DossierID string
}

// ListArtifacts returns the evidence index for a dossier.
func (s *Service) ListArtifacts(ctx context.Context, req ListArtifactsReq) (Result, error) {
	d, _, err := s.store.Read(req.DossierID)
	if err != nil {
		return Result{}, err
	}
	index, warnings := s.evidenceIndex(d.Frontmatter.ID, d.DistilledState.Body)
	return Result{OK: true, Data: index, Warnings: warnings}, nil
}

type ListReq struct {
	Status     string
	Interfaces []string
	Query      string
}

func matchesInterfaces(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, requested := range want {
		for _, assigned := range have {
			if requested == assigned {
				return true
			}
		}
	}
	return false
}

func priorityBefore(a, b Priority) bool {
	if a == b {
		return false
	}
	switch a {
	case PriorityMax:
		return true
	case PriorityHigh:
		return b != PriorityMax
	case PriorityMedium:
		return b == PriorityLow
	default:
		return false
	}
}

func sortFrontmatters(items []Frontmatter) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Priority != b.Priority {
			return priorityBefore(a.Priority, b.Priority)
		}
		if a.DueDate != b.DueDate {
			if a.DueDate == "" {
				return false
			}
			if b.DueDate == "" {
				return true
			}
			return a.DueDate < b.DueDate
		}
		return a.UpdatedAt.Before(b.UpdatedAt)
	})
}

func (s *Service) List(ctx context.Context, req ListReq) (Result, error) {
	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to list dossiers", err)
	}

	var filtered []Frontmatter
	query := NewQuery(req.Query)
	for _, fm := range fms {
		if !matchesInterfaces(fm.Interfaces, req.Interfaces) {
			continue
		}
		if !query.IsEmpty() && !query.Matches(Haystack(ListItem{
			Name:        fm.Name,
			Slug:        fm.Slug,
			Description: fm.Description,
			Lead:        fm.Lead,
			Interfaces:  fm.Interfaces,
		})) {
			continue
		}
		if req.Status == "" {
			if fm.Status.IsOpen() {
				filtered = append(filtered, fm)
			}
		} else if req.Status == "all" || string(fm.Status) == req.Status || fm.Status == NormalizeStatus(Status(req.Status)) {
			filtered = append(filtered, fm)
		}
	}

	sortFrontmatters(filtered)

	var items []ListItem
	for _, fm := range filtered {
		dossierPath := filepath.Join(s.cfg.DossierHome, fm.Slug)
		items = append(items, ListItem{
			ID:          fm.ID,
			Name:        fm.Name,
			Slug:        fm.Slug,
			Status:      string(fm.Status),
			Description: fm.Description,
			Lead:        fm.Lead,
			Interfaces:  append([]string(nil), fm.Interfaces...),
			NextAction:  fm.NextAction,
			Priority:    string(fm.Priority),
			DueDate:     fm.DueDate,
			Path:        dossierPath,
		})
	}

	return Result{
		OK:   true,
		Data: items,
	}, nil
}

type SearchReq struct {
	Query string
	Scope SearchScope
}

func (s *Service) Search(ctx context.Context, req SearchReq) (Result, error) {
	if req.Scope.DossierID != "" {
		d, _, err := s.store.Read(req.Scope.DossierID)
		if err != nil {
			return Result{}, err
		}
		req.Scope.DossierID = d.Frontmatter.ID
	}

	hits, err := s.search.Search(ctx, req.Query, req.Scope)
	if err != nil {
		return Result{}, WrapError(ErrInternal, "search failed", err)
	}

	return Result{
		OK:   true,
		Data: hits,
	}, nil
}

func (s *Service) ContextRefresh(ctx context.Context) (Result, error) {
	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to list dossiers for context refresh", err)
	}

	// Filter and sort open dossiers (non-archived) by canonical priority.
	var openDossierFrontmatter []Frontmatter
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			openDossierFrontmatter = append(openDossierFrontmatter, fm)
		}
	}
	sortFrontmatters(openDossierFrontmatter)

	var openDossiers []LibraryDossier
	for _, fm := range openDossierFrontmatter {
		openDossiers = append(openDossiers, LibraryDossier{
			Name:        fm.Name,
			Description: fm.Description,
			Status:      string(fm.Status),
			Slug:        fm.Slug,
			NextAction:  fm.NextAction,
			Priority:    string(fm.Priority),
		})
	}

	// Detect harnesses and capabilities
	harnesses := s.hreg.All()
	var activeHarness Harness
	var activeCaps Capabilities

	for _, h := range harnesses {
		caps, err := h.Detect()
		if err == nil && (caps.MCP || caps.SessionStartHook || caps.SessionEndHook || caps.PreCompactionHook || caps.TranscriptCapture) {
			activeHarness = h
			activeCaps = caps
			break
		}
	}

	harnessName := "CLI"
	harnessCaps := map[string]bool{
		"MCP":               false,
		"SessionStartHook":  false,
		"SessionEndHook":    false,
		"PreCompactionHook": false,
		"TranscriptCapture": false,
	}
	var warnings []string

	if activeHarness != nil {
		harnessName = displayHarnessName(activeHarness.Name())

		harnessCaps["MCP"] = activeCaps.MCP
		harnessCaps["SessionStartHook"] = activeCaps.SessionStartHook
		harnessCaps["SessionEndHook"] = activeCaps.SessionEndHook
		harnessCaps["PreCompactionHook"] = activeCaps.PreCompactionHook
		harnessCaps["TranscriptCapture"] = activeCaps.TranscriptCapture

		if !activeCaps.TranscriptCapture {
			warnings = append(warnings, "Transcript archive is unavailable in this session.")
		}
	} else {
		warnings = append(warnings, "No harness session active. Run from within a supported client harness for full integration.")
	}

	libData := LibraryData{
		Harness:      harnessName,
		Capabilities: harnessCaps,
		Warnings:     warnings,
		OpenDossiers: openDossiers,
	}

	if err := s.store.WriteLibraryContext(libData); err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to write library context", err)
	}

	return Result{
		OK: true,
	}, nil
}

type SwitchReq struct {
	ID        string
	SessionID string
	// HarnessName is the harness the adapter resolved this session id from, or
	// the configured launch profile for a newly spawned session. Empty means
	// unknown (explicit override, manual CLI), in which case the binding falls
	// back to whichever harness is detected.
	HarnessName string
}

func (s *Service) Switch(ctx context.Context, req SwitchReq) (Result, error) {
	if req.SessionID == "" {
		return Result{}, NewError(ErrInternal, "session_id is required for switch")
	}

	oldBinding, err := s.store.GetSessionBinding(req.SessionID)
	// The Guide is dossier-independent, so switching topics inside one session
	// earns no re-send; carry the delivery marker across the rebind.
	var guideDeliveredAt time.Time
	if err == nil && oldBinding != nil {
		guideDeliveredAt = oldBinding.GuideDeliveredAt
		if oldBinding.DossierID != "" {
			_ = s.store.ClearSessionBinding(req.SessionID)
		}
	}

	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	// Record the harness this session actually ran under. Detection order alone
	// would credit the first configured harness on the machine, so a Pi session
	// would be filed as Claude Code — along with Claude Code's capabilities.
	activeHarness, activeCaps := s.sessionHarness(req.HarnessName)

	harnessName := "CLI"
	if req.HarnessName != "" {
		harnessName = req.HarnessName
	}
	if activeHarness != nil {
		harnessName = activeHarness.Name()
	}

	binding := &SessionBinding{
		SessionBindingID: req.SessionID,
		Harness:          harnessName,
		DossierID:        d.Frontmatter.ID,
		BoundAt:          s.clock.Now(),
		LastSeenRevision: string(rev),
		Capabilities:     activeCaps,
		GuideDeliveredAt: guideDeliveredAt,
	}
	if err := s.store.SaveSessionBinding(binding); err != nil {
		return Result{}, WrapError(ErrInternal, "failed to save session binding", err)
	}

	return s.Recall(ctx, RecallReq{ID: d.Frontmatter.ID})
}

type ActiveReq struct {
	SessionID string
}

func (s *Service) Active(ctx context.Context, req ActiveReq) (Result, error) {
	if req.SessionID == "" {
		return Result{}, NewError(ErrInternal, "session_id is required")
	}

	binding, err := s.store.GetSessionBinding(req.SessionID)
	if err != nil {
		return Result{}, err
	}

	return Result{
		OK:   true,
		Data: binding,
	}, nil
}

type ArchiveReq struct {
	ID string
}

func (s *Service) Archive(ctx context.Context, req ArchiveReq) (Result, error) {
	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	d.Frontmatter.Status = StatusDone

	newRev, err := s.store.Write(d, rev)
	if err != nil {
		return Result{}, err
	}

	_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventArchived,
		Author:         s.cfg.Author,
		DossierID:      d.Frontmatter.ID,
		BeforeRevision: string(rev),
		AfterRevision:  string(newRev),
	})

	return Result{
		OK:   true,
		Data: newRev,
	}, nil
}

type PathReq struct {
	ID string
}

func (s *Service) Path(ctx context.Context, req PathReq) (Result, error) {
	d, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}

	dossierPath := filepath.Join(s.cfg.DossierHome, d.Frontmatter.Slug)
	return Result{
		OK:   true,
		Data: dossierPath,
	}, nil
}

// SessionStart returns the injected context payload for a harness session.
func (s *Service) SessionStart(ctx context.Context, sessionID string) (string, error) {
	if s.syncer != nil {
		syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, _ = s.Sync(syncCtx) // Best-effort bounded pull
	}

	binding, err := s.store.GetSessionBinding(sessionID)
	var activeDossierID string
	if err == nil && binding != nil {
		activeDossierID = binding.DossierID
	}

	// Fetch open dossiers
	fms, err := s.store.List("all")
	if err != nil {
		return "", err
	}

	sortFrontmatters(fms)
	var names []string
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			names = append(names, fm.Name)
		}
	}
	namesStr := "(none)"
	if len(names) > 0 {
		namesStr = strings.Join(names, ", ")
	}

	// Detect capabilities
	harnesses := s.hreg.All()
	var activeHarness Harness
	var activeCaps Capabilities
	for _, h := range harnesses {
		caps, err := h.Detect()
		if err == nil && (caps.MCP || caps.SessionStartHook || caps.SessionEndHook || caps.PreCompactionHook || caps.TranscriptCapture) {
			activeHarness = h
			activeCaps = caps
			break
		}
	}

	var sb strings.Builder
	sb.WriteString("# Dossier Library\n\n")

	if activeHarness != nil && !activeCaps.TranscriptCapture {
		sb.WriteString("Warning: Transcript archive is unavailable in this session.\n\n")
	}

	// Deliberately a single-line nudge, not a full payload: this fires on every
	// session regardless of relevance to Dossier, so it must stay cheap for
	// sessions that don't touch it. The heavy payload (Distillation Guide, full
	// Distilled State) is delivered by the MCP tool calls themselves — see
	// dossier_session's response — the moment the agent actually enters a
	// dossier's context, not passively here.
	sb.WriteString(fmt.Sprintf(
		"%d open dossier(s): %s. Before choosing or creating a topic, use dossier_list to check for a match; use dossier_promote for a confirmed new thread, dossier_session to bind/resume one, or dossier_recall to read its state. Guide: ~/.dossier/context/guide.md\n",
		len(names), namesStr,
	))

	if activeDossierID != "" {
		// Deliver the Guide here, at the earliest point in the session, so it is
		// in context ahead of any tool call the agent makes rather than arriving
		// alongside one. Resetting first is what makes that true after a
		// compaction: the marker survives in the binding, but the context the
		// Guide was written into does not.
		s.resetGuideDelivery(sessionID)
		if guide := s.GuideForSession(sessionID); guide != "" {
			sb.WriteString("\nDistillation Guide:\n")
			sb.WriteString(guide)
			sb.WriteString("\n")
		}

		recallRes, err := s.Recall(ctx, RecallReq{ID: activeDossierID})
		if err == nil {
			recData := recallRes.Data.(RecallResult)
			sb.WriteString("\nActive Dossier:\n")
			sb.WriteString(fmt.Sprintf("ID: %s\n", recData.Frontmatter.ID))
			sb.WriteString(fmt.Sprintf("Name: %s\n", recData.Frontmatter.Name))
			sb.WriteString(fmt.Sprintf("Revision: %s\n\n", recData.Revision))
			sb.WriteString("Distilled State:\n")
			sb.WriteString(recData.DistilledState)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func (s *Service) GetGuide() string {
	guide, err := s.store.ReadContextAsset("guide.md")
	if err != nil {
		return ""
	}
	return guide
}

// GuideForSession returns the Distillation Guide the first time it is requested
// within a session and "" on every request after that, so the session-start hook
// and the dossier_session response stop spending the same ~3.5k tokens twice on
// a resumed or post-compaction session.
//
// Suppression is deliberately biased toward delivering: an unknown session, an
// unreadable binding, or a failed write of the marker all return the Guide. The
// Guide must be in context *before* the agent composes a write, so a duplicate
// copy is a cost and a missing copy is a correctness failure — when the two
// trade off, pay the cost.
func (s *Service) GuideForSession(sessionID string) string {
	guide := s.GetGuide()
	if guide == "" || sessionID == "" {
		return guide
	}

	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil {
		return guide
	}
	if !binding.GuideDeliveredAt.IsZero() {
		return ""
	}

	binding.GuideDeliveredAt = s.clock.Now()
	_ = s.store.SaveSessionBinding(binding)
	return guide
}

// resetGuideDelivery clears the delivery marker so the next GuideForSession call
// re-sends. Called at session start, where the context window is new by
// definition — a startup, a resume, or the rebuild that follows compaction — and
// any Guide delivered earlier in the session is no longer in it.
func (s *Service) resetGuideDelivery(sessionID string) {
	if sessionID == "" {
		return
	}
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil || binding == nil || binding.GuideDeliveredAt.IsZero() {
		return
	}
	binding.GuideDeliveredAt = time.Time{}
	_ = s.store.SaveSessionBinding(binding)
}

func (s *Service) GetInstructions() string {
	instructions, err := s.store.ReadContextAsset("instructions.md")
	if err != nil {
		return ""
	}
	return instructions
}

// EnsureContextAssets refreshes the on-disk projection of the embedded context
// assets. Callers run it at wiring time so an upgraded binary never reads the
// previous release's Guide.
func (s *Service) EnsureContextAssets() ([]string, error) {
	return s.store.EnsureContextAssets()
}

// SessionEnd saves state and appends the transcript artifact on session completion.
func (s *Service) SessionEnd(ctx context.Context, sessionID string, distilledState string, transcript string) ([]Warning, error) {
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil {
		return nil, nil
	}

	now := s.clock.Now()
	finalRevision := Revision(binding.LastSeenRevision)
	var warnings []Warning

	// Read the revision before anything below writes, so this reflects only what
	// the session itself persisted. Sampling it later would fold in this hook's
	// own transcript artifact and report every session as having saved.
	var persistedDuringSession bool
	if _, revAtBoundary, revErr := s.store.Read(binding.DossierID); revErr == nil {
		persistedDuringSession = string(revAtBoundary) != binding.LastSeenRevision
	}

	if distilledState != "" {
		saveRes, err := s.Save(ctx, SaveReq{
			ID:                     binding.DossierID,
			BaseRevision:           Revision(binding.LastSeenRevision),
			DistilledStateMarkdown: distilledState,
			SessionID:              sessionID,
		})
		if err != nil {
			return warnings, err
		}
		finalRevision = saveRes.Data.(Revision)
	}

	if transcript != "" {
		// The stash keeps the raw trace byte-for-byte; the artifact stores the
		// compiled full view, whose physical line numbers are stable enough to
		// cite. A range into raw JSONL lands mid-record and cites nothing.
		compiled, compiledFormat, compileWarnings := CompileTranscript(transcript)
		for _, w := range compileWarnings {
			warnings = append(warnings, w)
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   string(w),
			})
		}

		if stashErr := s.store.WriteSessionStash(binding.DossierID, s.cfg.Author, sessionID, transcript); stashErr != nil {
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   fmt.Sprintf("Warning: failed to write session stash: %v", stashErr),
			})
		}

		art := Artifact{
			DossierID:     binding.DossierID,
			Type:          ArtifactTypeTranscript,
			Title:         binding.Harness + " Session Transcript",
			Provenance:    Provenance{Origin: binding.Harness + " session transcript (compiled)", Harness: binding.Harness},
			ContentFormat: compiledFormat,
			Content:       compiled,
			CapturedAt:    now,
			RefreshedAt:   now,
		}
		if err := s.store.WriteArtifact(binding.DossierID, &art); err != nil {
			return warnings, err
		}
		_, refreshedRev, err := s.store.Read(binding.DossierID)
		if err != nil {
			return warnings, err
		}
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:             now,
			Event:          AuditEventSave,
			Author:         s.cfg.Author,
			DossierID:      binding.DossierID,
			SessionID:      sessionID,
			BeforeRevision: string(finalRevision),
			AfterRevision:  string(refreshedRev),
			ArtifactsAdded: []string{art.ID},
		})
		finalRevision = refreshedRev
	} else {
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:        now,
			Event:     AuditEventTranscriptCaptureUnavailable,
			Author:    s.cfg.Author,
			DossierID: binding.DossierID,
			SessionID: sessionID,
			Message:   "Session boundary reached without transcript payload; no transcript artifact was captured.",
		})
	}

	if distilledState == "" {
		// No harness in the registry supplies a distilled_state payload on its
		// lifecycle hooks — a hook runs a binary, it cannot ask the agent to
		// distill. So this branch is the normal path, and the only question is
		// whether the session persisted anything on its own.
		if persistedDuringSession {
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventSave,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   "Session boundary reached without distilled_state payload; Distilled State was already saved during the session.",
			})
		} else {
			// Nothing reached the Distilled State this session and the boundary
			// cannot distill on the agent's behalf. Per the degrade-visibly rule
			// this is a surfaced warning, not an audit line nobody reads.
			w := Warning("Distilled State was not updated this session — the session-end boundary cannot distill on the agent's behalf, and nothing was saved while the session ran. The transcript is archived, so nothing is lost, but resuming this Dossier will show the state as of its last explicit save. Save during the session (dossier_save) as decisions land.")
			warnings = append(warnings, w)
			_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
				TS:        now,
				Event:     AuditEventDistilledStateNotCaptured,
				Author:    s.cfg.Author,
				DossierID: binding.DossierID,
				SessionID: sessionID,
				Message:   string(w),
			})
		}
	}

	if finalRevision != "" && string(finalRevision) != binding.LastSeenRevision {
		binding.LastSeenRevision = string(finalRevision)
		_ = s.store.SaveSessionBinding(binding)
	}

	if s.syncer != nil {
		syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, _ = s.Sync(syncCtx) // Best-effort bounded push
	}

	return warnings, nil
}

// Sync orchestrates the dossier team sync.
func (s *Service) Sync(ctx context.Context) (Result, error) {
	if s.syncer == nil {
		return Result{OK: false}, NewError(ErrInternal, "team sync is not configured; set team.remote in config")
	}

	report, err := s.syncer.Sync(ctx)
	if err != nil {
		return Result{OK: false}, fmt.Errorf("sync failed: %w", err)
	}

	var warnings []Warning
	if report.Error != "" {
		warnings = append(warnings, Warning(fmt.Sprintf("Sync network error: %s", report.Error)))
	}

	for _, excl := range report.Excluded {
		warnings = append(warnings, Warning(excl.Warning))
	}

	var createdConflicts []string
	for i, conf := range report.Conflicts {
		slugParts := strings.Split(filepath.ToSlash(conf.Path), "/")
		slug := slugParts[0] // always the dossier slug
		var targetID string
		fms, listErr := s.store.List("all")
		if listErr == nil {
			for _, fm := range fms {
				if fm.Slug == slug {
					targetID = fm.ID
					break
				}
			}
		}
		if targetID == "" {
			// If we couldn't resolve the dossier ID by slug, just use the slug as ID fallback.
			targetID = slug
		}

		confID := fmt.Sprintf("conf_%s_%s_%d", s.clock.Now().Format("20060102150405"), slug, i)
		conflict := &Conflict{
			ID:                 confID,
			DossierID:          targetID,
			Kind:               "sync_concurrent_edit",
			BaseRevision:       conf.LocalRevision,
			AttemptedRevision:  conf.RemoteRevision,
			TS:                 s.clock.Now(),
			RejectedBody:       string(conf.LocalContent),
			DiffAgainstCurrent: GenerateUnifiedDiff(string(conf.RemoteContent), string(conf.LocalContent)),
		}

		writeErr := s.store.WriteConflict(conflict)
		if writeErr == nil {
			createdConflicts = append(createdConflicts, confID)
			_ = s.store.AppendAudit(targetID, AuditEvent{
				TS:             s.clock.Now(),
				Event:          AuditEventConflictCreated,
				Author:         s.cfg.Author,
				DossierID:      targetID,
				BeforeRevision: conf.LocalRevision,
				AfterRevision:  conf.RemoteRevision,
				Message:        fmt.Sprintf("Conflict %s created due to sync concurrent edit on %s", confID, conf.Path),
			})
			warnings = append(warnings, Warning(fmt.Sprintf("wrote conflicts/%s.md — remote won the working tree, your version preserved", confID)))
		} else {
			warnings = append(warnings, Warning(fmt.Sprintf("failed to write conflict for %s: %v", conf.Path, writeErr)))
		}
	}

	return Result{
		OK:       report.Error == "",
		Data:     report,
		Warnings: warnings,
	}, nil
}

func (s *Service) SyncStatus(ctx context.Context) (Result, error) {
	if s.syncer == nil {
		return Result{OK: false}, NewError(ErrInternal, "team sync is not configured; set team.remote in config")
	}

	status, err := s.syncer.Status(ctx)
	if err != nil {
		return Result{OK: false}, fmt.Errorf("status failed: %w", err)
	}

	return Result{
		OK:   true,
		Data: status,
	}, nil
}
