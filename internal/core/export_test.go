package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

type exportFixture struct {
	store *fileTestStore
	svc   *Service
	rev   Revision
}

func newExportFixture(t *testing.T, body string, cfg Config, arts []Artifact, files map[string][]byte) *exportFixture {
	t.Helper()
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	st := &fileTestStore{
		localFakeStore: newLocalFakeStore(),
		files:          map[string][]WorkingFile{},
		contents:       map[string]map[string][]byte{"dos_x": files},
	}
	for p, data := range files {
		st.files["dos_x"] = append(st.files["dos_x"], WorkingFile{Path: p, Size: int64(len(data))})
	}
	d := &Dossier{Frontmatter: Frontmatter{
		ID: "dos_x", Name: "Pricing Review", Slug: "pricing-review",
		CreatedAt: now.AddDate(0, 0, -9), UpdatedAt: now.AddDate(0, 0, -2),
		Status: StatusActive, Priority: PriorityMedium, Lead: "alice", NextAction: "Send to Dana",
	}, DistilledState: DistilledState{Body: body}}
	st.dossiers["dos_x"] = d
	rev := CalculateRevision(d.Frontmatter, body, nil)
	st.revisions["dos_x"] = rev
	for i := range arts {
		arts[i].DossierID = "dos_x"
		if arts[i].CapturedAt.IsZero() {
			arts[i].CapturedAt = now.Add(-time.Duration(100-i) * time.Hour)
		}
		if arts[i].RefreshedAt.IsZero() {
			arts[i].RefreshedAt = arts[i].CapturedAt
		}
		if arts[i].ContentFormat == "" {
			arts[i].ContentFormat = ContentFormatMarkdown
		}
	}
	st.artifacts["dos_x"] = arts
	cfg.DossierHome = "/tmp/dossier-test"
	cfg.Author = "alice"
	svc := NewService(st, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, cfg, nil)
	return &exportFixture{store: st, svc: svc, rev: rev}
}

