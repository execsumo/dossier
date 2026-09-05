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

	"dossier/internal/config"
	"dossier/internal/core"
	"dossier/internal/harness"

	"github.com/charmbracelet/bubbles/help"
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
	// ViewEdit is the combined editor: stage, priority, due date, lead and next
	// action on one screen, saved together. It replaces the four single-field
	// screens those fields used to have.
	ViewEdit
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
	// ViewKanban is the second home surface: the same filtered dossier set as the
	// dashboard, laid out as stage columns. It is appended last so the existing
	// iota values stay stable for anything that persisted them.
	ViewKanban
	// ViewRenameSlug is separate from the ordinary metadata editor because a slug
	// change may move the complete backing directory in one store operation. The
	// view also supports an explicit title rename.
	ViewRenameSlug
	// ViewLinks is a contextual overlay over dossier detail. It presents active
	// monitors before passive references while preserving their distinct meaning.
	ViewLinks
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

type interfaceOption struct {
	label string
	value interfaceFilter
}

// Subheadline banner
const subheadline = "Durable episodic memory and delegation layer"
const modalBackgroundHex = "#25243A"

// Styling tokens
var (
	purple          = lipgloss.Color("99")
	lightGray       = lipgloss.Color("7") // Use terminal's standard light gray (ANSI 7)
	darkGray        = lipgloss.Color("8") // Use terminal's standard dark gray/bright black (ANSI 8)
	vibrantGreen    = lipgloss.Color("2") // Use terminal's standard green (ANSI 2)
	vibrantRed      = lipgloss.Color("1") // Use terminal's standard red (ANSI 1)
	warningGold     = lipgloss.Color("3") // Use terminal's standard yellow/gold (ANSI 3)
	modalBackground = lipgloss.Color(modalBackgroundHex)

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
			Foreground(lightGray).
			Align(lipgloss.Right).
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
			Background(modalBackground).
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
	err       error
	prevView  View
	targetID  string
	addedLead string
}
type renameSlugResultMsg struct {
	err      error
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
	interfaces   []string
	baseRevision core.Revision
}

// Model holds the application state.
type Model struct {
	svc          *core.Service
	currentView  View
	overlayBase  View
	overlayStack []View

	// listView records which home surface — the dashboard table or the Kanban
	// board — the user last chose. Every "go back" path returns here rather than
	// hardcoding the dashboard, so opening a dossier from the board and pressing
	// esc lands back on the board.
	listView View

	// Data
	items        []core.ListItem // full dossier list, source of truth
	visibleItems []core.ListItem // items[] narrowed by lead/interface filters and extrasExpanded; what the table shows
	haystacks    []string        // searchable text parallel to items
	liveCount    int             // visibleItems[:liveCount] are tier-0 (live) items; visibleItems[liveCount:] are extras, present only while expanded
	extrasCount  int             // resolved/archived items matching leadFilter but excluded from visibleItems while collapsed
	recallResult core.RecallResult

	// Lead selector state
	leadFilter           leadFilter   // active dashboard scope (defaults to All)
	leadOptions          []leadOption // every selectable lead, counts included
	leadResults          []leadOption // selectable lead options in the filter modal
	leadCursor           int
	filterColumn         int // 0 = Lead, 1 = Interface
	interfaceOptions     []interfaceOption
	interfaceCursor      int
	interfaceFilter      interfaceFilter
	configuredLeads      []string
	configuredInterfaces []string
	leadSearchInput      textinput.Model
	searchInput          textinput.Model
	searchActive         bool
	searchQuery          core.Query

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

	// Combined editor view state. editOriginal is the dossier as it was when the
	// form opened, so save can send only the fields that actually changed.
	editOriginal        targetDossier
	editFocus           editField
	editStatus          core.Status
	editPriority        core.Priority
	editLead            string
	editLeadCustom      bool
	editLeadCustomInput textinput.Model
	editInterfaces      []string
	editInterfaceCursor int
	dueDateInput        textinput.Model
	nextActionInput     textinput.Model
	renameSlugInput     textinput.Model
	renameNameInput     textinput.Model
	renameField         renameField

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

	// Kanban board view state. kanbanColumns is rebuilt by applyFilters — one
	// bucket per canonical stage, holding the same filtered items the dashboard
	// shows — so the board never groups per frame.
	kanbanColumns [][]core.ListItem
	kanbanCol     int
	kanbanRow     int

	// Artifact index / content view state
	artifactIndex      []core.ArtifactSummary
	artifactCursor     int
	artifactContent    core.ArtifactContent
	externalLinkCursor int

	// Cached markdown renderer, rebuilt only when the wrap width changes.
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
	help            help.Model

	// Seams for the configured agent handoff, defaulted in NewModel. They exist
	// so tests can press 'c' without an agent binary on PATH and without ever
	// spawning a process.
	persistConfiguredLead func(string) error
	openWith              string
	planOpenWith          func(string, harness.LaunchRequest) (harness.HandoffPlan, error)
	execProcess           func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	openURL               func(string) tea.Cmd

	watcher      *fsnotify.Watcher
	updateChan   chan string
	watchedPaths map[string]bool
}

