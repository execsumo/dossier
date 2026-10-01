package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// provenanceRefRE matches a citation and captures the artifact id plus the raw
// fragment. The fragment is captured (not discarded) so a line range can be
// parsed and checked against the artifact it points into: an unresolvable
// pointer is worse than no pointer, because it reads as evidence.
var provenanceRefRE = regexp.MustCompile(`\[src:([A-Za-z0-9_]+)(?:#([^\]]*))?\]`)

// lineRangeRE matches the #L<start>-L<end> fragment form.
var lineRangeRE = regexp.MustCompile(`^L(\d+)(?:-L(\d+))?$`)

// ProvenanceRef is a parsed [src:...] citation.
type ProvenanceRef struct {
	ArtifactID string
	Fragment   string
	StartLine  int
	EndLine    int
	HasRange   bool
}

// ParseProvenanceRef parses a citation's artifact id and optional line range.
// A fragment that is present but not a well-formed L-range is reported so the
// author can fix it rather than ship a pointer nothing can follow.
func ParseProvenanceRef(artifactID, fragment string) (ProvenanceRef, error) {
	ref := ProvenanceRef{ArtifactID: artifactID, Fragment: fragment}
	if fragment == "" {
		return ref, nil
	}

	m := lineRangeRE.FindStringSubmatch(fragment)
	if m == nil {
		return ref, fmt.Errorf("fragment %q is not a line range (expected #L<start> or #L<start>-L<end>)", fragment)
	}

	start, err := strconv.Atoi(m[1])
	if err != nil || start < 1 {
		return ref, fmt.Errorf("fragment %q has an invalid start line", fragment)
	}
	end := start
	if m[2] != "" {
		end, err = strconv.Atoi(m[2])
		if err != nil || end < 1 {
			return ref, fmt.Errorf("fragment %q has an invalid end line", fragment)
		}
	}
	if end < start {
		return ref, fmt.Errorf("fragment %q ends before it starts", fragment)
	}

	ref.StartLine = start
	ref.EndLine = end
	ref.HasRange = true
	return ref, nil
}

// ArtifactInfo reports whether an artifact exists and how many lines it has,
// so a cited range can be bounds-checked.
type ArtifactInfo func(artifactID string) (lineCount int, ok bool)

// validateDistilledStateProvenance checks the citations the Distilled State
// does carry: each must be well-formed, name a real artifact, and (when it
// cites a line range) point at lines that exist in that artifact.
//
// It deliberately does not flag uncited lines. Whether a line needs a
// citation is an authoring judgment; what doctor guards is that a pointer
// which reads as evidence still resolves, so a broken reference is the signal
// and the absence of one is not.
func validateDistilledStateProvenance(body string, dossierID string, info ArtifactInfo) []string {
	var issues []string
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.Contains(trimmed, "[src:") {
			continue
		}
		n := i + 1
		refs := provenanceRefRE.FindAllStringSubmatch(trimmed, -1)
		if strings.Count(trimmed, "[src:") != len(refs) {
			issues = append(issues, fmt.Sprintf("Dossier %s line %d has malformed provenance reference", dossierID, n))
			continue
		}
		for _, m := range refs {
			artifactID := m[1]
			lineCount, ok := info(artifactID)
			if !ok {
				issues = append(issues, fmt.Sprintf("Dossier %s line %d references missing artifact %s", dossierID, n, artifactID))
				continue
			}
			ref, err := ParseProvenanceRef(artifactID, m[2])
			if err != nil {
				issues = append(issues, fmt.Sprintf("Dossier %s line %d cites artifact %s but the %v", dossierID, n, artifactID, err))
				continue
			}
			if ref.HasRange && (ref.StartLine > lineCount || ref.EndLine > lineCount) {
				issues = append(issues, fmt.Sprintf(
					"Dossier %s line %d cites %s#%s but the artifact has only %d line(s)",
					dossierID, n, artifactID, formatLineRange(ref), lineCount))
			}
		}
	}
	return issues
}

// formatLineRange renders a cited range as L5-L10, or as the bare L5 form
// when the citation only ever named a single line.
func formatLineRange(ref ProvenanceRef) string {
	if ref.StartLine == ref.EndLine {
		return fmt.Sprintf("L%d", ref.StartLine)
	}
	return fmt.Sprintf("L%d-L%d", ref.StartLine, ref.EndLine)
}

// citedArtifactIDs returns the set of artifact ids the Distilled State points at.
func citedArtifactIDs(body string) map[string]bool {
	cited := map[string]bool{}
	for _, m := range provenanceRefRE.FindAllStringSubmatch(body, -1) {
		cited[m[1]] = true
	}
	return cited
}

// uncitedArtifacts lists archived artifacts the Distilled State never cites.
//
// This is the low-end counterpart to the token-target warning. A Distilled
// State can be too thin as well as too long, and the legible symptom is
// evidence sitting in the Archive that the curated view never points at.
//
// Transcript artifacts are exempt: they are captured automatically at
// session end (not authored as evidence), routinely run thousands of lines,
// and a Distilled State that never cites one is the common case, not a gap.
// Citing a transcript wholesale to silence this warning is exactly the
// anti-pattern the Distillation Guide warns against.
func uncitedArtifacts(body string, artifacts []Artifact) []Artifact {
	cited := citedArtifactIDs(body)
	var out []Artifact
	for _, art := range artifacts {
		if art.Type == ArtifactTypeTranscript {
			continue
		}
		if !cited[art.ID] {
			out = append(out, art)
		}
	}
	return out
}

// uncitedArtifactWarning renders the advisory for uncited artifacts, or "" if
// every artifact is cited.
func uncitedArtifactWarning(body string, artifacts []Artifact) string {
	uncited := uncitedArtifacts(body, artifacts)
	if len(uncited) == 0 {
		return ""
	}
	ids := make([]string, 0, len(uncited))
	for _, art := range uncited {
		ids = append(ids, art.ID)
	}
	shown := ids
	suffix := ""
	if len(shown) > 5 {
		shown = shown[:5]
		suffix = fmt.Sprintf(" (+%d more)", len(ids)-5)
	}
	return fmt.Sprintf(
		"%d archived artifact(s) are not cited by the Distilled State: %s%s. Archived evidence the curated view never points at is unreachable in practice; add [src:] citations or record why it is not material.",
		len(ids), strings.Join(shown, ", "), suffix)
}

// provenanceStripRE matches a citation with the single space that set it off, so
// removing it leaves no gap before punctuation or at end of line.
var provenanceStripRE = regexp.MustCompile(`[ \t]?` + provenanceRefRE.String())

// HumanView returns the Distilled State as a human reader should see it: [src:]
// citations are removed and the agent-facing ## Evidence index is dropped. Both
// exist so an agent can follow a compressed claim back to the Archive; for a
// person reading the brief they are noise. It is a display transform only — the
// stored body keeps everything, and callers that want the raw form use the body
// directly. Code fences are left untouched.
func HumanView(body string) string {
	if !strings.Contains(body, "[src:") && !evidenceHeadingRE.MatchString(body) {
		return body
	}
	var out []string
	inFence, skipping := false, false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		fence := strings.HasPrefix(trimmed, "```")
		if fence {
			inFence = !inFence
		}
		if !inFence && !fence && strings.HasPrefix(line, "#") {
			skipping = evidenceHeadingRE.MatchString(line)
		}
		if skipping {
			continue
		}
		if !inFence && !fence {
			line = provenanceStripRE.ReplaceAllString(line, "")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var evidenceHeadingRE = regexp.MustCompile(`(?m)^## Evidence[ \t]*$`)