func (f *exportFixture) export(t *testing.T) (ExportResult, []Warning) {
	t.Helper()
	res, err := f.svc.Export(context.Background(), ExportReq{ID: "dos_x"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	data, ok := res.Data.(ExportResult)
	if !ok || !res.OK {
		t.Fatalf("Export returned %T ok=%v", res.Data, res.OK)
	}
	return data, res.Warnings
}

func hasWarning(ws []Warning, sub string) bool {
	for _, w := range ws {
		if strings.Contains(string(w), sub) {
			return true
		}
	}
	return false
}

const exportPreambleWant = "**About this document.** This is a working Dossier exported on 2026-10-07 (last updated 2026-10-05): the\n" +
	"author's full working context on one outcome, broader than any formal write-up it contains.\n" +
	"If you are an AI assistant helping the reader: answer from this document; say which section or\n" +
	"supporting item an answer comes from; when the document does not cover something, say so\n" +
	"plainly rather than inferring. Citations of the form `[src:art_…]` refer to the supporting\n" +
	"items below by ID; items listed under \"Not included\" were deliberately left out.\n"

func TestExportDocumentShape(t *testing.T) {
	body := "## Objective\nDecide pricing. [src:art_b]\n\n## Findings\n- Competitors charge 10. [src:art_a#L1-L2]\n"
	f := newExportFixture(t, body, Config{}, []Artifact{
		{ID: "art_a", Type: ArtifactTypeSourceSnapshot, Title: "Competitor page", Provenance: Provenance{Origin: "web", URL: "https://x.test/a"}, Content: "line1\nline2\n"},
		{ID: "art_b", Type: ArtifactTypeDecisionEvidence, Title: "Survey", Content: "survey data"},
		{ID: "art_c", Type: ArtifactTypeLink, Title: "Uncited link", Content: "link"},
	}, nil)
	data, _ := f.export(t)
	doc := data.Markdown

	if strings.HasPrefix(doc, "---") || strings.Contains(doc, "dossier_id:") || strings.Contains(doc, string(f.rev)) {
		t.Fatalf("document must carry no YAML frontmatter or revision hash:\n%s", doc)
	}
	if !strings.HasPrefix(doc, "# Pricing Review\n\nExported: 2026-10-07 · Last updated: 2026-10-05 · Status: active · Lead: alice · Next action: Send to Dana\n\n") {
		t.Fatalf("unexpected title/header:\n%s", doc)
	}
	if !strings.Contains(doc, exportPreambleWant) {
		t.Fatalf("reader preamble is not verbatim ADR 0017 §2:\n%s", doc)
	}
	if !strings.Contains(doc, body) {
		t.Fatalf("Distilled State body is not byte-for-byte present:\n%s", doc)
	}
	order := []string{"## Objective", "## Supporting material", "### Survey", "### Competitor page", "### Uncited link", "## Not included"}
	last := -1
	for _, marker := range order {
		i := strings.Index(doc, marker)
		if i < 0 || i < last {
			t.Fatalf("section %q missing or out of order (cited by first citation, then uncited):\n%s", marker, doc)
		}
		last = i
	}
	if !strings.Contains(doc, "ID: art_a · Type: source_snapshot · Captured: ") || !strings.Contains(doc, "URL: https://x.test/a") {
		t.Fatalf("metadata line missing:\n%s", doc)
	}
	if got := data.ArtifactsIncluded; strings.Join(got, ",") != "art_b,art_a,art_c" {
		t.Fatalf("ArtifactsIncluded = %v", got)
	}
	if data.Revision != f.rev {
		t.Fatalf("Revision = %q, want %q", data.Revision, f.rev)
	}
}

func TestExportExcludesTranscriptsAndDedupesSnapshots(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	snap := func(id string, day int, content string) Artifact {
		return Artifact{ID: id, Type: ArtifactTypeSourceSnapshot, Title: "Pricing page", Provenance: Provenance{Origin: "web", URL: "https://x.test/p"},
			CapturedAt: base.AddDate(0, 0, day), RefreshedAt: base.AddDate(0, 0, day), Content: content}
	}
	f := newExportFixture(t, "## Findings\n- old claim [src:art_s1]\n", Config{}, []Artifact{
		snap("art_s1", 0, "OLDEST-CONTENT"),
		snap("art_s3", 20, "NEWEST-CONTENT"),
		snap("art_s2", 10, "MIDDLE-CONTENT"),
		{ID: "art_t1", Type: ArtifactTypeTranscript, Title: "Session 12", Content: "TRANSCRIPT-SENTINEL", CapturedAt: base},
		// Same title, different URL: a different group.
		{ID: "art_o", Type: ArtifactTypeSourceSnapshot, Title: "Pricing page", Provenance: Provenance{URL: "https://x.test/other"}, Content: "OTHER-URL-CONTENT", CapturedAt: base},
	}, nil)
	data, warns := f.export(t)
	doc := data.Markdown

	if strings.Contains(doc, "TRANSCRIPT-SENTINEL") {
		t.Fatalf("transcript content leaked:\n%s", doc)
	}
	notIncluded := doc[strings.Index(doc, "## Not included"):]
	if !strings.Contains(notIncluded, "art_t1") || !strings.Contains(notIncluded, "Session 12") {
		t.Fatalf("transcript id and title must be listed under Not included:\n%s", notIncluded)
	}
	if strings.Contains(doc, "OLDEST-CONTENT") || strings.Contains(doc, "MIDDLE-CONTENT") {
		t.Fatalf("superseded snapshots must not be inlined:\n%s", doc)
	}
	if !strings.Contains(doc, "NEWEST-CONTENT") || !strings.Contains(doc, "OTHER-URL-CONTENT") {
		t.Fatalf("newest of the group and the other-URL snapshot must be inlined:\n%s", doc)
	}
	if !strings.Contains(doc, "### Pricing page (supersedes art_s1, art_s2)") {
		t.Fatalf("heading must name the superseded IDs:\n%s", doc)
	}
	// art_s1 is the only cited member; its group, represented by art_s3, sorts first.
	if strings.Index(doc, "NEWEST-CONTENT") > strings.Index(doc, "OTHER-URL-CONTENT") {
		t.Fatalf("a group with a cited member must come before uncited items:\n%s", doc)
	}
	if !hasWarning(warns, "1 transcripts, 0 binary files, 0 external file references not included.") {
		t.Fatalf("exclusion summary warning missing: %v", warns)
	}
}

func TestExportWorkingFiles(t *testing.T) {
	body := "## Files\n- `files/writeup.md`: formal write-up\n- `files/deck.pptx`: slides\n- `/Users/zed/Desktop/model.xlsx`: outside\n- `files/gone.md`: removed\n"
	pptx := append([]byte("PK\x03\x04"), make([]byte, 2048)...)
	f := newExportFixture(t, body, Config{}, nil, map[string][]byte{
		"files/writeup.md": []byte("# Formal write-up\nFull text here.\n"),
		"files/deck.pptx":  pptx,
		"files/latin1.txt": {0x66, 0xe9, 0x6f}, // invalid UTF-8
	})
	data, warns := f.export(t)
	doc := data.Markdown

	if !strings.Contains(doc, "### files/writeup.md") || !strings.Contains(doc, "# Formal write-up\nFull text here.\n") {
		t.Fatalf("text working file not inlined in full:\n%s", doc)
	}
	if strings.Contains(doc, "PK\x03\x04") {
		t.Fatalf("binary content inlined")
	}
	notIncluded := doc[strings.Index(doc, "## Not included"):]
	for _, want := range []string{"files/deck.pptx", "binary file (2.0 KB)", "files/latin1.txt", "/Users/zed/Desktop/model.xlsx", "files/gone.md"} {
		if !strings.Contains(notIncluded, want) {
			t.Errorf("Not included missing %q:\n%s", want, notIncluded)
		}
	}
	if got := strings.Join(data.FilesIncluded, ","); got != "files/writeup.md" {
		t.Fatalf("FilesIncluded = %q", got)
	}
	if !hasWarning(warns, "0 transcripts, 2 binary files, 1 external file references not included.") {
		t.Fatalf("exclusion warning missing: %v", warns)
	}
	if !hasWarning(warns, "files/gone.md") {
		t.Fatalf("a missing indexed file must warn: %v", warns)
	}
}

func TestExportFenceRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		content string
		fence   string
	}{
		{"no fences", "plain text\n", "```"},
		{"triple fence", "before\n```go\nfmt.Println()\n```\nafter\n", "````"},
		{"quad fence", "````\nnested ```\n````\n", "`````"},
		{"inline run of 7", "x ``````` y\n", "````````"},
		{"no trailing newline", "no newline", "```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newExportFixture(t, "## Body\n", Config{}, []Artifact{{ID: "art_f", Type: ArtifactTypeLink, Title: "Fenced", Content: tt.content}}, nil)
			data, _ := f.export(t)
			doc := data.Markdown
			got, ok := extractFenced(doc, "Captured: ")
			// Metadata line is followed by a blank line and then the opening fence.
			if !ok || got.fence != tt.fence || got.content != strings.TrimSuffix(tt.content, "\n")+"\n" {
				t.Fatalf("content did not round-trip inside a %q fence (got %+v ok=%v):\n%s", tt.fence, got, ok, doc)
			}
		})
	}
}