// NewModel instantiates the root TUI model.
func NewModel(svc *core.Service) Model {
	return NewModelWithOpenWith(svc, harness.DefaultOpenWith)
}

// NewModelWithOpenWith instantiates the TUI with the configured launch profile.
// The profile name controls the executable and argv construction; its prompt
// remains owned by the profile implementation.
func NewModelWithOpenWith(svc *core.Service, openWith string) Model {
	if strings.TrimSpace(openWith) == "" {
		openWith = harness.DefaultOpenWith
	} else if canonical, err := harness.NormalizeOpenWith(openWith); err == nil {
		openWith = canonical
	}
	columns := []table.Column{
		{Title: "Dossier", Width: 30},
		{Title: "Priority", Width: 12},
		{Title: "Stage", Width: 10},
		{Title: "Lead", Width: 8},
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

	leadSearchInput := textinput.New()
	leadSearchInput.Placeholder = "Search leads…"
	leadSearchInput.Width = 32
	searchInput := textinput.New()
	searchInput.Placeholder = "Search dossiers…"
	searchInput.Width = 40
	renameSlugInput := textinput.New()
	renameSlugInput.Placeholder = "new-canonical-slug"
	renameSlugInput.Width = 48
	renameNameInput := textinput.New()
	renameNameInput.Placeholder = "new-title"
	renameNameInput.Width = 48

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
	helpView := help.New()
	helpView.Styles.ShortKey = lipgloss.NewStyle().Foreground(purple).Bold(true)
	helpView.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lightGray)
	helpView.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(darkGray)
	helpView.Styles.Ellipsis = lipgloss.NewStyle().Foreground(darkGray)
	helpView.Styles.FullKey = lipgloss.NewStyle().Foreground(purple).Bold(true)
	helpView.Styles.FullDesc = lipgloss.NewStyle().Foreground(lightGray)
	helpView.Styles.FullSeparator = lipgloss.NewStyle().Foreground(darkGray)

	return Model{
		svc:                  svc,
		currentView:          ViewDashboard,
		listView:             ViewDashboard,
		overlayBase:          ViewDashboard,
		table:                t,
		viewport:             vp,
		artifactViewport:     avp,
		conflictViewport:     cvp,
		loading:              true,
		leadSearchInput:      leadSearchInput,
		searchInput:          searchInput,
		help:                 helpView,
		renameSlugInput:      renameSlugInput,
		renameNameInput:      renameNameInput,
		configuredLeads:      svc.Leads(),
		configuredInterfaces: svc.Interfaces(),
		watcher:              watcher,
		updateChan:           updateChan,
		watchedPaths:         map[string]bool{},
		openWith:             openWith,
		planOpenWith:         harness.PlanOpenWith,
		persistConfiguredLead: func(name string) error {
			return persistLeadToConfig(filepath.Join(svc.DossierHome(), "config.yaml"), name)
		},
		execProcess: tea.ExecProcess,
		openURL:     launchExternalURL,
	}
}

// persistLeadToConfig expands the machine-local vocabulary only after a user
// explicitly enters a lead in the editor. Configured vocabularies remain
// ordered, so the new value is appended rather than unexpectedly resorted.
func persistLeadToConfig(path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	for _, existing := range cfg.Leads {
		if existing == name {
			return nil
		}
	}
	cfg.Leads = append(cfg.Leads, name)
	if err := cfg.Save(path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
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
			interfaces:   append([]string{}, fm.Interfaces...),
			baseRevision: m.recallResult.Revision,
		}, true
	}

	if m.currentView == ViewKanban {
		item, ok := m.selectedKanbanItem()
		if !ok {
			return targetDossier{}, false
		}
		return targetDossier{
			id:           item.ID,
			name:         item.Name,
			slug:         item.Slug,
			status:       core.Status(item.Status),
			priority:     core.Priority(item.Priority),
			dueDate:      item.DueDate,
			nextAction:   item.NextAction,
			lead:         item.Lead,
			interfaces:   append([]string{}, item.Interfaces...),
			baseRevision: "", // Skip check from the board, as from the dashboard
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
			interfaces:   append([]string{}, item.Interfaces...),
			baseRevision: "", // Skip check from dashboard
		}, true
	}
	return targetDossier{}, false
}

