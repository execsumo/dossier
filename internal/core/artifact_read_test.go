package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newDossierWithArtifact builds a service holding one dossier with one
// artifact whose body is `lines` numbered 1..n, so line ranges are easy to
// assert against.
func newDossierWithArtifact(t *testing.T, body string, artifactContent string) (*Service, *localFakeStore) {
	t.Helper()
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()},
		Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100000}, nil)

	if _, err := svc.Save(context.Background(), SaveReq{
		FrontmatterUpdates:     map[string]any{"name": "Billing lock"},
		DistilledStateMarkdown: body,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	art := Artifact{
		ID:            "art_evidence",
		Type:          ArtifactTypeDecisionEvidence,
		Title:         "Lock latency benchmark",
		ContentFormat: ContentFormatText,
		Provenance:    Provenance{Origin: "benchmark run"},
		Content:       artifactContent,
		CapturedAt:    time.Now(),
	}
	if err := store.WriteArtifact("dos_fake_id", &art); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	return svc, store
}

func numberedBody(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString("line ")
		sb.WriteString(strings.Repeat("x", 1))
		sb.WriteString(" ")
		sb.WriteString(itoa(i))
		sb.WriteString("\n")
	}
	return sb.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestReadArtifactResolvesCitedRange(t *testing.T) {
	svc, _ := newDossierWithArtifact(t,
		"## Findings\n- [observed] Latency spike. [src:art_evidence#L3-L5]",
		numberedBody(10))

	res, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID:  "dos_fake_id",
		ArtifactID: "art_evidence",
		Fragment:   "L3-L5",
	})
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}

	content, ok := res.Data.(ArtifactContent)
	if !ok {
		t.Fatalf("ReadArtifact() data = %T, want ArtifactContent", res.Data)
	}
	if content.StartLine != 3 || content.EndLine != 5 || !content.Ranged {
		t.Fatalf("range = %d-%d ranged=%v, want 3-5 ranged=true", content.StartLine, content.EndLine, content.Ranged)
	}
	if !content.Cited {
		t.Errorf("Cited = false, want true for an artifact the distilled state points at")
	}

	// Numbering must be absolute so the span read is the span cited.
	if !strings.HasPrefix(content.Content, "3\tline x 3\n") {
		t.Errorf("content does not start at absolute line 3:\n%s", content.Content)
	}
	if strings.Contains(content.Content, "line x 6") {
		t.Errorf("content leaked past the requested range:\n%s", content.Content)
	}
}

func TestReadArtifactAcceptsHashPrefixedFragment(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence#L2]", numberedBody(10))

	res, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_evidence", Fragment: "#L2",
	})
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	content := res.Data.(ArtifactContent)
	if content.StartLine != 2 || content.EndLine != 2 {
		t.Fatalf("range = %d-%d, want 2-2", content.StartLine, content.EndLine)
	}
}

func TestReadArtifactRejectsMalformedFragment(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence]", numberedBody(10))

	_, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_evidence", Fragment: "section-2",
	})
	if err == nil || !strings.Contains(err.Error(), "not a line range") {
		t.Fatalf("ReadArtifact() error = %v, want a malformed-fragment error", err)
	}
}

func TestReadArtifactWarnsRatherThanTruncatingOverlongRange(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence]", numberedBody(10))

	res, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_evidence", StartLine: 8, EndLine: 400,
	})
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	content := res.Data.(ArtifactContent)
	if content.EndLine != 10 {
		t.Fatalf("EndLine = %d, want clamped to 10", content.EndLine)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("clamping a range produced no warning; it must degrade visibly")
	}
}

func TestReadArtifactFullFetchReturnsWholeArtifact(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence]", numberedBody(4))

	res, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_evidence",
	})
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	content := res.Data.(ArtifactContent)
	if content.Ranged {
		t.Errorf("Ranged = true, want false for an unranged fetch")
	}
	if content.StartLine != 1 || content.EndLine != 4 {
		t.Fatalf("range = %d-%d, want 1-4", content.StartLine, content.EndLine)
	}
	for i := 1; i <= 4; i++ {
		if !strings.Contains(content.Content, itoa(i)+"\tline x "+itoa(i)) {
			t.Errorf("content missing line %d:\n%s", i, content.Content)
		}
	}
}

func TestReadArtifactUnknownArtifactIsNotFound(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence]", numberedBody(4))

	_, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_nope",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ReadArtifact() error = %v, want not found", err)
	}
}

