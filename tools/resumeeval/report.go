package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Tally is a pass count out of a total.
type Tally struct{ Pass, Total int }

func (t *Tally) add(pass bool) {
	t.Total++
	if pass {
		t.Pass++
	}
}

// Rate is the pass fraction, 0 when empty.
func (t Tally) Rate() float64 {
	if t.Total == 0 {
		return 0
	}
	return float64(t.Pass) / float64(t.Total)
}

func (t Tally) String() string {
	return fmt.Sprintf("%.1f%% (%d/%d)", t.Rate()*100, t.Pass, t.Total)
}

// Spread summarises per-run overall pass rates for one variant.
type Spread struct {
	Min, Max, Mean, StdDev float64
	Runs                   int
}

// Diff is a probe whose pass rate differs between variants.
type Diff struct {
	Case, ProbeID, Kind string
	Rates               map[string]Tally // by variant
}

// Summary is the aggregated view rendered into report.md.
type Summary struct {
	Variants []string
	Overall  map[string]Tally
	ByKind   map[string]map[string]Tally // kind -> variant -> tally
	ByCase   map[string]map[string]Tally // case -> variant -> tally
	Spread   map[string]Spread
	Diffs    []Diff
	Errors   map[string]int // variant -> errored probe results
	Kinds    []string
	Cases    []string
}

func summarize(variants []string, records []RunRecord) Summary {
	s := Summary{
		Variants: variants,
		Overall:  map[string]Tally{},
		ByKind:   map[string]map[string]Tally{},
		ByCase:   map[string]map[string]Tally{},
		Spread:   map[string]Spread{},
		Errors:   map[string]int{},
	}
	perRun := map[string]map[int]*Tally{}     // variant -> run -> tally
	perProbe := map[string]map[string]Tally{} // "case\x00probe\x00kind" -> variant -> tally
	kinds, cases := map[string]bool{}, map[string]bool{}

	bump := func(m map[string]map[string]Tally, k, v string, pass bool) {
		if m[k] == nil {
			m[k] = map[string]Tally{}
		}
		t := m[k][v]
		t.add(pass)
		m[k][v] = t
	}
	for _, rec := range records {
		for _, r := range rec.Results {
			t := s.Overall[r.Variant]
			t.add(r.Pass)
			s.Overall[r.Variant] = t
			bump(s.ByKind, r.Kind, r.Variant, r.Pass)
			bump(s.ByCase, r.Case, r.Variant, r.Pass)
			bump(perProbe, r.Case+"\x00"+r.ProbeID+"\x00"+r.Kind, r.Variant, r.Pass)
			if perRun[r.Variant] == nil {
				perRun[r.Variant] = map[int]*Tally{}
			}
			if perRun[r.Variant][r.Run] == nil {
				perRun[r.Variant][r.Run] = &Tally{}
			}
			perRun[r.Variant][r.Run].add(r.Pass)
			if r.Error {
				s.Errors[r.Variant]++
			}
			kinds[r.Kind], cases[r.Case] = true, true
		}
	}
	for v, runs := range perRun {
		var rates []float64
		for _, t := range runs {
			rates = append(rates, t.Rate())
		}
		s.Spread[v] = spreadOf(rates)
	}
	for k := range kinds {
		s.Kinds = append(s.Kinds, k)
	}
	for c := range cases {
		s.Cases = append(s.Cases, c)
	}
	sort.Strings(s.Kinds)
	sort.Strings(s.Cases)

	keys := make([]string, 0, len(perProbe))
	for k := range perProbe {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		byVar := perProbe[k]
		differs := false
		var first *Tally
		for _, v := range variants {
			t := byVar[v]
			if first == nil {
				first = &t
			} else if t.Pass*first.Total != first.Pass*t.Total { // compare rates exactly
				differs = true
			}
		}
		if differs {
			parts := strings.Split(k, "\x00")
			s.Diffs = append(s.Diffs, Diff{Case: parts[0], ProbeID: parts[1], Kind: parts[2], Rates: byVar})
		}
	}
	return s
}