type fencedBlock struct{ fence, content string }

// extractFenced reads the first fenced block after the line containing marker,
// the way a CommonMark reader would: the block ends at the first line made only
// of backticks that is at least as long as the opening fence.
func extractFenced(doc, marker string) (fencedBlock, bool) {
	lines := strings.Split(doc, "\n")
	i := 0
	for i < len(lines) && !strings.Contains(lines[i], marker) {
		i++
	}
	for i++; i < len(lines) && !strings.HasPrefix(lines[i], "```"); i++ {
	}
	if i >= len(lines) {
		return fencedBlock{}, false
	}
	open := lines[i]
	var content strings.Builder
	for i++; i < len(lines); i++ {
		l := lines[i]
		if strings.Trim(l, "`") == "" && len(l) >= len(open) {
			return fencedBlock{fence: open, content: content.String()}, true
		}
		content.WriteString(l + "\n")
	}
	return fencedBlock{}, false
}

func TestExportNeverReadsMachineLocalOrHistoryData(t *testing.T) {
	f := newExportFixture(t, "## Body\nok\n", Config{}, nil, nil)
	now := time.Now()
	f.store.audits["dos_x"] = []AuditEvent{{Event: "save", Message: "AUDIT-SENTINEL"}}
	f.store.conflicts["con_1"] = &Conflict{ID: "con_1", DossierID: "dos_x", RejectedBody: "CONFLICT-SENTINEL", DiffAgainstCurrent: "CONFLICT-DIFF-SENTINEL", TS: now}
	f.store.history["rev_old"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_x"}, DistilledState: DistilledState{Body: "HISTORY-SENTINEL"}}
	data, warns := f.export(t)
	for _, s := range []string{"AUDIT-SENTINEL", "CONFLICT-SENTINEL", "CONFLICT-DIFF-SENTINEL", "HISTORY-SENTINEL"} {
		if strings.Contains(data.Markdown, s) {
			t.Errorf("%s leaked into the export", s)
		}
	}
	if !hasWarning(warns, "1 unresolved conflict(s)") || !hasWarning(warns, "con_1") {
		t.Fatalf("unresolved conflict warning missing: %v", warns)
	}
}

func TestExportConflictsOnOtherDossiersOrResolvedDoNotWarn(t *testing.T) {
	f := newExportFixture(t, "## Body\n", Config{}, nil, nil)
	resolved := time.Now()
	f.store.conflicts["con_other"] = &Conflict{ID: "con_other", DossierID: "dos_other"}
	f.store.conflicts["con_done"] = &Conflict{ID: "con_done", DossierID: "dos_x", ResolvedAt: &resolved}
	_, warns := f.export(t)
	if hasWarning(warns, "conflict") {
		t.Fatalf("unexpected conflict warning: %v", warns)
	}
}

func TestExportLocalPathWarnings(t *testing.T) {
	body := "## Constraints\n- Data lives in /home/alice/projects/pricing/data.csv and (~/notes/plan.md).\n- Mac copy: /Users/bob/Desktop/x.xlsx.\n- Not a path: http://x.test/home/alice and a/b/c.\n"
	f := newExportFixture(t, body, Config{}, []Artifact{
		{ID: "art_p", Type: ArtifactTypeFileSnapshot, Title: "Note", Content: "see /home/alice/projects/pricing/data.csv again\n"},
	}, nil)
	data, warns := f.export(t)

	if !strings.Contains(data.Markdown, body) {
		t.Fatalf("local paths must stay in the output unchanged")
	}
	var pathWarnings []string
	for _, w := range warns {
		if strings.HasPrefix(string(w), "Local path ") {
			pathWarnings = append(pathWarnings, string(w))
		}
	}
	if len(pathWarnings) != 3 {
		t.Fatalf("want one warning per distinct path (3), got %d: %v", len(pathWarnings), pathWarnings)
	}
	first := pathWarnings[0]
	if !strings.Contains(first, "/home/alice/projects/pricing/data.csv") || !strings.Contains(first, "the Distilled State") || !strings.Contains(first, "supporting item art_p") {
		t.Fatalf("a path appearing twice is named once with both locations: %s", first)
	}
	joined := strings.Join(pathWarnings, "\n")
	for _, want := range []string{"~/notes/plan.md", "/Users/bob/Desktop/x.xlsx"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning for %s: %v", want, pathWarnings)
		}
	}
	if strings.Contains(joined, "http://x.test") {
		t.Errorf("a URL containing /home/ must not warn: %v", pathWarnings)
	}
}

