package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseProvenanceRef(t *testing.T) {
	tests := []struct {
		name      string
		fragment  string
		wantStart int
		wantEnd   int
		wantRange bool
		wantErr   string
	}{
		{name: "no fragment", fragment: ""},
		{name: "single line", fragment: "L42", wantStart: 42, wantEnd: 42, wantRange: true},
		{name: "range", fragment: "L42-L68", wantStart: 42, wantEnd: 68, wantRange: true},
		{name: "not a range", fragment: "section-3", wantErr: "not a line range"},
		{name: "bare numbers", fragment: "42-68", wantErr: "not a line range"},
		{name: "inverted", fragment: "L68-L42", wantErr: "ends before it starts"},
		{name: "zero start", fragment: "L0", wantErr: "invalid start line"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseProvenanceRef("art_x", tt.fragment)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseProvenanceRef(%q) error = %v, want containing %q", tt.fragment, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseProvenanceRef(%q) error = %v", tt.fragment, err)
			}
			if ref.HasRange != tt.wantRange || ref.StartLine != tt.wantStart || ref.EndLine != tt.wantEnd {
				t.Fatalf("ParseProvenanceRef(%q) = %+v, want start=%d end=%d range=%v",
					tt.fragment, ref, tt.wantStart, tt.wantEnd, tt.wantRange)
			}
		})
	}
}

func TestValidateDistilledStateProvenanceChecksLineRanges(t *testing.T) {
	info := func(id string) (int, bool) {
		if id == "art_ok" {
			return 100, true
		}
		return 0, false
	}

	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{
			name: "in-range citation is clean",
			body: "## Findings\n- [observed] Lock contention. [src:art_ok#L42-L68]",
		},
		{
			name:    "range past end of artifact",
			body:    "## Findings\n- [observed] Lock contention. [src:art_ok#L420-L680]",
			wantSub: "has only 100 line(s)",
		},
		{
			name:    "range starts in bounds but ends past the artifact",
			body:    "## Findings\n- [observed] Lock contention. [src:art_ok#L90-L680]",
			wantSub: "has only 100 line(s)",
		},
		{
			name:    "single-line citation past the artifact renders as a bare line, not a range",
			body:    "## Findings\n- [observed] Lock contention. [src:art_ok#L500]",
			wantSub: "cites art_ok#L500 but the artifact has only 100 line(s)",
		},
		{
			name:    "fragment that is not a line range",
			body:    "## Findings\n- [observed] Lock contention. [src:art_ok#section-3]",
			wantSub: "not a line range",
		},
		{
			name:    "missing artifact",
			body:    "## Findings\n- [observed] Lock contention. [src:art_gone#L1-L2]",
			wantSub: "references missing artifact art_gone",
		},
		{
			name:    "a malformed citation is not masked by a valid one on the same line",
			body:    "## Findings\n- [observed] Finding. [src:art-1] and [src:art_ok]",
			wantSub: "has malformed provenance reference",
		},
		{
			name: "headings and fences are exempt",
			body: "# Title\n\n## Findings\n\n```\nraw block with no citation\n```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := validateDistilledStateProvenance(tt.body, "dos_1", info)
			if tt.wantSub == "" {
				if len(issues) != 0 {
					t.Fatalf("issues = %v, want none", issues)
				}
				return
			}
			joined := strings.Join(issues, "\n")
			if !strings.Contains(joined, tt.wantSub) {
				t.Fatalf("issues = %v, want one containing %q", issues, tt.wantSub)
			}
		})
	}
}

func TestUncitedArtifactWarning(t *testing.T) {
	artifacts := []Artifact{
		{ID: "art_cited"},
		{ID: "art_orphan"},
	}
	body := "## Findings\n- [observed] Something. [src:art_cited#L1-L4]"

	msg := uncitedArtifactWarning(body, artifacts)
	if !strings.Contains(msg, "art_orphan") {
		t.Fatalf("warning = %q, want it to name art_orphan", msg)
	}
	if strings.Contains(msg, "art_cited") {
		t.Fatalf("warning = %q, should not name the cited artifact", msg)
	}

	if msg := uncitedArtifactWarning(body, artifacts[:1]); msg != "" {
		t.Fatalf("warning = %q, want none when every artifact is cited", msg)
	}
}

