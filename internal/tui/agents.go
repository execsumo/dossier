package tui

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"dossier/internal/core"
	"dossier/internal/harness"
)

// The herdr session switcher (ADR 0014). Inside herdr the TUI polls the live
// agent list, joins it against the session bindings, and shows which Dossiers
// have an agent running and whether it needs the user. Nothing is persisted:
// the Dossier→pane mapping is re-derived on every poll.

// agentPollInterval is how often the TUI asks herdr for agent status.
const agentPollInterval = 2 * time.Second

// herdrLaunchGrace is how long a pane this process launched stays in the
// launched map without herdr reporting an agent in it. Detection takes a few
// seconds after launch, so pruning immediately would forget fresh launches.
const herdrLaunchGrace = 30 * time.Second

type herdrLaunch struct {
	dossierID string
	at        time.Time
}

type agentPollTickMsg struct{}

type agentPollMsg struct {
	agents []harness.HerdrAgent
	bound  map[string]string
	err    error
}

// herdrPickMsg carries the outcome of looking for a live agent on target: the
// agent to focus, or nil to open a new session.
type herdrPickMsg struct {
	target targetDossier
	agent  *harness.HerdrAgent
	err    error
}

type herdrFocusMsg struct{ err error }

func agentPollTick() tea.Cmd {
	return tea.Tick(agentPollInterval, func(time.Time) tea.Msg { return agentPollTickMsg{} })
}

// fetchAgents lists herdr's agents and resolves which Dossier each session key
// is bound to. It runs inside tea.Cmds, off the Update goroutine.
func fetchAgents(list func() ([]harness.HerdrAgent, error), svc *core.Service) ([]harness.HerdrAgent, map[string]string, error) {
	agents, err := list()
	if err != nil {
		return nil, nil, err
	}
	keys := make([]string, 0, len(agents))
	for _, a := range agents {
		keys = append(keys, a.SessionKey())
	}
	return agents, svc.SessionDossiers(keys), nil
}

func (m Model) agentPollCmd() tea.Cmd {
	if m.listHerdrAgents == nil {
		return nil
	}
	list, svc := m.listHerdrAgents, m.svc
	return func() tea.Msg {
		agents, bound, err := fetchAgents(list, svc)
		return agentPollMsg{agents: agents, bound: bound, err: err}
	}
}

// launchedSnapshot copies the launched map into the shape MatchHerdrAgents
// takes. Commands run on other goroutines, so they get a copy, never the map.
func (m Model) launchedSnapshot() map[string]string {
	out := make(map[string]string, len(m.herdrLaunched))
	for pane, l := range m.herdrLaunched {
		out[pane] = l.dossierID
	}
	return out
}

func (m Model) recordHerdrLaunch(paneID, dossierID string) {
	if paneID != "" && m.herdrLaunched != nil {
		m.herdrLaunched[paneID] = herdrLaunch{dossierID: dossierID, at: time.Now()}
	}
}

// pruneHerdrLaunched forgets launched panes that no longer host an agent, once
// they are past the detection grace period.
func (m Model) pruneHerdrLaunched(agents []harness.HerdrAgent) {
	live := make(map[string]bool, len(agents))
	for _, a := range agents {
		live[a.PaneID] = true
	}
	for pane, l := range m.herdrLaunched {
		if !live[pane] && time.Since(l.at) > herdrLaunchGrace {
			delete(m.herdrLaunched, pane)
		}
	}
}

// pickHerdrAgentCmd checks herdr for a live agent on t with a fresh list, so
// `c` never acts on a poll that is a couple of seconds old.
func (m Model) pickHerdrAgentCmd(t targetDossier) tea.Cmd {
	list, svc, launched := m.listHerdrAgents, m.svc, m.launchedSnapshot()
	return func() tea.Msg {
		agents, bound, err := fetchAgents(list, svc)
		if err != nil {
			return herdrPickMsg{target: t, err: err}
		}
		matches := harness.MatchHerdrAgents(agents, bound, launched)[t.id]
		if len(matches) == 0 {
			return herdrPickMsg{target: t}
		}
		best := harness.MostRecentAgent(matches)
		return herdrPickMsg{target: t, agent: &best}
	}
}

// cycleAgent focuses the agent of the next (or previous) Dossier in list order
// that has one, starting from the selected Dossier.
func (m Model) cycleAgent(forward bool) (tea.Model, tea.Cmd) {
	var withAgents []int
	current := -1
	selected, _ := m.getTargetDossier()
	for i, item := range m.visibleItems {
		if item.ID == selected.id {
			current = i
		}
		if len(m.agentMatches[item.ID]) > 0 {
			withAgents = append(withAgents, i)
		}
	}
	if len(withAgents) == 0 {
		m.warnings = []core.Warning{core.Warning("no Dossier in this list has a running agent")}
		return m, nil
	}
	next := withAgents[0]
	if forward {
		for _, i := range withAgents {
			if i > current {
				next = i
				break
			}
		}
	} else {
		next = withAgents[len(withAgents)-1]
		for j := len(withAgents) - 1; j >= 0; j-- {
			if withAgents[j] < current {
				next = withAgents[j]
				break
			}
		}
	}
	pane := harness.MostRecentAgent(m.agentMatches[m.visibleItems[next].ID]).PaneID
	focus := m.focusHerdrAgent
	return m, func() tea.Msg { return herdrFocusMsg{err: focus(pane)} }
}

// agentBadges are plain glyphs: table cells are truncated before styling, so
// colour codes cannot be baked into them (see itemTableRow).
var agentBadges = map[string]string{
	"working": "●",
	"blocked": "▲",
	"done":    "✓",
	"idle":    "○",
	"unknown": "?",
}

// agentBadge is the status glyph for a Dossier's live agents, or "" for none.
func (m Model) agentBadge(dossierID string) string {
	agents := m.agentMatches[dossierID]
	if len(agents) == 0 {
		return ""
	}
	return agentBadges[harness.MostUrgentStatus(agents)]
}

// agentBadgeSignature summarizes what the badges show, so a poll that changes
// nothing visible does not rebuild the table rows.
func agentBadgeSignature(matches map[string][]harness.HerdrAgent) string {
	parts := make([]string, 0, len(matches))
	for id, agents := range matches {
		parts = append(parts, id+"="+harness.MostUrgentStatus(agents))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
