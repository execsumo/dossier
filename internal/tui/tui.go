package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dossier/internal/core"
	"dossier/internal/harness"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
)

// View represents the current active TUI screen view.
type View int

const (
	ViewDashboard View = iota
	ViewDetail
	ViewStatusPicker
	ViewNextActionEditor
	ViewLeadEditor
	ViewPriorityEditor
	ViewLinkInput
	ViewLinkSelector
	ViewMergeSelector
	ViewMergeConflictResolver
	// ViewLeadSelector scopes the dashboard by lead, with search-as-you-type.
	ViewLeadSelector
	// ViewArtifactIndex lists a dossier's archived artifacts, mirroring
	// `dossier artifact <slug>` so a [src:] citation can be followed from the TUI.
	ViewArtifactIndex
	// ViewArtifactContent shows one artifact's line-numbered content, mirroring
	// `dossier artifact <slug> <artifact-id>`.
	ViewArtifactContent
)

// leadFilterKind enumerates the three ways the dashboard can be scoped by lead.
type leadFilterKind int

const (
	filterAll        leadFilterKind = iota // every dossier, regardless of lead
	filterUnassigned                       // dossiers with no lead set
	filterByName                           // dossiers owned by a specific lead
)

// leadFilter scopes the dashboard to a subset of dossiers by lead. It is a typed
// value rather than a sentinel string so a lead literally named "All" or
// "Unassigned" can never be confused with the pinned filter modes.
type leadFilter struct {
	kind leadFilterKind
	name string // meaningful only when kind == filterByName
}

// interfaceFilter scopes the dashboard to dossiers assigned to one discussion
// forum. The empty value means all interfaces.
type interfaceFilter string

func (f interfaceFilter) matches(item core.ListItem) bool {
	if f == "" {
		return true
	}
	for _, name := range item.Interfaces {
		if name == string(f) {
			return true
		}
	}
	return false
}

func (f interfaceFilter) label() string {
	if f == "" {
		return "All"
	}
	return string(f)
}

// matches reports whether item belongs in this filter's view.
func (f leadFilter) matches(item core.ListItem) bool {
	switch f.kind {
	case filterUnassigned:
		return item.Lead == ""
	case filterByName:
		return item.Lead == f.name
	default: // filterAll
		return true
	}
}

// label is the human-facing name shown in the selector and dashboard.
func (f leadFilter) label() string {
	switch f.kind {
	case filterUnassigned:
		return "Unassigned"
	case filterByName:
		return f.name
	default:
		return "All"
	}
}

// leadOption is one selectable row in the lead landing screen: a filter plus the
// number of (live, tier-0) dossiers it would show, computed once when the
// option list is built.
type leadOption struct {
	filter leadFilter
	count  int
}

// Styling tokens
var (
	purple       = lipgloss.Color("99")
	lightGray    = lipgloss.Color("7") // Use terminal's standard light gray (ANSI 7)
	darkGray     = lipgloss.Color("8") // Use terminal's standard dark gray/bright black (ANSI 8)
	vibrantGreen = lipgloss.Color("2") // Use terminal's standard green (ANSI 2)
	vibrantRed   = lipgloss.Color("1") // Use terminal's standard red (ANSI 1)
	warningGold  = lipgloss.Color("3") // Use terminal's standard yellow/gold (ANSI 3)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")). // Force crisp white text on purple bg
			Background(purple).
			Padding(0, 2).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(darkGray). // Inherit terminal theme's gray (ANSI 8)
			Italic(true)

	headerStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	footerStyle = lipgloss.NewStyle().
			Reverse(true). // Inverted foreground and background dynamically to match terminal theme status bar
			Padding(0, 1)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningGold).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(vibrantRed).
			Bold(true)

	metaLabelStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	metaValueStyle = lipgloss.NewStyle() // Inherit terminal's default text foreground color
	mutedStyle     = lipgloss.NewStyle().Foreground(darkGray)

	statusSparkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7D7")).Bold(true)
	statusDefineStyle    = lipgloss.NewStyle().Foreground(vibrantGreen).Bold(true)
	statusDelegatedStyle = lipgloss.NewStyle().Foreground(warningGold)
	statusReviewStyle    = lipgloss.NewStyle().Foreground(purple).Bold(true)
	statusBlockedStyle   = lipgloss.NewStyle().Foreground(vibrantRed).Bold(true)
	statusDoneStyle      = lipgloss.NewStyle().Foreground(darkGray)

	// Legacy aliases for backward compatibility
	statusActiveStyle   = statusDefineStyle
	statusWaitingStyle  = statusDelegatedStyle
	statusResolvedStyle = statusDoneStyle
	statusArchivedStyle = lipgloss.NewStyle().Foreground(darkGray).Faint(true)

	editorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple).
			Padding(1, 2).
			Margin(1, 0)

	focusedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")). // Force crisp white text on purple bg
				Background(purple).
				Bold(true).
				Padding(0, 1)

	activeOptionStyle = lipgloss.NewStyle().
				Foreground(vibrantGreen).
				Bold(true)
)

// Messages
type listDossiersMsg []core.ListItem
type recallDossierMsg struct {
	id       string
	result   core.RecallResult
	err      error
	warnings []core.Warning
}
type mutationResultMsg struct {
	err      error
	prevView View
	targetID string
}
type linkResultMsg struct {
	err     error
	result  core.Result
	content string
}
type linkConfirmResultMsg struct {
	err error
}
type mergeResultMsg struct {
	err      error
	result   core.Result
	sourceID string
	targetID string
}
type artifactIndexMsg struct {
	dossierID string
	index     []core.ArtifactSummary
	err       error
}
type artifactContentMsg struct {
	content  core.ArtifactContent
	warnings []core.Warning
	err      error
}

type errMsg error

type dossierUpdatedMsg struct{}

func waitForUpdate(updateChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		<-updateChan
		return dossierUpdatedMsg{}
	}
}

type targetDossier struct {
	id           string
	name         string
	slug         string
	status       core.Status
	priority     core.Priority
	dueDate      string
	nextAction   string
	lead         string
	baseRevision core.Revision
}

// Model holds the application state.
type Model struct {
	svc         *core.Service
	currentView View

	// Data
	items        []core.ListItem // full dossier list, source of truth
	visibleItems []core.ListItem // items[] narrowed by lead/interface filters and extrasExpanded; what the table shows
	liveCount    int             // visibleItems[:liveCount] are tier-0 (live) items; visibleItems[liveCount:] are extras, present only while expanded
	extrasCount  int             // resolved/archived items matching leadFilter but excluded from visibleItems while collapsed
	recallResult core.RecallResult

	// Lead selector state
	leadFilter           leadFilter      // active dashboard scope (defaults to All)
	leadSearch           textinput.Model // search-as-you-type box
	leadOptions          []leadOption    // every selectable lead, counts included
	leadResults          []leadOption    // leadOptions narrowed by the search box
	leadCursor           int
	interfaceFilter      interfaceFilter
	configuredLeads      []string
	configuredInterfaces []string

	// extrasExpanded controls the dashboard's "Show More... / Hide Extras..." row:
	// when false (the default), resolved/archived dossiers are collapsed out of
	// visibleItems and represented by that single trailing toggle row instead.
	extrasExpanded bool

	// Viewport & Table. Detail and artifact content intentionally use separate
	// viewports so following evidence cannot replace or reposition the rendered
	// Distilled State.
	table            table.Model
	viewport         viewport.Model
	artifactViewport viewport.Model
	conflictViewport viewport.Model
	width            int
	height           int

	// Error / Warning tracking
	err      error
	warnings []core.Warning

	// View state helpers
	loading bool

	// Mutation target cache
	previousView       View
	targetID           string
	targetName         string
	targetBaseRevision core.Revision

	// Status Picker view state
	statusOptions []core.Status
	statusCursor  int

	// Next Action Editor view state
	nextActionInput textinput.Model

	// Lead Editor view state
	leadInput          textinput.Model
	leadSuggestions    []string
	leadSuggestionText string

	// Priority Editor view state
	priorityFocus int // 0 = Priority, 1 = Due Date
	editPriority  core.Priority
	dueDateInput  textinput.Model

	// Link view state
	linkTextInput   textinput.Model
	linkContent     string
	linkSuggestions []core.Suggestion
	linkCursor      int

	// Merge view state
	mergeSourceID          string
	mergeSourceName        string
	mergeTargetID          string
	mergeTargets           []core.ListItem
	mergeCursor            int
	mergeConflict          *core.Conflict
	conflictResolverCursor int // 0 = Resolve/Force, 1 = Cancel

	// Artifact index / content view state
	artifactIndex   []core.ArtifactSummary
	artifactCursor  int
	artifactContent core.ArtifactContent

	watcher      *fsnotify.Watcher
	updateChan   chan string
	watchedPaths map[string]bool

	// Cached markdown renderer, rebuilt only when the wrap width changes.
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int

	// Seams for the "open in Claude" handoff, defaulted in NewModel. They exist
	// so tests can press 'c' without a claude binary on PATH and without ever
	// spawning a process.
	claudeBin   func() (string, error)
	execProcess func(*exec.Cmd, tea.ExecCallback) tea.Cmd
}