func TestArtifactLineCount(t *testing.T) {
	tests := []struct {
		content string
		want    int
	}{
		{"", 0},
		{"one", 1},
		{"one\n", 1},
		{"one\ntwo", 2},
		{"one\ntwo\n", 2},
	}
	for _, tt := range tests {
		if got := artifactLineCount(tt.content); got != tt.want {
			t.Errorf("artifactLineCount(%q) = %d, want %d", tt.content, got, tt.want)
		}
	}
}

func TestDoctorFlagsUnresolvableLineRange(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore := newLocalFakeStore()
	fakeStore.dossiers["dos_range"] = &Dossier{
		Frontmatter: Frontmatter{
			ID: "dos_range", Name: "Range Dossier", Slug: "range-dossier",
			CreatedAt: now, UpdatedAt: now,
			Status: StatusActive, Priority: PriorityLow,
		},
		DistilledState: DistilledState{
			Body: "# Range Dossier\n\n## Findings\n- [observed] A claim. [src:art_short#L40-L60]",
		},
	}
	fakeStore.artifacts["dos_range"] = []Artifact{{
		ID: "art_short", DossierID: "dos_range", Type: ArtifactTypeDecisionEvidence,
		Title: "Short evidence", CapturedAt: now, RefreshedAt: now,
		Provenance: Provenance{Origin: "unit test"}, ContentFormat: ContentFormatText,
		Content: "one\ntwo\nthree\n",
	}}
	fakeStore.revisions["dos_range"] = CalculateRevision(
		fakeStore.dossiers["dos_range"].Frontmatter,
		fakeStore.dossiers["dos_range"].DistilledState.Body,
		fakeStore.artifacts["dos_range"])

	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	if res.OK {
		t.Fatalf("expected doctor to flag the unresolvable range")
	}
	joined := warningsText(res.Warnings)
	if !strings.Contains(joined, "has only 3 line(s)") {
		t.Fatalf("expected an out-of-range citation warning, got:\n%s", joined)
	}
}

// TestDoctorBoundsCitationsByArtifactContent: the artifact fetch path bounds a
// range by the artifact's actual content, so doctor must too. Recorded `lines:`
// metadata that disagrees with the content (here 40 vs 3) must neither hide a
// citation that cannot be followed nor go unreported itself.
func TestDoctorBoundsCitationsByArtifactContent(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore := newLocalFakeStore()
	fakeStore.dossiers["dos_stale"] = &Dossier{
		Frontmatter: Frontmatter{
			ID: "dos_stale", Name: "Stale", Slug: "stale",
			CreatedAt: now, UpdatedAt: now, Status: StatusActive, Priority: PriorityLow,
		},
		DistilledState: DistilledState{Body: "# Stale\n\n## Findings\n- [observed] A claim. [src:art_stale#L10-L12]\n- [observed] Uncited range. [src:art_fine#L1]"},
	}
	fakeStore.artifacts["dos_stale"] = []Artifact{
		{ID: "art_stale", DossierID: "dos_stale", Type: ArtifactTypeDecisionEvidence, Title: "Stale",
			CapturedAt: now, RefreshedAt: now, Provenance: Provenance{Origin: "unit test"},
			ContentFormat: ContentFormatText, Content: "b1\nb2\nb3\n", Lines: 40},
		{ID: "art_fine", DossierID: "dos_stale", Type: ArtifactTypeDecisionEvidence, Title: "Fine",
			CapturedAt: now, RefreshedAt: now, Provenance: Provenance{Origin: "unit test"},
			ContentFormat: ContentFormatText, Content: "a1\n", Lines: 1},
	}
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := warningsText(res.Warnings)
	if res.OK {
		t.Fatalf("a citation past the artifact's content must fail doctor:\n%s", joined)
	}
	for _, want := range []string{
		"cites art_stale#L10-L12 but the artifact has only 3 line(s)",
		"artifact art_stale records lines: 40 but its content has 3 line(s)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "art_fine") {
		t.Fatalf("an artifact whose metadata matches its content must be silent:\n%s", joined)
	}
	if got := fakeStore.dossiers["dos_stale"].DistilledState.Body; !strings.Contains(got, "#L10-L12") {
		t.Fatalf("doctor must not edit the Distilled State: %q", got)
	}
}

