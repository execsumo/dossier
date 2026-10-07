package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// gitSpec splits "git:<ref>:<path>". The ref is everything up to the first
// colon after the prefix (refs like "b2a4f73^" contain no colon).
func gitSpec(s string) (ref, p string, ok bool) {
	rest, found := strings.CutPrefix(s, "git:")
	if !found {
		return "", "", false
	}
	ref, p, found = strings.Cut(rest, ":")
	if !found || ref == "" || p == "" {
		return "", "", false
	}
	return ref, p, true
}

// resolveSpec reads the text a spec (file path or git:<ref>:<path>) points at.
// repoDir is where `git show` runs for git: specs.
func resolveSpec(spec, repoDir string) (string, error) {
	if strings.HasPrefix(spec, "git:") {
		ref, p, ok := gitSpec(spec)
		if !ok {
			return "", fmt.Errorf("bad git spec %q: want git:<ref>:<path>", spec)
		}
		cmd := exec.Command("git", "show", ref+":"+p)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git show %s:%s: %w: %s", ref, p, err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}
	b, err := os.ReadFile(spec)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", spec, err)
	}
	return string(b), nil
}

// defaultInstructionsSpec picks the instructions source when none is given:
// the sibling of a git: guide at the same ref, else assets/instructions.md.
func defaultInstructionsSpec(guideSpec string) string {
	if ref, p, ok := gitSpec(guideSpec); ok {
		return "git:" + ref + ":" + path.Join(path.Dir(p), "instructions.md")
	}
	return "assets/instructions.md"
}

// loadVariant resolves one guide/instructions pair.
func loadVariant(name, guide, instructions, repoDir string) (Variant, error) {
	if guide == "" {
		return Variant{}, fmt.Errorf("variant %s: guide not set", name)
	}
	if instructions == "" {
		instructions = defaultInstructionsSpec(guide)
	}
	g, err := resolveSpec(guide, repoDir)
	if err != nil {
		return Variant{}, fmt.Errorf("variant %s guide: %w", name, err)
	}
	in, err := resolveSpec(instructions, repoDir)
	if err != nil {
		return Variant{}, fmt.Errorf("variant %s instructions: %w", name, err)
	}
	return Variant{Name: name, GuideSpec: guide, InstructionsSpec: instructions, Guide: g, Instructions: in}, nil
}