// NewModel instantiates the root TUI model.
func NewModel(svc *core.Service) Model {
	// Initialize default empty table
	columns := []table.Column{
		{Title: "Name", Width: 30},
		{Title: "Priority", Width: 12},
		{Title: "Lead", Width: 8},
		{Title: "Status", Width: 10},
		{Title: "Due", Width: 8},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(darkGray).
		BorderBottom(true).
		Foreground(purple).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")). // Force crisp white text on purple bg
		Background(purple).
		Bold(true)
	t.SetStyles(s)

	vp := viewport.New(0, 0)
	avp := viewport.New(0, 0)
	cvp := viewport.New(0, 0)

	statusOptions := core.CanonicalStatuses()

	leadSearch := textinput.New()
	leadSearch.Placeholder = "Type a lead's name to search…"
	leadSearch.Focus()
	leadSearch.Width = 40

	watcher, err := fsnotify.NewWatcher()
	updateChan := make(chan string, 100)
	if err == nil {
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					if event.Op.Has(fsnotify.Write) || event.Op.Has(fsnotify.Rename) || event.Op.Has(fsnotify.Create) {
						updateChan <- "update"
					}
				case <-watcher.Errors:
				}
			}
		}()
	}

	return Model{
		svc:                  svc,
		currentView:          ViewDashboard,
		table:                t,
		viewport:             vp,
		artifactViewport:     avp,
		conflictViewport:     cvp,
		loading:              true,
		statusOptions:        statusOptions,
		leadSearch:           leadSearch,
		configuredLeads:      svc.Leads(),
		configuredInterfaces: svc.Interfaces(),
		watcher:              watcher,
		updateChan:           updateChan,
		watchedPaths:         map[string]bool{},
		claudeBin:            harness.ClaudeBin,
		execProcess:          tea.ExecProcess,
	}
}

// syncWatches makes the fsnotify watch set exactly match paths, adding new ones
// and dropping stale ones. Failures to add/remove a single path are non-fatal.
func (m *Model) syncWatches(paths []string) {
	if m.watcher == nil {
		return
	}
	desired := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		desired[p] = true
		if !m.watchedPaths[p] {
			if err := m.watcher.Add(p); err == nil {
				m.watchedPaths[p] = true
			}
		}
	}
	for p := range m.watchedPaths {
		if !desired[p] {
			_ = m.watcher.Remove(p)
			delete(m.watchedPaths, p)
		}
	}
}

// ensureWatch adds a single path to the watch set without disturbing the others.
func (m *Model) ensureWatch(path string) {
	if m.watcher == nil || path == "" || m.watchedPaths[path] {
		return
	}
	if err := m.watcher.Add(path); err == nil {
		m.watchedPaths[path] = true
	}
}

// Init initializes the tea program, triggering initial loads.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.listDossiersCmd(), waitForUpdate(m.updateChan))
}

// listDossiersCmd fetches the dossier list asynchronously.
func (m Model) listDossiersCmd() tea.Cmd {
	return func() tea.Msg {
		// Fetch every status so resolved/archived dossiers are available for meeting
		// prep and can be surfaced when the dashboard's extras are expanded.
		res, err := m.svc.List(context.Background(), core.ListReq{Status: "all"})
		if err != nil {
			return errMsg(err)
		}
		items, ok := res.Data.([]core.ListItem)
		if !ok {
			return errMsg(fmt.Errorf("invalid list data type"))
		}
		return listDossiersMsg(items)
	}
}

// recallDossierCmd fetches the details of a specific dossier.
func (m Model) recallDossierCmd(id string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.svc.Recall(context.Background(), core.RecallReq{ID: id})
		if err != nil {
			return recallDossierMsg{id: id, err: err}
		}
		recallRes, ok := res.Data.(core.RecallResult)
		if !ok {
			return recallDossierMsg{id: id, err: fmt.Errorf("invalid recall data type")}
		}
		return recallDossierMsg{
			id:       id,
			result:   recallRes,
			warnings: res.Warnings,
		}
	}
}

// listArtifactsCmd fetches a dossier's evidence index, mirroring
// `dossier artifact <slug>`.
func (m Model) listArtifactsCmd(dossierID string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.svc.ListArtifacts(context.Background(), core.ListArtifactsReq{DossierID: dossierID})
		if err != nil {
			return artifactIndexMsg{dossierID: dossierID, err: err}
		}
		index, ok := res.Data.([]core.ArtifactSummary)
		if !ok {
			return artifactIndexMsg{dossierID: dossierID, err: fmt.Errorf("invalid artifact index data type")}
		}
		return artifactIndexMsg{dossierID: dossierID, index: index}
	}
}

// readArtifactCmd fetches one artifact's line-numbered content, mirroring
// `dossier artifact <slug> <artifact-id>`.
func (m Model) readArtifactCmd(dossierID, artifactID string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.svc.ReadArtifact(context.Background(), core.ReadArtifactReq{DossierID: dossierID, ArtifactID: artifactID})
		if err != nil {
			return artifactContentMsg{err: err}
		}
		content, ok := res.Data.(core.ArtifactContent)
		if !ok {
			return artifactContentMsg{err: fmt.Errorf("invalid artifact content data type")}
		}
		return artifactContentMsg{content: content, warnings: res.Warnings}
	}
}

func (m Model) firstLinkCmd(content string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.svc.Link(context.Background(), core.LinkReq{
			ID:      "",
			Content: content,
			Title:   "TUI Interactive Link",
		})
		return linkResultMsg{err: err, result: res, content: content}
	}
}

func (m Model) confirmLinkCmd(id string, content string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Link(context.Background(), core.LinkReq{
			ID:      id,
			Content: content,
			Title:   "TUI Interactive Link",
		})
		return linkConfirmResultMsg{err: err}
	}
}

func (m Model) mergeCmd(sourceID, targetID string, resolved []string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.svc.Merge(context.Background(), core.MergeReq{
			SourceID:          sourceID,
			TargetID:          targetID,
			ResolvedConflicts: resolved,
		})
		return mergeResultMsg{
			err:      err,
			result:   res,
			sourceID: sourceID,
			targetID: targetID,
		}
	}
}

func (m Model) setStatusCmd(id string, baseRev core.Revision, status core.Status) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Save(context.Background(), core.SaveReq{
			ID:                 id,
			BaseRevision:       baseRev,
			FrontmatterUpdates: map[string]any{"status": string(status)},
		})
		return mutationResultMsg{err: err, prevView: m.previousView, targetID: id}
	}
}

func (m Model) saveNextActionCmd(id string, baseRev core.Revision, nextAction string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Save(context.Background(), core.SaveReq{
			ID:                 id,
			BaseRevision:       baseRev,
			FrontmatterUpdates: map[string]any{"next_action": nextAction},
		})
		return mutationResultMsg{err: err, prevView: m.previousView, targetID: id}
	}
}

func (m Model) saveLeadCmd(id string, baseRev core.Revision, lead string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Save(context.Background(), core.SaveReq{
			ID:                 id,
			BaseRevision:       baseRev,
			FrontmatterUpdates: map[string]any{"lead": lead},
		})
		return mutationResultMsg{err: err, prevView: m.previousView, targetID: id}
	}
}

func (m Model) savePriorityCmd(id string, baseRev core.Revision, priority core.Priority, dueDate string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Save(context.Background(), core.SaveReq{
			ID:           id,
			BaseRevision: baseRev,
			FrontmatterUpdates: map[string]any{
				"priority": string(priority),
				"due_date": dueDate,
			},
		})
		return mutationResultMsg{err: err, prevView: m.previousView, targetID: id}
	}
}

func (m Model) getTargetDossier() (targetDossier, bool) {
	if m.currentView == ViewDetail {
		fm := m.recallResult.Frontmatter
		return targetDossier{
			id:           fm.ID,
			name:         fm.Name,
			slug:         fm.Slug,
			status:       fm.Status,
			priority:     fm.Priority,
			dueDate:      fm.DueDate,
			nextAction:   fm.NextAction,
			lead:         fm.Lead,
			baseRevision: m.recallResult.Revision,
		}, true
	}

	// Dashboard view
	itemIdx, isToggle := m.rowToItemIndex(m.table.Cursor())
	if !isToggle && itemIdx >= 0 && itemIdx < len(m.visibleItems) {
		item := m.visibleItems[itemIdx]
		return targetDossier{
			id:           item.ID,
			name:         item.Name,
			slug:         item.Slug,
			status:       core.Status(item.Status),
			priority:     core.Priority(item.Priority),
			dueDate:      item.DueDate,
			nextAction:   item.NextAction,
			lead:         item.Lead,
			baseRevision: "", // Skip check from dashboard
		}, true
	}
	return targetDossier{}, false
}

// openInClaude hands the focused Dossier off to a fresh Claude Code session.
//
// The TUI still resolves no session identity of its own (ADR 0004) — it *mints*
// one for a session that does not exist yet, binds the Dossier to that id, and
// then launches Claude with the same id via --session-id. Claude's session-start
// hook therefore fires already bound and injects the Distilled State. See
// ADR 0006.
//
// The binary is looked up first, before anything is written, so a missing claude
// cannot leave a binding behind for a session that never starts. Every failure
// sets m.err rather than silently doing nothing.
func (m Model) openInClaude(t targetDossier) (tea.Model, tea.Cmd) {
	bin, err := m.claudeBin()
	if err != nil {
		m.err = err
		return m, nil
	}

	ctx := context.Background()
	res, err := m.svc.Path(ctx, core.PathReq{ID: t.id})
	if err != nil {
		m.err = err
		return m, nil
	}
	dir, _ := res.Data.(string)
	if dir == "" {
		// Launching with an empty Dir would silently run Claude in the TUI's own
		// working directory against the wrong (or no) Dossier. Fail visibly.
		m.err = fmt.Errorf("could not resolve the directory for dossier %s", t.id)
		return m, nil
	}

	sessionID, err := harness.NewClaudeSessionID()
	if err != nil {
		m.err = err
		return m, nil
	}

	if _, err := m.svc.Switch(ctx, core.SwitchReq{
		ID:          t.id,
		SessionID:   sessionID,
		HarnessName: "claude-code",
	}); err != nil {
		m.err = err
		return m, nil
	}

	slug := t.slug
	if slug == "" {
		slug = filepath.Base(dir)
	}
	plan := harness.PlanClaudeHandoff(bin, sessionID, dir, t.name, slug)

	id, fromView := t.id, m.currentView
	return m, m.execProcess(plan.Command(), func(err error) tea.Msg {
		return claudeFinishedMsg{err: err, id: id, fromView: fromView}
	})
}

