package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"dossier/internal/core"
	"dossier/internal/evaluator"

	"github.com/spf13/cobra"
)

// effectiveVersion is the version stamped on session metrics (ADR 0016). A
// release build carries its -ldflags tag. An unstamped build reports
// "dev+<rev>" (with "-dirty" for uncommitted changes) from Go's VCS build
// info, so local dogfood builds stay distinguishable instead of pooling under
// one "dev" row.
func effectiveVersion() string {
	if Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return Version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	v := "dev+" + rev
	if dirty {
		v += "-dirty"
	}
	return v
}

// evalDir is the machine-local home of eval detail and the background log.
// local/ is gitignored by Team Sync, so probe text never leaves the machine.
func evalDir(home string) string {
	return filepath.Join(home, "local", "evals")
}

// spawnDetached starts the background eval; tests replace it.
var spawnDetached = evaluator.SpawnDetached

// startSessionEval records the session end and, when an eval is due, starts
// it detached so the hook returns at once. Failures are reported, never fatal:
// the session itself is already archived.
func startSessionEval(out io.Writer, svc *core.Service, home, sessionID string) {
	if err := svc.RecordSessionEnded(sessionID); err != nil {
		fmt.Fprintf(out, "Warning: could not record session end for stats: %v\n", err)
	}
	dossierID, due := svc.SessionEvalDue(sessionID)
	if !due {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(out, "Warning: session eval not started: %v\n", err)
		return
	}
	args := []string{"--home", home, "eval", "run", "--dossier", dossierID, "--session", sessionID}
	if err := spawnDetached(exe, args, filepath.Join(evalDir(home), "eval.log")); err != nil {
		fmt.Fprintf(out, "Warning: session eval not started: %v\n", err)
		return
	}
	fmt.Fprintf(out, "Session eval started in the background (turn off with eval.enabled: false in config.yaml).\n")
}

func newEvalCmd() *cobra.Command {
	evalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Automatic session evals (resumption fidelity of real sessions)",
	}
	var dossierFlag, sessionFlagLocal string
	var asJSON bool
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Score one ended session: can a fresh agent recover what it established from the Distilled State?",
		Long: "Runs the three model calls of a session eval (extract probes from the session transcript, answer them from the current Distilled State, judge the answers) and records the result.\n" +
			"Session-end hooks start this automatically when eval.enabled is on. Running it by hand works regardless of the knob.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dossierFlag == "" || sessionFlagLocal == "" {
				return fmt.Errorf("--dossier and --session are required")
			}
			home := resolveHomeDir()
			svc, err := wire(home)
			if err != nil {
				return err
			}
			started := time.Now()
			res, evalErr := svc.EvaluateSession(context.Background(), dossierFlag, sessionFlagLocal)
			if res.DossierID != "" {
				if err := writeEvalDetail(home, res); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: eval detail not written: %v\n", err)
				}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s session %s (%s, guide %s): %s [%s]\n",
					started.Format(time.RFC3339), sessionFlagLocal, res.Version, res.GuideHash, evalOutcome(res.Summary), time.Since(started).Round(time.Second))
			}
			return evalErr
		},
	}
	runCmd.Flags().StringVar(&dossierFlag, "dossier", "", "Dossier ID or slug")
	runCmd.Flags().StringVar(&sessionFlagLocal, "session", "", "Session ID")
	runCmd.Flags().BoolVar(&asJSON, "json", false, "Output the full result as JSON")
	evalCmd.AddCommand(runCmd)
	return evalCmd
}

func evalOutcome(s core.EvalSummary) string {
	if s.Skipped != "" {
		return "skipped: " + s.Skipped
	}
	return fmt.Sprintf("%d/%d probes recovered (%s), $%.4f", s.Passed, s.Probes, pct(s.Passed, s.Probes), s.CostUSD)
}

// writeEvalDetail keeps probe text, answers and judge reasons machine-local.
func writeEvalDetail(home string, res core.EvalResult) error {
	dir := filepath.Join(evalDir(home), res.DossierID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sanitizeFileName(res.SessionID)+".json"), b, 0644)
}

func sanitizeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == 0 {
			return '_'
		}
		return r
	}, s)
}

func newStatsCmd() *cobra.Command {
	var allAuthors, asJSON bool
	var author, since string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Everyday-use session outcomes and eval scores by version",
		Long: "Aggregates session outcomes from the audit logs, grouped by binary version and guide hash:\n" +
			"sessions, how many had a boundary with nothing saved, saves per session, and automatic eval scores by probe kind.\n" +
			"Only sessions that ended on a version that records session_ended are counted.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			req := core.StatsReq{Author: author, AllAuthors: allAuthors}
			if since != "" {
				t, err := time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("--since must be YYYY-MM-DD: %w", err)
				}
				req.Since = t
			}
			rep, err := svc.Stats(req)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			renderStats(cmd.OutOrStdout(), rep, svc.EvalConfig())
			return nil
		},
	}
	cmd.Flags().BoolVar(&allAuthors, "all-authors", false, "Include every author's sessions")
	cmd.Flags().StringVar(&author, "author", "", "Limit to one author (default: you)")
	cmd.Flags().StringVar(&since, "since", "", "Only sessions ended on or after YYYY-MM-DD")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func renderStats(w io.Writer, rep core.StatsReport, evalCfg core.EvalConfig) {
	scope := "your sessions"
	if rep.AllAuthors {
		scope = "all authors"
	} else if rep.Author != "" {
		scope = "sessions by " + rep.Author
	}
	knob := "on"
	if !evalCfg.Enabled {
		knob = "off"
	}
	fmt.Fprintf(w, "Session outcomes by version (%s). Automatic evals: %s, model %s.\n\n", scope, knob, evalCfg.Model)
	if len(rep.Rows) == 0 {
		fmt.Fprintln(w, "No sessions recorded yet. Sessions are counted from the first session that ends on a version with stats.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tGUIDE\tSESSIONS\tUNSAVED\tSAVES/SESSION\tEVALS\tRECOVERED\tSKIPPED\tEVAL COST\tLAST")
	for _, r := range rep.Rows {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%.1f\t%d\t%s\t%d\t$%.2f\t%s\n",
			orDash(r.Version), orDash(r.GuideHash), r.Sessions, pct(r.Unsaved, r.Sessions),
			float64(r.Saves)/float64(max(r.Sessions, 1)), r.Evals, pct(r.EvalPassed, r.EvalProbes),
			r.EvalSkipped, r.EvalCostUSD, r.Last.Local().Format("2006-01-02"))
	}
	tw.Flush()

	for _, r := range rep.Rows {
		if len(r.ByKind) == 0 && len(r.SkipReasons) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s (guide %s)\n", orDash(r.Version), orDash(r.GuideHash))
		kinds := make([]string, 0, len(r.ByKind))
		for k := range r.ByKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			v := r.ByKind[k]
			fmt.Fprintf(w, "  %-11s %s (%d/%d)\n", k, pct(v.Passed, v.Probes), v.Passed, v.Probes)
		}
		reasons := make([]string, 0, len(r.SkipReasons))
		for k := range r.SkipReasons {
			reasons = append(reasons, k)
		}
		sort.Strings(reasons)
		for _, k := range reasons {
			fmt.Fprintf(w, "  skipped: %s (%d)\n", k, r.SkipReasons[k])
		}
	}
	fmt.Fprintln(w, "\nUNSAVED: sessions with a boundary (compaction or end) that found nothing saved. RECOVERED: eval probes a fresh agent answered from the Distilled State.")
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(d))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