func spreadOf(rates []float64) Spread {
	if len(rates) == 0 {
		return Spread{}
	}
	sp := Spread{Min: rates[0], Max: rates[0], Runs: len(rates)}
	sum := 0.0
	for _, r := range rates {
		sp.Min, sp.Max = math.Min(sp.Min, r), math.Max(sp.Max, r)
		sum += r
	}
	sp.Mean = sum / float64(len(rates))
	for _, r := range rates {
		sp.StdDev += (r - sp.Mean) * (r - sp.Mean)
	}
	sp.StdDev = math.Sqrt(sp.StdDev / float64(len(rates))) // population stddev
	return sp
}

func tableRow(cells ...string) string { return "| " + strings.Join(cells, " | ") + " |\n" }

func tableHead(cells ...string) string {
	seps := make([]string, len(cells))
	for i := range seps {
		seps[i] = "---"
	}
	return tableRow(cells...) + tableRow(seps...)
}

// renderReport builds report.md.
func renderReport(variants []Variant, opt Options, records []RunRecord, cost float64) string {
	names := make([]string, len(variants))
	for i, v := range variants {
		names[i] = v.Name
	}
	s := summarize(names, records)
	var b strings.Builder

	b.WriteString("# Resumption-fidelity report\n\n")
	fmt.Fprintf(&b, "Model: `%s` · Judge: `%s` · Runs per variant: %d · Reported cost: $%.4f\n\n", opt.Model, opt.JudgeModel, opt.Runs, cost)
	for _, v := range variants {
		fmt.Fprintf(&b, "- **%s**: guide `%s`, instructions `%s`\n", v.Name, v.GuideSpec, v.InstructionsSpec)
	}

	b.WriteString("\n## Overall pass rate\n\n")
	b.WriteString(tableHead(append([]string{"Variant"}, "Pass rate", "Errored probes")...))
	for _, v := range names {
		b.WriteString(tableRow(v, s.Overall[v].String(), fmt.Sprint(s.Errors[v])))
	}
	b.WriteString("\nErrored probes failed because the pipeline broke (model error, unparseable output), not because a fact was lost; they count as failures.\n")

	b.WriteString("\n## Run-to-run spread\n\nPer-run overall pass rate.\n\n")
	b.WriteString(tableHead("Variant", "Runs", "Min", "Max", "Mean", "Std dev"))
	for _, v := range names {
		sp := s.Spread[v]
		b.WriteString(tableRow(v, fmt.Sprint(sp.Runs), pct(sp.Min), pct(sp.Max), pct(sp.Mean), pct(sp.StdDev)))
	}

	b.WriteString("\n## Pass rate by probe kind\n\n")
	b.WriteString(tableHead(append([]string{"Kind"}, names...)...))
	for _, k := range s.Kinds {
		row := []string{k}
		for _, v := range names {
			row = append(row, s.ByKind[k][v].String())
		}
		b.WriteString(tableRow(row...))
	}

	b.WriteString("\n## Per-case breakdown\n\n")
	b.WriteString(tableHead(append([]string{"Case"}, names...)...))
	for _, c := range s.Cases {
		row := []string{c}
		for _, v := range names {
			row = append(row, s.ByCase[c][v].String())
		}
		b.WriteString(tableRow(row...))
	}

	b.WriteString("\n## Probes that differ between variants\n\n")
	if len(s.Diffs) == 0 {
		b.WriteString("None: every probe had the same pass rate in all variants.\n")
	} else {
		b.WriteString(tableHead(append([]string{"Case", "Probe", "Kind"}, names...)...))
		for _, d := range s.Diffs {
			row := []string{d.Case, d.ProbeID, d.Kind}
			for _, v := range names {
				row = append(row, d.Rates[v].String())
			}
			b.WriteString(tableRow(row...))
		}
	}

	var errs []string
	for _, rec := range records {
		if rec.Error != "" {
			errs = append(errs, fmt.Sprintf("- %s / %s / run %d: %s", rec.Case, rec.Variant, rec.Run, rec.Error))
		}
	}
	if len(errs) > 0 {
		b.WriteString("\n## Pipeline errors\n\n" + strings.Join(errs, "\n") + "\n")
	}
	return b.String()
}

func pct(f float64) string { return fmt.Sprintf("%.1f%%", f*100) }