// deriveLeadOptions builds the lead selector's rows from the full dossier
// list: "All" and "Unassigned" pinned first, then each distinct lead in
// case-insensitive alphabetical order, every row annotated with its dossier
// count. Counts reflect only live (tier-0) work, matching the dashboard's
// default collapsed view; resolved/archived dossiers are surfaced per-lead via
// the dashboard's own "Show More..." row, not counted here. Pure: depends only
// on items.
func deriveLeadOptions(items []core.ListItem, configured ...[]string) []leadOption {
	var all, unassigned int
	counts := make(map[string]int)
	for _, item := range items {
		if item.ID == "" {
			continue // skip placeholder/header rows
		}
		if statusTier(item.Status) == 1 {
			continue
		}
		all++
		if item.Lead == "" {
			unassigned++
			continue
		}
		counts[item.Lead]++
	}

	names := make([]string, 0, len(counts))
	seen := make(map[string]bool)
	if len(configured) > 0 {
		for _, name := range configured[0] {
			name = strings.TrimSpace(name)
			if name != "" && !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
	}
	var observed []string
	for name := range counts {
		if !seen[name] {
			observed = append(observed, name)
		}
	}
	sort.Slice(observed, func(i, j int) bool {
		return strings.ToLower(observed[i]) < strings.ToLower(observed[j])
	})
	names = append(names, observed...)

	opts := make([]leadOption, 0, len(names)+3)
	opts = append(opts,
		leadOption{filter: leadFilter{kind: filterAll}, count: all},
		leadOption{filter: leadFilter{kind: filterUnassigned}, count: unassigned},
	)
	for _, name := range names {
		opts = append(opts, leadOption{
			filter: leadFilter{kind: filterByName, name: name},
			count:  counts[name],
		})
	}
	return opts
}

// statusTier ranks a dossier's lifecycle status for dashboard ordering: open
// work (spark/define/delegated/review/blocked) is tier 0, terminal work (done) is
// tier 1, so terminal dossiers always sort below open ones at any priority.
func statusTier(status string) int {
	if core.Status(status).IsTerminal() {
		return 1
	}
	return 0
}

// filterLeadOptions narrows opts to those whose label contains query
// (case-insensitive). An empty query returns opts unchanged. Pure.
func filterLeadOptions(opts []leadOption, query string) []leadOption {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return opts
	}
	out := make([]leadOption, 0, len(opts))
	for _, o := range opts {
		if strings.Contains(strings.ToLower(o.filter.label()), query) {
			out = append(out, o)
		}
	}
	return out
}

// applyLeadFilter recomputes the dashboard's visible items from the full set and
// the active lead filter. It is the single choke point that keeps the table rows
// in sync with the filter, so cursor lookups can index visibleItems directly.
// Live (tier-0) items always come first, followed by extras (resolved/archived)
// only while extrasExpanded is set — so visibleItems[:liveCount] is always the
// live set and visibleItems[liveCount:] is always the extras set, regardless of
// expansion state. That invariant lets the toggle row live at a stable row
// index (liveCount) rather than always trailing the last row.
func (m *Model) applyLeadFilter() {
	visible := make([]core.ListItem, 0, len(m.items))
	var extraItems []core.ListItem
	for _, item := range m.items {
		if !m.leadFilter.matches(item) || !m.interfaceFilter.matches(item) {
			continue
		}
		if statusTier(item.Status) == 1 {
			extraItems = append(extraItems, item)
			continue
		}
		visible = append(visible, item)
	}
	m.liveCount = len(visible)
	m.extrasCount = len(extraItems)
	if m.extrasExpanded {
		visible = append(visible, extraItems...)
	}
	m.visibleItems = visible
}

// rowToItemIndex translates a table cursor row into an index into
// visibleItems. When extras exist, the toggle row occupies row liveCount, so
// rows after it are offset by one; isToggle reports whether idx landed there.
func (m *Model) rowToItemIndex(idx int) (itemIdx int, isToggle bool) {
	if m.extrasCount == 0 || idx < m.liveCount {
		return idx, false
	}
	if idx == m.liveCount {
		return -1, true
	}
	return idx - 1, false
}

// openLeadSelector enters the lead selector with a fresh search, the option list
// rebuilt from current data, and the cursor parked on the active filter.
func (m *Model) openLeadSelector() {
	m.previousView = m.currentView
	m.currentView = ViewLeadSelector

	m.leadSearch = textinput.New()
	m.leadSearch.Placeholder = "Type a lead's name to search…"
	m.leadSearch.Focus()
	m.leadSearch.Width = 40

	m.leadOptions = deriveLeadOptions(m.items, m.configuredLeads)
	m.leadResults = m.leadOptions
	m.leadCursor = 0
	for i, o := range m.leadResults {
		if o.filter == m.leadFilter {
			m.leadCursor = i
			break
		}
	}
}

// chooseLead applies the option under the cursor and drops into the dashboard.
func (m *Model) chooseLead() {
	if m.leadCursor >= 0 && m.leadCursor < len(m.leadResults) {
		m.leadFilter = m.leadResults[m.leadCursor].filter
	}
	m.applyLeadFilter()
	m.populateTableRows()
	m.currentView = ViewDashboard
	m.table.SetCursor(0)
	m.table.Focus()
}

func (m *Model) startEditStatus(t targetDossier) {
	m.previousView = m.currentView
	m.currentView = ViewStatusPicker
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision

	m.statusCursor = 0
	for i, o := range m.statusOptions {
		if o == t.status {
			m.statusCursor = i
			break
		}
	}
}

func (m *Model) startEditNextAction(t targetDossier) {
	m.previousView = m.currentView
	m.currentView = ViewNextActionEditor
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision

	m.nextActionInput = textinput.New()
	// Set the existing value before applying the limit so legacy overlong
	// actions are not silently truncated; the user can edit them down to size.
	m.nextActionInput.SetValue(t.nextAction)
	m.nextActionInput.CharLimit = core.MaxNextActionLength
	m.nextActionInput.Focus()
	m.nextActionInput.Width = 60
}

