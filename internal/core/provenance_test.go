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
			name:    "uncited claim",
			body:    "## Findings\n- [observed] Lock contention.",
			wantSub: "is missing provenance",
		},
		{
			name: "headings and fences are exempt",
			body: "# Title\n\n## Findings\n\n```\nraw block with no citation\n```\n",
		},
		{
			name: "Evidence section lines are exempt (they describe the archive, not a claim)",
			body: "## Evidence\n- `art_ok` (transcript, 100 lines): full session capture; background only.",
		},
		{
			name: "an assumed line is exempt (unverified by definition, nothing to cite)",
			body: "## Findings\n- [assumed] Production concurrency resembles the load-test profile; unverified against telemetry.",
		},
		{
			name:    "a claim still needs provenance outside the Evidence section",
			body:    "## Findings\n- [observed] Lock contention.\n## Evidence\n- `art_ok` (transcript, 100 lines): full session capture.",
			wantSub: "is missing provenance",
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

func TestDoctorAdvisesOnUncitedEvidenceWithoutFailing(t *testing.T) {
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
	// Under-citation is a distillation smell, not store damage: advise, do not fail.
	if !res.OK {
		t.Fatalf("uncited evidence must be advisory, not a doctor failure:\n%s", warningsText(res.Warnings))
	}
	if !strings.Contains(warningsText(res.Warnings), "art_orphan") {
		t.Fatalf("expected an uncited-evidence advisory naming art_orphan, got:\n%s", warningsText(res.Warnings))
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
