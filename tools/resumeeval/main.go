// Command resumeeval measures whether a Distillation Guide produces Distilled
// States that let a fresh agent resume without losing facts. See README.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type config struct {
	cases, out, model, judgeModel, bin, repo string
	guideA, guideB, instrA, instrB           string
	runs                                     int
	dryRun, bare                             bool
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	var c config
	fs := flag.NewFlagSet("resumeeval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.cases, "cases", "", "directory of case directories (required)")
	fs.StringVar(&c.out, "out", "resumeeval-out", "output directory")
	fs.StringVar(&c.guideA, "guide-a", "", "guide A: file path or git:<ref>:<path>")
	fs.StringVar(&c.guideB, "guide-b", "", "guide B: file path or git:<ref>:<path> (optional)")
	fs.StringVar(&c.instrA, "instructions-a", "", "instructions A (default: same ref as guide A, else assets/instructions.md)")
	fs.StringVar(&c.instrB, "instructions-b", "", "instructions B (default: same ref as guide B, else assets/instructions.md)")
	fs.IntVar(&c.runs, "runs", 3, "runs per case and variant (for variance)")
	fs.StringVar(&c.model, "model", "", "model for distill and resume (passed to claude --model)")
	fs.StringVar(&c.judgeModel, "judge-model", "", "model for the judge (default: --model)")
	fs.StringVar(&c.bin, "bin", "claude", "path to the claude binary")
	fs.StringVar(&c.repo, "repo", ".", "git repo used to resolve git: specs")
	fs.BoolVar(&c.bare, "bare", false, "also pass --bare to claude (requires ANTHROPIC_API_KEY auth)")
	fs.BoolVar(&c.dryRun, "dry-run", false, "print the plan and estimated model calls, then exit")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	switch {
	case c.cases == "":
		return c, fmt.Errorf("--cases is required")
	case c.guideA == "":
		return c, fmt.Errorf("--guide-a is required")
	case c.runs < 1:
		return c, fmt.Errorf("--runs must be >= 1")
	}
	if c.judgeModel == "" {
		c.judgeModel = c.model
	}
	return c, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, be Backend) error {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}
	cases, err := loadCases(cfg.cases)
	if err != nil {
		return err
	}
	va, err := loadVariant("A", cfg.guideA, cfg.instrA, cfg.repo)
	if err != nil {
		return err
	}
	variants := []Variant{va}
	if cfg.guideB != "" {
		vb, err := loadVariant("B", cfg.guideB, cfg.instrB, cfg.repo)
		if err != nil {
			return err
		}
		variants = append(variants, vb)
	}
	opt := Options{Model: cfg.model, JudgeModel: cfg.judgeModel, OutDir: cfg.out, Runs: cfg.runs}

	if cfg.dryRun {
		printPlan(stdout, cases, variants, opt)
		return nil
	}
	if be == nil {
		be = ClaudeCLI{Bin: cfg.bin, Bare: cfg.bare}
	}
	if err := os.MkdirAll(cfg.out, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	var records []RunRecord
	cost := 0.0
	total := plannedCalls(cases, len(variants), cfg.runs)
	done := 0
	for _, c := range cases {
		for run := 1; run <= cfg.runs; run++ {
			for _, v := range variants {
				rec := runOne(ctx, be, opt, c, v, run)
				records = append(records, rec)
				cost += rec.CostUSD
				done += 2 + judgeCount(c)
				fmt.Fprintf(stderr, "[%d/%d calls] case=%s variant=%s run=%d%s\n", done, total, c.Name, v.Name, run, errNote(rec))
			}
		}
	}

	results := struct {
		GeneratedAt time.Time   `json:"generated_at"`
		Model       string      `json:"model"`
		JudgeModel  string      `json:"judge_model"`
		Runs        int         `json:"runs"`
		CostUSD     float64     `json:"cost_usd"`
		Variants    []Variant   `json:"variants"`
		Records     []RunRecord `json:"records"`
	}{time.Now().UTC(), opt.Model, opt.JudgeModel, opt.Runs, cost, variants, records}
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.out, "results.json"), append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write results.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.out, "report.md"), []byte(renderReport(variants, opt, records, cost)), 0o644); err != nil {
		return fmt.Errorf("write report.md: %w", err)
	}
	fmt.Fprintf(stdout, "wrote %s and %s\n", filepath.Join(cfg.out, "results.json"), filepath.Join(cfg.out, "report.md"))
	return nil
}

func errNote(r RunRecord) string {
	if r.Error != "" {
		return " ERROR: " + r.Error
	}
	return ""
}

func printPlan(w io.Writer, cases []Case, variants []Variant, opt Options) {
	fmt.Fprintf(w, "Plan (dry run, no model calls made)\n")
	fmt.Fprintf(w, "  model: %q  judge model: %q  runs: %d\n", opt.Model, opt.JudgeModel, opt.Runs)
	for _, v := range variants {
		fmt.Fprintf(w, "  variant %s: guide=%s (%d bytes) instructions=%s (%d bytes)\n", v.Name, v.GuideSpec, len(v.Guide), v.InstructionsSpec, len(v.Instructions))
	}
	for _, c := range cases {
		fmt.Fprintf(w, "  case %s: %d probes (%d judge), transcript %d bytes, %d calls per variant-run\n",
			c.Name, len(c.Probes), judgeCount(c), len(c.Transcript), 2+judgeCount(c))
	}
	fmt.Fprintf(w, "Estimated model calls: %d\n", plannedCalls(cases, len(variants), opt.Runs))
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, nil); err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, "resumeeval:", err)
		os.Exit(1)
	}
}