func TestDoctorIgnoresUncitedEvidence(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore := newLocalFakeStore()
	fakeStore.dossiers["dos_thin"] = &Dossier{
		Frontmatter: Frontmatter{
			ID: "dos_thin", Name: "Thin Dossier", Slug: "thin-dossier",
			CreatedAt: now, UpdatedAt: now,
			Status: StatusActive, Priority: PriorityLow,
		},
		DistilledState: DistilledState{
			Body: "# Thin Dossier\n\n## Findings\n- [observed] A claim. [src:art_cited]",
		},
	}
	fakeStore.artifacts["dos_thin"] = []Artifact{
		{
			ID: "art_cited", DossierID: "dos_thin", Type: ArtifactTypeDecisionEvidence,
			Title: "Cited", CapturedAt: now, RefreshedAt: now,
			Provenance: Provenance{Origin: "unit test"}, ContentFormat: ContentFormatText,
			Content: "one\n",
		},
		{
			ID: "art_orphan", DossierID: "dos_thin", Type: ArtifactTypeDecisionEvidence,
			Title: "Orphan", CapturedAt: now, RefreshedAt: now,
			Provenance: Provenance{Origin: "unit test"}, ContentFormat: ContentFormatText,
			Content: "two\n",
		},
	}
	fakeStore.revisions["dos_thin"] = CalculateRevision(
		fakeStore.dossiers["dos_thin"].Frontmatter,
		fakeStore.dossiers["dos_thin"].DistilledState.Body,
		fakeStore.artifacts["dos_thin"])

	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	// Under-citation is an authoring judgment, not store damage or a broken
	// pointer: doctor stays quiet. Save and Recall still surface it to the agent.
	if !res.OK {
		t.Fatalf("uncited evidence must not fail doctor:\n%s", warningsText(res.Warnings))
	}
	if strings.Contains(warningsText(res.Warnings), "art_orphan") {
		t.Fatalf("doctor must not report uncited evidence, got:\n%s", warningsText(res.Warnings))
	}
}

func TestUncitedArtifactsExemptsTranscripts(t *testing.T) {
	body := "# Dossier\n\n## Findings\n- [observed] A claim. [src:art_cited]"
	artifacts := []Artifact{
		{ID: "art_cited", Type: ArtifactTypeDecisionEvidence},
		{ID: "art_transcript", Type: ArtifactTypeTranscript},
	}
	uncited := uncitedArtifacts(body, artifacts)
	for _, art := range uncited {
		if art.Type == ArtifactTypeTranscript {
			t.Fatalf("transcript artifact %s must be exempt from the uncited check", art.ID)
		}
	}
	if msg := uncitedArtifactWarning(body, artifacts); strings.Contains(msg, "art_transcript") {
		t.Fatalf("uncited warning must not name the transcript artifact, got: %q", msg)
	}
}

func TestUncitedLinesAreNotFlagged(t *testing.T) {
	body := "# Title\n\n## Objective\nShip the offer.\n\n## Situation\nLaunch slipped a week.\n\n" +
		"## Findings\n- [observed] Lock contention at 200ms.\n- [assumed] Load resembles prod.\n\n" +
		"## Next Steps\n- Draft the brief.\n"
	issues := validateDistilledStateProvenance(body, "dos_1", func(string) (int, bool) { return 0, false })
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none: uncited lines are not doctor's concern", issues)
	}
}

func TestBrokenCitationsStillReportedAmongUncitedLines(t *testing.T) {
	body := "## Situation\nUncited context.\n- Cited claim. [src:art_gone#L1-L2]\nMore uncited context."
	issues := validateDistilledStateProvenance(body, "dos_1", func(string) (int, bool) { return 0, false })
	if len(issues) != 1 || !strings.Contains(issues[0], "line 3 references missing artifact art_gone") {
		t.Fatalf("issues = %v, want exactly the missing-artifact issue on line 3", issues)
	}
}

func TestCitationsInsideFencesAreIgnored(t *testing.T) {
	body := "```\n[src:art_gone]\n```\n"
	issues := validateDistilledStateProvenance(body, "dos_1", func(string) (int, bool) { return 0, false })
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}
