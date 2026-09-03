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

// contentLine pairs a 1-indexed body line number with its trimmed text.
type contentLine struct {
	Number int
	Text   string
}

// bodyContentLines walks the Distilled State body and yields the 1-indexed
// lines that are expected to carry provenance: prose and list items, but not
// headings, fences, fenced content, blockquotes, or separators.
//
// Two structural exemptions apply, both because a citation on these lines
// would be either meaningless or self-contradictory:
//   - The "## Evidence" section describes the Archive's own artifacts (an
//     `art_<id>` is already the pointer; asking it to also carry a [src:] to
//     itself is circular), so its lines are excluded.
//   - A [assumed] line is by definition "believed but unverified" (guide
//     §3) — it has nothing to cite by design.
func bodyContentLines(body string) []contentLine {
	var out []contentLine
	inFence := false
	inEvidenceSection := false
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			inEvidenceSection = isEvidenceHeading(trimmed)
			continue
		}
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "---") ||
			strings.HasPrefix(trimmed, ">") ||
			inEvidenceSection ||
			strings.Contains(trimmed, "[assumed]") {
			continue
		}
		out = append(out, contentLine{Number: i + 1, Text: trimmed})
	}
	return out
}

// isEvidenceHeading reports whether a trimmed heading line is the "Evidence"
// section header (of any level), matched by name so the exemption doesn't
// depend on the heading's exact depth.
func isEvidenceHeading(trimmed string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), "Evidence")
}

// validateDistilledStateProvenance checks that every content line carries a
// citation, that each citation names a real artifact, and that any cited line
// range actually exists in that artifact.
func validateDistilledStateProvenance(body string, dossierID string, info ArtifactInfo) []string {
	var issues []string

	for _, cl := range bodyContentLines(body) {
		n, trimmed := cl.Number, cl.Text
		refs := provenanceRefRE.FindAllStringSubmatch(trimmed, -1)
		if strings.Count(trimmed, "[src:") != len(refs) {
			issues = append(issues, fmt.Sprintf("Dossier %s line %d has malformed provenance reference", dossierID, n))
			continue
		}
		if len(refs) == 0 {
			issues = append(issues, fmt.Sprintf("Dossier %s line %d is missing provenance", dossierID, n))
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
