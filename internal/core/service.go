package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Config holds the service-level configurations used by the core logic.
type Config struct {
	DossierHome string
	TokenTarget int
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
}

// RecallResult carries the output fields for dossier recall queries.
type RecallResult struct {
	DistilledState string      `json:"distilled_state"`
	Frontmatter    Frontmatter `json:"frontmatter"`
	Revision       Revision    `json:"revision"`
	TokenEstimate  int         `json:"token_estimate"`
	Path           string      `json:"path"`
}

// ListItem represents a single summary item for dossier listings.
type ListItem struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Slug          string   `json:"slug"`
	Status        string   `json:"status"`
	Lead          string   `json:"lead,omitempty"`
	NextAction    string   `json:"next_action"`
	OpenQuestions []string `json:"open_questions"`
	Importance    string   `json:"importance"`
	Urgency       string   `json:"urgency"`
	DueDate       string   `json:"due_date,omitempty"`
	StalenessDays int      `json:"staleness_days"`
	PriorityScore int      `json:"priority_score"`
	Path          string   `json:"path"`
}

// DoctorReport summarizes integrity checks run by Doctor.
type DoctorReport struct {
	DossiersChecked  int      `json:"dossiers_checked"`
	ArtifactsChecked int      `json:"artifacts_checked"`
	AuditLogsChecked int      `json:"audit_logs_checked"`
	ConflictsFound   int      `json:"conflicts_found"`
	Issues           []string `json:"issues,omitempty"`
}

// NewService instantiates the core orchestration service.
func NewService(store Store, search Searcher, tok Tokenizer, hreg HarnessRegistry, clock Clock, cfg Config) *Service {
	return &Service{
		store:  store,
		search: search,
		tok:    tok,
		hreg:   hreg,
		clock:  clock,
		cfg:    cfg,
	}
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
		// Report legacy/invalid enum values as healable rather than fatal: they
		// are coerced toward attention on the next write. Anything still invalid
		// after normalization (e.g. a missing id or name) is a real problem.
		normalized := fm
		for _, fix := range normalized.Normalize() {
			addIssue("Dossier %s has legacy %s %q; it will heal to %q on next edit.", fm.ID, fix.Field, fix.From, fix.To)
		}
		if err := normalized.Validate(); err != nil {
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
		for _, art := range artifacts {
			report.ArtifactsChecked++
			fullArt, err := s.store.ReadArtifact(fm.ID, art.ID)
			if err != nil {
				addIssue("Dossier %s artifact %s could not be read: %v", fm.ID, art.ID, err)
				continue
			}
			if err := fullArt.Validate(); err != nil {
				addIssue("Dossier %s artifact %s is invalid: %v", fm.ID, art.ID, err)
			}
			if strings.TrimSpace(fullArt.Provenance.Origin) == "" {
				addIssue("Dossier %s artifact %s is missing provenance.origin", fm.ID, art.ID)
			}
		}

		for _, issue := range validateDistilledStateProvenance(d.DistilledState.Body, fm.ID, func(artifactID string) bool {
			_, err := s.store.ReadArtifact(fm.ID, artifactID)
			return err == nil
		}) {
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

	return Result{
		OK:       len(report.Issues) == 0,
		Data:     report,
		Warnings: warnings,
	}, nil
}

// CurrentSchemaVersion is the frontmatter schema version this build expects.
// Bump it whenever Frontmatter.Normalize gains a rule that should be applied
// eagerly to existing stores; the startup sweep runs once when the store's
// recorded version is older than this.
const CurrentSchemaVersion = 1

// MigrateReport summarizes a one-time frontmatter migration sweep.
type MigrateReport struct {
	DossiersScanned int `json:"dossiers_scanned"`
	DossiersHealed  int `json:"dossiers_healed"`
}

// Migrate normalizes every Dossier's frontmatter to the current schema and
// rewrites the ones that changed. It is the eager, store-wide counterpart to the
// lazy heal that Save performs on a single Dossier: idempotent (re-running over
// an already-canonical store is a no-op), non-destructive (rewrites go through
// the audited Write path), and safe to run on an empty store. Each coercion is
// reported as a Warning so the migration is never silent.
func (s *Service) Migrate(ctx context.Context) (Result, error) {
	if s.store == nil {
		return Result{OK: false}, NewError(ErrInternal, "store not configured")
	}

	report := MigrateReport{}
	var warnings []Warning

	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, err
	}

	for _, fm := range fms {
		report.DossiersScanned++

		d, rev, err := s.store.Read(fm.ID)
		if err != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Dossier %s could not be read during migration: %v", fm.ID, err)))
			continue
		}

		fixes := d.Frontmatter.Normalize()
		if len(fixes) == 0 {
			continue
		}

		if _, err := s.store.Write(d, rev); err != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Dossier %s could not be healed during migration: %v", fm.ID, err)))
			continue
		}

		report.DossiersHealed++
		for _, fix := range fixes {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"Dossier %s: coerced %s %q -> %q for backward compatibility (mapping toward attention).",
				fm.ID, fix.Field, fix.From, fix.To)))
		}
	}

	return Result{
		OK:       true,
		Data:     report,
		Warnings: warnings,
	}, nil
}

