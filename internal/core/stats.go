package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// StatsReq selects the sessions that per-version stats aggregate.
type StatsReq struct {
	// Author limits stats to one author's sessions. Empty means the
	// configured author, unless AllAuthors is set.
	Author     string
	AllAuthors bool
	// Since drops sessions that ended before it. Zero means no limit.
	Since time.Time
	// By lists the dimensions rows are grouped by, from StatsDimensions.
	// Empty means version and guide.
	By []string
}

// Stats grouping dimensions. version/guide are what Dossier controls; model
// and effort are the session's (the user's choice); eval is the evaluator's
// own model/effort, which also moves the score.
const (
	StatsByVersion = "version"
	StatsByGuide   = "guide"
	StatsByModel   = "model"
	StatsByEffort  = "effort"
	StatsByEval    = "eval"
)

// StatsDimensions are the valid StatsReq.By values.
var StatsDimensions = []string{StatsByVersion, StatsByGuide, StatsByModel, StatsByEffort, StatsByEval}

// DefaultStatsBy is the grouping when none is asked for.
var DefaultStatsBy = []string{StatsByVersion, StatsByGuide}

// StatsRow aggregates the sessions that ran one binary version with one
// guide hash. Sessions are counted from session_ended events, so sessions
// that ended before version stamping existed are not included.
type StatsRow struct {
	// Only the dimensions grouped by are set; the rest are empty.
	Version       string `json:"version,omitempty"`
	GuideHash     string `json:"guide_hash,omitempty"`
	SessionModel  string `json:"session_model,omitempty"`
	SessionEffort string `json:"session_effort,omitempty"`
	// EvalSetup is the evaluator's "model/effort" for the session's eval.
	EvalSetup string    `json:"eval_setup,omitempty"`
	First     time.Time `json:"first"`
	Last      time.Time `json:"last"`
	Sessions  int       `json:"sessions"`
	// Unsaved counts sessions with at least one boundary (compaction or end)
	// that found nothing saved since the previous one: the
	// distilled_state_not_captured signal.
	Unsaved int `json:"unsaved"`
	// Saves counts agent saves made during the counted sessions.
	Saves int `json:"saves"`
	// Evals counts scored session evals; EvalSkipped counts recorded skips.
	Evals       int                      `json:"evals"`
	EvalSkipped int                      `json:"eval_skipped"`
	EvalProbes  int                      `json:"eval_probes"`
	EvalPassed  int                      `json:"eval_passed"`
	ByKind      map[string]EvalKindScore `json:"by_kind,omitempty"`
	EvalCostUSD float64                  `json:"eval_cost_usd"`
	// SkipReasons tallies why evals were skipped, so a coverage gap names
	// its cause.
	SkipReasons map[string]int `json:"skip_reasons,omitempty"`
}

// StatsReport is per-version session outcomes, newest version first.
type StatsReport struct {
	By         []string   `json:"by"`
	Author     string     `json:"author,omitempty"`
	AllAuthors bool       `json:"all_authors,omitempty"`
	Rows       []StatsRow `json:"rows"`
}

type statsSession struct {
	ended  AuditEvent
	saves  int
	missed bool
	eval   *EvalSummary
	evalTS time.Time
}