func TestExportOverTokenLimitWarnsAndDoesNotTrim(t *testing.T) {
	body := "## Body\n" + strings.Repeat("a long line of working context\n", 200)
	f := newExportFixture(t, body, Config{TokenLimit: 100}, []Artifact{{ID: "art_big", Type: ArtifactTypeLink, Title: "Big", Content: strings.Repeat("evidence\n", 500)}}, nil)
	data, warns := f.export(t)
	if !strings.Contains(data.Markdown, body) || strings.Count(data.Markdown, "evidence\n") != 500 {
		t.Fatalf("over-limit export was trimmed")
	}
	if data.TokenEstimate <= 100 || !hasWarning(warns, "token_limit of 100") {
		t.Fatalf("want a token_limit warning with estimate, got estimate=%d warnings=%v", data.TokenEstimate, warns)
	}

	f = newExportFixture(t, "## Body\nshort\n", Config{}, nil, nil)
	if _, warns := f.export(t); hasWarning(warns, "token_limit") {
		t.Fatalf("a small export must not warn about size: %v", warns)
	}
}

func TestExportDoesNotChangeRevisionAndRecordsAuditOnce(t *testing.T) {
	f := newExportFixture(t, "## Body\n[src:art_a]\n", Config{}, []Artifact{{ID: "art_a", Type: ArtifactTypeLink, Title: "A", Content: "x"}}, map[string][]byte{"files/n.md": []byte("n")})
	data, _ := f.export(t)
	if got := len(f.store.audits["dos_x"]); got != 0 {
		t.Fatalf("Export itself must not audit (the adapter records after writing): %d events", got)
	}

	err := f.svc.RecordExport(context.Background(), RecordExportReq{Export: data, Output: "pricing-review-export-2026-10-07.md", Actor: "agent:claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	events := f.store.audits["dos_x"]
	if len(events) != 1 {
		t.Fatalf("want exactly one audit event, got %d", len(events))
	}
	e := events[0]
	if e.Event != "exported" {
		t.Fatalf("event = %q", e.Event)
	}
	if e.Revision != string(f.rev) || strings.Join(e.ArtifactsIncluded, ",") != "art_a" || strings.Join(e.FilesIncluded, ",") != "files/n.md" {
		t.Fatalf("event payload wrong: %+v", e)
	}
	if e.Output != "pricing-review-export-2026-10-07.md" || strings.Contains(e.Output, "/") {
		t.Fatalf("output must be the basename: %q", e.Output)
	}
	if e.Actor != "agent:claude-code" || e.DossierID != "dos_x" {
		t.Fatalf("actor/dossier wrong: %+v", e)
	}
	if _, rev, _ := f.store.Read("dos_x"); rev != f.rev {
		t.Fatalf("revision changed: %q -> %q", f.rev, rev)
	}
}

func TestExportUnknownDossier(t *testing.T) {
	f := newExportFixture(t, "## Body\n", Config{}, nil, nil)
	if _, err := f.svc.Export(context.Background(), ExportReq{ID: "dos_missing"}); err == nil {
		t.Fatal("expected not_found")
	}
}

func TestIsTextContent(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, true},
		{"ascii", []byte("hello\n"), true},
		{"utf8", []byte("naïve – ok"), true},
		{"NUL early", []byte("ab\x00cd"), false},
		{"invalid utf8", []byte{0xff, 0xfe, 'a'}, false},
		{"NUL after first 8KB is not checked", append([]byte(strings.Repeat("a", 8192)), 0), true},
		{"NUL at 8191", append([]byte(strings.Repeat("a", 8191)), 0), false},
		{"pptx header", []byte("PK\x03\x04\x00\x00"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTextContent(tt.data); got != tt.want {
				t.Fatalf("IsTextContent = %v, want %v", got, tt.want)
			}
		})
	}
}