var provenanceRefRE = regexp.MustCompile(`\[src:([A-Za-z0-9_]+)(?:#[^\]]+)?\]`)

func validateDistilledStateProvenance(body string, dossierID string, artifactExists func(string) bool) []string {
	var issues []string
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		if strings.Contains(trimmed, "[src:") && !provenanceRefRE.MatchString(trimmed) {
			issues = append(issues, fmt.Sprintf("Dossier %s line %d has malformed provenance reference", dossierID, i+1))
			continue
		}
		refs := provenanceRefRE.FindAllStringSubmatch(trimmed, -1)
		if len(refs) == 0 {
			issues = append(issues, fmt.Sprintf("Dossier %s line %d is missing provenance", dossierID, i+1))
			continue
		}
		for _, ref := range refs {
			if len(ref) < 2 || !artifactExists(ref[1]) {
				issues = append(issues, fmt.Sprintf("Dossier %s line %d references missing artifact %s", dossierID, i+1, ref[1]))
			}
		}
	}
	return issues
}

// Stubs for future milestones

type PromoteReq struct {
	Name                   string
	DistilledStateMarkdown string
	FromFilePath           string
	Content                string
	Lead                   string
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

	saveRes, err := s.Save(ctx, SaveReq{
		DistilledStateMarkdown: req.DistilledStateMarkdown,
		FrontmatterUpdates: map[string]any{
			"name": req.Name,
			"lead": req.Lead,
		},
	})
	if err != nil {
		return Result{}, err
	}

	newRevision := saveRes.Data.(Revision)
	fms, err := s.store.List("all")
	var newID string
	if err == nil {
		for _, fm := range fms {
			if fm.Name == req.Name {
				newID = fm.ID
				break
			}
		}
	}

	var warnings []Warning
	if req.Content != "" && newID != "" {
		art := Artifact{
			DossierID:     newID,
			Type:          ArtifactTypeTranscript,
			Title:         "Captured Session Transcript",
			Provenance:    Provenance{Origin: "promote session content"},
			ContentFormat: ContentFormatText,
			Content:       req.Content,
			CapturedAt:    now,
			RefreshedAt:   now,
		}
		_ = s.store.WriteArtifact(newID, &art)

		_ = s.store.AppendAudit(newID, AuditEvent{
			TS:             now,
			Event:          AuditEventSave,
			DossierID:      newID,
			BeforeRevision: string(newRevision),
			AfterRevision:  string(newRevision),
			ArtifactsAdded: []string{art.ID},
		})
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
	case "status":
		return string(fm.Status)
	case "lead":
		return fm.Lead
	case "next_action":
		return fm.NextAction
	case "importance":
		return string(fm.Importance)
	case "urgency":
		return string(fm.Urgency)
	case "due_date":
		return fm.DueDate
	case "token_target":
		return fm.TokenTarget
	case "open_questions":
		return strings.Join(fm.OpenQuestions, "|||")
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
	if val, ok := updates["status"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Status = Status(strVal)
		}
	}
	if val, ok := updates["lead"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Lead = strVal
		}
	}
	if val, ok := updates["next_action"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.NextAction = strVal
		}
	}
	if val, ok := updates["importance"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Importance = Importance(strVal)
		}
	}
	if val, ok := updates["urgency"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.Urgency = Urgency(strVal)
		}
	}
	if val, ok := updates["due_date"]; ok {
		if strVal, ok := val.(string); ok {
			d.Frontmatter.DueDate = strVal
		}
	}
	if val, ok := updates["token_target"]; ok {
		if intVal, ok := val.(int); ok {
			d.Frontmatter.TokenTarget = intVal
		} else if floatVal, ok := val.(float64); ok {
			d.Frontmatter.TokenTarget = int(floatVal)
		}
	}
	if val, ok := updates["open_questions"]; ok {
		if listVal, ok := val.([]string); ok {
			d.Frontmatter.OpenQuestions = listVal
		} else if anyListVal, ok := val.([]any); ok {
			var questions []string
			for _, q := range anyListVal {
				if qStr, ok := q.(string); ok {
					questions = append(questions, qStr)
				}
			}
			d.Frontmatter.OpenQuestions = questions
		}
	}
}

