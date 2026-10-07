package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"dossier/internal/core"

	"gopkg.in/yaml.v3"
)

var validKinds = []string{"value", "correction", "rejected", "decision", "assumption", "state"}
var validMatches = []string{"verbatim", "judge"}

// Probe is one question with the fact a resumed agent must recover.
type Probe struct {
	ID       string `yaml:"id" json:"id"`
	Kind     string `yaml:"kind" json:"kind"`
	Question string `yaml:"question" json:"question"`
	Expect   string `yaml:"expect" json:"expect"`
	Match    string `yaml:"match" json:"match"`
}

// Case is one transcript plus its probes.
type Case struct {
	Name       string
	Transcript string
	Probes     []Probe
}

// Variant is one guide + instructions pair under test.
type Variant struct {
	Name             string `json:"name"`
	GuideSpec        string `json:"guide"`
	InstructionsSpec string `json:"instructions"`
	Guide            string `json:"-"`
	Instructions     string `json:"-"`
}

// validateProbes checks a probe list and names the offending probe.
func validateProbes(probes []Probe) error {
	if len(probes) == 0 {
		return fmt.Errorf("no probes")
	}
	seen := map[string]bool{}
	for i, p := range probes {
		where := fmt.Sprintf("probe %d (id %q)", i+1, p.ID)
		switch {
		case strings.TrimSpace(p.ID) == "":
			return fmt.Errorf("probe %d: id is required", i+1)
		case seen[p.ID]:
			return fmt.Errorf("%s: duplicate id", where)
		case !slices.Contains(validKinds, p.Kind):
			return fmt.Errorf("%s: invalid kind %q (want one of %s)", where, p.Kind, strings.Join(validKinds, ", "))
		case !slices.Contains(validMatches, p.Match):
			return fmt.Errorf("%s: invalid match %q (want one of %s)", where, p.Match, strings.Join(validMatches, ", "))
		case strings.TrimSpace(p.Question) == "":
			return fmt.Errorf("%s: question is required", where)
		case strings.TrimSpace(p.Expect) == "":
			return fmt.Errorf("%s: expect is required", where)
		}
		seen[p.ID] = true
	}
	return nil
}

func parseProbes(data []byte) ([]Probe, error) {
	var probes []Probe
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&probes); err != nil {
		return nil, fmt.Errorf("parse probes.yaml: %w", err)
	}
	if err := validateProbes(probes); err != nil {
		return nil, err
	}
	return probes, nil
}

// loadCase reads one case directory.
func loadCase(dir string) (Case, error) {
	c := Case{Name: filepath.Base(dir)}
	md, mdErr := os.ReadFile(filepath.Join(dir, "transcript.md"))
	jl, jlErr := os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	switch {
	case mdErr == nil && jlErr == nil:
		return c, fmt.Errorf("case %s: both transcript.md and transcript.jsonl exist; keep one", c.Name)
	case mdErr == nil:
		c.Transcript = string(md)
	case jlErr == nil:
		compiled, _, _ := core.CompileTranscript(string(jl))
		c.Transcript = compiled
	default:
		return c, fmt.Errorf("case %s: need transcript.md or transcript.jsonl", c.Name)
	}
	if strings.TrimSpace(c.Transcript) == "" {
		return c, fmt.Errorf("case %s: transcript is empty", c.Name)
	}
	pb, err := os.ReadFile(filepath.Join(dir, "probes.yaml"))
	if err != nil {
		return c, fmt.Errorf("case %s: %w", c.Name, err)
	}
	if c.Probes, err = parseProbes(pb); err != nil {
		return c, fmt.Errorf("case %s: %w", c.Name, err)
	}
	return c, nil
}

// loadCases loads every case subdirectory of dir, sorted by name.
func loadCases(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read cases dir: %w", err)
	}
	var cases []Case
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := loadCase(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no cases found under %s", dir)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases, nil
}