func TestRecallReturnsEvidenceIndexAndFlagsUncited(t *testing.T) {
	svc, store := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence#L1-L2]", numberedBody(6))

	orphan := Artifact{
		ID: "art_orphan", Type: ArtifactTypeTranscript, Title: "Session",
		ContentFormat: ContentFormatText, Provenance: Provenance{Origin: "session"},
		Content: numberedBody(3), CapturedAt: time.Now(),
	}
	if err := store.WriteArtifact("dos_fake_id", &orphan); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}

	res, err := svc.Recall(context.Background(), RecallReq{ID: "dos_fake_id"})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	data := res.Data.(RecallResult)
	if len(data.Artifacts) != 2 {
		t.Fatalf("evidence index has %d entries, want 2", len(data.Artifacts))
	}

	byID := map[string]ArtifactSummary{}
	for _, a := range data.Artifacts {
		byID[a.ID] = a
	}
	if !byID["art_evidence"].Cited {
		t.Errorf("art_evidence Cited = false, want true")
	}
	if byID["art_orphan"].Cited {
		t.Errorf("art_orphan Cited = true, want false")
	}
	if byID["art_evidence"].Lines != 6 {
		t.Errorf("art_evidence Lines = %d, want 6", byID["art_evidence"].Lines)
	}

	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(string(w), "art_orphan") {
			found = true
		}
	}
	if !found {
		t.Errorf("Recall() warnings = %v, want one naming the uncited artifact", res.Warnings)
	}
}

func TestSaveWarnsOnUncitedEvidence(t *testing.T) {
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence#L1-L2]", numberedBody(6))

	res, err := svc.Save(context.Background(), SaveReq{
		ID:                     "dos_fake_id",
		DistilledStateMarkdown: "## Findings\n- [observed] Something else entirely.",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(string(w), "art_evidence") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Save() warnings = %v, want one naming the now-uncited artifact", res.Warnings)
	}
}

func TestSessionEndArchivesCompiledTranscript(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()},
		Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100000}, nil)

	if _, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{"name": "Billing lock"}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.SaveSessionBinding(&SessionBinding{SessionBindingID: "sess_1", DossierID: "dos_fake_id", Harness: "claude-code"}); err != nil {
		t.Fatalf("SaveSessionBinding() error = %v", err)
	}

	if err := svc.SessionEnd(context.Background(), "sess_1", "", sampleTrace); err != nil {
		t.Fatalf("SessionEnd() error = %v", err)
	}

	arts, err := store.ListArtifacts("dos_fake_id")
	if err != nil {
		t.Fatalf("ListArtifacts() error = %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("archived %d artifacts, want 1", len(arts))
	}
	if !strings.Contains(arts[0].Content, "## [3] tool_call Bash") {
		t.Errorf("archived transcript was not compiled into role-tagged nodes:\n%s", arts[0].Content)
	}
	if strings.Contains(arts[0].Content, `"parentUuid"`) {
		t.Errorf("archived transcript still carries raw JSONL envelope fields")
	}
	if arts[0].ContentFormat != ContentFormatMarkdown {
		t.Errorf("ContentFormat = %q, want %q", arts[0].ContentFormat, ContentFormatMarkdown)
	}
}

func TestReadArtifactPreservesBlankLinesInRange(t *testing.T) {
	// A range whose last line is blank must still report that line. Losing it
	// would make EndLine disagree with the content actually returned.
	svc, _ := newDossierWithArtifact(t, "## Findings\n- [observed] X. [src:art_evidence]", "alpha\n\nbravo\n\n")

	res, err := svc.ReadArtifact(context.Background(), ReadArtifactReq{
		DossierID: "dos_fake_id", ArtifactID: "art_evidence", Fragment: "L1-L4",
	})
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	content := res.Data.(ArtifactContent)
	if content.Lines != 4 {
		t.Fatalf("Lines = %d, want 4", content.Lines)
	}

	got := strings.Split(strings.TrimSuffix(content.Content, "\n"), "\n")
	want := []string{"1\talpha", "2\t", "3\tbravo", "4\t"}
	if len(got) != len(want) {
		t.Fatalf("returned %d numbered lines, want %d:\n%q", len(got), len(want), content.Content)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestSplitContentLinesIsCanonical(t *testing.T) {
	tests := []struct {
		content string
		want    []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"a\n\n", []string{"a", ""}},
		{"a\nb\n", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := splitContentLines(tt.content)
		if len(got) != len(tt.want) {
			t.Errorf("splitContentLines(%q) = %q, want %q", tt.content, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("splitContentLines(%q)[%d] = %q, want %q", tt.content, i, got[i], tt.want[i])
			}
		}
		// The count used to bound citations must agree with the split.
		if artifactLineCount(tt.content) != len(got) {
			t.Errorf("artifactLineCount(%q) disagrees with splitContentLines", tt.content)
		}
	}
}