// describeFrontmatterChanges returns a human-readable, audit-friendly summary of
// which frontmatter fields changed between before and after, each as "field old→new".
// It returns "" when nothing material changed, so callers can fall back to a generic message.
func describeFrontmatterChanges(before, after Frontmatter) string {
	var parts []string
	add := func(field, oldVal, newVal string) {
		if oldVal != newVal {
			parts = append(parts, fmt.Sprintf("%s %q→%q", field, oldVal, newVal))
		}
	}
	add("name", before.Name, after.Name)
	add("status", string(before.Status), string(after.Status))
	add("lead", before.Lead, after.Lead)
	add("next_action", before.NextAction, after.NextAction)
	add("importance", string(before.Importance), string(after.Importance))
	add("urgency", string(before.Urgency), string(after.Urgency))
	add("due_date", before.DueDate, after.DueDate)
	if strings.Join(before.OpenQuestions, "|||") != strings.Join(after.OpenQuestions, "|||") {
		parts = append(parts, fmt.Sprintf("open_questions (%d→%d)", len(before.OpenQuestions), len(after.OpenQuestions)))
	}
	return strings.Join(parts, "; ")
}

func (s *Service) Save(ctx context.Context, req SaveReq) (Result, error) {
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
				Status:     StatusActive,
				Importance: ImportanceHigh,
				Urgency:    UrgencyLow,
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
							if f == "status" || f == "importance" || f == "urgency" || f == "lead" {
								if sVal, ok := proposedVal.(string); ok {
									normProposedVal = sVal
								}
							} else if f == "open_questions" {
								if list, ok := proposedVal.([]string); ok {
									normProposedVal = strings.Join(list, "|||")
								} else if list, ok := proposedVal.([]any); ok {
									var qList []string
									for _, qi := range list {
										if qs, ok := qi.(string); ok {
											qList = append(qList, qs)
										}
									}
									normProposedVal = strings.Join(qList, "|||")
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

	d.Frontmatter.LastTouchedAt = s.clock.Now()

	// Backward compatibility: heal any legacy/invalid frontmatter (e.g. a value
	// from a removed enum, or a field absent in an older build) before the write
	// path validates it. The coercion is persisted (the file self-heals) and
	// surfaced as a warning so it is never silent.
	var warnings []Warning
	for _, fix := range d.Frontmatter.Normalize() {
		warnings = append(warnings, Warning(fmt.Sprintf(
			"Coerced %s %q -> %q for backward compatibility (mapping toward attention).",
			fix.Field, fix.From, fix.To)))
	}

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

	return Result{
		OK:       true,
		Data:     newRev,
		Warnings: warnings,
	}, nil
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

	d.Frontmatter.LastTouchedAt = now.UTC().Truncate(time.Second)
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
		DossierID: targetD.Frontmatter.ID,
		Message:   fmt.Sprintf("Starting merge of source %s into target %s", req.SourceID, req.TargetID),
	})

	if targetD.Frontmatter.NextAction == "" {
		targetD.Frontmatter.NextAction = sourceD.Frontmatter.NextAction
	}
	for _, q := range sourceD.Frontmatter.OpenQuestions {
		found := false
		for _, tq := range targetD.Frontmatter.OpenQuestions {
			if tq == q {
				found = true
				break
			}
		}
		if !found {
			targetD.Frontmatter.OpenQuestions = append(targetD.Frontmatter.OpenQuestions, q)
		}
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
	target := d.Frontmatter.TokenTarget
	if target == 0 {
		target = s.cfg.TokenTarget
	}
	if target == 0 {
		target = 100000
	}
	if tokens > target {
		warnings = append(warnings, Warning(fmt.Sprintf("Distilled State exceeds token target (%d > %d tokens). Consider condensing.", tokens, target)))
	}

	dossierPath := filepath.Join(s.cfg.DossierHome, d.Frontmatter.Slug)
	return Result{
		OK:       true,
		Data:     RecallResult{DistilledState: d.DistilledState.Body, Frontmatter: d.Frontmatter, Revision: rev, TokenEstimate: tokens, Path: dossierPath},
		Warnings: warnings,
	}, nil
}

type ListReq struct {
	Status string
}

func (s *Service) List(ctx context.Context, req ListReq) (Result, error) {
	fms, err := s.store.List("all")
	if err != nil {
		return Result{OK: false}, WrapError(ErrInternal, "failed to list dossiers", err)
	}

	var filtered []Frontmatter
	for _, fm := range fms {
		if req.Status == "" {
			if fm.Status == StatusActive || fm.Status == StatusWaiting || fm.Status == StatusBlocked {
				filtered = append(filtered, fm)
			}
		} else if req.Status == "all" || string(fm.Status) == req.Status {
			filtered = append(filtered, fm)
		}
	}

	type scoredMeta struct {
		fm    Frontmatter
		score int
	}

	now := s.clock.Now()
	var scored []scoredMeta
	for _, fm := range filtered {
		score := CalculatePriorityScore(fm, now)
		scored = append(scored, scoredMeta{fm: fm, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		d1 := scored[i].fm.DueDate
		d2 := scored[j].fm.DueDate
		if d1 != d2 {
			if d1 == "" {
				return false
			}
			if d2 == "" {
				return true
			}
			return d1 < d2
		}
		if !scored[i].fm.LastTouchedAt.Equal(scored[j].fm.LastTouchedAt) {
			return scored[i].fm.LastTouchedAt.Before(scored[j].fm.LastTouchedAt)
		}
		return scored[i].fm.UpdatedAt.Before(scored[j].fm.UpdatedAt)
	})

	var items []ListItem
	for _, sItem := range scored {
		daysSinceTouched := int(now.Sub(sItem.fm.LastTouchedAt).Hours() / 24)
		if daysSinceTouched < 0 {
			daysSinceTouched = 0
		}

		dossierPath := filepath.Join(s.cfg.DossierHome, sItem.fm.Slug)
		items = append(items, ListItem{
			ID:            sItem.fm.ID,
			Name:          sItem.fm.Name,
			Slug:          sItem.fm.Slug,
			Status:        string(sItem.fm.Status),
			Lead:          sItem.fm.Lead,
			NextAction:    sItem.fm.NextAction,
			OpenQuestions: sItem.fm.OpenQuestions,
			Importance:    string(sItem.fm.Importance),
			Urgency:       string(sItem.fm.Urgency),
			DueDate:       sItem.fm.DueDate,
			StalenessDays: daysSinceTouched,
			PriorityScore: sItem.score,
			Path:          dossierPath,
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

	// Filter and score open dossiers (non-archived)
	type scoredMeta struct {
		fm    Frontmatter
		score int
	}

	now := s.clock.Now()
	var scored []scoredMeta
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			score := CalculatePriorityScore(fm, now)
			scored = append(scored, scoredMeta{fm: fm, score: score})
		}
	}

	// Sort open dossiers by priority score ascending
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		d1 := scored[i].fm.DueDate
		d2 := scored[j].fm.DueDate
		if d1 != d2 {
			if d1 == "" {
				return false
			}
			if d2 == "" {
				return true
			}
			return d1 < d2
		}
		if !scored[i].fm.LastTouchedAt.Equal(scored[j].fm.LastTouchedAt) {
			return scored[i].fm.LastTouchedAt.Before(scored[j].fm.LastTouchedAt)
		}
		return scored[i].fm.UpdatedAt.Before(scored[j].fm.UpdatedAt)
	})

	var openDossiers []LibraryDossier
	for _, sItem := range scored {
		openDossiers = append(openDossiers, LibraryDossier{
			Name:          sItem.fm.Name,
			Status:        string(sItem.fm.Status),
			Slug:          sItem.fm.Slug,
			NextAction:    sItem.fm.NextAction,
			PriorityScore: sItem.score,
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
	// HarnessName is the harness the adapter resolved this session id from
	// ("claude-code", "pi"). Empty means unknown (explicit override, manual CLI),
	// in which case the binding falls back to whichever harness is detected.
	HarnessName string
}

func (s *Service) Switch(ctx context.Context, req SwitchReq) (Result, error) {
	if req.SessionID == "" {
		return Result{}, NewError(ErrInternal, "session_id is required for switch")
	}

	oldBinding, err := s.store.GetSessionBinding(req.SessionID)
	if err == nil && oldBinding != nil && oldBinding.DossierID != "" {
		oldD, oldRev, err := s.store.Read(oldBinding.DossierID)
		if err == nil {
			oldD.Frontmatter.LastTouchedAt = s.clock.Now()
			_, _ = s.store.Write(oldD, oldRev)
		}
		_ = s.store.ClearSessionBinding(req.SessionID)
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

	d.Frontmatter.Status = StatusArchived
	d.Frontmatter.LastTouchedAt = s.clock.Now()

	newRev, err := s.store.Write(d, rev)
	if err != nil {
		return Result{}, err
	}

	_ = s.store.AppendAudit(d.Frontmatter.ID, AuditEvent{
		TS:             s.clock.Now(),
		Event:          AuditEventArchived,
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

	now := s.clock.Now()
	type scoredMeta struct {
		fm    Frontmatter
		score int
	}
	var scored []scoredMeta
	for _, fm := range fms {
		if fm.Status != StatusArchived {
			score := CalculatePriorityScore(fm, now)
			scored = append(scored, scoredMeta{fm: fm, score: score})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		d1 := scored[i].fm.DueDate
		d2 := scored[j].fm.DueDate
		if d1 != d2 {
			if d1 == "" {
				return false
			}
			if d2 == "" {
				return true
			}
			return d1 < d2
		}
		if !scored[i].fm.LastTouchedAt.Equal(scored[j].fm.LastTouchedAt) {
			return scored[i].fm.LastTouchedAt.Before(scored[j].fm.LastTouchedAt)
		}
		return scored[i].fm.UpdatedAt.Before(scored[j].fm.UpdatedAt)
	})

	var names []string
	for _, sItem := range scored {
		names = append(names, sItem.fm.Name)
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
		"%d open dossier(s): %s. Use dossier_list for details, dossier_session to resume one, or dossier_promote for a new thread (it flags likely duplicates automatically). Guide: ~/.dossier/context/guide.md\n",
		len(scored), namesStr,
	))

	if activeDossierID != "" {
		guidePath := filepath.Join(s.cfg.DossierHome, "context", "guide.md")
		if guideBytes, err := os.ReadFile(guidePath); err == nil {
			sb.WriteString("\nDistillation Guide:\n")
			sb.WriteString(string(guideBytes))
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
	guidePath := filepath.Join(s.cfg.DossierHome, "context", "guide.md")
	if guideBytes, err := os.ReadFile(guidePath); err == nil {
		return string(guideBytes)
	}
	return ""
}

func (s *Service) GetInstructions() string {
	instructionsPath := filepath.Join(s.cfg.DossierHome, "context", "instructions.md")
	if instructionsBytes, err := os.ReadFile(instructionsPath); err == nil {
		return string(instructionsBytes)
	}
	return ""
}

// SessionEnd saves state and appends the transcript artifact on session completion.
func (s *Service) SessionEnd(ctx context.Context, sessionID string, distilledState string, transcript string) error {
	binding, err := s.store.GetSessionBinding(sessionID)
	if err != nil {
		return nil
	}

	now := s.clock.Now()
	finalRevision := Revision(binding.LastSeenRevision)

	if distilledState != "" {
		saveRes, err := s.Save(ctx, SaveReq{
			ID:                     binding.DossierID,
			BaseRevision:           Revision(binding.LastSeenRevision),
			DistilledStateMarkdown: distilledState,
			SessionID:              sessionID,
		})
		if err != nil {
			return err
		}
		finalRevision = saveRes.Data.(Revision)
	}

	if transcript != "" {
		art := Artifact{
			DossierID:     binding.DossierID,
			Type:          ArtifactTypeTranscript,
			Title:         binding.Harness + " Session Transcript",
			Provenance:    Provenance{Origin: binding.Harness + " session transcript", Harness: binding.Harness},
			ContentFormat: ContentFormatText,
			Content:       transcript,
			CapturedAt:    now,
			RefreshedAt:   now,
		}
		if err := s.store.WriteArtifact(binding.DossierID, &art); err != nil {
			return err
		}
		_, refreshedRev, err := s.store.Read(binding.DossierID)
		if err != nil {
			return err
		}
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:             now,
			Event:          AuditEventSave,
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
			DossierID: binding.DossierID,
			SessionID: sessionID,
			Message:   "Session boundary reached without transcript payload; no transcript artifact was captured.",
		})
	}

	if distilledState == "" {
		_ = s.store.AppendAudit(binding.DossierID, AuditEvent{
			TS:        now,
			Event:     AuditEventSave,
			DossierID: binding.DossierID,
			SessionID: sessionID,
			Message:   "Session boundary reached without distilled_state payload; retained available artifacts and left Distilled State unchanged.",
		})
	}

	if finalRevision != "" && string(finalRevision) != binding.LastSeenRevision {
		binding.LastSeenRevision = string(finalRevision)
		_ = s.store.SaveSessionBinding(binding)
	}

	return nil
}