// Stats aggregates everyday-use session outcomes by binary version and guide
// hash, from the audit logs of every Dossier (archived included). It reads
// only synced audit data, so it answers the same on every machine.
func (s *Service) Stats(req StatsReq) (StatsReport, error) {
	author := strings.TrimSpace(req.Author)
	if author == "" && !req.AllAuthors {
		author = s.cfg.Author
	}
	by, err := normalizeStatsBy(req.By)
	if err != nil {
		return StatsReport{}, err
	}
	report := StatsReport{By: by, Author: author, AllAuthors: req.AllAuthors}

	listed, err := s.store.List("all")
	if err != nil {
		return report, err
	}
	sessions := map[string]*statsSession{} // key: dossier \x00 session
	var order []string
	for _, fm := range listed {
		events, err := s.store.ReadAuditLog(fm.ID)
		if err != nil {
			continue
		}
		perDossier := map[string]*statsSession{}
		for _, e := range events {
			if e.Event == AuditEventSessionEnded && e.SessionID != "" {
				if !req.AllAuthors && e.Author != author {
					continue
				}
				if !req.Since.IsZero() && e.TS.Before(req.Since) {
					continue
				}
				ss := perDossier[e.SessionID]
				if ss == nil {
					ss = &statsSession{}
					perDossier[e.SessionID] = ss
					key := fm.ID + "\x00" + e.SessionID
					sessions[key] = ss
					order = append(order, key)
				}
				// A resumed session can end more than once; the latest end
				// names the version it finished under.
				if ss.ended.TS.IsZero() || e.TS.After(ss.ended.TS) {
					ss.ended = e
				}
			}
		}
		for _, e := range events {
			ss := perDossier[e.SessionID]
			if ss == nil {
				continue
			}
			switch {
			case isAgentSave(e):
				ss.saves++
			case e.Event == AuditEventDistilledStateNotCaptured:
				ss.missed = true
			case e.Event == AuditEventSessionEval && e.Eval != nil:
				if ss.eval == nil || e.TS.After(ss.evalTS) {
					ev := *e.Eval
					ss.eval, ss.evalTS = &ev, e.TS
				}
			}
		}
	}

	rows := map[string]*StatsRow{}
	var keys []string
	for _, key := range order {
		ss := sessions[key]
		dims := ss.dimensions()
		var row *StatsRow
		var keyParts []string
		probe := StatsRow{}
		for _, d := range by {
			keyParts = append(keyParts, dims[d])
			switch d {
			case StatsByVersion:
				probe.Version = dims[d]
			case StatsByGuide:
				probe.GuideHash = dims[d]
			case StatsByModel:
				probe.SessionModel = dims[d]
			case StatsByEffort:
				probe.SessionEffort = dims[d]
			case StatsByEval:
				probe.EvalSetup = dims[d]
			}
		}
		rk := strings.Join(keyParts, "\x00")
		if row = rows[rk]; row == nil {
			row = &probe
			rows[rk] = row
			keys = append(keys, rk)
		}
		ts := ss.ended.TS
		if row.First.IsZero() || ts.Before(row.First) {
			row.First = ts
		}
		if ts.After(row.Last) {
			row.Last = ts
		}
		row.Sessions++
		row.Saves += ss.saves
		if ss.missed {
			row.Unsaved++
		}
		if ev := ss.eval; ev != nil {
			row.EvalCostUSD += ev.CostUSD
			if ev.Skipped != "" {
				row.EvalSkipped++
				if row.SkipReasons == nil {
					row.SkipReasons = map[string]int{}
				}
				row.SkipReasons[skipReasonClass(ev.Skipped)]++
				continue
			}
			row.Evals++
			row.EvalProbes += ev.Probes
			row.EvalPassed += ev.Passed
			if row.ByKind == nil {
				row.ByKind = map[string]EvalKindScore{}
			}
			for k, v := range ev.ByKind {
				agg := row.ByKind[k]
				agg.Probes += v.Probes
				agg.Passed += v.Passed
				row.ByKind[k] = agg
			}
		}
	}

	for _, k := range keys {
		report.Rows = append(report.Rows, *rows[k])
	}
	// Newest first: by the last session seen for each version.
	sort.SliceStable(report.Rows, func(i, j int) bool { return report.Rows[i].Last.After(report.Rows[j].Last) })
	return report, nil
}

// skipReasonClass collapses a skip message to its stable prefix so reasons
// with embedded sizes or errors still tally together.
func skipReasonClass(reason string) string {
	if i := strings.IndexAny(reason, "(:"); i > 0 {
		return strings.TrimSpace(reason[:i])
	}
	return reason
}

// dimensions returns this session's value for every StatsDimensions entry.
func (ss *statsSession) dimensions() map[string]string {
	evalSetup := ""
	if ss.eval != nil && ss.eval.Model != "" {
		evalSetup = ss.eval.Model
		if ss.eval.Effort != "" {
			evalSetup += "/" + ss.eval.Effort
		}
	}
	return map[string]string{
		StatsByVersion: ss.ended.Version,
		StatsByGuide:   ss.ended.GuideHash,
		StatsByModel:   ss.ended.SessionModel,
		StatsByEffort:  ss.ended.SessionEffort,
		StatsByEval:    evalSetup,
	}
}

// normalizeStatsBy validates and de-duplicates the grouping dimensions.
func normalizeStatsBy(by []string) ([]string, error) {
	if len(by) == 0 {
		return append([]string{}, DefaultStatsBy...), nil
	}
	valid := map[string]bool{}
	for _, d := range StatsDimensions {
		valid[d] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range by {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			continue
		}
		if !valid[d] {
			return nil, NewError(ErrInvalidFrontmatter, fmt.Sprintf("unknown stats dimension %q (valid: %s)", d, strings.Join(StatsDimensions, ", ")))
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) == 0 {
		return append([]string{}, DefaultStatsBy...), nil
	}
	return out, nil
}