// openInAgent hands the focused Dossier off to a fresh configured agent session.
//
// The TUI still resolves no session identity of its own (ADR 0004) — it mints
// one for a session that does not exist yet, binds the Dossier to that id, and
// then lets the selected launch profile carry it using that agent's mechanism.
//
// The binary is looked up first, before anything is written, so a missing agent
// cannot leave a binding behind for a session that never starts. Every failure
// sets m.err rather than silently doing nothing.
func (m Model) openInAgent(t targetDossier) (tea.Model, tea.Cmd) {
	ctx := context.Background()
	res, err := m.svc.Path(ctx, core.PathReq{ID: t.id})
	if err != nil {
		m.err = err
		return m, nil
	}
	dir, _ := res.Data.(string)
	if dir == "" {
		// Launching with an empty Dir would silently run the agent in the TUI's own
		// working directory against the wrong (or no) Dossier. Fail visibly.
		m.err = fmt.Errorf("could not resolve the directory for dossier %s", t.id)
		return m, nil
	}

	sessionID, err := harness.NewSessionID()
	if err != nil {
		m.err = err
		return m, nil
	}

	slug := t.slug
	if slug == "" {
		slug = filepath.Base(dir)
	}
	plan, err := m.planOpenWith(m.openWith, harness.LaunchRequest{
		SessionID:  sessionID,
		DossierDir: dir,
		Name:       t.name,
		Slug:       slug,
	})
	if err != nil {
		m.err = err
		return m, nil
	}

	if _, err := m.svc.Switch(ctx, core.SwitchReq{
		ID:          t.id,
		SessionID:   sessionID,
		HarnessName: m.openWith,
	}); err != nil {
		m.err = err
		return m, nil
	}

	id, fromView := t.id, m.currentView
	return m, m.execProcess(plan.Command(), func(err error) tea.Msg {
		return agentFinishedMsg{err: err, id: id, fromView: fromView}
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

// applyFilters recomputes both home surfaces from the full set and
// the active lead filter. It is the single choke point that keeps the table rows
// in sync with the filter, so cursor lookups can index visibleItems directly.
// Live (tier-0) items always come first, followed by extras (resolved/archived)
// only while extrasExpanded is set — so visibleItems[:liveCount] is always the
// live set and visibleItems[liveCount:] is always the extras set, regardless of
// expansion state. That invariant lets the toggle row live at a stable row
// index (liveCount) rather than always trailing the last row.
//
// The same pass also rebuilds the Kanban columns. The board shares the lead and
// lead and interface filters but deliberately ignores the extras collapse: a board whose
// Done column hid done work would be lying about the stage it names. Grouping
// here — rather than in the renderer — keeps it O(n) per data/filter change
// instead of per frame.
// setItems replaces the full dossier list and rebuilds the parallel search
// haystacks in the same step. Binding the two together is what lets applyFilters
// index m.haystacks[i] directly: a length check would not catch a same-length
// change (a rename, or one dossier added and one removed in one refresh), so the
// invariant is kept by construction rather than by re-checking at each use.
func (m *Model) setItems(items []core.ListItem) {
	m.items = items
	m.haystacks = make([]string, len(items))
	for i, item := range items {
		m.haystacks[i] = core.Haystack(item)
	}
}

func (m *Model) applyFilters() {
	visible := make([]core.ListItem, 0, len(m.items))
	var extraItems []core.ListItem
	matched := make([]core.ListItem, 0, len(m.items))
	for i, item := range m.items {
		if !m.leadFilter.matches(item) || !m.interfaceFilter.matches(item) {
			continue
		}
		if !m.searchQuery.Matches(m.haystacks[i]) {
			continue
		}
		matched = append(matched, item)
		if statusTier(item.Status) == 1 {
			extraItems = append(extraItems, item)
			continue
		}
		visible = append(visible, item)
	}
	m.liveCount = len(visible)
	m.extrasCount = len(extraItems)
	expanded := m.extrasExpanded || !m.searchQuery.IsEmpty()
	if expanded {
		visible = append(visible, extraItems...)
	}
	m.visibleItems = visible

	m.kanbanColumns = groupByStage(matched)
	m.clampKanbanCursor()
}

func (m *Model) openSelectedDossier() tea.Cmd {
	var item core.ListItem
	var ok bool
	if m.currentView == ViewKanban {
		item, ok = m.selectedKanbanItem()
	} else if m.currentView == ViewDashboard {
		itemIdx, isToggle := m.rowToItemIndex(m.table.Cursor())
		if !isToggle && itemIdx >= 0 && itemIdx < len(m.visibleItems) {
			item, ok = m.visibleItems[itemIdx], true
		}
	}
	if !ok || item.ID == "" {
		return nil
	}
	// Search is an input mode, not a view. Leave it before entering detail so
	// keys such as q are handled by the normal global bindings on return.
	m.searchActive = false
	m.searchInput.Blur()
	m.loading = true
	m.err = nil
	return m.recallDossierCmd(item.ID)
}

// isListView reports whether the current view is a home surface — the dashboard
// table or the Kanban board. Both list the same filtered dossiers, so the
// filter, link, merge and handoff keys behave identically on either.
func (m Model) isListView() bool {
	return m.currentView == ViewDashboard || m.currentView == ViewKanban
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

// openLeadSelector enters the lead filter selector with the cursor parked on
// the currently active filter.
func (m *Model) openLeadSelector() {
	m.previousView = m.currentView
	m.pushOverlay(ViewLeadSelector)

	// Lead search is local to the selector. Do not carry a prior query into a
	// newly opened radio list, where it would make options appear to vanish.
	m.leadSearchInput.SetValue("")
	m.leadSearchInput.Focus()
	m.leadOptions = deriveLeadOptions(m.items, m.configuredLeads)
	m.leadResults = m.leadOptions
	m.leadCursor = 0
	for i, o := range m.leadResults {
		if o.filter == m.leadFilter {
			m.leadCursor = i
			break
		}
	}
	m.interfaceOptions = m.buildInterfaceOptions()
	m.interfaceCursor = 0
	for i, option := range m.interfaceOptions {
		if option.value == m.interfaceFilter {
			m.interfaceCursor = i
			break
		}
	}
	m.filterColumn = 0
}

func (m Model) buildInterfaceOptions() []interfaceOption {
	options := make([]interfaceOption, 0, len(m.configuredInterfaces)+1)
	options = append(options, interfaceOption{label: "All"})
	for _, value := range m.configuredInterfaces {
		options = append(options, interfaceOption{label: value, value: interfaceFilter(value)})
	}
	return options
}

// chooseLead applies the option under the cursor and drops into the dashboard.
func (m *Model) chooseLead() {
	m.leadSearchInput.Blur()
	if m.leadCursor >= 0 && m.leadCursor < len(m.leadResults) {
		m.leadFilter = m.leadResults[m.leadCursor].filter
	}
	if m.interfaceCursor >= 0 && m.interfaceCursor < len(m.interfaceOptions) {
		m.interfaceFilter = m.interfaceOptions[m.interfaceCursor].value
	}
	m.applyFilters()
	m.populateTableRows()
	m.popOverlay()
	if m.currentView != m.listView {
		m.currentView = m.listView
	}
	m.table.SetCursor(0)
	m.table.Focus()
}

func (m *Model) startLinkInput() {
	m.previousView = m.currentView
	m.pushOverlay(ViewLinkInput)
	m.linkTextInput = textinput.New()
	m.linkTextInput.Placeholder = "Enter raw content or description to link"
	m.linkTextInput.Focus()
	m.linkTextInput.Width = 60
}

func (m *Model) startMergeSelector(sourceID, sourceName string) {
	m.previousView = m.currentView
	m.pushOverlay(ViewMergeSelector)
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
		if m.searchActive && m.isListView() {
			switch msg.String() {
			case "ctrl+c":
				// Quit stays reachable from inside the search box; without this
				// the key falls through to searchInput and is swallowed.
				return m, tea.Quit
			case "esc":
				m.searchInput.SetValue("")
				m.searchQuery = core.Query{}
				m.searchActive = false
				m.searchInput.Blur()
				m.applyFilters()
				m.populateTableRows()
				m.recalculateTableLayout()
				m.table.SetCursor(0)
				m.kanbanRow = 0
				m.table.Focus()
				return m, nil
			case "tab":
				m.searchActive = false
				m.searchInput.Blur()
				m.table.Focus()
				m.recalculateTableLayout()
				return m, nil
			case "enter":
				return m, m.openSelectedDossier()
			case "up":
				if m.currentView == ViewDashboard {
					m.table.MoveUp(1)
				} else {
					m.moveKanbanRow(-1)
				}
				return m, nil
			case "down":
				if m.currentView == ViewDashboard {
					m.table.MoveDown(1)
				} else {
					m.moveKanbanRow(1)
				}
				return m, nil
			case "left":
				if m.currentView == ViewKanban {
					m.moveKanbanColumn(-1)
				}
				return m, nil
			case "right":
				if m.currentView == ViewDashboard {
					return m, m.openSelectedDossier()
				}
				m.moveKanbanColumn(1)
				return m, nil
			}
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.searchQuery = core.NewQuery(m.searchInput.Value())
			m.applyFilters()
			m.populateTableRows()
			m.recalculateTableLayout()
			m.table.SetCursor(0)
			m.kanbanRow = 0
			return m, cmd
		}

		// View-specific key overrides
		if msg.String() == "?" && (m.isListView() || m.currentView == ViewDetail || m.currentView == ViewArtifactIndex || m.currentView == ViewArtifactContent || m.currentView == ViewLinks) {
			m.toggleHelp()
			return m, nil
		}

		switch m.currentView {
		case ViewKanban:
			// Only the keys the board owns are intercepted; everything else
			// (s/l/p/n/c/f/i/k/m/r/q) falls through to the global switch so the
			// board and the dashboard offer the same verbs.
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "backspace":
				m.listView = ViewDashboard
				m.currentView = ViewDashboard
				m.table.Focus()
				return m, nil
			case "v":
				// v is the board's view toggle: always return to the dashboard.
				m.listView = ViewDashboard
				m.currentView = ViewDashboard
				m.table.Focus()
				return m, nil
			case "left":
				m.moveKanbanColumn(-1)
				return m, nil
			case "right":
				m.moveKanbanColumn(1)
				return m, nil
			case "up":
				m.moveKanbanRow(-1)
				return m, nil
			case "down":
				m.moveKanbanRow(1)
				return m, nil
			case "enter":
				if item, ok := m.selectedKanbanItem(); ok && item.ID != "" {
					m.loading = true
					m.err = nil
					return m, m.recallDossierCmd(item.ID)
				}
				return m, nil
			}

		case ViewLeadSelector:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.leadSearchInput.Blur()
				// Skip selection: fall through to the dashboard with the current
				// filter (All by default). On the startup landing this means
				// "show everything"; reopened via 'f' it cancels the change.
				m.applyFilters()
				m.populateTableRows()
				m.popOverlay()
				m.table.SetCursor(0)
				m.table.Focus()
				return m, nil
			case "left":
				m.filterColumn = 0
				m.leadSearchInput.Focus()
				return m, nil
			case "right", "tab":
				m.filterColumn = 1
				m.leadSearchInput.Blur()
				return m, nil
			case "up", "ctrl+p":
				if m.filterColumn == 0 && len(m.leadResults) > 0 {
					m.leadCursor = (m.leadCursor - 1 + len(m.leadResults)) % len(m.leadResults)
				} else if len(m.interfaceOptions) > 0 {
					m.interfaceCursor = (m.interfaceCursor - 1 + len(m.interfaceOptions)) % len(m.interfaceOptions)
				}
				return m, nil
			case "down", "ctrl+n":
				if m.filterColumn == 0 && len(m.leadResults) > 0 {
					m.leadCursor = (m.leadCursor + 1) % len(m.leadResults)
				} else if len(m.interfaceOptions) > 0 {
					m.interfaceCursor = (m.interfaceCursor + 1) % len(m.interfaceOptions)
				}
				return m, nil
			case "enter":
				if len(m.leadResults) > 0 {
					m.chooseLead()
				}
				return m, nil
			}
			// The lead column remains a radio list: typing only narrows the
			// available radio options; it never changes the selected filter.
			if m.filterColumn == 0 {
				m.leadSearchInput, cmd = m.leadSearchInput.Update(msg)
				m.leadResults = filterLeadOptions(m.leadOptions, m.leadSearchInput.Value())
				m.leadCursor = 0
			}
			return m, cmd

		case ViewLinkInput:
			switch msg.String() {
			case "esc":
				m.popOverlay()
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
				m.dismissOverlays()
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
				m.dismissOverlays()
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
				m.dismissOverlays()
				return m, nil
			case "tab", "shift+tab":
				m.conflictResolverCursor = (m.conflictResolverCursor + 1) % 2
			case "enter":
				if m.conflictResolverCursor == 0 {
					m.loading = true
					m.err = nil
					return m, m.mergeCmd(m.mergeSourceID, m.mergeTargetID, []string{m.mergeConflict.ID})
				} else {
					m.dismissOverlays()
					return m, nil
				}
			}
			m.conflictViewport, cmd = m.conflictViewport.Update(msg)
			return m, cmd

		case ViewArtifactIndex:
			switch msg.String() {
			case "esc":
				m.popOverlay()
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
				m.popOverlay()
				m.err = nil
				return m, nil
			}
			m.artifactViewport, cmd = m.artifactViewport.Update(msg)
			return m, cmd

		case ViewLinks:
			switch msg.String() {
			case "esc":
				m.popOverlay()
				m.err = nil
				return m, nil
			case "up", "k":
				links := m.externalLinkEntries()
				if len(links) > 0 {
					m.externalLinkCursor = (m.externalLinkCursor - 1 + len(links)) % len(links)
				}
			case "down", "j":
				links := m.externalLinkEntries()
				if len(links) > 0 {
					m.externalLinkCursor = (m.externalLinkCursor + 1) % len(links)
				}
			case "enter":
				return m, m.openSelectedExternalLink()
			}
			return m, nil

		case ViewEdit:
			return m.updateEditor(msg)
		case ViewRenameSlug:
			return m.updateSlugRename(msg)
		}

		// Global keys for Dashboard and Detail Views
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "backspace", "left":
			switch m.currentView {
			case ViewDetail:
				m.currentView = m.listView
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
		case "enter", "right":
			if m.currentView == ViewDashboard {
				itemIdx, isToggle := m.rowToItemIndex(m.table.Cursor())
				if isToggle {
					// The "Show More.../Hide Extras..." row, between live items and extras.
					m.extrasExpanded = !m.extrasExpanded
					m.applyFilters()
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
		case "r":
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				m.startSlugRename()
				return m, nil
			}
		case "e":
			// One key for every field a dossier is triaged by. The four
			// single-field screens this replaced are gone, not hidden.
			if t, ok := m.getTargetDossier(); ok && t.id != "" {
				m.startEdit(t)
				return m, nil
			}
		case "o":
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
			if m.isListView() || m.currentView == ViewDetail {
				if t, ok := m.getTargetDossier(); ok && t.id != "" {
					return m.openInAgent(t)
				}
			}
		case "a":
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				m.loading = true
				m.err = nil
				return m, m.listArtifactsCmd(m.recallResult.Frontmatter.ID)
			}
		case "l":
			if m.currentView == ViewDetail && m.recallResult.Frontmatter.ID != "" {
				m.externalLinkCursor = 0
				m.pushOverlay(ViewLinks)
				return m, nil
			}
		case "m":
			// Dashboard only, for the same reason as k, and because a merge is
			// consequential enough to deserve the deliberate surface.
			if m.currentView == ViewDashboard {
				if t, ok := m.getTargetDossier(); ok && t.id != "" {
					m.startMergeSelector(t.id, t.name)
					return m, nil
				}
			}
		case "f":
			if m.isListView() {
				m.openLeadSelector()
				return m, nil
			}
		case "/", "ctrl+f":
			if m.isListView() {
				m.searchActive = true
				m.searchInput.Focus()
				m.recalculateTableLayout()
				return m, nil
			}
		case "k":
			if m.currentView == ViewDashboard || m.currentView == ViewDetail {
				m.startLinkInput()
				return m, nil
			}
		case "v":
			// The board leg is handled in its view-specific block. From detail,
			// v is the same back-to-previous-view action as esc/left.
			if m.currentView == ViewDashboard {
				m.listView = ViewKanban
				m.currentView = ViewKanban
				return m, nil
			}
			if m.currentView == ViewDetail {
				m.currentView = m.listView
				m.warnings = nil
				m.err = nil
				m.table.Focus()
				return m, m.listDossiersCmd()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
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

		m.setItems(msg)

		// Re-derive lead options on every refresh so newly-assigned leads appear,
		// while preserving the active filter (and the search box) across hot-reloads.
		m.leadOptions = deriveLeadOptions(m.items, m.configuredLeads)
		m.leadResults = m.leadOptions
		if m.leadCursor >= len(m.leadResults) {
			m.leadCursor = len(m.leadResults) - 1
		}
		if m.leadCursor < 0 {
			m.leadCursor = 0
		}

		m.applyFilters()
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
			m.pushOverlay(ViewArtifactIndex)
			m.artifactIndex = msg.index
			m.artifactCursor = 0
			m.err = nil
		}

	case artifactContentMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.pushOverlay(ViewArtifactContent)
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
					m.overlayStack[len(m.overlayStack)-1] = ViewLinkSelector
					m.currentView = ViewLinkSelector
					m.linkSuggestions = suggestions
					m.linkContent = msg.content
					m.linkCursor = 0
					return m, nil
				}
			}
			m.err = msg.err
			m.dismissOverlays()
		} else {
			m.dismissOverlays()
			m.err = nil
			return m, m.listDossiersCmd()
		}

	case linkConfirmResultMsg:
		m.loading = false
		m.dismissOverlays()
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
					m.overlayStack[len(m.overlayStack)-1] = ViewMergeConflictResolver
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
			m.dismissOverlays()
		} else {
			m.dismissOverlays()
			m.err = nil
			// Show success info
			return m, m.listDossiersCmd()
		}

	case renameSlugResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.currentView = ViewRenameSlug
		} else {
			m.renameSlugInput.Blur()
			m.renameNameInput.Blur()
			m.dismissOverlays()
			m.currentView = ViewDetail
			m.err = nil
			return m, m.recallDossierCmd(msg.targetID)
		}

	case mutationResultMsg:
		m.loading = false
		if m.currentView == ViewEdit && m.hasOverlay() {
			m.popOverlay()
		}
		if msg.err != nil {
			m.err = msg.err
			m.currentView = msg.prevView
		} else {
			m.currentView = msg.prevView
			m.err = nil
			if msg.addedLead != "" && !m.isConfiguredLead(msg.addedLead) {
				m.configuredLeads = append(m.configuredLeads, msg.addedLead)
			}
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

	case agentFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// The agent almost certainly wrote to the Dossier while the TUI was
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
		if (m.currentView == ViewDetail || m.currentView == ViewLinks) && m.recallResult.Frontmatter.ID != "" {
			m.loading = true
			cmds = append(cmds, m.recallDossierCmd(m.recallResult.Frontmatter.ID))
		} else if m.isListView() || m.currentView == ViewLeadSelector {
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

// agentFinishedMsg reports that a handed-off agent session has exited and
// the TUI has the terminal back. fromView records where 'c' was pressed so the
// right view is refreshed.
type agentFinishedMsg struct {
	err      error
	id       string
	fromView View
}

// tableColumnsConfig reports which width-sensitive columns the dashboard shows.
// Dossier, Stage, and Lead are always present; Priority and Due are revealed
// progressively as the terminal widens.
func (m *Model) tableColumnsConfig() (showPriority, showDue bool) {
	w := m.width
	if w < 44 {
		w = 44
	}
	return w >= 55, w >= 65
}

// itemTableRow builds a single dossier row. Cells mirror the column order
// Dossier, [Priority], Stage, Lead, [Due]; the optional cells are included only
// when the corresponding column is shown.
func itemTableRow(item core.ListItem, showPriority, showDue bool) table.Row {
	if item.ID == "" {
		row := table.Row{item.Name}
		if showPriority {
			row = append(row, "")
		}
		row = append(row, "", "") // Stage, Lead
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
	row = append(row, item.Status, leadStr)
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
	row = append(row, "", "") // Stage, Lead
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
		rows = append(rows, extrasToggleTableRow(m.extrasExpanded || !m.searchQuery.IsEmpty(), showPriority, showDue))
	}
	for _, item := range m.visibleItems[m.liveCount:] {
		rows = append(rows, itemTableRow(item, showPriority, showDue))
	}

	m.table.SetRows(rows)
}

// recalculateTableLayout fits the table to the screen size. Columns appear in
// the canonical order Dossier, Priority, Stage, Lead, Due; Dossier is always present
// and flexes to absorb the leftover width, while the fixed-width columns are
// revealed progressively as the terminal widens.
func (m *Model) recalculateTableLayout() {
	footerH := m.footerHeight(ViewDashboard)
	searchH := 0
	if m.searchBarVisible() {
		searchH = 1
	}
	tableHeight := m.height - 4 - footerH - searchH
	if tableHeight < 3 {
		tableHeight = 3
	}
	m.table.SetHeight(tableHeight)

	showPriority, showDue := m.tableColumnsConfig()

	const (
		widthPriority = 12
		widthStage    = 10
		widthLead     = 8
		widthDue      = 8
		minNameWidth  = 12
	)

	// Tally the fixed-width columns first so Dossier can take whatever remains.
	fixedUsed := widthStage + widthLead
	numCols := 3 // Dossier + Stage + Lead
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
		table.Column{Title: "Dossier", Width: nameWidth},
	}
	if showPriority {
		cols = append(cols, table.Column{Title: "Priority", Width: widthPriority})
	}
	cols = append(cols,
		table.Column{Title: "Stage", Width: widthStage},
		table.Column{Title: "Lead", Width: widthLead},
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

func (m Model) renderLeadSelector() string {
	var sb strings.Builder
	sb.WriteString("Filters — scope the dashboard before a meeting.\n\n")

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
	sb.WriteString("↑/↓ move • enter apply • esc cancel")
	return editorBoxStyle.Render(sb.String())
}

// leadVisibleRows is how many option rows the lead selector shows at once,
// derived from the terminal height. Remaining rows scroll into view with the
// cursor. The constant reserves space for the screen chrome (title, subtitle,
// box padding, intro line, search box, the two "more" indicators, help, footer).
func (m Model) leadVisibleRows() int {
	chrome := 14
	if m.hasOverlay() {
		chrome = 18
	}
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
	const chrome = 6
	rows := m.height - chrome - m.footerHeight(ViewArtifactIndex)
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

			tgtLine := fmt.Sprintf("%s (%s) - stage: %s", tgt.Name, tgt.ID, tgt.Status)
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
	sb.WriteString(renderRow("Slug:", fm.Slug))
	if fm.Description != "" {
		sb.WriteString(renderRow("Summary:", fm.Description))
	}
	sb.WriteString(renderTwoCols(
		"Stage:", string(fm.Status),
		"Priority:", string(fm.Priority),
	))
	sb.WriteString(renderTwoCols(
		"Lead:", leadLabel,
		"Interfaces:", strings.Join(fm.Interfaces, ", "),
	))
	if fm.DueDate != "" {
		sb.WriteString(renderRow("Due:", fm.DueDate))
	}
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

	if m.searchActive && m.isListView() {
		footerParts = append(footerParts, "type: filter • tab: keep filter • esc: clear")
	} else {
		w := m.width
		if w <= 0 {
			w = 80
		}
		m.help.Width = w
		footerParts = append(footerParts, m.help.View(m.helpKeyMap(v)))
	}

	w := m.width
	if w <= 0 {
		w = 80
	}
	return footerStyle.Width(w).Render(strings.Join(footerParts, "\n"))
}

// toggleHelp changes the Bubbles help mode and immediately refits every
// surface whose usable height depends on the footer.
func (m *Model) toggleHelp() {
	m.help.ShowAll = !m.help.ShowAll
	m.recalculateTableLayout()
	m.recalculateViewportLayout()
	m.recalculateArtifactViewportLayout()
	m.recalculateConflictViewportLayout()
}

func (m Model) footerHeight(v View) int {
	h := lipgloss.Height(m.footerContent(v))
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) searchBarVisible() bool {
	return m.searchActive || !m.searchQuery.IsEmpty()
}

func (m Model) renderSearchBar() string {
	return " Search: " + m.searchInput.View()
}

// renderListSubtitle fits a home surface's subtitle line to the terminal.
//
// Nothing else constrains it, and the dashboard and board subtitles are the two
// that grow with state (filter labels, the extras note, the stage window). Past
// the terminal width a real terminal soft-wraps them onto a second row and
// pushes the footer off the bottom of the screen — a wrap lipgloss never emits
// as a "\n", so line-count assertions cannot see it. Truncation happens on the
// plain text so the ellipsis lands inside the styled span instead of cutting an
// ANSI sequence in half.
func (m Model) renderListSubtitle(text string) string {
	if m.width <= 0 {
		return subtitleStyle.Render(text)
	}
	return subtitleStyle.Render(truncateCell(text, m.width))
}

// emptyListNotice is the message a home surface shows when nothing survives the
// active search or filters. Both surfaces share it so the wording — and the way
// it is fitted to the terminal — cannot drift between the dashboard and board.
func (m Model) emptyListNotice() string {
	if !m.searchQuery.IsEmpty() {
		return m.renderListSubtitle(fmt.Sprintf(" No dossiers match %q — esc to clear", m.searchInput.Value()))
	}
	return m.renderListSubtitle(fmt.Sprintf(" No dossiers for lead: %s / interface: %s — press f to change filters.", m.leadFilter.label(), m.interfaceFilter.label()))
}

// pinEmptyNotice writes notice over the first body line of an already rendered
// surface, leaving its first headerLines lines untouched. Headers are the frame
// the user is searching within: a query that matches nothing should empty the
// body, not erase the columns that explain what the body would have held.
func pinEmptyNotice(view, notice string, headerLines int) string {
	lines := strings.Split(view, "\n")
	if len(lines) <= headerLines {
		return view + "\n" + notice
	}
	lines[headerLines] = notice
	return strings.Join(lines, "\n")
}

// View renders the base screen and any contextual overlays based on state.
func (m Model) View() string {
	if m.hasOverlay() {
		return m.renderLayeredView()
	}
	return m.renderNormalView()
}

// renderNormalView renders a single full-screen surface. Overlay rendering
// calls this with the underlying base view so the parent remains visible.
func (m Model) renderNormalView() string {
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
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Select Lead", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLeadSelector())
		sb.WriteString("\n")

	case ViewDashboard:
		archivedNote := ""
		if m.extrasCount > 0 && !m.extrasExpanded {
			archivedNote = " · resolved/archived hidden"
		}
		searchNote := ""
		if !m.searchQuery.IsEmpty() {
			searchNote = fmt.Sprintf(" · Search: %q", m.searchInput.Value())
		}
		sb.WriteString(m.renderListSubtitle(fmt.Sprintf(" %s — Dashboard · Lead: %s · Interface: %s%s%s", subheadline, m.leadFilter.label(), m.interfaceFilter.label(), archivedNote, searchNote)))
		sb.WriteString("\n\n")
		if m.searchBarVisible() {
			sb.WriteString(m.renderSearchBar())
			sb.WriteString("\n")
		}

		if m.loading && len(m.items) == 0 {
			sb.WriteString(" Loading dossiers...\n")
		} else {
			view := m.table.View()
			if len(m.visibleItems) == 0 && m.extrasCount == 0 {
				view = pinEmptyNotice(view, m.emptyListNotice(), 1)
			}
			// The newline stays outside the fitted text so a width cut can never
			// eat it and collapse the layout.
			sb.WriteString(view)
			sb.WriteString("\n")
		}

	case ViewKanban:
		stageNote := ""
		stages := core.CanonicalStatuses()
		if start, end := m.kanbanStageWindow(); end-start < len(stages) {
			stageNote = fmt.Sprintf(" · stages %d–%d of %d", start+1, end, len(stages))
		}
		searchNote := ""
		if !m.searchQuery.IsEmpty() {
			searchNote = fmt.Sprintf(" · Search: %q", m.searchInput.Value())
		}
		sb.WriteString(m.renderListSubtitle(fmt.Sprintf(" %s — Board · Lead: %s · Interface: %s%s%s", subheadline, m.leadFilter.label(), m.interfaceFilter.label(), stageNote, searchNote)))
		sb.WriteString("\n\n")
		if m.searchBarVisible() {
			sb.WriteString(m.renderSearchBar())
			sb.WriteString("\n")
		}

		if m.loading && len(m.items) == 0 {
			sb.WriteString(" Loading dossiers...\n")
		} else {
			view := m.renderKanban()
			if m.kanbanIsEmpty() {
				// Two lines of frame per column: the stage label and its rule.
				view = pinEmptyNotice(view, m.emptyListNotice(), 2)
			}
			// The newline stays outside the fitted text so a width cut can never
			// eat it and collapse the layout.
			sb.WriteString(view)
			sb.WriteString("\n")
		}

	case ViewDetail:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Recall Detail", subheadline)))
		sb.WriteString("\n\n")

		sb.WriteString(m.renderDetailMetadata())
		sb.WriteString("\n")

		// Scrollable viewport
		sb.WriteString(m.viewport.View())
		sb.WriteString("\n")

	case ViewArtifactIndex:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Evidence Index: %s", subheadline, m.recallResult.Frontmatter.Name)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderArtifactIndexBody())
		sb.WriteString("\n")

	case ViewArtifactContent:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Artifact", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.artifactViewport.View())
		sb.WriteString("\n")

	case ViewEdit:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Edit", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderEditor())
		sb.WriteString("\n")

	case ViewRenameSlug:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Rename", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderSlugRename())
		sb.WriteString("\n")

	case ViewLinkInput:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Link Content", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLinkInput())
		sb.WriteString("\n")

	case ViewLinkSelector:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Resolve Ambiguous Link", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderLinkSelector())
		sb.WriteString("\n")

	case ViewMergeSelector:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Merge Dossier", subheadline)))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderMergeSelector())
		sb.WriteString("\n")

	case ViewMergeConflictResolver:
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf(" %s — Resolve Merge Conflict", subheadline)))
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
func Run(ctx context.Context, svc *core.Service, openWith ...string) error {
	configured := harness.DefaultOpenWith
	if len(openWith) > 0 && strings.TrimSpace(openWith[0]) != "" {
		configured = openWith[0]
	}
	m := NewModelWithOpenWith(svc, configured)
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
