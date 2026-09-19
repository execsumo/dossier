package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
//
// Service may be shared by MCP request handling and its background sync
// debouncer. Port implementations own their own concurrency guarantees. The
// only mutable Service-owned state is the configurable vocabulary, protected
// by cfgMu; all other configuration and dependencies are immutable after
// construction.
type Service struct {
	store  Store
	search Searcher
	tok    Tokenizer
	hreg   HarnessRegistry
	clock  Clock
	cfgMu  sync.RWMutex
	cfg    Config
	syncer Syncer
}

// RecallResult carries the output fields for dossier recall queries.
type RecallResult struct {
	DistilledState string         `json:"distilled_state"`
	Frontmatter    Frontmatter    `json:"frontmatter"`
	Revision       Revision       `json:"revision"`
	TokenEstimate  int            `json:"token_estimate"`
	Path           string         `json:"path"`
	References     []ExternalLink `json:"references,omitempty"`
	ActiveMonitors []ExternalLink `json:"active_monitors,omitempty"`
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
	Revision    Revision `json:"revision,omitempty"`
	// HasOpenDelegationContract reports whether any Delegation Contract block
	// (guide.md §4) has a field that isn't yet [decided] — an attention signal
	// a list surface can show without opening the dossier.
	HasOpenDelegationContract bool `json:"has_open_delegation_contract"`
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
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return append([]string{}, s.cfg.Interfaces...)
}

// DossierHome returns the machine-local store root. Adapters use it to locate
// configuration-owned files without reaching into Service's private state.
func (s *Service) DossierHome() string {
	return s.cfg.DossierHome
}

// AddLead updates the in-memory lead vocabulary after an adapter persists the
// same change to config.yaml. This keeps subsequent Saves in this process
// consistent with the newly expanded vocabulary.
func (s *Service) AddLead(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	for _, existing := range s.cfg.Leads {
		if existing == name {
			return
		}
	}
	s.cfg.Leads = append(s.cfg.Leads, name)
}

// AddInterface updates the in-memory interface vocabulary after an adapter
// persists the same change to config.yaml. This keeps subsequent Saves in this
// process consistent with the newly expanded vocabulary.
func (s *Service) AddInterface(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	for _, existing := range s.cfg.Interfaces {
		if existing == name {
			return
		}
	}
	s.cfg.Interfaces = append(s.cfg.Interfaces, name)
}

// Leads returns the configured lead vocabulary in display order. An empty list
// preserves free-form lead assignment for backwards compatibility.
func (s *Service) Leads() []string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return append([]string{}, s.cfg.Leads...)
}

// TokenLimit returns the configured token limit for distilled state.
func (s *Service) TokenLimit() int {
	if s.cfg.TokenLimit <= 0 {
		return DefaultTokenLimit
	}
	return s.cfg.TokenLimit
}

// scanDossiers visits each dossier body through the store's streaming
// capability when available, with a compatibility fallback for older stores.
// Point-read failures in the fallback are warnings rather than silent skips.
func (s *Service) scanDossiers(statusFilter string, visit func(*Dossier) error) ([]Warning, error) {
	if scanner, ok := s.store.(DossierScanner); ok {
		return nil, scanner.ScanDossiers(statusFilter, visit)
	}
	fms, err := s.store.List(statusFilter)
	if err != nil {
		return nil, err
	}
	var warnings []Warning
	for _, fm := range fms {
		d, _, readErr := s.store.Read(fm.ID)
		if readErr != nil {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Skipped dossier %s during full-state scan: %v", fm.ID, readErr)))
			continue
		}
		if err := visit(d); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
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
			if errors.Is(installErr, ErrInstallSkipped) {
				warnings = append(warnings, Warning(installErr.Error()))
			} else {
				warnings = append(warnings, Warning(fmt.Sprintf("Failed to install for %s: %v", h.Name(), installErr)))
			}
		}

		// Re-detect: installing an integration is what turns a capability on
		// (the Pi extension supplies session identity), so the pre-install
		// snapshot would understate what the user now has.
		if postCaps, err := h.Detect(); err == nil {
			caps = postCaps
		}
		report := newHarnessReport(h.Name(), caps)
		report.Notes = append(report.Notes, harnessAdvisories(h.Name(), caps)...)

		if adv, ok := h.(PostInstallAdvisor); ok {
			report.Notes = append(report.Notes, adv.PostInstallNotes()...)
		}
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
				LastSync:       status.LastAttempt,
				Dirty:          status.Dirty,
				ConflictsFound: len(conflicts),
			}
		}
	}

	return Result{
		OK:       len(report.Issues) == 0,
		Data:     report,
		Warnings: warnings,
	}, nil
}