// leadAutocomplete returns unique previously used lead names that begin with query,
// sorted case-insensitively. It is intentionally prefix-only: Tab/Enter can accept
// the first suggestion without surprising the user with fuzzy replacements.
func leadAutocomplete(items []core.ListItem, query string, configured ...[]string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lowerQuery := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	if len(configured) > 0 && len(configured[0]) > 0 {
		for _, configuredLead := range configured[0] {
			name := strings.TrimSpace(configuredLead)
			if name == "" || seen[name] || !strings.HasPrefix(strings.ToLower(name), lowerQuery) {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	} else {
		for _, item := range items {
			name := strings.TrimSpace(item.Lead)
			if name == "" || seen[name] || !strings.HasPrefix(strings.ToLower(name), lowerQuery) {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

func (m *Model) startEditLead(t targetDossier) {
	m.previousView = m.currentView
	m.currentView = ViewLeadEditor
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision

	m.leadInput = textinput.New()
	m.leadInput.Placeholder = "e.g. Alice"
	m.leadInput.SetValue(t.lead)
	m.leadInput.Focus()
	m.leadInput.Width = 40
	m.leadSuggestions = nil
	m.leadSuggestionText = ""
}

func (m *Model) updateLeadSuggestions() {
	m.leadSuggestions = leadAutocomplete(m.items, m.leadInput.Value(), m.configuredLeads)
	m.leadSuggestionText = ""
	if len(m.leadSuggestions) > 0 && !strings.EqualFold(m.leadSuggestions[0], m.leadInput.Value()) {
		m.leadSuggestionText = m.leadSuggestions[0]
	}
}

func (m *Model) acceptLeadSuggestion() bool {
	if m.leadSuggestionText == "" {
		return false
	}
	m.leadInput.SetValue(m.leadSuggestionText)
	m.updateLeadSuggestions()
	return true
}

func (m *Model) startEditPriority(t targetDossier) {
	m.previousView = m.currentView
	m.currentView = ViewPriorityEditor
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision

	m.editPriority = t.priority
	if !m.editPriority.IsValid() {
		m.editPriority = core.PriorityHigh
	}

	m.dueDateInput = textinput.New()
	m.dueDateInput.Placeholder = "YYYY-MM-DD"
	m.dueDateInput.SetValue(t.dueDate)
	m.priorityFocus = 0
}

func (m *Model) startLinkInput() {
	m.previousView = m.currentView
	m.currentView = ViewLinkInput
	m.linkTextInput = textinput.New()
	m.linkTextInput.Placeholder = "Enter raw content or description to link"
	m.linkTextInput.Focus()
	m.linkTextInput.Width = 60
}

func (m *Model) startMergeSelector(sourceID, sourceName string) {
	m.previousView = m.currentView
	m.currentView = ViewMergeSelector
	m.mergeSourceID = sourceID
	m.mergeSourceName = sourceName

	// filter other dossiers
	m.mergeTargets = nil
	for _, item := range m.items {
		if item.ID != sourceID && item.ID != "" {
			m.mergeTargets = append(m.mergeTargets, item)
		}
	}
	m.mergeCursor = 0
}

func cyclePriority(curr core.Priority, forward bool) core.Priority {
	opts := []core.Priority{core.PriorityLow, core.PriorityMedium, core.PriorityHigh, core.PriorityMax}
	idx := -1
	for i, option := range opts {
		if option == curr {
			idx = i
			break
		}
	}
	if idx < 0 {
		return core.PriorityHigh
	}
	if forward {
		return opts[(idx+1)%len(opts)]
	}
	return opts[(idx-1+len(opts))%len(opts)]
}

func priorityBefore(a, b core.Priority) bool {
	if a == b {
		return false
	}
	switch a {
	case core.PriorityMax:
		return true
	case core.PriorityHigh:
		return b != core.PriorityMax
	case core.PriorityMedium:
		return b == core.PriorityLow
	default:
		return false
	}
}

func (m *Model) renderMarkdown(content string) string {
	wrapWidth := m.width - 2 // small margin
	if wrapWidth < 40 {
		wrapWidth = 40
	}
	// Rebuild the renderer only when the wrap width changes; constructing one is
	// relatively expensive and renderMarkdown runs on every resize/refresh.
	if m.mdRenderer == nil || m.mdRendererWidth != wrapWidth {
		// Use the default dark style but remove the markdown header prefixes
		cfg := *styles.DefaultStyles["dark"]
		cfg.H1.Prefix = ""
		cfg.H2.Prefix = ""
		cfg.H3.Prefix = ""
		cfg.H4.Prefix = ""
		cfg.H5.Prefix = ""
		cfg.H6.Prefix = ""

		// Reset document colors to inherit terminal defaults (supporting light/dark themes)
		cfg.Document.Color = nil
		cfg.Document.BackgroundColor = nil

		// Signature purple accent for headings
		purpleStr := "99"
		whiteStr := "#FFFFFF"
		cfg.Heading.Color = &purpleStr
		cfg.Heading.BackgroundColor = nil

		// Highlight H1 with signature purple background and crisp white text
		cfg.H1.Color = &whiteStr
		cfg.H1.BackgroundColor = &purpleStr

		cfg.H2.Color = &purpleStr
		cfg.H2.BackgroundColor = nil
		cfg.H3.Color = &purpleStr
		cfg.H3.BackgroundColor = nil
		cfg.H4.Color = &purpleStr
		cfg.H4.BackgroundColor = nil
		cfg.H5.Color = &purpleStr
		cfg.H5.BackgroundColor = nil
		cfg.H6.Color = &purpleStr
		cfg.H6.BackgroundColor = nil

		// Make blockquote left border signature purple as well
		cfg.BlockQuote.Color = &purpleStr

		// Cyan for links (standard ANSI 6, theme adaptive)
		cyanStr := "6"
		cfg.Link.Color = &cyanStr
		cfg.LinkText.Color = &cyanStr

		// Inline code: cyan color and no background color to avoid contrast issues on light/dark backgrounds
		cfg.Code.Color = &cyanStr
		cfg.Code.BackgroundColor = nil

		// Gray for horizontal rules (standard ANSI 8)
		grayStr := "8"
		cfg.HorizontalRule.Color = &grayStr

		// Use bw theme for syntax highlighting to avoid hardcoded dark/light colors
		cfg.CodeBlock.Chroma = nil
		cfg.CodeBlock.Theme = "bw"

		r, err := glamour.NewTermRenderer(
			glamour.WithStyles(cfg),
			glamour.WithWordWrap(wrapWidth),
		)
		if err != nil {
			return content
		}
		m.mdRenderer = r
		m.mdRendererWidth = wrapWidth
	}
	if rendered, err := m.mdRenderer.Render(content); err == nil {
		return rendered
	}
	return content
}

// Update handles incoming messages and updates model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// View-specific key overrides
		switch m.currentView {
		case ViewLeadSelector:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Skip selection: fall through to the dashboard with the current
				// filter (All by default). On the startup landing this means
				// "show everything"; reopened via 'f' it cancels the change.
				m.applyLeadFilter()
				m.populateTableRows()
				m.currentView = ViewDashboard
				m.table.SetCursor(0)
				m.table.Focus()
				return m, nil
			case "up", "ctrl+p":
				if len(m.leadResults) > 0 {
					m.leadCursor = (m.leadCursor - 1 + len(m.leadResults)) % len(m.leadResults)
				}
				return m, nil
			case "down", "ctrl+n":
				if len(m.leadResults) > 0 {
					m.leadCursor = (m.leadCursor + 1) % len(m.leadResults)
				}
				return m, nil
			case "enter":
				if len(m.leadResults) > 0 {
					m.chooseLead()
				}
				return m, nil
			}
			// Any other key edits the search box; re-filter and keep the cursor valid.
			m.leadSearch, cmd = m.leadSearch.Update(msg)
			m.leadResults = filterLeadOptions(m.leadOptions, m.leadSearch.Value())
			if m.leadCursor >= len(m.leadResults) {
				m.leadCursor = len(m.leadResults) - 1
			}
			if m.leadCursor < 0 {
				m.leadCursor = 0
			}
			return m, cmd

		case ViewLinkInput:
			switch msg.String() {
			case "esc":
				m.currentView = m.previousView
				return m, nil
			case "enter":
				m.loading = true
				m.err = nil
				return m, m.firstLinkCmd(m.linkTextInput.Value())
			}
			m.linkTextInput, cmd = m.linkTextInput.Update(msg)
			return m, cmd

		case ViewLinkSelector:
			switch msg.String() {
			case "esc":
				m.currentView = ViewDashboard
				return m, nil
			case "up", "k":
				m.linkCursor = (m.linkCursor - 1 + len(m.linkSuggestions)) % len(m.linkSuggestions)
			case "down", "j":
				m.linkCursor = (m.linkCursor + 1) % len(m.linkSuggestions)
			case "enter":
				m.loading = true
				m.err = nil
				return m, m.confirmLinkCmd(m.linkSuggestions[m.linkCursor].ID, m.linkContent)
			}
			return m, nil

		case ViewMergeSelector:
			switch msg.String() {
			case "esc":
				m.currentView = ViewDashboard
				return m, nil
			case "up", "k":
				m.mergeCursor = (m.mergeCursor - 1 + len(m.mergeTargets)) % len(m.mergeTargets)
			case "down", "j":
				m.mergeCursor = (m.mergeCursor + 1) % len(m.mergeTargets)
			case "enter":
				if len(m.mergeTargets) > 0 {
					m.loading = true
					m.err = nil
					m.mergeTargetID = m.mergeTargets[m.mergeCursor].ID
					return m, m.mergeCmd(m.mergeSourceID, m.mergeTargetID, nil)
				}
			}
			return m, nil

		case ViewMergeConflictResolver:
			switch msg.String() {
			case "esc":
				m.currentView = ViewDashboard
				return m, nil
			case "tab", "shift+tab":
				m.conflictResolverCursor = (m.conflictResolverCursor + 1) % 2
			case "enter":
				if m.conflictResolverCursor == 0 {
					m.loading = true
					m.err = nil
					return m, m.mergeCmd(m.mergeSourceID, m.mergeTargetID, []string{m.mergeConflict.ID})
				} else {
					m.currentView = ViewDashboard
					return m, nil
				}
			}
			m.conflictViewport, cmd = m.conflictViewport.Update(msg)
			return m, cmd

		case ViewArtifactIndex:
			switch msg.String() {
			case "esc":
				m.currentView = ViewDetail
				m.err = nil
				return m, nil
			case "up", "k":
				if len(m.artifactIndex) > 0 {
					m.artifactCursor = (m.artifactCursor - 1 + len(m.artifactIndex)) % len(m.artifactIndex)
				}
			case "down", "j":
				if len(m.artifactIndex) > 0 {
					m.artifactCursor = (m.artifactCursor + 1) % len(m.artifactIndex)
				}
			case "enter":
				if m.artifactCursor >= 0 && m.artifactCursor < len(m.artifactIndex) {
					m.loading = true
					m.err = nil
					return m, m.readArtifactCmd(m.recallResult.Frontmatter.ID, m.artifactIndex[m.artifactCursor].ID)
				}
			}
			return m, nil

		case ViewArtifactContent:
			switch msg.String() {
			case "esc":
				m.currentView = ViewArtifactIndex
				m.err = nil
				return m, nil
			}
			m.artifactViewport, cmd = m.artifactViewport.Update(msg)
			return m, cmd

		case ViewNextActionEditor:
			switch msg.String() {
			case "esc":
				m.currentView = m.previousView
				return m, nil
			case "enter":
				m.loading = true
				m.err = nil
				return m, m.saveNextActionCmd(m.targetID, m.targetBaseRevision, m.nextActionInput.Value())
			}
			m.nextActionInput, cmd = m.nextActionInput.Update(msg)
			return m, cmd

		case ViewLeadEditor:
			switch msg.String() {
			case "esc":
				m.currentView = m.previousView
				return m, nil
			case "tab":
				if m.acceptLeadSuggestion() {
					return m, nil
				}
			case "enter":
				if m.acceptLeadSuggestion() {
					return m, nil
				}
				m.loading = true
				m.err = nil
				return m, m.saveLeadCmd(m.targetID, m.targetBaseRevision, m.leadInput.Value())
			}
			m.leadInput, cmd = m.leadInput.Update(msg)
			m.updateLeadSuggestions()
			return m, cmd

		case ViewStatusPicker:
			switch msg.String() {
			case "esc":
				m.currentView = m.previousView
				return m, nil
			case "up", "k":
				m.statusCursor = (m.statusCursor - 1 + len(m.statusOptions)) % len(m.statusOptions)
			case "down", "j":
				m.statusCursor = (m.statusCursor + 1) % len(m.statusOptions)
			case "enter":
				m.loading = true
				m.err = nil
				return m, m.setStatusCmd(m.targetID, m.targetBaseRevision, m.statusOptions[m.statusCursor])
			}
			return m, nil

		case ViewPriorityEditor:
			switch msg.String() {
			case "esc":
				m.currentView = m.previousView
				return m, nil
			case "up", "k":
				m.priorityFocus = (m.priorityFocus - 1 + 2) % 2
				if m.priorityFocus == 1 {
					m.dueDateInput.Focus()
				} else {
					m.dueDateInput.Blur()
				}
			case "down", "j", "tab":
				m.priorityFocus = (m.priorityFocus + 1) % 2
				if m.priorityFocus == 1 {
					m.dueDateInput.Focus()
				} else {
					m.dueDateInput.Blur()
				}
			case "shift+tab":
				m.priorityFocus = (m.priorityFocus - 1 + 2) % 2
				if m.priorityFocus == 1 {
					m.dueDateInput.Focus()
				} else {
					m.dueDateInput.Blur()
				}
			case "left", "h":
				if m.priorityFocus == 0 {
					m.editPriority = cyclePriority(m.editPriority, false)
				}
			case "right", "l":
				if m.priorityFocus == 0 {
					m.editPriority = cyclePriority(m.editPriority, true)
				}
			case "enter":
				m.loading = true
				m.err = nil
				return m, m.savePriorityCmd(m.targetID, m.targetBaseRevision, m.editPriority, m.dueDateInput.Value())
			}

			if m.priorityFocus == 1 {
				m.dueDateInput, cmd = m.dueDateInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Global keys for Dashboard and Detail Views
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "backspace":
			switch m.currentView {
			case ViewDetail:
				m.currentView = ViewDashboard
				m.warnings = nil
				m.err = nil
				m.table.Focus()
				return m, m.listDossiersCmd()
			case ViewDashboard:
				// Esc has no dashboard action; filters are opened explicitly
				// with f. Preserve the existing backspace shortcut.
				if msg.String() == "backspace" {
					m.openLeadSelector()
					return m, nil
				}
			}
		case "r":
			m.loading = true
			m.err = nil
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				return m, m.recallDossierCmd(m.recallResult.Frontmatter.ID)
			}
			return m, m.listDossiersCmd()
		case "enter":
			if m.currentView == ViewDashboard {
				itemIdx, isToggle := m.rowToItemIndex(m.table.Cursor())
				if isToggle {
					// The "Show More.../Hide Extras..." row, between live items and extras.
					m.extrasExpanded = !m.extrasExpanded
					m.applyLeadFilter()
					m.populateTableRows()
					m.table.SetCursor(m.liveCount)
					return m, nil
				}
				if itemIdx >= 0 && itemIdx < len(m.visibleItems) {
					dossierID := m.visibleItems[itemIdx].ID
					if dossierID == "" {
						return m, nil // prevent selection of header
					}
					m.loading = true
					m.err = nil
					return m, m.recallDossierCmd(dossierID)
				}
			}
		case "s":
			if t, ok := m.getTargetDossier(); ok {
				m.startEditStatus(t)
				return m, nil
			}
		case "p":
			if t, ok := m.getTargetDossier(); ok && t.id != "" {
				m.startEditPriority(t)
				return m, nil
			}
		case "n":
			if t, ok := m.getTargetDossier(); ok && t.id != "" {
				m.startEditNextAction(t)
				return m, nil
			}
		case "l":
			if t, ok := m.getTargetDossier(); ok && t.id != "" {
				m.startEditLead(t)
				return m, nil
			}
		case "e":
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				res, err := m.svc.Path(context.Background(), core.PathReq{ID: m.recallResult.Frontmatter.ID})
				if err != nil {
					m.err = err
					return m, nil
				}
				dossierPath := filepath.Join(res.Data.(string), "dossier.md")
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "nano"
				}
				cmd := exec.Command(editor, dossierPath)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return editorFinishedMsg{err: err, id: m.recallResult.Frontmatter.ID}
				})
			}
		case "c":
			if m.currentView == ViewDashboard || m.currentView == ViewDetail {
				if t, ok := m.getTargetDossier(); ok && t.id != "" {
					return m.openInClaude(t)
				}
			}
		case "a":
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				m.loading = true
				m.err = nil
				return m, m.listArtifactsCmd(m.recallResult.Frontmatter.ID)
			}
		case "f":
			if m.currentView == ViewDashboard {
				m.openLeadSelector()
				return m, nil
			}
		case "i":
			if m.currentView == ViewDashboard {
				m.interfaceFilter = nextInterfaceFilter(m.interfaceFilter, m.configuredInterfaces)
				m.applyLeadFilter()
				m.populateTableRows()
				m.table.SetCursor(0)
				return m, nil
			}
		case "k":
			if m.currentView == ViewDashboard {
				m.startLinkInput()
				return m, nil
			}
		case "m":
			if m.currentView == ViewDashboard {
				itemIdx, isToggle := m.rowToItemIndex(m.table.Cursor())
				if !isToggle && itemIdx >= 0 && itemIdx < len(m.visibleItems) {
					m.startMergeSelector(m.visibleItems[itemIdx].ID, m.visibleItems[itemIdx].Name)
					return m, nil
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateTableLayout()
		m.recalculateViewportLayout()
		m.recalculateArtifactViewportLayout()
		m.recalculateConflictViewportLayout()

		// Re-render cached content even when its view is hidden. A resize in the
		// artifact browser must not leave the detail view wrapped to the old width
		// when the user returns to it. SetContent and the layout helpers preserve
		// each viewport's independent scroll offset (clamped to its new bounds).
		if m.recallResult.Frontmatter.ID != "" {
			m.viewport.SetContent(m.renderMarkdown(m.recallResult.DistilledState))
			m.viewport.SetYOffset(m.viewport.YOffset)
		}
		if m.artifactContent.ID != "" {
			m.artifactViewport.SetContent(renderArtifactContent(m.artifactContent))
			m.artifactViewport.SetYOffset(m.artifactViewport.YOffset)
		}
		if m.currentView == ViewMergeConflictResolver && m.mergeConflict != nil {
			diffMd := fmt.Sprintf("```diff\n%s\n```", m.mergeConflict.DiffAgainstCurrent)
			m.conflictViewport.SetContent(m.renderMarkdown(diffMd))
		}

	case listDossiersMsg:
		m.loading = false

		sort.Slice(msg, func(i, j int) bool {
			// Live work (active/waiting/blocked) always sorts above terminal work
			// (resolved/archived). We fetch all statuses so a lead's finished
			// dossiers are on hand for meeting prep, but that must never bury open
			// work beneath a high-priority archived item.
			if ti, tj := statusTier(msg[i].Status), statusTier(msg[j].Status); ti != tj {
				return ti < tj
			}
			if msg[i].Priority != msg[j].Priority {
				return priorityBefore(core.Priority(msg[i].Priority), core.Priority(msg[j].Priority))
			}
			d1 := msg[i].DueDate
			d2 := msg[j].DueDate
			if d1 != d2 {
				if d1 == "" {
					return false
				}
				if d2 == "" {
					return true
				}
				return d1 < d2
			}
			return false
		})

		m.items = msg

		// Re-derive lead options on every refresh so newly-assigned leads appear,
		// while preserving the active filter (and the search box) across hot-reloads.
		m.leadOptions = deriveLeadOptions(m.items, m.configuredLeads)
		m.leadResults = filterLeadOptions(m.leadOptions, m.leadSearch.Value())
		if m.leadCursor >= len(m.leadResults) {
			m.leadCursor = len(m.leadResults) - 1
		}
		if m.leadCursor < 0 {
			m.leadCursor = 0
		}

		m.applyLeadFilter()
		m.populateTableRows()
		if len(m.visibleItems) > 0 {
			m.table.SetCursor(0)
		}

		// Watch every dossier directory so the dashboard live-refreshes on
		// external edits, plus the currently open dossier if it isn't listed.
		var watchPaths []string
		for _, item := range msg {
			watchPaths = append(watchPaths, item.Path)
		}
		if m.currentView == ViewDetail {
			watchPaths = append(watchPaths, m.recallResult.Path)
		}
		m.syncWatches(watchPaths)

	case recallDossierMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.currentView = ViewDetail
			m.recallResult = msg.result
			m.warnings = msg.warnings
			m.viewport.SetContent(m.renderMarkdown(msg.result.DistilledState))
			m.recalculateViewportLayout()
			m.viewport.YOffset = 0

			// Recall returns the dossier's directory path; sync watches including
			// the new path and any currently listed dashboard items to prevent leaks
			// from navigating deep into links.
			var watchPaths []string
			for _, item := range m.items {
				if item.Path != "" {
					watchPaths = append(watchPaths, item.Path)
				}
			}
			watchPaths = append(watchPaths, m.recallResult.Path)
			m.syncWatches(watchPaths)
		}

	case artifactIndexMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.currentView = ViewArtifactIndex
			m.artifactIndex = msg.index
			m.artifactCursor = 0
			m.err = nil
		}

	case artifactContentMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.currentView = ViewArtifactContent
			m.artifactContent = msg.content
			m.warnings = msg.warnings
			m.artifactViewport.SetContent(renderArtifactContent(msg.content))
			m.recalculateArtifactViewportLayout()
			m.artifactViewport.GotoTop()
			m.err = nil
		}

	case linkResultMsg:
		m.loading = false
		if msg.err != nil {
			// Check if it's a domain error code for ambiguity
			if dErr, ok := msg.err.(*core.DomainError); ok && dErr.Code == core.ErrAmbiguousTarget {
				suggestions, ok := msg.result.Data.([]core.Suggestion)
				if ok && len(suggestions) > 0 {
					m.currentView = ViewLinkSelector
					m.linkSuggestions = suggestions
					m.linkContent = msg.content
					m.linkCursor = 0
					return m, nil
				}
			}
			m.err = msg.err
			m.currentView = ViewDashboard
		} else {
			m.currentView = ViewDashboard
			m.err = nil
			return m, m.listDossiersCmd()
		}

	case linkConfirmResultMsg:
		m.loading = false
		m.currentView = ViewDashboard
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			return m, m.listDossiersCmd()
		}

	case mergeResultMsg:
		m.loading = false
		if msg.err != nil {
			if dErr, ok := msg.err.(*core.DomainError); ok && dErr.Code == core.ErrConflictDetected {
				conflict, ok := msg.result.Data.(*core.Conflict)
				if ok {
					m.currentView = ViewMergeConflictResolver
					m.mergeConflict = conflict
					diffMd := fmt.Sprintf("```diff\n%s\n```", conflict.DiffAgainstCurrent)
					m.conflictViewport.SetContent(m.renderMarkdown(diffMd))
					m.recalculateConflictViewportLayout()
					m.conflictResolverCursor = 0
					return m, nil
				}
			}
			m.err = msg.err
			m.currentView = ViewDashboard
		} else {
			m.currentView = ViewDashboard
			m.err = nil
			// Show success info
			return m, m.listDossiersCmd()
		}

	case mutationResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.currentView = msg.prevView
		} else {
			m.currentView = msg.prevView
			m.err = nil
			if m.currentView == ViewDetail {
				return m, m.recallDossierCmd(msg.targetID)
			} else {
				return m, m.listDossiersCmd()
			}
		}

	case editorFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.loading = true
		return m, m.recallDossierCmd(msg.id)

	case claudeFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Claude almost certainly wrote to the Dossier while the TUI was
		// suspended; refresh whichever view we came back to.
		m.loading = true
		if msg.fromView == ViewDetail && msg.id != "" {
			return m, m.recallDossierCmd(msg.id)
		}
		return m, m.listDossiersCmd()

	case errMsg:
		m.loading = false
		m.err = msg

	case dossierUpdatedMsg:
		cmds = append(cmds, waitForUpdate(m.updateChan))
		if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
			m.loading = true
			cmds = append(cmds, m.recallDossierCmd(m.recallResult.Frontmatter.ID))
		} else if m.currentView == ViewDashboard || m.currentView == ViewLeadSelector {
			cmds = append(cmds, m.listDossiersCmd())
		}
	}

	// Update view-specific sub-components
	if m.currentView == ViewDashboard {
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.currentView == ViewDetail {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

type editorFinishedMsg struct {
	err error
	id  string
}

// claudeFinishedMsg reports that a handed-off Claude Code session has exited and
// the TUI has the terminal back. fromView records where 'c' was pressed so the
// right view is refreshed.
type claudeFinishedMsg struct {
	err      error
	id       string
	fromView View
}

// tableColumnsConfig reports which width-sensitive columns the dashboard shows.
// Name, Lead, and Status are always present; Priority and Due are revealed
// progressively as the terminal widens.
func (m *Model) tableColumnsConfig() (showPriority, showDue bool) {
	w := m.width
	if w < 44 {
		w = 44
	}
	return w >= 55, w >= 65
}

// itemTableRow builds a single dossier row. Cells mirror the column order
// Name, [Priority], Lead, Status, [Due]; the optional cells are included only
// when the corresponding column is shown.
func itemTableRow(item core.ListItem, showPriority, showDue bool) table.Row {
	if item.ID == "" {
		row := table.Row{item.Name}
		if showPriority {
			row = append(row, "")
		}
		row = append(row, "", "") // Lead, Status
		if showDue {
			row = append(row, "")
		}
		return row
	}

	leadStr := item.Lead
	if leadStr != "" {
		parts := strings.Fields(leadStr)
		if len(parts) > 1 {
			leadStr = parts[0] + " " + string(parts[len(parts)-1][0])
		}
	}

	priorityStr := item.Priority

	dueStr := ""
	if item.DueDate != "" {
		t, err := time.Parse("2006-01-02", item.DueDate)
		if err == nil {
			dueStr = t.Format("01/02")
		} else {
			dueStr = item.DueDate
		}
	}

	row := table.Row{item.Name}
	if showPriority {
		row = append(row, priorityStr)
	}
	row = append(row, leadStr, item.Status)
	if showDue {
		row = append(row, dueStr)
	}
	return row
}

// extrasToggleTableRow builds the "Show More.../Hide Extras..." row, padding
// empty cells to match the visible column count.
func extrasToggleTableRow(expanded bool, showPriority, showDue bool) table.Row {
	label := "Show More..."
	if expanded {
		label = "Hide Extras..."
	}
	row := table.Row{label}
	if showPriority {
		row = append(row, "")
	}
	row = append(row, "", "") // Lead, Status
	if showDue {
		row = append(row, "")
	}
	return row
}

// populateTableRows maps visibleItems into the table rows, inserting the
// extras toggle row between the live items (visibleItems[:liveCount]) and any
// expanded extras (visibleItems[liveCount:]) so it always reads as the
// boundary between the two groups rather than trailing the whole list.
func (m *Model) populateTableRows() {
	showPriority, showDue := m.tableColumnsConfig()

	rows := make([]table.Row, 0, len(m.visibleItems)+1)
	for _, item := range m.visibleItems[:m.liveCount] {
		rows = append(rows, itemTableRow(item, showPriority, showDue))
	}
	if m.extrasCount > 0 {
		rows = append(rows, extrasToggleTableRow(m.extrasExpanded, showPriority, showDue))
	}
	for _, item := range m.visibleItems[m.liveCount:] {
		rows = append(rows, itemTableRow(item, showPriority, showDue))
	}

	m.table.SetRows(rows)
}

// recalculateTableLayout fits the table to the screen size. Columns appear in
// the canonical order Name, Priority, Lead, Status, Due; Name is always present
// and flexes to absorb the leftover width, while the fixed-width columns are
// revealed progressively as the terminal widens.
func (m *Model) recalculateTableLayout() {
	footerH := m.footerHeight(ViewDashboard)
	tableHeight := m.height - 4 - footerH
	if tableHeight < 3 {
		tableHeight = 3
	}
	m.table.SetHeight(tableHeight)

	showPriority, showDue := m.tableColumnsConfig()

	const (
		widthPriority = 12
		widthLead     = 8
		widthStatus   = 10
		widthDue      = 8
		minNameWidth  = 12
	)

	// Tally the fixed-width columns first so Name can take whatever remains.
	fixedUsed := widthLead + widthStatus
	numCols := 3 // Name + Lead + Status
	if showPriority {
		fixedUsed += widthPriority
		numCols++
	}
	if showDue {
		fixedUsed += widthDue
		numCols++
	}

	// Per-column padding consumes space in every row (1 char left, 1 char right per column).
	overhead := numCols * 2
	nameWidth := m.width - fixedUsed - overhead
	if nameWidth < minNameWidth {
		nameWidth = minNameWidth
	}

	cols := []table.Column{
		table.Column{Title: "Name", Width: nameWidth},
	}
	if showPriority {
		cols = append(cols, table.Column{Title: "Priority", Width: widthPriority})
	}
	cols = append(cols,
		table.Column{Title: "Lead", Width: widthLead},
		table.Column{Title: "Status", Width: widthStatus},
	)
	if showDue {
		cols = append(cols, table.Column{Title: "Due", Width: widthDue})
	}

	tableWidth := nameWidth + fixedUsed + overhead
	m.table.SetWidth(tableWidth)

	cursor := m.table.Cursor()
	m.table.SetRows(nil) // prevent bubbles/table looping old rows against new columns
	m.table.SetColumns(cols)
	m.populateTableRows()
	if cursor >= 0 {
		m.table.SetCursor(cursor)
	}
}

// recalculateViewportLayout fits the viewport to the screen.
func (m *Model) recalculateViewportLayout() {
	m.viewport.Width = m.width
	metaH := 0
	if m.recallResult.Frontmatter.ID != "" {
		metaH = lipgloss.Height(m.renderDetailMetadata())
	}
	footerH := m.footerHeight(ViewDetail)
	viewportHeight := m.height - 4 - metaH - footerH
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	m.viewport.Height = viewportHeight
	m.viewport.SetYOffset(m.viewport.YOffset)
}

// recalculateArtifactViewportLayout fits artifact content to its simpler screen
// chrome. It is separate from the detail viewport both for sizing and state.
func (m *Model) recalculateArtifactViewportLayout() {
	m.artifactViewport.Width = m.width
	footerH := m.footerHeight(ViewArtifactContent)
	artifactHeight := m.height - 4 - footerH
	if artifactHeight < 3 {
		artifactHeight = 3
	}
	m.artifactViewport.Height = artifactHeight
	m.artifactViewport.SetYOffset(m.artifactViewport.YOffset)
}

// recalculateConflictViewportLayout fits the conflict viewport to the screen.
func (m *Model) recalculateConflictViewportLayout() {
	m.conflictViewport.Width = m.width - 6
	m.conflictViewport.Height = m.height - 17
	if m.conflictViewport.Height < 3 {
		m.conflictViewport.Height = 3
	}
}

func (m Model) renderStatusPicker() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Select new status for %s:\n\n", m.targetName))

	for i, opt := range m.statusOptions {
		cursor := "  "
		if i == m.statusCursor {
			cursor = "> "
		}

		statusStr := string(opt)
		var style lipgloss.Style
		switch opt {
		case core.StatusSpark:
			style = statusSparkStyle
		case core.StatusDefine, core.StatusActive:
			style = statusDefineStyle
		case core.StatusDelegated, core.StatusWaiting:
			style = statusDelegatedStyle
		case core.StatusReview:
			style = statusReviewStyle
		case core.StatusBlocked:
			style = statusBlockedStyle
		case core.StatusDone, core.StatusResolved, core.StatusArchived:
			style = statusDoneStyle
		}

		if i == m.statusCursor {
			sb.WriteString(focusedItemStyle.Render(fmt.Sprintf("%s%s", cursor, statusStr)))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s", cursor, style.Render(statusStr)))
		}
		sb.WriteString("\n")
	}

	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderNextActionEditor() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Edit Next Action for %s:\n\n", m.targetName))
	sb.WriteString(m.nextActionInput.View())
	sb.WriteString("\n\n")
	sb.WriteString("press enter to save • esc to cancel")
	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderLeadEditor() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Assigning %s\n\n", lipgloss.NewStyle().Foreground(vibrantGreen).Bold(true).Render(m.targetName)))
	sb.WriteString("Lead (full name):\n")
	sb.WriteString(m.leadInput.View())
	if m.leadSuggestionText != "" {
		sb.WriteString(fmt.Sprintf("  suggestion: %s (tab/enter)", mutedStyle.Render(m.leadSuggestionText)))
	}
	sb.WriteString("\n\n")
	sb.WriteString("press enter to save • esc to cancel")
	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderPriorityEditor() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Edit Priority & Due Date for %s:\n\n", m.targetName))

	// Priority
	sb.WriteString(" Priority: ")
	var priorityOptions []string
	for _, option := range []core.Priority{core.PriorityLow, core.PriorityMedium, core.PriorityHigh, core.PriorityMax} {
		value := string(option)
		if option == m.editPriority {
			value = activeOptionStyle.Render("[" + value + "]")
		} else {
			value = " " + value + " "
		}
		priorityOptions = append(priorityOptions, value)
	}
	priorityRow := strings.Join(priorityOptions, " ")
	if m.priorityFocus == 0 {
		sb.WriteString(focusedItemStyle.Render(priorityRow))
	} else {
		sb.WriteString(priorityRow)
	}
	sb.WriteString("\n\n")

	// Due Date
	sb.WriteString(" Due Date:   ")
	if m.priorityFocus == 1 {
		sb.WriteString(m.dueDateInput.View())
	} else {
		val := m.dueDateInput.Value()
		if val == "" {
			val = "(empty)"
		}
		sb.WriteString(metaValueStyle.Render(val))
	}
	sb.WriteString("\n\n")

	// Buttons
	sb.WriteString("press enter to save • esc to cancel")

	return editorBoxStyle.Render(sb.String())
}

func nextInterfaceFilter(current interfaceFilter, configured ...[]string) interfaceFilter {
	values := core.DefaultDiscussionInterfaces()
	if len(configured) > 0 {
		values = configured[0]
	}
	options := make([]interfaceFilter, 0, len(values)+1)
	options = append(options, "")
	for _, value := range values {
		options = append(options, interfaceFilter(value))
	}
	for i, option := range options {
		if option == current {
			return options[(i+1)%len(options)]
		}
	}
	return options[0]
}

func (m Model) renderLeadSelector() string {
	var sb strings.Builder
	sb.WriteString("Filters — scope the dashboard before a meeting.\n\n")
	sb.WriteString(m.leadSearch.View())
	sb.WriteString("\n\n")

	if m.loading && len(m.items) == 0 {
		sb.WriteString(" Loading leads…\n")
		return editorBoxStyle.Render(sb.String())
	}

	if len(m.leadResults) == 0 {
		sb.WriteString(subtitleStyle.Render(" No leads match your search.\n"))
		sb.WriteString("\n")
		sb.WriteString("type to refine • esc to show all • q to quit")
		return editorBoxStyle.Render(sb.String())
	}

	// Render only the window of options around the cursor so a long lead list
	// scrolls instead of overflowing the screen.
	start, end := m.leadWindow()
	if start > 0 {
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↑ %d more above\n", start)))
	}
	for i := start; i < end; i++ {
		opt := m.leadResults[i]
		cursor := "  "
		if i == m.leadCursor {
			cursor = "> "
		}

		noun := "dossiers"
		if opt.count == 1 {
			noun = "dossier"
		}
		line := fmt.Sprintf("%-24s %d %s", opt.filter.label(), opt.count, noun)
		if i == m.leadCursor {
			sb.WriteString(focusedItemStyle.Render(cursor + line))
		} else {
			sb.WriteString(cursor + line)
		}
		sb.WriteString("\n")
	}
	if end < len(m.leadResults) {
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↓ %d more below\n", len(m.leadResults)-end)))
	}

	sb.WriteString("\n")
	sb.WriteString("type to search • ↑/↓ to move • enter to open • esc to show all")
	return editorBoxStyle.Render(sb.String())
}

// leadVisibleRows is how many option rows the lead selector shows at once,
// derived from the terminal height. Remaining rows scroll into view with the
// cursor. The constant reserves space for the screen chrome (title, subtitle,
// box padding, intro line, search box, the two "more" indicators, help, footer).
func (m Model) leadVisibleRows() int {
	const chrome = 14
	rows := m.height - chrome
	if rows < 3 {
		rows = 3
	}
	return rows
}

// leadWindow returns the [start, end) slice of leadResults to render, scrolled so
// the cursor stays visible and roughly centered within the available height.
func (m Model) leadWindow() (start, end int) {
	return centeredWindow(len(m.leadResults), m.leadCursor, m.leadVisibleRows())
}

// artifactVisibleRows bounds the evidence index to the terminal height. The
// reserved chrome covers title/subtitle, both possible clipping indicators,
// spacing, and the footer.
func (m Model) artifactVisibleRows() int {
	const chrome = 7
	rows := m.height - chrome
	if rows < 1 {
		rows = 1
	}
	return rows
}

// artifactWindow returns the visible evidence-index slice with the cursor kept
// in view. It deliberately shares the lead selector's centering behavior.
func (m Model) artifactWindow() (start, end int) {
	return centeredWindow(len(m.artifactIndex), m.artifactCursor, m.artifactVisibleRows())
}

func centeredWindow(n, cursor, height int) (start, end int) {
	if n == 0 || height <= 0 {
		return 0, 0
	}
	if height >= n {
		return 0, n
	}
	if cursor < 0 {
		cursor = 0
	} else if cursor >= n {
		cursor = n - 1
	}
	start = cursor - height/2
	if start < 0 {
		start = 0
	}
	end = start + height
	if end > n {
		end = n
		start = end - height
	}
	return start, end
}

func renderArtifactContent(content core.ArtifactContent) string {
	header := fmt.Sprintf("Artifact: %s (%s)\nType: %s  Lines: %d-%d of %d\n\n",
		content.Title, content.ID, content.Type, content.StartLine, content.EndLine, content.Lines)
	return header + content.Content
}

func (m Model) renderLinkInput() string {
	var sb strings.Builder
	sb.WriteString("Link Session Content:\n\n")
	sb.WriteString("Enter raw content or description to link to a dossier:\n\n")
	sb.WriteString(m.linkTextInput.View())
	sb.WriteString("\n\n")
	sb.WriteString("press enter to analyze matches • esc to cancel")
	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderLinkSelector() string {
	var sb strings.Builder
	sb.WriteString("Ambiguous Link Targets:\n")
	sb.WriteString("Multiple dossiers match. Select target to confirm link:\n\n")

	for i, sug := range m.linkSuggestions {
		cursor := "  "
		if i == m.linkCursor {
			cursor = "> "
		}

		sugLine := fmt.Sprintf("%-20s (Confidence: %-7s) - Reason: %s", sug.Name, sug.Confidence, sug.Reason)
		if i == m.linkCursor {
			sb.WriteString(focusedItemStyle.Render(cursor + sugLine))
		} else {
			sb.WriteString(cursor + sugLine)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString("press enter to confirm • esc to cancel")
	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderMergeSelector() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Merge Dossier: %s (Source)\n", m.mergeSourceName))
	sb.WriteString("Choose the surviving TARGET dossier to merge into:\n\n")

	if len(m.mergeTargets) == 0 {
		sb.WriteString(" No other dossiers available to merge into.\n")
	} else {
		for i, tgt := range m.mergeTargets {
			cursor := "  "
			if i == m.mergeCursor {
				cursor = "> "
			}

			tgtLine := fmt.Sprintf("%s (%s) - status: %s", tgt.Name, tgt.ID, tgt.Status)
			if i == m.mergeCursor {
				sb.WriteString(focusedItemStyle.Render(cursor + tgtLine))
			} else {
				sb.WriteString(cursor + tgtLine)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString("press enter to perform merge • esc to cancel")
	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderMergeConflictResolver() string {
	var sb strings.Builder
	sb.WriteString(warningStyle.Render("⚡ MERGE CONFLICT DETECTED\n"))
	sb.WriteString("Divergent distilled states or statuses cannot be merged automatically.\n")
	sb.WriteString("Review the diff below representing incoming source changes against target:\n\n")

	sb.WriteString(m.conflictViewport.View())
	sb.WriteString("\n\n")

	sb.WriteString(subtitleStyle.Render("ℹ Note: Source dossier files are retained and archived, never deleted.\n\n"))

	resolveBtn := "[ Resolve Conflict & Force Merge ]"
	if m.conflictResolverCursor == 0 {
		resolveBtn = focusedItemStyle.Render(resolveBtn)
	}

	cancelBtn := "[ Cancel Merge ]"
	if m.conflictResolverCursor == 1 {
		cancelBtn = focusedItemStyle.Render(cancelBtn)
	}

	sb.WriteString(fmt.Sprintf(" %s   %s", resolveBtn, cancelBtn))

	return editorBoxStyle.Render(sb.String())
}

func (m Model) renderDetailMetadata() string {
	fm := m.recallResult.Frontmatter
	if fm.ID == "" {
		return ""
	}

	var sb strings.Builder

	lblStyle := metaLabelStyle.Copy().
		Width(12).
		Align(lipgloss.Right).
		MarginRight(1)

	valWidth := m.width - 14
	if valWidth < 10 {
		valWidth = 10
	}
	valStyle := metaValueStyle.Copy().Width(valWidth)

	renderRow := func(label, value string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top,
			lblStyle.Render(label),
			valStyle.Render(value),
		) + "\n"
	}

	col1ValWidth := 20
	col1ValStyle := metaValueStyle.Copy().Width(col1ValWidth)

	col2ValWidth := m.width - 14 - 13 - col1ValWidth
	if col2ValWidth < 10 {
		col2ValWidth = 10
	}
	col2ValStyle := metaValueStyle.Copy().Width(col2ValWidth)

	renderTwoCols := func(l1, v1, l2, v2 string) string {
		if m.width < 90 {
			return renderRow(l1, v1) + renderRow(l2, v2)
		}
		col1 := lipgloss.JoinHorizontal(lipgloss.Top,
			lblStyle.Render(l1),
			col1ValStyle.Render(v1),
		)
		col2 := lipgloss.JoinHorizontal(lipgloss.Top,
			lblStyle.Render(l2),
			col2ValStyle.Render(v2),
		)
		return lipgloss.JoinHorizontal(lipgloss.Top, col1, col2) + "\n"
	}

	leadLabel := fm.Lead
	if leadLabel == "" {
		leadLabel = "Unassigned (Me)"
	}

	sb.WriteString(renderRow("Dossier:", fm.Name))
	if fm.Description != "" {
		sb.WriteString(renderRow("Summary:", fm.Description))
	}
	sb.WriteString(renderTwoCols(
		"Status:", string(fm.Status),
		"Lead:", leadLabel,
	))
	sb.WriteString(renderRow("Priority:", string(fm.Priority)))
	sb.WriteString(renderRow("Interfaces:", strings.Join(fm.Interfaces, ", ")))
	sb.WriteString(renderRow("Tokens:", fmt.Sprintf("%d", m.recallResult.TokenEstimate)))
	sb.WriteString(renderRow("Next:", fm.NextAction))

	w := m.width
	if w <= 0 {
		w = 80
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(darkGray).Render(strings.Repeat("─", w)))

	return sb.String()
}

func (m Model) footerContent(v View) string {
	var footerParts []string
	if len(m.warnings) > 0 {
		for _, w := range m.warnings {
			footerParts = append(footerParts, warningStyle.Render(fmt.Sprintf("⚠ %s", w)))
		}
	}

	keyHelp := "↑/↓: select • f: filters • s: status • l: lead • p: priority • n: next action • k: link • m: merge • c: claude"
	switch v {
	case ViewLeadSelector:
		keyHelp = "type: search leads • ↑/↓: select • esc: cancel"
	case ViewDetail:
		keyHelp = "↑/↓/pgup/pgdn: scroll • s: status • l: lead • p: priority • n: next action • a: artifacts • c: claude • esc: back"
	case ViewArtifactIndex:
		keyHelp = "↑/↓: select • enter: view artifact • esc: back"
	case ViewArtifactContent:
		keyHelp = "↑/↓/pgup/pgdn: scroll • esc: back"
	case ViewStatusPicker:
		keyHelp = "↑/↓: select status • esc: cancel"
	case ViewNextActionEditor:
		keyHelp = "esc: cancel"
	case ViewLeadEditor:
		keyHelp = "esc: cancel"
	case ViewPriorityEditor:
		keyHelp = "↑/↓: focus • ←/→: cycle priority • esc: cancel"
	case ViewLinkInput:
		keyHelp = "esc: cancel"
	case ViewLinkSelector:
		keyHelp = "↑/↓: select target dossier • esc: cancel"
	case ViewMergeSelector:
		keyHelp = "↑/↓: select target dossier • esc: cancel"
	case ViewMergeConflictResolver:
		keyHelp = "↑/↓/pgup/pgdn: scroll diff • tab: switch button • esc: cancel"
	}
	footerParts = append(footerParts, keyHelp)

	w := m.width
	if w <= 0 {
		w = 80
	}
	return footerStyle.Width(w).Render(strings.Join(footerParts, " │ "))
}

func (m Model) footerHeight(v View) int {
	h := lipgloss.Height(m.footerContent(v))
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the screen based on state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing TUI..."
	}

	var sb strings.Builder

	// 1. Header Banner
	sb.WriteString(titleStyle.Render(" DOSSIER TUI "))
	sb.WriteString("\n")

	// Check if there is a primary error message to show
	if m.err != nil {
		sb.WriteString(errorStyle.Render(fmt.Sprintf(" Error: %v\n\n", m.err)))
	}

	switch m.currentView {
	case ViewLeadSelector:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Select Lead"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLeadSelector())
		sb.WriteString("\n")

	case ViewDashboard:
		archivedNote := ""
		if m.extrasCount > 0 && !m.extrasExpanded {
			archivedNote = " · resolved/archived hidden"
		}
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" Durable memory layer for agentic workflows — Dashboard · Lead: %s · Interface: %s%s", m.leadFilter.label(), m.interfaceFilter.label(), archivedNote)))
		sb.WriteString("\n\n")

		if m.loading && len(m.items) == 0 {
			sb.WriteString(" Loading dossiers...\n")
		} else if len(m.visibleItems) == 0 && m.extrasCount == 0 {
			sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" No dossiers for lead: %s / interface: %s — press f or i to change filters.\n", m.leadFilter.label(), m.interfaceFilter.label())))
		} else {
			sb.WriteString(m.table.View())
			sb.WriteString("\n")
		}

	case ViewDetail:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Recall Detail"))
		sb.WriteString("\n\n")

		sb.WriteString(m.renderDetailMetadata())
		sb.WriteString("\n")

		// Scrollable viewport
		sb.WriteString(m.viewport.View())
		sb.WriteString("\n")

	case ViewArtifactIndex:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" Durable memory layer for agentic workflows — Evidence Index: %s", m.recallResult.Frontmatter.Name)))
		sb.WriteString("\n\n")
		if len(m.artifactIndex) == 0 {
			sb.WriteString(" No artifacts archived for this dossier.\n")
		} else {
			start, end := m.artifactWindow()
			if start > 0 {
				sb.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↑ %d more above\n", start)))
			}
			for i := start; i < end; i++ {
				a := m.artifactIndex[i]
				cited := "uncited"
				if a.Cited {
					cited = "cited"
				}
				row := fmt.Sprintf("%-28s %-18s %6d lines  %-8s %s", a.ID, a.Type, a.Lines, cited, a.Title)
				if i == m.artifactCursor {
					sb.WriteString(focusedItemStyle.Render("> " + row))
				} else {
					sb.WriteString("  " + row)
				}
				sb.WriteString("\n")
			}
			if end < len(m.artifactIndex) {
				sb.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↓ %d more below\n", len(m.artifactIndex)-end)))
			}
		}

	case ViewArtifactContent:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Artifact"))
		sb.WriteString("\n\n")
		sb.WriteString(m.artifactViewport.View())
		sb.WriteString("\n")

	case ViewStatusPicker:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Update Status"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderStatusPicker())
		sb.WriteString("\n")

	case ViewNextActionEditor:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Update Next Action"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderNextActionEditor())
		sb.WriteString("\n")

	case ViewLeadEditor:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Update Lead"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLeadEditor())
		sb.WriteString("\n")

	case ViewPriorityEditor:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Update Priority"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderPriorityEditor())
		sb.WriteString("\n")

	case ViewLinkInput:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Link Content"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLinkInput())
		sb.WriteString("\n")

	case ViewLinkSelector:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Resolve Ambiguous Link"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLinkSelector())
		sb.WriteString("\n")

	case ViewMergeSelector:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Merge Dossier"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderMergeSelector())
		sb.WriteString("\n")

	case ViewMergeConflictResolver:
		sb.WriteString(subtitleStyle.Render(" Durable memory layer for agentic workflows — Resolve Merge Conflict"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderMergeConflictResolver())
		sb.WriteString("\n")
	}

	// 3. Footer / Help area
	sb.WriteString("\n")
	sb.WriteString(m.footerContent(m.currentView))

	return sb.String()
}

// Run sets up the program, enters the alt-screen, and executes.
//
// NOTE (ADR 0004): the TUI does not resolve or carry a session identity. It is a
// read/edit viewer over the dossier store; the per-session "active" binding (Switch)
// is intentionally not exposed here — see ADR 0004 and BUILD-DECISIONS B9.
func Run(ctx context.Context, svc *core.Service) error {
	m := NewModel(svc)
	if m.watcher != nil {
		defer m.watcher.Close()
	}
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := p.Run()
	return err
}
