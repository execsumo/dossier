package core

import (
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
}

// StatsRow aggregates the sessions that ran one binary version with one
// guide hash. Sessions are counted from session_ended events, so sessions
// that ended before version stamping existed are not included.
type StatsRow struct {
	Version   string    `json:"version"`
	GuideHash string    `json:"guide_hash"`
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
	report := StatsReport{Author: author, AllAuthors: req.AllAuthors}

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
		rk := ss.ended.Version + "\x00" + ss.ended.GuideHash
		row := rows[rk]
		if row == nil {
			row = &StatsRow{Version: ss.ended.Version, GuideHash: ss.ended.GuideHash}
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
